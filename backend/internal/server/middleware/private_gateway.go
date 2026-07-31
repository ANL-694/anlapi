package middleware

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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

	privateGatewayServiceIDHeader  = "X-ANL-Service-ID"
	privateGatewayTimestampHeader  = "X-ANL-Timestamp"
	privateGatewayNonceHeader      = "X-ANL-Nonce"
	privateGatewayBodySHA256Header = "X-ANL-Body-SHA256"
	privateGatewaySignatureHeader  = "X-ANL-Signature"

	privateGatewayRequestContextKey       = "_anl_private_gateway_request"
	privateGatewayPreauthenticatedKey     = "_anl_private_gateway_api_key"
	privateGatewayIdempotencyReplayHeader = "Idempotency-Replayed"
)

var privateGatewayNoncePattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

type PrivateGatewayAPIKeyLoader func(context.Context, int64) (*service.APIKey, error)

type privateGatewayRequestContext struct {
	serviceID  string
	bodySHA256 string
}

// ANLPrivateGateway owns process-local replay and idempotency state for the
// Hub service-authenticated branch of the existing OpenAI-compatible routes.
type ANLPrivateGateway struct {
	cfg         config.PrivateGatewayConfig
	configValid bool
	loadAPIKey  PrivateGatewayAPIKeyLoader
	now         func() time.Time
	nonces      *privateGatewayNonceStore
	idempotency *privateGatewayIdempotencyStore
}

func NewANLPrivateGateway(cfg config.PrivateGatewayConfig, loader PrivateGatewayAPIKeyLoader) *ANLPrivateGateway {
	return newANLPrivateGatewayWithRuntime(cfg, loader, time.Now)
}

