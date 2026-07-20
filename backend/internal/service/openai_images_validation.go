package service

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	openAIImageMaxCount               = 10
	openAIImageMaxPartialImages       = 3
	openAIImageMaxCompression         = 100
	openAIImageMaxPromptRunes         = 32000
	openAIImageMaxInputImages         = 16
	openAIImage2MaxEdge               = 3840
	openAIImage2MinPixels       int64 = 655360
	openAIImage2MaxPixels       int64 = 8294400
)

var openAIImageStandardSizes = map[string]struct{}{
	"1024x1024": {},
	"1536x1024": {},
	"1024x1536": {},
}

func validateOpenAIImagesRequest(req *OpenAIImagesRequest) error {
	if req == nil {
		return fmt.Errorf("images request is required")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return fmt.Errorf("prompt is required")
	}
	if utf8.RuneCountInString(req.Prompt) > openAIImageMaxPromptRunes {
		return fmt.Errorf("prompt must not exceed %d characters", openAIImageMaxPromptRunes)
	}
	if !IsGPTImageGenerationModel(req.Model) {
		return nil
	}
	if req.N < 1 || req.N > openAIImageMaxCount {
		return fmt.Errorf("n must be between 1 and %d", openAIImageMaxCount)
	}
	if req.OutputCompression != nil && (*req.OutputCompression < 0 || *req.OutputCompression > openAIImageMaxCompression) {
		return fmt.Errorf("output_compression must be between 0 and %d", openAIImageMaxCompression)
	}
	if req.PartialImages != nil {
		if *req.PartialImages < 0 || *req.PartialImages > openAIImageMaxPartialImages {
			return fmt.Errorf("partial_images must be between 0 and %d", openAIImageMaxPartialImages)
		}
		if *req.PartialImages > 0 && !req.Stream {
			return fmt.Errorf("partial_images requires stream=true")
		}
	}

	// response_format is a gateway compatibility extension for older clients.
	// It is removed before forwarding to GPT Image upstreams.
	req.ResponseFormat = strings.ToLower(strings.TrimSpace(req.ResponseFormat))
	if !oneOfOrEmpty(req.ResponseFormat, "b64_json", "url") {
		return fmt.Errorf("response_format must be one of b64_json or url")
	}

	var err error
	if req.Quality, err = normalizeOpenAIImageEnum("quality", req.Quality); err != nil {
		return err
	}
	if req.Background, err = normalizeOpenAIImageEnum("background", req.Background); err != nil {
		return err
	}
	if req.OutputFormat, err = normalizeOpenAIImageEnum("output_format", req.OutputFormat); err != nil {
		return err
	}
	if req.Moderation, err = normalizeOpenAIImageEnum("moderation", req.Moderation); err != nil {
		return err
	}
	if req.InputFidelity, err = normalizeOpenAIImageEnum("input_fidelity", req.InputFidelity); err != nil {
		return err
	}
	if req.Style, err = normalizeOpenAIImageEnum("style", req.Style); err != nil {
		return err
	}
	req.Size = strings.TrimSpace(req.Size)

	if !oneOfOrEmpty(req.Quality, "auto", "low", "medium", "high") {
		return fmt.Errorf("quality must be one of auto, low, medium, or high")
	}
	if isGPTImage2Model(req.Model) {
		if !oneOfOrEmpty(req.Background, "auto", "opaque") {
			return fmt.Errorf("gpt-image-2 background must be one of auto or opaque")
		}
	} else if !oneOfOrEmpty(req.Background, "auto", "opaque", "transparent") {
		return fmt.Errorf("background must be one of auto, opaque, or transparent")
	}
	if !oneOfOrEmpty(req.OutputFormat, "png", "jpeg", "webp") {
		return fmt.Errorf("output_format must be one of png, jpeg, or webp")
	}
	if !oneOfOrEmpty(req.Moderation, "auto", "low") {
		return fmt.Errorf("moderation must be one of auto or low")
	}
	if !oneOfOrEmpty(req.InputFidelity, "low", "high") {
		return fmt.Errorf("input_fidelity must be one of low or high")
	}
	if isGPTImage2Model(req.Model) && req.InputFidelity != "" {
		return fmt.Errorf("input_fidelity is not supported by gpt-image-2")
	}
	if req.InputFidelity != "" && !req.IsEdits() {
		return fmt.Errorf("input_fidelity is only supported for image edits")
	}
	if req.Style != "" {
		return fmt.Errorf("style is not supported by GPT Image models")
	}

	effectiveFormat := req.OutputFormat
	if effectiveFormat == "" {
		effectiveFormat = "png"
	}
	if req.OutputCompression != nil && effectiveFormat != "jpeg" && effectiveFormat != "webp" {
		return fmt.Errorf("output_compression requires output_format jpeg or webp")
	}
	if req.Background == "transparent" && effectiveFormat != "png" && effectiveFormat != "webp" {
		return fmt.Errorf("transparent background requires output_format png or webp")
	}
	if err := validateOpenAIImageInputs(req); err != nil {
		return err
	}

	if isGPTImage2Model(req.Model) {
		return validateGPTImage2Size(req.Size)
	}
	return validateLegacyGPTImageSize(req.Size)
}

