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
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	privateGatewayProtocol        = "ANLAPI-HUB-V1"
	privateGatewayTimestampWindow = 60 * time.Second
	privateGatewayNonceTTL        = 10 * time.Minute
	privateGatewayIdempotencyTTL  = 24 * time.Hour
	privateGatewayProcessingTTL   = 30 * time.Second
	privateGatewayMaxReplayBytes  = 2 << 20

	privateGatewayServiceIDHeader         = "X-ANL-Service-ID"
	privateGatewayTimestampHeader         = "X-ANL-Timestamp"
	privateGatewayNonceHeader             = "X-ANL-Nonce"
	privateGatewayBodySHA256Header        = "X-ANL-Body-SHA256"
	privateGatewaySignatureHeader         = "X-ANL-Signature"
	privateGatewayPreauthenticatedKey     = "_anl_private_gateway_api_key"
	privateGatewayRequestContextKey       = "_anl_private_gateway_request"
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

// ANLPrivateGateway authenticates the narrow Hub-to-anlapi service branch.
// Execution idempotency and response replay are deliberately separate batches.
type ANLPrivateGateway struct {
	cfg         config.PrivateGatewayConfig
	configValid bool
	loadAPIKey  PrivateGatewayAPIKeyLoader
	nonceRepo   service.PrivateGatewayNonceRepository
	state       service.PrivateGatewayStateRepository
	now         func() time.Time
}

func NewANLPrivateGateway(cfg *config.Config, apiKeyService *service.APIKeyService, nonceRepo service.PrivateGatewayNonceRepository) *ANLPrivateGateway {
	var gatewayCfg config.PrivateGatewayConfig
	if cfg != nil {
		gatewayCfg = cfg.PrivateGateway
	}
	var loader PrivateGatewayAPIKeyLoader
	if apiKeyService != nil {
		loader = apiKeyService.GetByID
	}
	return newANLPrivateGatewayWithRuntime(gatewayCfg, loader, nonceRepo, time.Now)
}

// NewANLPrivateGatewayWithState wires the durable private-gateway execution
// state in addition to the B03 nonce repository. There is no production memory
// fallback: a nil state repository fails closed in Idempotency.
func NewANLPrivateGatewayWithState(
	cfg *config.Config,
	apiKeyService *service.APIKeyService,
	nonceRepo service.PrivateGatewayNonceRepository,
	state service.PrivateGatewayStateRepository,
) *ANLPrivateGateway {
	var gatewayCfg config.PrivateGatewayConfig
	if cfg != nil {
		gatewayCfg = cfg.PrivateGateway
	}
	var loader PrivateGatewayAPIKeyLoader
	if apiKeyService != nil {
		loader = apiKeyService.GetByID
	}
	return newANLPrivateGatewayWithRuntimeAndState(gatewayCfg, loader, nonceRepo, state, time.Now)
}

func newANLPrivateGatewayWithRuntime(
	cfg config.PrivateGatewayConfig,
	loader PrivateGatewayAPIKeyLoader,
	nonceRepo service.PrivateGatewayNonceRepository,
	now func() time.Time,
) *ANLPrivateGateway {
	return newANLPrivateGatewayWithRuntimeAndState(cfg, loader, nonceRepo, nil, now)
}

func newANLPrivateGatewayWithRuntimeAndState(
	cfg config.PrivateGatewayConfig,
	loader PrivateGatewayAPIKeyLoader,
	nonceRepo service.PrivateGatewayNonceRepository,
	state service.PrivateGatewayStateRepository,
	now func() time.Time,
) *ANLPrivateGateway {
	if now == nil {
		now = time.Now
	}
	return &ANLPrivateGateway{
		cfg:         cfg,
		configValid: cfg.Validate() == nil,
		loadAPIKey:  loader,
		nonceRepo:   nonceRepo,
		state:       state,
		now:         now,
	}
}