func newANLPrivateGatewayWithRuntime(
	cfg config.PrivateGatewayConfig,
	loader PrivateGatewayAPIKeyLoader,
	now func() time.Time,
) *ANLPrivateGateway {
	if now == nil {
		now = time.Now
	}
	return &ANLPrivateGateway{
		cfg:         cfg,
		configValid: cfg.Validate() == nil,
		loadAPIKey:  loader,
		now:         now,
		nonces:      newPrivateGatewayNonceStore(),
		idempotency: newPrivateGatewayIdempotencyStore(now),
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

		if !g.nonces.use(serviceID, nonce, now) {
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

		key, err := service.NormalizeIdempotencyKey(c.GetHeader("Idempotency-Key"))
		if err != nil || key == "" {
			abortPrivateGatewayError(c, http.StatusBadRequest, "INVALID_REQUEST", "A valid Idempotency-Key is required")
			return
		}

		scope := privateGatewayIdempotencyScope(requestContext.serviceID, c.Request, key)
		fingerprint := privateGatewayIdempotencyFingerprint(requestContext, c.Request)
		claim, replay := g.idempotency.claim(scope, fingerprint)
		switch claim {
		case privateGatewayIdempotencyConflict:
			abortPrivateGatewayError(c, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "Idempotency-Key was reused with a different request")
			return
		case privateGatewayIdempotencyInProgress:
			c.Header("Retry-After", "1")
			abortPrivateGatewayError(c, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS", "The idempotent request is still in progress")
			return
		case privateGatewayIdempotencyReplay:
			writePrivateGatewayReplay(c, replay)
			return
		}

		writer := &privateGatewayCaptureWriter{ResponseWriter: c.Writer}
		c.Writer = writer
		completed := false
		defer func() {
			if !completed {
				g.idempotency.abandon(scope, fingerprint)
			}
		}()

		c.Next()
		response := privateGatewayHTTPResponse{
			status: c.Writer.Status(),
			header: clonePrivateGatewayReplayHeader(c.Writer.Header()),
			body:   append([]byte(nil), writer.body.Bytes()...),
		}
		g.idempotency.complete(scope, fingerprint, response)
		completed = true
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
		return false
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

type privateGatewayNonceStore struct {
	mu          sync.Mutex
	entries     map[string]time.Time
	nextCleanup time.Time
}

func newPrivateGatewayNonceStore() *privateGatewayNonceStore {
	return &privateGatewayNonceStore{entries: make(map[string]time.Time)}
}

func (s *privateGatewayNonceStore) use(serviceID, nonce string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !now.Before(s.nextCleanup) {
		for key, expiresAt := range s.entries {
			if !expiresAt.After(now) {
				delete(s.entries, key)
			}
		}
		s.nextCleanup = now.Add(time.Minute)
	}
	key := serviceID + "\n" + nonce
	if expiresAt, exists := s.entries[key]; exists && expiresAt.After(now) {
		return false
	}
	s.entries[key] = now.Add(privateGatewayNonceTTL)
	return true
}

type privateGatewayIdempotencyClaim int

const (
	privateGatewayIdempotencyOwner privateGatewayIdempotencyClaim = iota
	privateGatewayIdempotencyConflict
	privateGatewayIdempotencyInProgress
	privateGatewayIdempotencyReplay
)

type privateGatewayHTTPResponse struct {
	status int
	header http.Header
	body   []byte
}

type privateGatewayIdempotencyEntry struct {
	fingerprint string
	processing  bool
	expiresAt   time.Time
	response    privateGatewayHTTPResponse
}

type privateGatewayIdempotencyStore struct {
	mu          sync.Mutex
	now         func() time.Time
	entries     map[string]*privateGatewayIdempotencyEntry
	nextCleanup time.Time
}

func newPrivateGatewayIdempotencyStore(now func() time.Time) *privateGatewayIdempotencyStore {
	return &privateGatewayIdempotencyStore{
		now:     now,
		entries: make(map[string]*privateGatewayIdempotencyEntry),
	}
}

func (s *privateGatewayIdempotencyStore) claim(scope, fingerprint string) (privateGatewayIdempotencyClaim, privateGatewayHTTPResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	if !now.Before(s.nextCleanup) {
		for key, entry := range s.entries {
			if !entry.expiresAt.After(now) {
				delete(s.entries, key)
			}
		}
		s.nextCleanup = now.Add(10 * time.Minute)
	}
	if existing := s.entries[scope]; existing != nil {
		if existing.fingerprint != fingerprint {
			return privateGatewayIdempotencyConflict, privateGatewayHTTPResponse{}
		}
		if existing.processing {
			return privateGatewayIdempotencyInProgress, privateGatewayHTTPResponse{}
		}
		return privateGatewayIdempotencyReplay, clonePrivateGatewayHTTPResponse(existing.response)
	}
	s.entries[scope] = &privateGatewayIdempotencyEntry{
		fingerprint: fingerprint,
		processing:  true,
		expiresAt:   now.Add(privateGatewayIdempotencyTTL),
	}
	return privateGatewayIdempotencyOwner, privateGatewayHTTPResponse{}
}

func (s *privateGatewayIdempotencyStore) complete(scope, fingerprint string, response privateGatewayHTTPResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entries[scope]
	if entry == nil || entry.fingerprint != fingerprint || !entry.processing {
		return
	}
	entry.processing = false
	entry.response = clonePrivateGatewayHTTPResponse(response)
	entry.expiresAt = s.now().UTC().Add(privateGatewayIdempotencyTTL)
}

func (s *privateGatewayIdempotencyStore) abandon(scope, fingerprint string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entries[scope]
	if entry != nil && entry.processing && entry.fingerprint == fingerprint {
		delete(s.entries, scope)
	}
}

func privateGatewayIdempotencyScope(serviceID string, r *http.Request, key string) string {
	raw := strings.Join([]string{serviceID, strings.ToUpper(r.Method), r.URL.EscapedPath(), key}, "\n")
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
	body bytes.Buffer
}

func (w *privateGatewayCaptureWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *privateGatewayCaptureWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	_, _ = w.body.Write(data[:n])
	return n, err
}

func (w *privateGatewayCaptureWriter) WriteString(data string) (int, error) {
	n, err := w.ResponseWriter.WriteString(data)
	_, _ = w.body.WriteString(data[:n])
	return n, err
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
		status: response.status,
		header: clonePrivateGatewayReplayHeader(response.header),
		body:   append([]byte(nil), response.body...),
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
