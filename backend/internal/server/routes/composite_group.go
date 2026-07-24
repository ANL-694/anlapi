package routes

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	pkghttputil "anlapi/internal/pkg/httputil"
	"anlapi/internal/server/middleware"
	"anlapi/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func compositeTargetPlatformMiddleware(resolver *service.CompositeRouteResolver) gin.HandlerFunc {
	if resolver == nil {
		resolver = service.NewCompositeRouteResolver(nil)
	}
	return func(c *gin.Context) {
		apiKey, ok := middleware.GetAPIKeyFromContext(c)
		if !ok || apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformComposite {
			c.Next()
			return
		}
		if c.Request == nil || c.Request.Method == http.MethodGet {
			c.Next()
			return
		}

		body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil {
			status := http.StatusBadRequest
			message := "Failed to read request body"
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				status = http.StatusRequestEntityTooLarge
				message = "Request body is too large"
			}
			c.JSON(status, gin.H{"error": gin.H{"type": "invalid_request_error", "message": message}})
			c.Abort()
			return
		}

		model := compositeRequestModelFromBody(c.GetHeader("Content-Type"), body)
		if model != "" {
			endpoint := compositeRouteEndpointForPath(c.Request.URL.Path)
			decision, resolveErr := resolver.Resolve(c.Request.Context(), apiKey.Group.ID, model, endpoint)
			if resolveErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"type": "server_error", "message": "Failed to resolve composite model route"}})
				c.Abort()
				return
			}
			if decision.Matched {
				requestCtx := service.WithCompositeRouteDecision(c.Request.Context(), decision)
				resolvedKey, filterErr := filterCompositeGroupRoutes(requestCtx, apiKey, decision, resolver, endpoint)
				if filterErr != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"type": "server_error", "message": "Failed to resolve composite group routes"}})
					c.Abort()
					return
				}
				if resolvedKey != apiKey {
					c.Set(string(middleware.ContextKeyAPIKey), resolvedKey)
				}
				c.Request = c.Request.WithContext(requestCtx)
				if upstreamModel := strings.TrimSpace(decision.UpstreamModel); upstreamModel != "" && upstreamModel != model {
					if rewritten, rewriteErr := rewriteCompositeRequestModel(c.GetHeader("Content-Type"), body, upstreamModel); rewriteErr == nil {
						body = rewritten
					} else {
						c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "invalid_request_error", "message": "Failed to rewrite composite model route"}})
						c.Abort()
						return
					}
				}
			}
		}
		resetRequestBody(c, body)
		c.Next()
	}
}

func filterCompositeGroupRoutes(ctx context.Context, apiKey *service.APIKey, decision service.CompositeRouteDecision, resolver *service.CompositeRouteResolver, endpoint string) (*service.APIKey, error) {
	return resolver.FilterGroupRoutes(ctx, apiKey, decision, endpoint)
}

func compositeRequestModelFromBody(contentType string, body []byte) string {
	if model := strings.TrimSpace(gjson.GetBytes(body, "model").String()); model != "" {
		return model
	}
	return compositeMultipartModelFromBody(contentType, body)
}

func compositeMultipartModelFromBody(contentType string, body []byte) string {
	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return ""
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return ""
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			return ""
		}
		if nextErr != nil {
			return ""
		}
		if part.FormName() != "model" || part.FileName() != "" {
			continue
		}
		data, readErr := io.ReadAll(part)
		if readErr != nil {
			return ""
		}
		return strings.TrimSpace(string(data))
	}
}

func rewriteCompositeRequestModel(contentType string, body []byte, upstreamModel string) ([]byte, error) {
	upstreamModel = strings.TrimSpace(upstreamModel)
	if upstreamModel == "" {
		return body, nil
	}
	if gjson.ValidBytes(body) {
		return sjson.SetBytes(body, "model", upstreamModel)
	}
	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return nil, errors.New("unsupported composite request body")
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return nil, errors.New("multipart boundary is missing")
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var output bytes.Buffer
	writer := multipart.NewWriter(&output)
	if err := writer.SetBoundary(boundary); err != nil {
		return nil, err
	}
	found := false
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nil, nextErr
		}
		data, readErr := io.ReadAll(part)
		if readErr != nil {
			return nil, readErr
		}
		if part.FormName() == "model" && part.FileName() == "" {
			data = []byte(upstreamModel)
			found = true
		}
		partWriter, createErr := writer.CreatePart(part.Header)
		if createErr != nil {
			return nil, createErr
		}
		if _, writeErr := partWriter.Write(data); writeErr != nil {
			return nil, writeErr
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	if !found {
		return body, nil
	}
	return output.Bytes(), nil
}

