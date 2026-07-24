package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"anlapi/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	defaultOpenAIProxyStreamFailureThreshold  = 2
	defaultOpenAIProxyStreamFailureWindow     = time.Minute
	defaultOpenAIProxyStreamQuarantineTTL     = 10 * time.Minute
	defaultOpenAIProxyStreamCircuitMaxEntries = 4096
)

type openAIProxyStreamCircuitSettings struct {
	failureThreshold int
	failureWindow    time.Duration
	quarantineTTL    time.Duration
	maxEntries       int
}

type openAIProxyStreamCircuitEntry struct {
	failureCount int
	windowStart  time.Time
	blockedUntil time.Time
	lastTouched  time.Time
}

type openAIProxyStreamCircuit struct {
	mu       sync.Mutex
	settings openAIProxyStreamCircuitSettings
	entries  map[int64]openAIProxyStreamCircuitEntry
}

func resolveOpenAIProxyStreamCircuitSettings(service *OpenAIGatewayService) openAIProxyStreamCircuitSettings {
	settings := openAIProxyStreamCircuitSettings{
		failureThreshold: defaultOpenAIProxyStreamFailureThreshold,
		failureWindow:    defaultOpenAIProxyStreamFailureWindow,
		quarantineTTL:    defaultOpenAIProxyStreamQuarantineTTL,
		maxEntries:       defaultOpenAIProxyStreamCircuitMaxEntries,
	}
	if service == nil || service.cfg == nil {
		return settings
	}
	config := service.cfg.Gateway.OpenAIProxyStreamCircuit
	if config.FailureThreshold > 0 {
		settings.failureThreshold = config.FailureThreshold
	}
	if config.WindowSeconds > 0 {
		settings.failureWindow = time.Duration(config.WindowSeconds) * time.Second
	}
	if config.TTLSeconds > 0 {
		settings.quarantineTTL = time.Duration(config.TTLSeconds) * time.Second
	}
	return settings
}

func newOpenAIProxyStreamCircuit(settings openAIProxyStreamCircuitSettings) *openAIProxyStreamCircuit {
	if settings.failureThreshold <= 0 {
		settings.failureThreshold = defaultOpenAIProxyStreamFailureThreshold
	}
	if settings.failureWindow <= 0 {
		settings.failureWindow = defaultOpenAIProxyStreamFailureWindow
	}
	if settings.quarantineTTL <= 0 {
		settings.quarantineTTL = defaultOpenAIProxyStreamQuarantineTTL
	}
	if settings.maxEntries <= 0 {
		settings.maxEntries = defaultOpenAIProxyStreamCircuitMaxEntries
	}
	return &openAIProxyStreamCircuit{
		settings: settings,
		entries:  make(map[int64]openAIProxyStreamCircuitEntry),
	}
}

func (service *OpenAIGatewayService) getOpenAIProxyStreamCircuit() *openAIProxyStreamCircuit {
	if service == nil {
		return nil
	}
	service.openaiProxyStreamCircuitOnce.Do(func() {
		if service.openaiProxyStreamCircuit == nil {
			service.openaiProxyStreamCircuit = newOpenAIProxyStreamCircuit(resolveOpenAIProxyStreamCircuitSettings(service))
		}
	})
	return service.openaiProxyStreamCircuit
}

func (circuit *openAIProxyStreamCircuit) recordFailure(proxyID int64, now time.Time) (bool, time.Time) {
	if circuit == nil || proxyID <= 0 {
		return false, time.Time{}
	}
	circuit.mu.Lock()
	defer circuit.mu.Unlock()

	entry, exists := circuit.entries[proxyID]
	if exists && now.Before(entry.blockedUntil) {
		entry.lastTouched = now
		circuit.entries[proxyID] = entry
		return false, entry.blockedUntil
	}
	if !exists {
		circuit.ensureCapacityLocked(now)
	}
	if entry.windowStart.IsZero() || now.Before(entry.windowStart) || now.Sub(entry.windowStart) > circuit.settings.failureWindow {
		entry.failureCount = 0
		entry.windowStart = now
		entry.blockedUntil = time.Time{}
	}
	entry.failureCount++
	entry.lastTouched = now
	tripped := entry.failureCount >= circuit.settings.failureThreshold
	if tripped {
		entry.blockedUntil = now.Add(circuit.settings.quarantineTTL)
	}
	circuit.entries[proxyID] = entry
	return tripped, entry.blockedUntil
}