// Authentication verifies HMAC headers before the existing API key middleware.
// Requests without private headers retain the official public API-key branch.
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
		if g.nonceRepo == nil {
			abortPrivateGatewayError(c, http.StatusServiceUnavailable, "ANL_SERVICE_AUTH_UNAVAILABLE", "Service authentication is temporarily unavailable")
			return
		}

		serviceID := c.GetHeader(privateGatewayServiceIDHeader)
		timestampRaw := c.GetHeader(privateGatewayTimestampHeader)
		nonce := c.GetHeader(privateGatewayNonceHeader)
		bodyDigestRaw := c.GetHeader(privateGatewayBodySHA256Header)
		signatureRaw := c.GetHeader(privateGatewaySignatureHeader)
		if serviceID != g.cfg.ServiceID || len(timestampRaw) > 20 || !privateGatewayNoncePattern.MatchString(nonce) {
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
			abortPrivateGatewayError(c, http.StatusBadRequest, "INVALID_REQUEST", "Request body is invalid")
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

		apiKey, err := g.loadAPIKey(c.Request.Context(), g.cfg.APIKeyID)
		if err != nil {
			if errors.Is(err, service.ErrAPIKeyNotFound) {
				abortPrivateGatewayError(c, http.StatusUnauthorized, "ANL_SERVICE_AUTH_INVALID", "Service authentication is invalid")
				return
			}
			abortPrivateGatewayError(c, http.StatusServiceUnavailable, "ANL_SERVICE_AUTH_UNAVAILABLE", "Service authentication is temporarily unavailable")
			return
		}
		if apiKey == nil {
			abortPrivateGatewayError(c, http.StatusUnauthorized, "ANL_SERVICE_AUTH_INVALID", "Service authentication is invalid")
			return
		}

		nonceHash := service.HashIdempotencyKey(nonce)
		nonceClaimed, err := g.nonceRepo.ClaimPrivateGatewayNonce(
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

		setPrivateGatewayPreauthenticatedAPIKey(c, apiKey)
		c.Set(privateGatewayRequestContextKey, privateGatewayRequestContext{
			serviceID:  serviceID,
			bodySHA256: bodyDigestRaw,
		})
		stripPrivateGatewayAuthHeaders(c.Request.Header)
		c.Next()
	}
}