func compositeGeminiTargetPlatformMiddleware(resolver *service.CompositeRouteResolver) gin.HandlerFunc {
	if resolver == nil {
		resolver = service.NewCompositeRouteResolver(nil)
	}
	return func(c *gin.Context) {
		apiKey, ok := middleware.GetAPIKeyFromContext(c)
		if ok && apiKey != nil && apiKey.Group != nil && apiKey.Group.Platform == service.PlatformComposite {
			model := compositeGeminiModelFromParams(c)
			if model != "" {
				decision, err := resolver.Resolve(c.Request.Context(), apiKey.Group.ID, model, service.CompositeRouteEndpointGemini)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"type": "server_error", "message": "Failed to resolve composite model route"}})
					c.Abort()
					return
				}
				if decision.Matched {
					c.Request = c.Request.WithContext(service.WithCompositeRouteDecision(c.Request.Context(), decision))
					rewriteCompositeGeminiModelParams(c, model, decision.UpstreamModel)
				}
			}
		}
		c.Next()
	}
}

func rewriteCompositeGeminiModelParams(c *gin.Context, publicModel, upstreamModel string) {
	if c == nil || strings.TrimSpace(publicModel) == "" || strings.TrimSpace(upstreamModel) == "" || publicModel == upstreamModel {
		return
	}
	for index := range c.Params {
		switch c.Params[index].Key {
		case "model":
			if c.Params[index].Value == publicModel {
				c.Params[index].Value = upstreamModel
			}
		case "modelAction":
			c.Params[index].Value = rewriteCompositeGeminiModelAction(c.Params[index].Value, publicModel, upstreamModel)
		}
	}
}

func rewriteCompositeGeminiModelAction(value, publicModel, upstreamModel string) string {
	prefix := ""
	if strings.HasPrefix(value, "/") {
		prefix = "/"
		value = strings.TrimPrefix(value, "/")
	}
	for _, separator := range []string{":", "/"} {
		if index := strings.Index(value, separator); index > 0 && value[:index] == publicModel {
			return prefix + upstreamModel + value[index:]
		}
	}
	if value == publicModel {
		return prefix + upstreamModel
	}
	return prefix + value
}

func compositeGeminiModelFromParams(c *gin.Context) string {
	if model := strings.TrimSpace(c.Param("model")); model != "" {
		return model
	}
	modelAction := strings.TrimPrefix(strings.TrimSpace(c.Param("modelAction")), "/")
	if idx := strings.LastIndex(modelAction, ":"); idx >= 0 {
		return strings.TrimSpace(modelAction[:idx])
	}
	return modelAction
}

func resetRequestBody(c *gin.Context, body []byte) {
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.ContentLength = int64(len(body))
	c.Request.Header.Set("Content-Length", strconv.Itoa(len(body)))
}

func compositeRouteEndpointForPath(path string) string {
	switch {
	case strings.Contains(path, "/messages/count_tokens"):
		return service.CompositeRouteEndpointCountTokens
	case strings.Contains(path, "/messages"):
		return service.CompositeRouteEndpointMessages
	case strings.Contains(path, "/responses"):
		return service.CompositeRouteEndpointResponses
	case strings.Contains(path, "/chat/completions"):
		return service.CompositeRouteEndpointChatCompletions
	case strings.Contains(path, "/embeddings"):
		return service.CompositeRouteEndpointEmbeddings
	case strings.Contains(path, "/images/"):
		return service.CompositeRouteEndpointImages
	case strings.Contains(path, "/v1beta/"):
		return service.CompositeRouteEndpointGemini
	default:
		return service.CompositeRouteEndpointAny
	}
}
