package middleware

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"anlapi/internal/config"
	"anlapi/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	privateGatewayProtocol        = "ANLAPI-HUB-V1"
	privateGatewayTimestampWindow = 60 * time.Second
	privateGatewayNonceTTL        = 10 * time.Minute
	privateGatewayIdempotencyTTL  = 24 * time.Hour
	privateGatewayProcessingTTL   = 30 * time.Second
	privateGatewayMaxReplayBytes  = 2 << 20

	privateGatewayServiceIDHeader  = "X-ANL-Service-ID"
	privateGatewayTimestampHeader  = "X-ANL-Timestamp"
	privateGatewayNonceHeader      = "X-ANL-Nonce"
	privateGatewayBodySHA256Header = "X-ANL-Body-SHA256"
	privateGatewaySignatureHeader  = "X-ANL-Signature"

	privateGatewayRequestContextKey       = "_anl_private_gateway_request"
	privateGatewayPreauthenticatedKey     = "_anl_private_gateway_api_key"
	privateGatewayIdempotencyReplayHeader = "Idempotency-Replayed"
	privateGatewayOutcomeUnknownKey       = "_anl_private_gateway_outcome_unknown"
	privateGatewayRetryableFailureKey     = "_anl_private_gateway_retryable_failure"
)

var (
	privateGatewayNoncePattern          = regexp.MustCompile(`^[0-9a-f]{32}$`)
	privateGatewayIdempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{15,159}$`)
)

type PrivateGatewayAPIKeyLoader func(context.Context, int64) (*service.APIKey, error)

type privateGatewayRequestContext struct {
	serviceID  string
	bodySHA256 string
}

// ANLPrivateGateway owns the Hub service-authenticated branch of the existing
// OpenAI-compatible routes and delegates replay state to a durable store.
type ANLPrivateGateway struct {
	cfg         config.PrivateGatewayConfig
	configValid bool
	loadAPIKey  PrivateGatewayAPIKeyLoader
	now         func() time.Time
	state       service.PrivateGatewayStateRepository
}

func NewANLPrivateGateway(cfg config.PrivateGatewayConfig, loader PrivateGatewayAPIKeyLoader) *ANLPrivateGateway {
	return NewANLPrivateGatewayWithState(cfg, loader, newPrivateGatewayMemoryState(time.Now))
}

// NewANLPrivateGatewayWithState wires the durable state boundary used by the
// production router. Tests may provide a shared in-memory implementation.
func NewANLPrivateGatewayWithState(cfg config.PrivateGatewayConfig, loader PrivateGatewayAPIKeyLoader, state service.PrivateGatewayStateRepository) *ANLPrivateGateway {
	return newANLPrivateGatewayWithRuntimeAndState(cfg, loader, time.Now, state)
}

func newANLPrivateGatewayWithRuntime(
	cfg config.PrivateGatewayConfig,
	loader PrivateGatewayAPIKeyLoader,
	now func() time.Time,
) *ANLPrivateGateway {
	return newANLPrivateGatewayWithRuntimeAndState(cfg, loader, now, newPrivateGatewayMemoryState(now))
}

func newANLPrivateGatewayWithRuntimeAndState(
	cfg config.PrivateGatewayConfig,
	loader PrivateGatewayAPIKeyLoader,
	now func() time.Time,
	state service.PrivateGatewayStateRepository,
) *ANLPrivateGateway {
	if now == nil {
		now = time.Now
	}
	if state == nil {
		state = newPrivateGatewayMemoryState(now)
	}
	return &ANLPrivateGateway{
		cfg:         cfg,
		configValid: cfg.Validate() == nil,
		loadAPIKey:  loader,
		now:         now,
		state:       state,
	}
}

// Authentication verifies Hub HMAC headers before the existing API key
// middleware. Existing public callers keep using their normal API key.
func (g *ANLPrivateGateway) Authentication() gin.HandlerFunc {
	return func(c *gin.Context) {
		if g == nil || !g.cfg.Enabled || !isPrivateGatewayProtectedRoute(c.Request) {
			c.Next()
			return
		}

		if !hasPrivateGatewayAuthHeader(c.Request.Header) {
			if hasAPIKeyCredentialInput(c) || strings.TrimSpace(c.Query("key")) != "" || strings.TrimSpace(c.Query("api_key")) != "" {
				c.Next()
				return
			}
			abortPrivateGatewayError(c, http.StatusUnauthorized, "ANL_SERVICE_AUTH_INVALID", "Service authentication is invalid")
			return
		}

		if !g.configValid || g.loadAPIKey == nil {
			abortPrivateGatewayError(c, http.StatusUnauthorized, "ANL_SERVICE_AUTH_INVALID", "Service authentication is invalid")
			return
		}

		serviceID := c.GetHeader(privateGatewayServiceIDHeader)
		timestampRaw := c.GetHeader(privateGatewayTimestampHeader)
		nonce := c.GetHeader(privateGatewayNonceHeader)
		bodyDigestRaw := c.GetHeader(privateGatewayBodySHA256Header)
		signatureRaw := c.GetHeader(privateGatewaySignatureHeader)
		if serviceID != g.cfg.ServiceID || len(timestampRaw) > 20 ||
			!privateGatewayNoncePattern.MatchString(nonce) {
			abortPrivateGatewayError(c, http.StatusUnauthorized, "ANL_SERVICE_AUTH_INVALID", "Service authentication is invalid")
			return
		}

		timestamp, err := strconv.ParseInt(timestampRaw, 10, 64)
		if err != nil || strconv.FormatInt(timestamp, 10) != timestampRaw {
			abortPrivateGatewayError(c, http.StatusUnauthorized, "ANL_SERVICE_AUTH_INVALID", "Service authentication is invalid")
			return
		}
		now := g.now().UTC()
		if timestamp < now.Add(-privateGatewayTimestampWindow).Unix() || timestamp > now.Add(privateGatewayTimestampWindow).Unix() {
			abortPrivateGatewayError(c, http.StatusUnauthorized, "ANL_SERVICE_AUTH_EXPIRED", "Service authentication timestamp is expired")
			return
		}

		body, err := readAndRestorePrivateGatewayBody(c.Request)
		if err != nil {
			status := http.StatusBadRequest
			code := "INVALID_REQUEST"
			message := "Request body is invalid"
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				status = http.StatusRequestEntityTooLarge
				code = "REQUEST_TOO_LARGE"
				message = "Request body is too large"
			}
			abortPrivateGatewayError(c, status, code, message)
			return
		}

		bodyDigest, ok := decodePrivateGatewayHexHeader(bodyDigestRaw, sha256.Size)
		if !ok {
			abortPrivateGatewayError(c, http.StatusUnauthorized, "ANL_SERVICE_AUTH_INVALID", "Service authentication is invalid")
			return
		}
		actualBodyDigest := sha256.Sum256(body)
		if !hmac.Equal(bodyDigest, actualBodyDigest[:]) {
			abortPrivateGatewayError(c, http.StatusUnauthorized, "ANL_SERVICE_AUTH_INVALID", "Service authentication is invalid")
			return
		}

		signature, ok := decodePrivateGatewayHexHeader(signatureRaw, sha256.Size)
		if !ok {
			abortPrivateGatewayError(c, http.StatusUnauthorized, "ANL_SERVICE_AUTH_INVALID", "Service authentication is invalid")
			return
		}
		canonical := strings.Join([]string{
			privateGatewayProtocol,
			serviceID,
			timestampRaw,
			nonce,
			strings.ToUpper(c.Request.Method),
			privateGatewayEscapedPathAndQuery(c.Request),
			bodyDigestRaw,
		}, "\n")
		mac := hmac.New(sha256.New, []byte(g.cfg.SigningKey))
		_, _ = mac.Write([]byte(canonical))
		if !hmac.Equal(signature, mac.Sum(nil)) {
			abortPrivateGatewayError(c, http.StatusUnauthorized, "ANL_SERVICE_AUTH_INVALID", "Service authentication is invalid")
			return
		}

		nonceHash := service.HashIdempotencyKey(nonce)
		nonceClaimed, err := g.state.ClaimPrivateGatewayNonce(
			c.Request.Context(),
			privateGatewayNonceScope(serviceID),
			nonceHash,
			now.Add(privateGatewayNonceTTL),
		)
		if err != nil {
			abortPrivateGatewayError(c, http.StatusServiceUnavailable, "ANL_SERVICE_AUTH_UNAVAILABLE", "Service authentication is temporarily unavailable")
			return
		}
		if !nonceClaimed {
			abortPrivateGatewayError(c, http.StatusConflict, "ANL_SERVICE_REPLAYED", "Service request nonce was already used")
			return
		}

		apiKey, err := g.loadAPIKey(c.Request.Context(), g.cfg.APIKeyID)
		if err != nil || apiKey == nil {
			abortPrivateGatewayError(c, http.StatusUnauthorized, "ANL_SERVICE_AUTH_INVALID", "Service authentication is invalid")
			return
		}
		setPrivateGatewayPreauthenticatedAPIKey(c, apiKey)
		c.Set(privateGatewayRequestContextKey, privateGatewayRequestContext{
			serviceID:  serviceID,
			bodySHA256: bodyDigestRaw,
		})
		stripPrivateGatewayAuthHeaders(c.Request.Header)
		c.Request.Header.Del("Authorization")
		c.Request.Header.Del("x-api-key")
		c.Request.Header.Del("x-goog-api-key")
		c.Next()
	}
}

// Idempotency applies only to authenticated requests that produce model
// output. Model discovery remains a normal authenticated GET.
func (g *ANLPrivateGateway) Idempotency() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestContext, ok := getPrivateGatewayRequestContext(c)
		if g == nil || !ok || !isPrivateGatewayOutputRoute(c.Request) {
			c.Next()
			return
		}

		key := normalizePrivateGatewayIdempotencyKey(c.GetHeader("Idempotency-Key"))
		if key == "" {
			abortPrivateGatewayError(c, http.StatusBadRequest, "INVALID_REQUEST", "A valid Idempotency-Key is required")
			return
		}

		scope := privateGatewayIdempotencyScope(requestContext.serviceID, c.Request, key)
		fingerprint := privateGatewayIdempotencyFingerprint(requestContext, c.Request)
		now := g.now().UTC()
		keyHash := service.HashIdempotencyKey(key)
		expiresAt := now.Add(privateGatewayIdempotencyTTL)
		lockedUntil := now.Add(privateGatewayProcessingTTL)
		record := &service.IdempotencyRecord{
			Scope:              scope,
			IdempotencyKeyHash: keyHash,
			RequestFingerprint: fingerprint,
			Status:             service.IdempotencyStatusProcessing,
			LockedUntil:        &lockedUntil,
			ExpiresAt:          expiresAt,
		}
		owner, err := g.state.CreateProcessing(c.Request.Context(), record)
		if err != nil {
			abortPrivateGatewayError(c, http.StatusServiceUnavailable, "IDEMPOTENCY_STORE_UNAVAILABLE", "Idempotency state is temporarily unavailable")
			return
		}
		if !owner {
			record, err = g.state.GetByScopeAndKeyHash(c.Request.Context(), scope, keyHash)
			if err != nil || record == nil {
				abortPrivateGatewayError(c, http.StatusServiceUnavailable, "IDEMPOTENCY_STORE_UNAVAILABLE", "Idempotency state is temporarily unavailable")
				return
			}
			if record.RequestFingerprint != fingerprint {
				abortPrivateGatewayError(c, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "Idempotency-Key was reused with a different request")
				return
			}
			if record.Status == service.IdempotencyStatusProcessing {
				if record.LockedUntil != nil && record.LockedUntil.After(now) {
					c.Header("Retry-After", "1")
					abortPrivateGatewayError(c, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS", "The idempotent request is still in progress")
					return
				}
				if err := g.markOutcomeUnknown(record.ID); err != nil {
					abortPrivateGatewayError(c, http.StatusServiceUnavailable, "IDEMPOTENCY_STORE_UNAVAILABLE", "Idempotency state is temporarily unavailable")
					return
				}
				abortPrivateGatewayError(c, http.StatusConflict, "IDEMPOTENCY_OUTCOME_UNKNOWN", "The upstream outcome is unknown; retry with the same Idempotency-Key")
				return
			}
			if record.Status == service.PrivateGatewayIdempotencyStatusFailedRetryable {
				if record.LockedUntil != nil && record.LockedUntil.After(now) {
					c.Header("Retry-After", strconv.Itoa(retryAfterSeconds(record.LockedUntil, now)))
					abortPrivateGatewayError(c, http.StatusConflict, "IDEMPOTENCY_RETRY_BACKOFF", "The idempotent request is in retry backoff")
					return
				}
				taken, reclaimErr := g.state.ReclaimPrivateGatewayRetryable(c.Request.Context(), record.ID, now, lockedUntil, expiresAt)
				if reclaimErr != nil {
					abortPrivateGatewayError(c, http.StatusServiceUnavailable, "IDEMPOTENCY_STORE_UNAVAILABLE", "Idempotency state is temporarily unavailable")
					return
				}
				if taken {
					owner = true
					record.Status = service.IdempotencyStatusProcessing
					record.LockedUntil = &lockedUntil
					record.ExpiresAt = expiresAt
				} else {
					record, err = g.state.GetByScopeAndKeyHash(c.Request.Context(), scope, keyHash)
					if err != nil || record == nil {
						abortPrivateGatewayError(c, http.StatusServiceUnavailable, "IDEMPOTENCY_STORE_UNAVAILABLE", "Idempotency state is temporarily unavailable")
						return
					}
				}
			}
			if !owner {
				if record.Status == service.IdempotencyStatusProcessing {
					c.Header("Retry-After", "1")
					abortPrivateGatewayError(c, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS", "The idempotent request is still in progress")
					return
				}
				if record.Status == service.PrivateGatewayIdempotencyStatusOutcomeUnknown {
					abortPrivateGatewayError(c, http.StatusConflict, "IDEMPOTENCY_OUTCOME_UNKNOWN", "The upstream outcome is unknown; retry with the same Idempotency-Key")
					return
				}
				if !record.ExpiresAt.After(now) {
					if err := g.markExpired(record.ID); err != nil {
						abortPrivateGatewayError(c, http.StatusServiceUnavailable, "IDEMPOTENCY_STORE_UNAVAILABLE", "Idempotency state is temporarily unavailable")
						return
					}
					abortPrivateGatewayError(c, http.StatusConflict, "IDEMPOTENCY_EXPIRED", "The idempotency record has expired; create a new request key")
					return
				}
				replay, decodeErr := decodePrivateGatewayStoredResponse(record.ResponseBody)
				if decodeErr != nil {
					abortPrivateGatewayError(c, http.StatusServiceUnavailable, "IDEMPOTENCY_STORE_UNAVAILABLE", "Stored idempotency response is unavailable")
					return
				}
				writePrivateGatewayReplay(c, replay)
				return
			}
		}

		writer := &privateGatewayCaptureWriter{ResponseWriter: c.Writer, maxBytes: privateGatewayMaxReplayBytes}
		c.Writer = writer
		defer func() {
			if recovered := recover(); recovered != nil {
				_ = g.markOutcomeUnknown(record.ID)
				panic(recovered)
			}
			if !privateGatewayRequestCompleted(c, writer) {
				_ = g.markOutcomeUnknown(record.ID)
			}
		}()

		c.Next()
		if privateGatewayRequestCompleted(c, writer) {
			response := privateGatewayHTTPResponse{
				status:    c.Writer.Status(),
				header:    clonePrivateGatewayReplayHeader(c.Writer.Header()),
				body:      append([]byte(nil), writer.body.Bytes()...),
				usage:     privateGatewayExtractUsage(writer.body.Bytes()),
				retryable: privateGatewayIsRetryableFailure(c),
				errorCode: privateGatewayResponseErrorCode(privateGatewayHTTPResponse{body: writer.body.Bytes()}),
			}
			if err := g.markTerminal(record.ID, response); err != nil {
				// Keep the processing row. A later same-key request must fail
				// closed rather than execute the upstream request again.
				return
			}
		}
	}
}

// Status returns a redacted idempotency/billing receipt for a Hub request.
func (g *ANLPrivateGateway) Status() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := normalizePrivateGatewayIdempotencyKey(c.Param("key"))
		if key == "" {
			abortPrivateGatewayError(c, http.StatusBadRequest, "INVALID_REQUEST", "A valid Idempotency-Key is required")
			return
		}
		var record *service.IdempotencyRecord
		var lookupErr error
		for _, route := range []string{"/v1/chat/completions", "/v1/images/generations", "/v1/images/edits"} {
			scope := privateGatewayIdempotencyScopeForPath(g.cfg.ServiceID, http.MethodPost, route, key)
			record, lookupErr = g.state.GetByScopeAndKeyHash(c.Request.Context(), scope, service.HashIdempotencyKey(key))
			if lookupErr != nil || record != nil {
				break
			}
		}
		if lookupErr != nil {
			abortPrivateGatewayError(c, http.StatusServiceUnavailable, "IDEMPOTENCY_STORE_UNAVAILABLE", "Idempotency state is temporarily unavailable")
			return
		}
		if record == nil {
			abortPrivateGatewayError(c, http.StatusNotFound, "IDEMPOTENCY_NOT_FOUND", "No request receipt was found")
			return
		}
		response := gin.H{
			"status":           record.Status,
			"billing_status":   "pending",
			"usage_status":     "pending",
			"replay_available": record.ResponseBody != nil,
			"created_at":       record.CreatedAt,
			"updated_at":       record.UpdatedAt,
			"expires_at":       record.ExpiresAt,
		}
		if record.ResponseStatus != nil {
			response["response_status"] = *record.ResponseStatus
		}
		if record.ResponseBody != nil {
			if stored, decodeErr := decodePrivateGatewayStoredResponse(record.ResponseBody); decodeErr == nil && len(stored.usage) > 0 {
				var usage any
				if json.Unmarshal(stored.usage, &usage) == nil {
					response["usage"] = usage
					response["usage_status"] = "reported"
				}
			}
		}
		if record.Status == service.IdempotencyStatusSucceeded {
			if stored, decodeErr := decodePrivateGatewayStoredResponse(record.ResponseBody); decodeErr == nil && len(stored.usage) > 0 && json.Valid(stored.usage) {
				response["usage"] = stored.usage
				response["usage_status"] = "reported"
			}
		} else if record.Status == service.PrivateGatewayIdempotencyStatusFailed || record.Status == service.PrivateGatewayIdempotencyStatusFailedRetryable {
			response["billing_status"] = "not_charged"
		} else if record.Status == service.PrivateGatewayIdempotencyStatusOutcomeUnknown {
			response["billing_status"] = "unknown"
			response["usage_status"] = "unknown"
		}
		if record.LockedUntil != nil && record.LockedUntil.After(g.now().UTC()) {
			response["retry_after"] = retryAfterSeconds(record.LockedUntil, g.now().UTC())
		}
		c.JSON(http.StatusOK, response)
	}
}

func privateGatewayRequestCompleted(c *gin.Context, writer *privateGatewayCaptureWriter) bool {
	if c == nil || writer == nil || writer.writeErr != nil || writer.overflow {
		return false
	}
	if c.Request != nil && c.Request.Context().Err() != nil {
		return false
	}
	value, exists := c.Get(privateGatewayOutcomeUnknownKey)
	if exists && value == true {
		return false
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(c.Writer.Header().Get("Content-Type"))), "text/event-stream") {
		return privateGatewayStreamHasUsageAndTerminal(writer.body.Bytes())
	}
	return true
}

func privateGatewayStreamHasUsageAndTerminal(body []byte) bool {
	if len(privateGatewayExtractUsage(body)) == 0 {
		return false
	}
	for _, line := range bytes.Split(body, []byte("\n")) {
		if bytes.Equal(bytes.TrimSpace(line), []byte("data: [DONE]")) {
			return true
		}
	}
	return false
}

func privateGatewayIsRetryableFailure(c *gin.Context) bool {
	value, exists := c.Get(privateGatewayRetryableFailureKey)
	return exists && value == true
}

func retryAfterSeconds(until *time.Time, now time.Time) int {
	if until == nil {
		return 1
	}
	seconds := int(until.Sub(now).Seconds())
	if seconds < 1 {
		return 1
	}
	return seconds
}

func (g *ANLPrivateGateway) markTerminal(id int64, response privateGatewayHTTPResponse) error {
	stored, err := encodePrivateGatewayStoredResponse(response)
	if err != nil {
		return g.markOutcomeUnknown(id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if response.status >= http.StatusBadRequest {
		if response.retryable {
			return g.state.MarkPrivateGatewayFailedRetryable(ctx, id, response.status, stored, response.errorCode, g.now().UTC().Add(privateGatewayProcessingTTL), g.now().UTC().Add(privateGatewayIdempotencyTTL))
		}
		return g.state.MarkPrivateGatewayFailed(ctx, id, response.status, stored, privateGatewayResponseErrorCode(response), g.now().UTC().Add(privateGatewayIdempotencyTTL))
	}
	return g.state.MarkSucceeded(ctx, id, response.status, stored, g.now().UTC().Add(privateGatewayIdempotencyTTL))
}

func (g *ANLPrivateGateway) markOutcomeUnknown(id int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return g.state.MarkPrivateGatewayOutcomeUnknown(ctx, id, "IDEMPOTENCY_OUTCOME_UNKNOWN", g.now().UTC().Add(privateGatewayIdempotencyTTL))
}

func (g *ANLPrivateGateway) markExpired(id int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return g.state.MarkPrivateGatewayExpired(ctx, id, "IDEMPOTENCY_EXPIRED")
}

// MarkPrivateGatewayOutcomeUnknown lets a handler classify an upstream
// failure whose acceptance cannot be determined from the HTTP response.
func MarkPrivateGatewayOutcomeUnknown(c *gin.Context) {
	if c != nil {
		c.Set(privateGatewayOutcomeUnknownKey, true)
	}
}

// MarkPrivateGatewayRetryableFailure marks a known HTTP failure as retryable.
// It never converts an unknown upstream outcome into a retryable failure.
func MarkPrivateGatewayRetryableFailure(c *gin.Context) {
	if c != nil {
		c.Set(privateGatewayRetryableFailureKey, true)
	}
}

func privateGatewayResponseErrorCode(response privateGatewayHTTPResponse) string {
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(response.body, &payload) == nil && strings.TrimSpace(payload.Error.Code) != "" {
		return payload.Error.Code
	}
	return "HTTP_ERROR"
}

func privateGatewayExtractUsage(body []byte) json.RawMessage {
	if usage := privateGatewayUsageFromJSON(body); len(usage) > 0 {
		return usage
	}
	var usage json.RawMessage
	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		candidate := privateGatewayUsageFromJSON(bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:"))))
		if len(candidate) > 0 {
			usage = candidate
		}
	}
	return usage
}

func privateGatewayUsageFromJSON(body []byte) json.RawMessage {
	var payload struct {
		Usage json.RawMessage `json:"usage"`
	}
	if json.Unmarshal(body, &payload) != nil || len(payload.Usage) == 0 || string(payload.Usage) == "null" {
		return nil
	}
	return append(json.RawMessage(nil), payload.Usage...)
}

func encodePrivateGatewayStoredResponse(response privateGatewayHTTPResponse) (string, error) {
	if len(response.body) > privateGatewayMaxReplayBytes {
		return "", errors.New("private gateway response exceeds replay limit")
	}
	payload, err := json.Marshal(struct {
		Status    int             `json:"status"`
		Headers   http.Header     `json:"headers"`
		Body      []byte          `json:"body"`
		Usage     json.RawMessage `json:"usage,omitempty"`
		ErrorCode string          `json:"error_code,omitempty"`
	}{response.status, response.header, response.body, response.usage, response.errorCode})
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func decodePrivateGatewayStoredResponse(raw *string) (privateGatewayHTTPResponse, error) {
	if raw == nil || *raw == "" {
		return privateGatewayHTTPResponse{}, errors.New("stored response is empty")
	}
	var payload struct {
		Status    int             `json:"status"`
		Headers   http.Header     `json:"headers"`
		Body      []byte          `json:"body"`
		Usage     json.RawMessage `json:"usage"`
		ErrorCode string          `json:"error_code"`
	}
	if err := json.Unmarshal([]byte(*raw), &payload); err != nil || payload.Status < 100 || payload.Status > 599 || len(payload.Body) > privateGatewayMaxReplayBytes {
		return privateGatewayHTTPResponse{}, errors.New("stored response is invalid")
	}
	return privateGatewayHTTPResponse{status: payload.Status, header: payload.Headers, body: payload.Body, usage: payload.Usage, errorCode: payload.ErrorCode}, nil
}

func isPrivateGatewayProtectedRoute(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	switch r.Method + " " + r.URL.Path {
	case http.MethodGet + " /v1/models",
		http.MethodPost + " /v1/chat/completions",
		http.MethodPost + " /v1/images/generations",
		http.MethodPost + " /v1/images/edits":
		return true
	default:
		return r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/sub2api/idempotency/") && len(strings.TrimPrefix(r.URL.Path, "/v1/sub2api/idempotency/")) > 0
	}
}

func isPrivateGatewayOutputRoute(r *http.Request) bool {
	if r == nil || r.URL == nil || r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/v1/chat/completions", "/v1/images/generations", "/v1/images/edits":
		return true
	default:
		return false
	}
}

func normalizePrivateGatewayIdempotencyKey(raw string) string {
	key := strings.TrimSpace(raw)
	if !privateGatewayIdempotencyKeyPattern.MatchString(key) {
		return ""
	}
	return key
}

func hasPrivateGatewayAuthHeader(header http.Header) bool {
	for _, name := range []string{
		privateGatewayServiceIDHeader,
		privateGatewayTimestampHeader,
		privateGatewayNonceHeader,
		privateGatewayBodySHA256Header,
		privateGatewaySignatureHeader,
	} {
		if header.Get(name) != "" {
			return true
		}
	}
	return false
}

func stripPrivateGatewayAuthHeaders(header http.Header) {
	for _, name := range []string{
		privateGatewayServiceIDHeader,
		privateGatewayTimestampHeader,
		privateGatewayNonceHeader,
		privateGatewayBodySHA256Header,
		privateGatewaySignatureHeader,
	} {
		header.Del(name)
	}
}

func readAndRestorePrivateGatewayBody(r *http.Request) ([]byte, error) {
	if r == nil || r.Body == nil {
		return []byte{}, nil
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func decodePrivateGatewayHexHeader(raw string, size int) ([]byte, bool) {
	if len(raw) != size*2 || raw != strings.ToLower(raw) {
		return nil, false
	}
	decoded, err := hex.DecodeString(raw)
	return decoded, err == nil && len(decoded) == size
}

func privateGatewayEscapedPathAndQuery(r *http.Request) string {
	path := r.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	return path
}

func abortPrivateGatewayError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    "invalid_request_error",
			"param":   nil,
			"code":    code,
		},
	})
	c.Abort()
}

func setPrivateGatewayPreauthenticatedAPIKey(c *gin.Context, apiKey *service.APIKey) {
	if c != nil && apiKey != nil {
		c.Set(privateGatewayPreauthenticatedKey, apiKey)
	}
}

func takePrivateGatewayPreauthenticatedAPIKey(c *gin.Context) (*service.APIKey, bool) {
	if c == nil {
		return nil, false
	}
	value, ok := c.Get(privateGatewayPreauthenticatedKey)
	if !ok {
		return nil, false
	}
	c.Set(privateGatewayPreauthenticatedKey, nil)
	apiKey, ok := value.(*service.APIKey)
	return apiKey, ok && apiKey != nil
}

func getPrivateGatewayRequestContext(c *gin.Context) (privateGatewayRequestContext, bool) {
	if c == nil {
		return privateGatewayRequestContext{}, false
	}
	value, ok := c.Get(privateGatewayRequestContextKey)
	if !ok {
		return privateGatewayRequestContext{}, false
	}
	requestContext, ok := value.(privateGatewayRequestContext)
	return requestContext, ok && requestContext.serviceID != "" && requestContext.bodySHA256 != ""
}

// IsPrivateGatewayRequest reports whether the current request passed the Hub
// service-authenticated gateway branch.
func IsPrivateGatewayRequest(c *gin.Context) bool {
	_, ok := getPrivateGatewayRequestContext(c)
	return ok
}

type privateGatewayHTTPResponse struct {
	status    int
	header    http.Header
	body      []byte
	usage     json.RawMessage
	retryable bool
	errorCode string
}

type privateGatewayMemoryState struct {
	mu      sync.Mutex
	now     func() time.Time
	nonces  map[string]time.Time
	entries map[string]*service.IdempotencyRecord
}

func newPrivateGatewayMemoryState(now func() time.Time) *privateGatewayMemoryState {
	if now == nil {
		now = time.Now
	}
	return &privateGatewayMemoryState{
		now: now, nonces: make(map[string]time.Time), entries: make(map[string]*service.IdempotencyRecord),
	}
}

func (s *privateGatewayMemoryState) ClaimPrivateGatewayNonce(_ context.Context, scope, nonceHash string, expiresAt time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	key := scope + "\n" + nonceHash
	if existing, ok := s.nonces[key]; ok && existing.After(now) {
		return false, nil
	}
	s.nonces[key] = expiresAt
	return true, nil
}

func (s *privateGatewayMemoryState) CreateProcessing(_ context.Context, record *service.IdempotencyRecord) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := record.Scope + "\n" + record.IdempotencyKeyHash
	if _, exists := s.entries[key]; exists {
		return false, nil
	}
	record.ID = int64(len(s.entries) + 1)
	s.entries[key] = clonePrivateGatewayRecord(record)
	return true, nil
}

func (s *privateGatewayMemoryState) GetByScopeAndKeyHash(_ context.Context, scope, keyHash string) (*service.IdempotencyRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clonePrivateGatewayRecord(s.entries[scope+"\n"+keyHash]), nil
}

func (s *privateGatewayMemoryState) ReclaimPrivateGatewayExpired(_ context.Context, id int64, now, lockedUntil, expiresAt time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, record := range s.entries {
		if record.ID == id && !record.ExpiresAt.After(now) && record.Status != service.IdempotencyStatusProcessing {
			record.Status = service.IdempotencyStatusProcessing
			record.LockedUntil = &lockedUntil
			record.ExpiresAt = expiresAt
			record.ResponseStatus = nil
			record.ResponseBody = nil
			record.ErrorReason = nil
			s.entries[key] = record
			return true, nil
		}
	}
	return false, nil
}

func (s *privateGatewayMemoryState) ReclaimPrivateGatewayRetryable(_ context.Context, id int64, now, lockedUntil, expiresAt time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.entries {
		if record.ID == id && record.Status == service.PrivateGatewayIdempotencyStatusFailedRetryable &&
			record.LockedUntil != nil && !record.LockedUntil.After(now) && record.ExpiresAt.After(now) {
			record.Status = service.IdempotencyStatusProcessing
			record.LockedUntil = &lockedUntil
			record.ExpiresAt = expiresAt
			record.ResponseStatus = nil
			record.ResponseBody = nil
			record.ErrorReason = nil
			return true, nil
		}
	}
	return false, nil
}

func (s *privateGatewayMemoryState) MarkSucceeded(_ context.Context, id int64, status int, body string, expiresAt time.Time) error {
	return s.mark(id, service.IdempotencyStatusSucceeded, status, body, "", expiresAt)
}

func (s *privateGatewayMemoryState) MarkPrivateGatewayFailed(_ context.Context, id int64, status int, body, reason string, expiresAt time.Time) error {
	return s.mark(id, service.PrivateGatewayIdempotencyStatusFailed, status, body, reason, expiresAt)
}

func (s *privateGatewayMemoryState) MarkPrivateGatewayFailedRetryable(_ context.Context, id int64, status int, body, reason string, lockedUntil, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.entries {
		if record.ID == id && record.Status == service.IdempotencyStatusProcessing {
			record.Status = service.PrivateGatewayIdempotencyStatusFailedRetryable
			record.ResponseStatus = intPointer(status)
			record.ResponseBody = stringPointer(body)
			record.ErrorReason = stringPointer(reason)
			record.LockedUntil = &lockedUntil
			record.ExpiresAt = expiresAt
			return nil
		}
	}
	return nil
}

func (s *privateGatewayMemoryState) MarkPrivateGatewayOutcomeUnknown(_ context.Context, id int64, reason string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.entries {
		if record.ID == id && record.Status == service.IdempotencyStatusProcessing {
			record.Status = service.PrivateGatewayIdempotencyStatusOutcomeUnknown
			record.ResponseStatus = nil
			record.ResponseBody = nil
			record.ErrorReason = &reason
			record.LockedUntil = nil
			record.ExpiresAt = expiresAt
			return nil
		}
	}
	return nil
}

func (s *privateGatewayMemoryState) MarkPrivateGatewayExpired(_ context.Context, id int64, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.entries {
		if record.ID == id && record.Status != service.IdempotencyStatusProcessing {
			record.Status = service.PrivateGatewayIdempotencyStatusExpired
			record.ResponseStatus = nil
			record.ResponseBody = nil
			record.ErrorReason = stringPointer(reason)
			record.LockedUntil = nil
			return nil
		}
	}
	return nil
}

func (s *privateGatewayMemoryState) mark(id int64, status string, responseStatus int, body, reason string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.entries {
		if record.ID == id && record.Status == service.IdempotencyStatusProcessing {
			record.Status = status
			record.ResponseStatus = &responseStatus
			record.ResponseBody = &body
			if reason != "" {
				record.ErrorReason = &reason
			}
			record.LockedUntil = nil
			record.ExpiresAt = expiresAt
			return nil
		}
	}
	return nil
}

func intPointer(value int) *int { return &value }

func stringPointer(value string) *string { return &value }

func clonePrivateGatewayRecord(record *service.IdempotencyRecord) *service.IdempotencyRecord {
	if record == nil {
		return nil
	}
	clone := *record
	if record.ResponseStatus != nil {
		value := *record.ResponseStatus
		clone.ResponseStatus = &value
	}
	if record.ResponseBody != nil {
		value := *record.ResponseBody
		clone.ResponseBody = &value
	}
	if record.ErrorReason != nil {
		value := *record.ErrorReason
		clone.ErrorReason = &value
	}
	if record.LockedUntil != nil {
		value := *record.LockedUntil
		clone.LockedUntil = &value
	}
	return &clone
}

func privateGatewayNonceScope(serviceID string) string {
	sum := sha256.Sum256([]byte(serviceID))
	return "private_gateway_nonce:" + hex.EncodeToString(sum[:])
}

func privateGatewayIdempotencyScope(serviceID string, r *http.Request, key string) string {
	return privateGatewayIdempotencyScopeForPath(serviceID, r.Method, r.URL.EscapedPath(), key)
}

func privateGatewayIdempotencyScopeForPath(serviceID, method, path, key string) string {
	raw := strings.Join([]string{serviceID, strings.ToUpper(method), path, key}, "\n")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func privateGatewayIdempotencyFingerprint(requestContext privateGatewayRequestContext, r *http.Request) string {
	raw := strings.Join([]string{
		strings.ToUpper(r.Method),
		privateGatewayEscapedPathAndQuery(r),
		requestContext.bodySHA256,
	}, "\n")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

type privateGatewayCaptureWriter struct {
	gin.ResponseWriter
	body     bytes.Buffer
	maxBytes int
	overflow bool
	writeErr error
}

func (w *privateGatewayCaptureWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *privateGatewayCaptureWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if err != nil {
		w.writeErr = err
	}
	w.capture(data[:n])
	return n, err
}

func (w *privateGatewayCaptureWriter) WriteString(data string) (int, error) {
	n, err := w.ResponseWriter.WriteString(data)
	if err != nil {
		w.writeErr = err
	}
	w.capture([]byte(data[:n]))
	return n, err
}

func (w *privateGatewayCaptureWriter) capture(data []byte) {
	if w.maxBytes <= 0 || w.overflow {
		return
	}
	remaining := w.maxBytes - w.body.Len()
	if len(data) > remaining {
		if remaining > 0 {
			_, _ = w.body.Write(data[:remaining])
		}
		w.overflow = true
		return
	}
	_, _ = w.body.Write(data)
}

func clonePrivateGatewayReplayHeader(header http.Header) http.Header {
	cloned := make(http.Header)
	for name, values := range header {
		switch http.CanonicalHeaderKey(name) {
		case "Connection", "Content-Length", "Date", "Keep-Alive", "Proxy-Authenticate",
			"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade", "Set-Cookie":
			continue
		}
		cloned[name] = append([]string(nil), values...)
	}
	return cloned
}

func clonePrivateGatewayHTTPResponse(response privateGatewayHTTPResponse) privateGatewayHTTPResponse {
	return privateGatewayHTTPResponse{
		status:    response.status,
		header:    clonePrivateGatewayReplayHeader(response.header),
		body:      append([]byte(nil), response.body...),
		usage:     append(json.RawMessage(nil), response.usage...),
		retryable: response.retryable,
		errorCode: response.errorCode,
	}
}

func writePrivateGatewayReplay(c *gin.Context, response privateGatewayHTTPResponse) {
	for name, values := range response.header {
		for _, value := range values {
			c.Writer.Header().Add(name, value)
		}
	}
	c.Header(privateGatewayIdempotencyReplayHeader, "true")
	status := response.status
	if status == 0 {
		status = http.StatusOK
	}
	c.Status(status)
	if len(response.body) > 0 {
		_, _ = c.Writer.Write(response.body)
	}
	c.Abort()
}
