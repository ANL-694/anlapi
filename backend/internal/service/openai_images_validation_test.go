package service

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestValidateOpenAIImagesRequest_GPTImage2Sizes(t *testing.T) {
	tests := []struct {
		name    string
		size    string
		wantErr string
	}{
		{name: "standard", size: "1024x1024"},
		{name: "minimum pixels", size: "1024x640"},
		{name: "custom landscape", size: "2048x1024"},
		{name: "maximum portrait", size: "2160x3840"},
		{name: "maximum square pixels", size: "2880x2880"},
		{name: "auto", size: "auto"},
		{name: "not multiple of 16", size: "1025x1024", wantErr: "multiples of 16"},
		{name: "aspect ratio too wide", size: "3840x1024", wantErr: "between 1:3 and 3:1"},
		{name: "below minimum pixels", size: "1008x640", wantErr: "total pixels"},
		{name: "above maximum pixels", size: "2896x2880", wantErr: "total pixels"},
		{name: "edge too long", size: "3856x2144", wantErr: "must not exceed"},
		{name: "invalid format", size: "square", wantErr: "WIDTHxHEIGHT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &OpenAIImagesRequest{
				Endpoint: openAIImagesGenerationsEndpoint,
				Model:    "gpt-image-2",
				Prompt:   "draw a lighthouse",
				N:        1,
				Size:     tt.size,
			}
			err := validateOpenAIImagesRequest(req)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestValidateOpenAIImagesRequest_OfficialParameterBounds(t *testing.T) {
	compression := 80
	partialImages := 2
	req := &OpenAIImagesRequest{
		Endpoint:          openAIImagesEditsEndpoint,
		Model:             "gpt-image-2-2026-04-21",
		Prompt:            "replace the background",
		N:                 10,
		Size:              "1536x1024",
		Stream:            true,
		Quality:           "high",
		Background:        "opaque",
		OutputFormat:      "webp",
		Moderation:        "low",
		OutputCompression: &compression,
		PartialImages:     &partialImages,
		ResponseFormat:    "url",
		InputImageURLs:    []string{"https://example.com/source.png"},
	}

	require.NoError(t, validateOpenAIImagesRequest(req))
	require.Equal(t, "high", req.Quality)
	require.Equal(t, "webp", req.OutputFormat)
}

func TestValidateOpenAIImagesRequest_RejectsInvalidParameterCombinations(t *testing.T) {
	compression := 80
	partialImages := 1
	tests := []struct {
		name    string
		mutate  func(*OpenAIImagesRequest)
		wantErr string
	}{
		{name: "too many images", mutate: func(r *OpenAIImagesRequest) { r.N = 11 }, wantErr: "n must be between"},
		{name: "compression with png", mutate: func(r *OpenAIImagesRequest) { r.OutputCompression = &compression }, wantErr: "requires output_format jpeg or webp"},
		{name: "partials without stream", mutate: func(r *OpenAIImagesRequest) { r.PartialImages = &partialImages }, wantErr: "requires stream=true"},
		{name: "gpt image 2 input fidelity", mutate: func(r *OpenAIImagesRequest) { r.InputFidelity = "high" }, wantErr: "not supported by gpt-image-2"},
		{name: "gpt image 2 transparent background", mutate: func(r *OpenAIImagesRequest) { r.Background = "transparent" }, wantErr: "background must be one of auto or opaque"},
		{name: "unsupported style", mutate: func(r *OpenAIImagesRequest) { r.Style = "vivid" }, wantErr: "style is not supported"},
		{name: "invalid quality", mutate: func(r *OpenAIImagesRequest) { r.Quality = "ultra" }, wantErr: "quality must be one of"},
		{name: "uppercase quality", mutate: func(r *OpenAIImagesRequest) { r.Quality = "HIGH" }, wantErr: "must use a lowercase value"},
		{name: "legacy custom size", mutate: func(r *OpenAIImagesRequest) { r.Model = "gpt-image-1.5"; r.Size = "2048x1024" }, wantErr: "is not supported"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &OpenAIImagesRequest{
				Endpoint: openAIImagesGenerationsEndpoint,
				Model:    "gpt-image-2",
				Prompt:   "draw a lighthouse",
				N:        1,
				Size:     "1024x1024",
			}
			tt.mutate(req)
			require.ErrorContains(t, validateOpenAIImagesRequest(req), tt.wantErr)
		})
	}
}

func TestValidateOpenAIImagesRequest_DoesNotApplyGPTSizeRulesToGrok(t *testing.T) {
	req := &OpenAIImagesRequest{
		Endpoint: openAIImagesGenerationsEndpoint,
		Model:    "grok-imagine",
		Prompt:   "draw a lighthouse",
		N:        1,
		Size:     "1344x768",
	}
	require.NoError(t, validateOpenAIImagesRequest(req))
}

func TestOpenAIGatewayServiceParseOpenAIImagesRequest_RejectsFractionalIntegers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{
		`{"model":"gpt-image-2","prompt":"draw","n":1.5}`,
		`{"model":"gpt-image-2","prompt":"draw","output_format":"webp","output_compression":80.5}`,
		`{"model":"gpt-image-2","prompt":"draw","stream":true,"partial_images":1.5}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Request = req

		parsed, err := (&OpenAIGatewayService{}).ParseOpenAIImagesRequest(ctx, []byte(body))
		require.Nil(t, parsed)
		require.Error(t, err)
	}
}

func TestValidateOpenAIImagesRequest_EditInputLimits(t *testing.T) {
	base := func() *OpenAIImagesRequest {
		return &OpenAIImagesRequest{
			Endpoint:          openAIImagesEditsEndpoint,
			Model:             "gpt-image-2",
			Prompt:            "edit",
			N:                 1,
			InputImageFileIDs: []string{"file-source"},
		}
	}

	require.NoError(t, validateOpenAIImagesRequest(base()))

	tooMany := base()
	tooMany.InputImageFileIDs = make([]string, openAIImageMaxInputImages+1)
	require.ErrorContains(t, validateOpenAIImagesRequest(tooMany), "at most 16")

	invalidUpload := base()
	invalidUpload.InputImageFileIDs = nil
	invalidUpload.Uploads = []OpenAIImagesUpload{{FileName: "source.gif", ContentType: "image/gif", Data: []byte("gif")}}
	require.ErrorContains(t, validateOpenAIImagesRequest(invalidUpload), "PNG, JPEG, or WebP")

	invalidMask := base()
	invalidMask.MaskUpload = &OpenAIImagesUpload{FileName: "mask.jpg", ContentType: "image/jpeg", Data: []byte("jpeg")}
	require.ErrorContains(t, validateOpenAIImagesRequest(invalidMask), "mask image must be PNG")
}

func TestValidateOpenAIImagesRequest_RejectsOverlongPrompt(t *testing.T) {
	req := &OpenAIImagesRequest{
		Endpoint: openAIImagesGenerationsEndpoint,
		Model:    "gpt-image-2",
		Prompt:   strings.Repeat("图", openAIImageMaxPromptRunes+1),
		N:        1,
	}
	require.ErrorContains(t, validateOpenAIImagesRequest(req), "must not exceed 32000")
}

func TestOpenAIGatewayServiceParseOpenAIImagesRequest_AcceptsFileIDsForNativeEdits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-image-2","prompt":"edit","images":[{"file_id":"file-source"}],"mask":{"file_id":"file-mask"}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = req

	parsed, err := (&OpenAIGatewayService{}).ParseOpenAIImagesRequest(ctx, body)
	require.NoError(t, err)
	require.Equal(t, []string{"file-source"}, parsed.InputImageFileIDs)
	require.Equal(t, "file-mask", parsed.MaskImageFileID)
	require.True(t, parsed.HasMask)
	require.Equal(t, OpenAIImagesCapabilityNative, parsed.RequiredCapability)
}
