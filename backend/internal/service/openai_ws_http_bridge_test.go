package service

import (
	"testing"

	"anl-api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPrepareOpenAIWSHTTPBridgeBodyPreservesContinuationAndNumbers(t *testing.T) {
	body, err := prepareOpenAIWSHTTPBridgeBody([]byte(`{"type":"response.create","generate":true,"model":"gpt-5","stream":false,"previous_response_id":"resp_prev","max_output_tokens":9007199254740993,"input":"hi"}`))
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(body, "type").Exists())
	require.False(t, gjson.GetBytes(body, "generate").Exists())
	require.Equal(t, "resp_prev", gjson.GetBytes(body, "previous_response_id").String())
	require.Equal(t, "9007199254740993", gjson.GetBytes(body, "max_output_tokens").Raw)
	require.Equal(t, "gpt-5", gjson.GetBytes(body, "model").String())
	require.True(t, gjson.GetBytes(body, "stream").Bool())
}

func TestShouldBridgeOpenAIWSHTTP(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = true
	svc.cfg.Gateway.OpenAIWS.HTTPBridgeThresholdBytes = 100

	require.False(t, svc.shouldBridgeOpenAIWSHTTP(&Account{Platform: PlatformOpenAI}, 99, ""))
	require.True(t, svc.shouldBridgeOpenAIWSHTTP(&Account{Platform: PlatformOpenAI}, 100, ""))
	require.False(t, svc.shouldBridgeOpenAIWSHTTP(&Account{Platform: PlatformOpenAI}, 1000, "resp_existing"))
	require.True(t, svc.shouldBridgeOpenAIWSHTTP(&Account{Platform: PlatformGrok}, 1, "resp_existing"))
}
