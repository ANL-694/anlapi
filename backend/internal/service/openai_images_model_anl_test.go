package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIImagesResponsesMainModelCanBeOverridden(t *testing.T) {
	t.Setenv("SUB2API_IMAGES_MAIN_MODEL", " gpt-5.6-sol ")
	require.Equal(t, "gpt-5.6-sol", openAIImagesResponsesMainModelValue())

	t.Setenv("SUB2API_IMAGES_MAIN_MODEL", "")
	require.Equal(t, openAIImagesResponsesMainModel, openAIImagesResponsesMainModelValue())
}

func TestBuildOpenAIImagesResponsesRequestSeparatesDriverAndToolModel(t *testing.T) {
	t.Setenv("SUB2API_IMAGES_MAIN_MODEL", "gpt-5.6-sol")

	body, err := buildOpenAIImagesResponsesRequest(&OpenAIImagesRequest{
		Prompt: "draw a test chart",
		N:      1,
	}, "gpt-image-2.5-flare")
	require.NoError(t, err)
	require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(body, "model").String())
	require.Equal(t, "gpt-image-2.5-flare", gjson.GetBytes(body, "tools.0.model").String())
}

func TestOpenAIImagesToolUsageMapsImageInputTokens(t *testing.T) {
	usage, ok := openAIImagesToolUsageFromGJSON(gjson.Parse(`{
		"input_tokens": 120,
		"input_tokens_details": {"image_tokens": 80},
		"output_tokens": 30,
		"output_tokens_details": {"image_tokens": 20}
	}`))
	require.True(t, ok)
	require.Equal(t, 120, usage.InputTokens)
	require.Equal(t, 80, usage.ImageInputTokens)
	require.Equal(t, 20, usage.ImageOutputTokens)

	usage, ok = openAIImagesToolUsageFromGJSON(gjson.Parse(`{
		"input_tokens": 12,
		"input_tokens_details": {"image_tokens": 40},
		"output_tokens": 1,
		"output_tokens_details": {"image_tokens": 0}
	}`))
	require.True(t, ok)
	require.Equal(t, 12, usage.ImageInputTokens)
}