func (circuit *openAIProxyStreamCircuit) recordSuccess(proxyID int64) bool {
	if circuit == nil || proxyID <= 0 {
		return false
	}
	circuit.mu.Lock()
	defer circuit.mu.Unlock()
	if _, exists := circuit.entries[proxyID]; !exists {
		return false
	}
	delete(circuit.entries, proxyID)
	return true
}

func (circuit *openAIProxyStreamCircuit) isBlocked(proxyID int64, now time.Time) bool {
	if circuit == nil || proxyID <= 0 {
		return false
	}
	circuit.mu.Lock()
	defer circuit.mu.Unlock()
	entry, exists := circuit.entries[proxyID]
	if !exists || entry.blockedUntil.IsZero() {
		return false
	}
	if !now.Before(entry.blockedUntil) {
		delete(circuit.entries, proxyID)
		return false
	}
	return true
}

func (circuit *openAIProxyStreamCircuit) ensureCapacityLocked(now time.Time) {
	if len(circuit.entries) < circuit.settings.maxEntries {
		return
	}
	for proxyID, entry := range circuit.entries {
		staleObservation := entry.blockedUntil.IsZero() && now.Sub(entry.lastTouched) > circuit.settings.failureWindow
		expiredQuarantine := !entry.blockedUntil.IsZero() && !now.Before(entry.blockedUntil)
		if staleObservation || expiredQuarantine {
			delete(circuit.entries, proxyID)
		}
	}
	if len(circuit.entries) < circuit.settings.maxEntries {
		return
	}
	var oldestProxyID int64
	var oldest time.Time
	for proxyID, entry := range circuit.entries {
		if oldestProxyID == 0 || entry.lastTouched.Before(oldest) {
			oldestProxyID = proxyID
			oldest = entry.lastTouched
		}
	}
	if oldestProxyID > 0 {
		delete(circuit.entries, oldestProxyID)
	}
}

func openAIProxyStreamCircuitProxyID(account *Account) (int64, bool) {
	if account == nil || account.Platform != PlatformOpenAI || account.ProxyID == nil || *account.ProxyID <= 0 {
		return 0, false
	}
	return *account.ProxyID, true
}

func (service *OpenAIGatewayService) recordOpenAIProxyStreamDisconnect(account *Account, streamError error, upstreamRequestID string) {
	proxyID, ok := openAIProxyStreamCircuitProxyID(account)
	if !ok || streamError == nil || errors.Is(streamError, context.Canceled) || errors.Is(streamError, context.DeadlineExceeded) {
		return
	}
	circuit := service.getOpenAIProxyStreamCircuit()
	tripped, until := circuit.recordFailure(proxyID, time.Now())
	if !tripped {
		return
	}
	logger.L().With(zap.String("component", "service.openai_gateway")).Warn(
		"openai.proxy_quarantined_stream_disconnect",
		zap.Int64("proxy_id", proxyID),
		zap.Int64("account_id", account.ID),
		zap.Time("until", until),
		zap.String("upstream_request_id", upstreamRequestID),
		zap.String("error", sanitizeUpstreamErrorMessage(streamError.Error())),
	)
}

func (service *OpenAIGatewayService) clearOpenAIProxyStreamDisconnect(account *Account) {
	proxyID, ok := openAIProxyStreamCircuitProxyID(account)
	if !ok {
		return
	}
	if circuit := service.getOpenAIProxyStreamCircuit(); circuit != nil {
		circuit.recordSuccess(proxyID)
	}
}

func (service *OpenAIGatewayService) isOpenAIProxyStreamQuarantined(account *Account) bool {
	proxyID, ok := openAIProxyStreamCircuitProxyID(account)
	if !ok {
		return false
	}
	circuit := service.getOpenAIProxyStreamCircuit()
	return circuit != nil && circuit.isBlocked(proxyID, time.Now())
}
