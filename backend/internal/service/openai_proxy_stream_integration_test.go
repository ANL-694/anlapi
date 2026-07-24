package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"anlapi/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIStreamReadThenErrorCloser struct {
	reader *strings.Reader
	err    error
}

func (reader *openAIStreamReadThenErrorCloser) Read(buffer []byte) (int, error) {
	if reader.reader != nil && reader.reader.Len() > 0 {
		return reader.reader.Read(buffer)
	}
	return 0, reader.err
}

func (reader *openAIStreamReadThenErrorCloser) Close() error { return nil }

func TestOpenAIStreamingPostOutputDisconnectQuarantinesSharedProxyWithoutSameStreamFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	proxyID := int64(4698)
	account := &Account{
		ID:       469801,
		Name:     "oauth-on-shared-proxy",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		ProxyID:  &proxyID,
	}
	service := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		MaxLineSize: defaultMaxLineSize,
	}}}

	for _, readError := range []error{
		io.ErrUnexpectedEOF,
		errors.New("http2: client connection lost"),
	} {
		recorder := httptest.NewRecorder()
		ginContext, _ := gin.CreateTestContext(recorder)
		ginContext.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		response := &http.Response{
			StatusCode: http.StatusOK,
			Body: &openAIStreamReadThenErrorCloser{
				reader: strings.NewReader(strings.Join([]string{
					"event: response.output_text.delta",
					`data: {"type":"response.output_text.delta","delta":"partial"}`,
					"",
				}, "\n")),
				err: readError,
			},
			Header: http.Header{"X-Request-Id": []string{"rid-proxy-disconnect"}},
		}

		_, err := service.handleStreamingResponse(ginContext.Request.Context(), response, ginContext, account, time.Now(), "gpt-5.6-sol", "gpt-5.6-sol")
		require.Error(t, err)
		var failoverError *UpstreamFailoverError
		require.False(t, errors.As(err, &failoverError), "post-output disconnect must not fail over inside the same stream")
		require.Contains(t, recorder.Body.String(), "partial")
	}

	scheduler := &defaultOpenAIAccountScheduler{service: service}
	compatible, reason := scheduler.isAccountRequestCompatibleReason(context.Background(), account, OpenAIAccountScheduleRequest{})
	require.False(t, compatible, "the next request must exclude accounts sharing the quarantined proxy")
	require.Equal(t, "proxy_stream_quarantined", reason)
}

func TestOpenAIStreamingTerminalAndClientCancellationDoNotQuarantineProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	proxyID := int64(4699)
	account := &Account{ID: 469901, Name: "oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, ProxyID: &proxyID}
	service := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}

	terminalRecorder := httptest.NewRecorder()
	terminalContext, _ := gin.CreateTestContext(terminalRecorder)
	terminalContext.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	terminalResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body: &openAIStreamReadThenErrorCloser{
			reader: strings.NewReader(strings.Join([]string{
				"event: response.completed",
				`data: {"type":"response.completed","response":{"status":"completed","output":[]}}`,
				"",
			}, "\n")),
			err: io.ErrUnexpectedEOF,
		},
		Header: http.Header{},
	}
	_, err := service.handleStreamingResponse(terminalContext.Request.Context(), terminalResponse, terminalContext, account, time.Now(), "model", "model")
	require.NoError(t, err)

	for attempt := 0; attempt < 2; attempt++ {
		recorder := httptest.NewRecorder()
		ginContext, _ := gin.CreateTestContext(recorder)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		ginContext.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
		response := &http.Response{
			StatusCode: http.StatusOK,
			Body: &openAIStreamReadThenErrorCloser{
				reader: strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"),
				err:    context.Canceled,
			},
			Header: http.Header{},
		}
		_, err = service.handleStreamingResponse(ginContext.Request.Context(), response, ginContext, account, time.Now(), "model", "model")
		require.Error(t, err)
	}

	scheduler := &defaultOpenAIAccountScheduler{service: service}
	compatible, reason := scheduler.isAccountRequestCompatibleReason(context.Background(), account, OpenAIAccountScheduleRequest{})
	require.True(t, compatible)
	require.Empty(t, reason)
}

func TestOpenAIStreamingPassthroughPostOutputDisconnectQuarantinesSharedProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	proxyID := int64(4698)
	account := &Account{ID: 469804, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, ProxyID: &proxyID}
	service := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}

	for _, readError := range []error{io.ErrUnexpectedEOF, errors.New("http2: client connection lost")} {
		recorder := httptest.NewRecorder()
		ginContext, _ := gin.CreateTestContext(recorder)
		ginContext.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		response := &http.Response{
			StatusCode: http.StatusOK,
			Body: &openAIStreamReadThenErrorCloser{
				reader: strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"),
				err:    readError,
			},
			Header: http.Header{"X-Request-Id": []string{"rid-passthrough-proxy-disconnect"}},
		}

		_, err := service.handleStreamingResponsePassthrough(ginContext.Request.Context(), response, ginContext, account, time.Now(), "model", "model")
		require.Error(t, err)
		var failoverError *UpstreamFailoverError
		require.False(t, errors.As(err, &failoverError), "post-output disconnect must not fail over inside the same stream")
		require.Contains(t, recorder.Body.String(), "partial")
	}

	require.True(t, service.isOpenAIProxyStreamQuarantined(account))
}

func TestOpenAIGatewayServiceSelectAccountWithSchedulerSkipsQuarantinedSharedProxy(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	proxyA := int64(4698)
	proxyB := int64(4699)
	accounts := []Account{
		{ID: 469801, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, ProxyID: &proxyA},
		{ID: 469802, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, ProxyID: &proxyA},
		{ID: 469803, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5, ProxyID: &proxyB},
	}
	gatewayConfig := &config.Config{}
	gatewayConfig.Gateway.Scheduling.LoadBatchEnabled = false
	service := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cfg:                gatewayConfig,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
		openaiProxyStreamCircuit: newOpenAIProxyStreamCircuit(openAIProxyStreamCircuitSettings{
			failureThreshold: 1,
			failureWindow:    time.Minute,
			quarantineTTL:    10 * time.Minute,
			maxEntries:       16,
		}),
	}
	service.openaiProxyStreamCircuit.recordFailure(proxyA, time.Now())

	selection, _, err := service.SelectAccountWithScheduler(
		context.Background(), nil, "", "", "gpt-5.6-sol", nil, OpenAIUpstreamTransportAny, false,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(469803), selection.Account.ID)
}