// Idempotency protects only private service requests that produce model output.
// The state is durable and shared across instances; a nil store fails closed.
func (g *ANLPrivateGateway) Idempotency() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestContext, ok := getPrivateGatewayRequestContext(c)
		if g == nil || !ok || !isPrivateGatewayOutputRoute(c.Request) {
			c.Next()
			return
		}
		if g.state == nil {
			abortPrivateGatewayError(c, http.StatusServiceUnavailable, "IDEMPOTENCY_STORE_UNAVAILABLE", "Idempotency state is temporarily unavailable")
			return
		}

		key := normalizePrivateGatewayIdempotencyKey(c.GetHeader("Idempotency-Key"))
		if key == "" {
			abortPrivateGatewayError(c, http.StatusBadRequest, "INVALID_REQUEST", "A valid Idempotency-Key is required")
			return
		}

		now := g.now().UTC()
		scope := privateGatewayIdempotencyScope(requestContext.serviceID, c.Request, key)
		keyHash := service.HashIdempotencyKey(key)
		fingerprint := privateGatewayIdempotencyFingerprint(requestContext, c.Request)
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
			existing, getErr := g.state.GetByScopeAndKeyHash(c.Request.Context(), scope, keyHash)
			if getErr != nil || existing == nil {
				abortPrivateGatewayError(c, http.StatusServiceUnavailable, "IDEMPOTENCY_STORE_UNAVAILABLE", "Idempotency state is temporarily unavailable")
				return
			}
			if existing.RequestFingerprint != fingerprint {
				abortPrivateGatewayError(c, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "Idempotency-Key was reused with a different request")
				return
			}

			switch existing.Status {
			case service.IdempotencyStatusProcessing:
				if existing.LockedUntil != nil && existing.LockedUntil.After(now) {
					c.Header("Retry-After", strconv.Itoa(retryAfterSeconds(existing.LockedUntil, now)))
					abortPrivateGatewayError(c, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS", "The idempotent request is still in progress")
					return
				}
				if markErr := g.markOutcomeUnknown(existing.ID); markErr != nil {
					abortPrivateGatewayError(c, http.StatusServiceUnavailable, "IDEMPOTENCY_STORE_UNAVAILABLE", "Idempotency state is temporarily unavailable")
					return
				}
				abortPrivateGatewayError(c, http.StatusConflict, "IDEMPOTENCY_OUTCOME_UNKNOWN", "The upstream outcome is unknown; retry with the same Idempotency-Key")
				return
			}

			if !existing.ExpiresAt.After(now) {
				if markErr := g.markExpired(existing.ID); markErr != nil {
					abortPrivateGatewayError(c, http.StatusServiceUnavailable, "IDEMPOTENCY_STORE_UNAVAILABLE", "Idempotency state is temporarily unavailable")
					return
				}
				abortPrivateGatewayError(c, http.StatusConflict, "IDEMPOTENCY_EXPIRED", "The idempotency record has expired; create a new request key")
				return
			}

			if existing.Status == service.PrivateGatewayIdempotencyStatusOutcomeUnknown {
				abortPrivateGatewayError(c, http.StatusConflict, "IDEMPOTENCY_OUTCOME_UNKNOWN", "The upstream outcome is unknown; retry with the same Idempotency-Key")
				return
			}
			if existing.Status == service.PrivateGatewayIdempotencyStatusFailedRetryable {
				if existing.LockedUntil != nil && existing.LockedUntil.After(now) {
					c.Header("Retry-After", strconv.Itoa(retryAfterSeconds(existing.LockedUntil, now)))
					abortPrivateGatewayError(c, http.StatusConflict, "IDEMPOTENCY_RETRY_BACKOFF", "The idempotent request is in retry backoff")
					return
				}
				taken, reclaimErr := g.state.ReclaimPrivateGatewayRetryable(c.Request.Context(), existing.ID, now, lockedUntil, expiresAt)
				if reclaimErr != nil {
					abortPrivateGatewayError(c, http.StatusServiceUnavailable, "IDEMPOTENCY_STORE_UNAVAILABLE", "Idempotency state is temporarily unavailable")
					return
				}
				if !taken {
					c.Header("Retry-After", "1")
					abortPrivateGatewayError(c, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS", "The idempotent request is still in progress")
					return
				}
				record.ID = existing.ID
				owner = true
			}

			if !owner {
				replay, decodeErr := decodePrivateGatewayStoredResponse(existing.ResponseBody)
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
		if !privateGatewayRequestCompleted(c, writer) {
			return
		}
		response := privateGatewayHTTPResponse{
			status:    c.Writer.Status(),
			header:    clonePrivateGatewayReplayHeader(c.Writer.Header()),
			body:      append([]byte(nil), writer.body.Bytes()...),
			usage:     privateGatewayExtractUsage(writer.body.Bytes()),
			retryable: privateGatewayIsRetryableFailure(c),
			errorCode: privateGatewayResponseErrorCode(privateGatewayHTTPResponse{body: writer.body.Bytes()}),
		}
		if err := g.markTerminal(record.ID, response); err != nil {
			_ = g.markOutcomeUnknown(record.ID)
		}
	}
}

// Status returns a redacted receipt. It never returns stored response bodies.
func (g *ANLPrivateGateway) Status() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestContext, ok := getPrivateGatewayRequestContext(c)
		if g == nil || !ok {
			abortPrivateGatewayError(c, http.StatusUnauthorized, "ANL_SERVICE_AUTH_INVALID", "Service authentication is invalid")
			return
		}
		if g.state == nil {
			abortPrivateGatewayError(c, http.StatusServiceUnavailable, "IDEMPOTENCY_STORE_UNAVAILABLE", "Idempotency state is temporarily unavailable")
			return
		}
		key := normalizePrivateGatewayIdempotencyKey(c.Param("key"))
		if key == "" {
			abortPrivateGatewayError(c, http.StatusBadRequest, "INVALID_REQUEST", "A valid Idempotency-Key is required")
			return
		}

		keyHash := service.HashIdempotencyKey(key)
		scope := privateGatewayIdempotencyScopeForService(requestContext.serviceID)
		record, lookupErr := g.state.GetByScopeAndKeyHash(c.Request.Context(), scope, keyHash)
		if lookupErr != nil {
			abortPrivateGatewayError(c, http.StatusServiceUnavailable, "IDEMPOTENCY_STORE_UNAVAILABLE", "Idempotency state is temporarily unavailable")
			return
		}
		if record == nil {
			abortPrivateGatewayError(c, http.StatusNotFound, "IDEMPOTENCY_NOT_FOUND", "No request receipt was found")
			return
		}

		payload := gin.H{
			"status":           record.Status,
			"billing_status":   "pending",
			"usage_status":     "pending",
			"replay_available": record.ResponseBody != nil,
			"created_at":       record.CreatedAt,
			"updated_at":       record.UpdatedAt,
			"expires_at":       record.ExpiresAt,
		}
		if record.ResponseStatus != nil {
			payload["response_status"] = *record.ResponseStatus
		}
		if stored, decodeErr := decodePrivateGatewayStoredResponse(record.ResponseBody); decodeErr == nil && len(stored.usage) > 0 {
			var usage any
			if json.Unmarshal(stored.usage, &usage) == nil {
				payload["usage"] = usage
				payload["usage_status"] = "reported"
			}
		}
		switch record.Status {
		case service.PrivateGatewayIdempotencyStatusFailed, service.PrivateGatewayIdempotencyStatusFailedRetryable:
			payload["billing_status"] = "not_charged"
		case service.PrivateGatewayIdempotencyStatusOutcomeUnknown:
			payload["billing_status"] = "unknown"
			payload["usage_status"] = "unknown"
		case service.PrivateGatewayIdempotencyStatusExpired:
			payload["billing_status"] = "not_charged"
			payload["usage_status"] = "expired"
		}
		if record.LockedUntil != nil && record.LockedUntil.After(g.now().UTC()) {
			payload["retry_after"] = retryAfterSeconds(record.LockedUntil, g.now().UTC())
		}
		c.JSON(http.StatusOK, payload)
	}
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
		return r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/sub2api/idempotency/") && strings.TrimPrefix(r.URL.Path, "/v1/sub2api/idempotency/") != ""
	}
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
		"Authorization",
		"x-api-key",
		"x-goog-api-key",
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
	r.ContentLength = int64(len(body))
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