func normalizeOpenAIImageEnum(name, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	normalized := strings.ToLower(trimmed)
	if normalized != trimmed {
		return "", fmt.Errorf("%s must use a lowercase value", name)
	}
	return normalized, nil
}

func oneOfOrEmpty(value string, allowed ...string) bool {
	if value == "" {
		return true
	}
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func isGPTImage2Model(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return model == "gpt-image-2" || strings.HasPrefix(model, "gpt-image-2-")
}

func validateLegacyGPTImageSize(size string) error {
	if size == "" || size == "auto" {
		return nil
	}
	if _, ok := openAIImageStandardSizes[size]; ok {
		return nil
	}
	return fmt.Errorf("size %q is not supported by %s", size, "this GPT Image model")
}

func validateGPTImage2Size(size string) error {
	if size == "" || size == "auto" {
		return nil
	}
	width, height, err := parseOpenAIImageSize(size)
	if err != nil {
		return err
	}
	if width%16 != 0 || height%16 != 0 {
		return fmt.Errorf("gpt-image-2 size width and height must be multiples of 16")
	}
	if int64(width) > int64(height)*3 || int64(height) > int64(width)*3 {
		return fmt.Errorf("gpt-image-2 size aspect ratio must be between 1:3 and 3:1")
	}
	if width > openAIImage2MaxEdge || height > openAIImage2MaxEdge {
		return fmt.Errorf("gpt-image-2 size edges must not exceed %d pixels", openAIImage2MaxEdge)
	}
	pixels := int64(width) * int64(height)
	if pixels < openAIImage2MinPixels || pixels > openAIImage2MaxPixels {
		return fmt.Errorf("gpt-image-2 size total pixels must be between %d and %d", openAIImage2MinPixels, openAIImage2MaxPixels)
	}
	return nil
}

func validateOpenAIImageInputs(req *OpenAIImagesRequest) error {
	if req == nil || !req.IsEdits() {
		return nil
	}
	inputCount := len(req.Uploads) + len(req.InputImageURLs) + len(req.InputImageFileIDs)
	if inputCount == 0 {
		return fmt.Errorf("at least one input image is required")
	}
	if inputCount > openAIImageMaxInputImages {
		return fmt.Errorf("image edits support at most %d input images", openAIImageMaxInputImages)
	}
	for _, upload := range req.Uploads {
		if len(upload.Data) == 0 {
			return fmt.Errorf("input image %q is empty", upload.FileName)
		}
		if !isSupportedOpenAIImageUpload(upload) {
			return fmt.Errorf("input image %q must be PNG, JPEG, or WebP", upload.FileName)
		}
	}
	if req.MaskUpload != nil {
		if len(req.MaskUpload.Data) == 0 {
			return fmt.Errorf("mask image is empty")
		}
		if len(req.MaskUpload.Data) > openAIImageMaxMaskUploadSize {
			return fmt.Errorf("mask image exceeds the %d byte limit", openAIImageMaxMaskUploadSize)
		}
		if !isPNGOpenAIImageUpload(*req.MaskUpload) {
			return fmt.Errorf("mask image must be PNG")
		}
	}
	return nil
}

func isSupportedOpenAIImageUpload(upload OpenAIImagesUpload) bool {
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(upload.ContentType, ";")[0]))
	if contentType == "image/png" || contentType == "image/jpeg" || contentType == "image/webp" {
		return true
	}
	if contentType != "" && contentType != "application/octet-stream" {
		return false
	}
	switch strings.ToLower(filepath.Ext(upload.FileName)) {
	case ".png", ".jpg", ".jpeg", ".webp":
		return true
	default:
		return false
	}
}

func isPNGOpenAIImageUpload(upload OpenAIImagesUpload) bool {
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(upload.ContentType, ";")[0]))
	if contentType == "image/png" {
		return true
	}
	if contentType != "" && contentType != "application/octet-stream" {
		return false
	}
	return strings.EqualFold(filepath.Ext(upload.FileName), ".png")
}

func parseOpenAIImageSize(size string) (int, int, error) {
	parts := strings.Split(size, "x")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return 0, 0, fmt.Errorf("size must be auto or WIDTHxHEIGHT")
	}
	width, err := strconv.Atoi(parts[0])
	if err != nil || width <= 0 {
		return 0, 0, fmt.Errorf("size width must be a positive integer")
	}
	height, err := strconv.Atoi(parts[1])
	if err != nil || height <= 0 {
		return 0, 0, fmt.Errorf("size height must be a positive integer")
	}
	return width, height, nil
}