func privateGatewayNonceScope(serviceID string) string {
	return "private_gateway_nonce:" + service.HashIdempotencyKey(serviceID)
}

func abortPrivateGatewayError(c *gin.Context, status int, code, message string) {
	c.Abort()
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    "invalid_request_error",
			"param":   nil,
			"code":    code,
		},
	})
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
	apiKey, ok := value.(*service.APIKey)
	if ok {
		c.Set(privateGatewayPreauthenticatedKey, nil)
	}
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

// IsPrivateGatewayRequest reports whether the current request passed the HMAC
// service-authenticated branch.
func IsPrivateGatewayRequest(c *gin.Context) bool {
	_, ok := getPrivateGatewayRequestContext(c)
	return ok
}

// MarkPrivateGatewayOutcomeUnknown lets an upstream handler classify a failure
// whose acceptance cannot be determined from the HTTP response.
func MarkPrivateGatewayOutcomeUnknown(c *gin.Context) {
	if c != nil {
		c.Set(privateGatewayOutcomeUnknownKey, true)
	}
}

// IsPrivateGatewayOutcomeUnknown reports whether an upstream handler classified
// the current pre-output failure as having an indeterminate upstream outcome.
// It is intentionally request-scoped; durable state is written by Idempotency.
func IsPrivateGatewayOutcomeUnknown(c *gin.Context) bool {
	if c == nil {
		return false
	}
	value, exists := c.Get(privateGatewayOutcomeUnknownKey)
	return exists && value == true
}

// MarkPrivateGatewayRetryableFailure marks a known temporary HTTP failure.
func MarkPrivateGatewayRetryableFailure(c *gin.Context) {
	if c != nil {
		c.Set(privateGatewayRetryableFailureKey, true)
	}
}

// IsPrivateGatewayRetryableFailure reports whether a gateway handler classified
// the current pre-output failure as safe for idempotent retry. It is exposed
// for handler-level contract tests and remains an in-process request flag only;
// durable state is written by Idempotency.
func IsPrivateGatewayRetryableFailure(c *gin.Context) bool {
	if c == nil {
		return false
	}
	return privateGatewayIsRetryableFailure(c)
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

func privateGatewayIdempotencyScope(serviceID string, _ *http.Request, _ string) string {
	return privateGatewayIdempotencyScopeForService(serviceID)
}

// privateGatewayIdempotencyScopeForService keeps one durable key namespace per
// authenticated service. The request method/path/query remain in the request
// fingerprint, so reusing a key on another endpoint is a deterministic
// IDEMPOTENCY_CONFLICT instead of creating an ambiguous second receipt.
func privateGatewayIdempotencyScopeForService(serviceID string) string {
	raw := strings.Join([]string{"private_gateway_idempotency:v1", serviceID}, "\n")
	sum := sha256.Sum256([]byte(raw))
	return "private_gateway_idempotency:" + hex.EncodeToString(sum[:])
}

// privateGatewayIdempotencyScopeForPath is retained as a narrow test/helper
// compatibility shim. Path binding is deliberately handled by the fingerprint
// rather than by splitting the durable key namespace.
func privateGatewayIdempotencyScopeForPath(serviceID, _, _, _ string) string {
	return privateGatewayIdempotencyScopeForService(serviceID)
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

func privateGatewayRequestCompleted(c *gin.Context, writer *privateGatewayCaptureWriter) bool {
	if c == nil || writer == nil || writer.writeErr != nil || writer.overflow {
		return false
	}
	if c.Request != nil && c.Request.Context().Err() != nil {
		return false
	}
	if value, exists := c.Get(privateGatewayOutcomeUnknownKey); exists && value == true {
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

func (g *ANLPrivateGateway) markTerminal(id int64, response privateGatewayHTTPResponse) error {
	stored, err := encodePrivateGatewayStoredResponse(response)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	expiresAt := g.now().UTC().Add(privateGatewayIdempotencyTTL)
	if response.status >= http.StatusBadRequest {
		if response.retryable {
			return g.state.MarkPrivateGatewayFailedRetryable(ctx, id, response.status, stored, response.errorCode, g.now().UTC().Add(privateGatewayProcessingTTL), expiresAt)
		}
		return g.state.MarkPrivateGatewayFailed(ctx, id, response.status, stored, response.errorCode, expiresAt)
	}
	return g.state.MarkPrivateGatewaySucceeded(ctx, id, response.status, stored, expiresAt)
}

func (g *ANLPrivateGateway) markOutcomeUnknown(id int64) error {
	if g == nil || g.state == nil {
		return errors.New("private gateway state is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return g.state.MarkPrivateGatewayOutcomeUnknown(ctx, id, "IDEMPOTENCY_OUTCOME_UNKNOWN", g.now().UTC().Add(privateGatewayIdempotencyTTL))
}

func (g *ANLPrivateGateway) markExpired(id int64) error {
	if g == nil || g.state == nil {
		return errors.New("private gateway state is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return g.state.MarkPrivateGatewayExpired(ctx, id, "IDEMPOTENCY_EXPIRED")
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
	if response.status >= http.StatusBadRequest {
		return "HTTP_ERROR"
	}
	return ""
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
	if raw == nil || strings.TrimSpace(*raw) == "" {
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

func clonePrivateGatewayReplayHeader(header http.Header) http.Header {
	cloned := make(http.Header)
	for name, values := range header {
		switch http.CanonicalHeaderKey(name) {
		case "Connection", "Content-Length", "Date", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade", "Set-Cookie":
			continue
		}
		cloned[name] = append([]string(nil), values...)
	}
	return cloned
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

type privateGatewayHTTPResponse struct {
	status    int
	header    http.Header
	body      []byte
	usage     json.RawMessage
	retryable bool
	errorCode string
}

type privateGatewayCaptureWriter struct {
	gin.ResponseWriter
	body     bytes.Buffer
	maxBytes int
	overflow bool
	writeErr error
}

func (w *privateGatewayCaptureWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

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
