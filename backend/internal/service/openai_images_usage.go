package service

import "github.com/tidwall/gjson"

func (s *OpenAIGatewayService) parseOpenAIImagesSSEUsageBytes(data []byte, usage *OpenAIUsage) {
	s.parseSSEUsageBytes(data, usage)
	if usage == nil || !gjson.ValidBytes(data) || gjson.GetBytes(data, "type").String() != "response.completed" {
		return
	}
	if toolUsage, ok := openAIImagesToolUsageFromGJSON(gjson.GetBytes(data, "response.tool_usage.image_gen")); ok {
		*usage = toolUsage
	}
}

func openAIImagesToolUsageFromGJSON(value gjson.Result) (OpenAIUsage, bool) {
	if !value.Exists() || !value.IsObject() {
		return OpenAIUsage{}, false
	}
	inputTokens, inputOK := boundedJSONNonNegativeInt(value.Get("input_tokens"))
	outputTokens, outputOK := boundedJSONNonNegativeInt(value.Get("output_tokens"))
	imageOutputTokens, imageOutputOK := boundedJSONNonNegativeInt(value.Get("output_tokens_details.image_tokens"))
	if !inputOK || !outputOK || !imageOutputOK {
		return OpenAIUsage{}, false
	}
	return OpenAIUsage{
		InputTokens:       inputTokens,
		OutputTokens:      outputTokens,
		ImageOutputTokens: imageOutputTokens,
	}, true
}

// boundedJSONNonNegativeInt parses integral JSON exponent notation without
// invoking an arbitrary-precision parser on an upstream-controlled exponent.
func boundedJSONNonNegativeInt(value gjson.Result) (int, bool) {
	if !value.Exists() || value.Type != gjson.Number {
		return 0, false
	}
	raw := value.Raw
	if len(raw) == 0 || len(raw) > 64 || raw[0] == '-' {
		return 0, false
	}

	mantissaEnd := len(raw)
	for i, c := range raw {
		if c == 'e' || c == 'E' {
			mantissaEnd = i
			break
		}
	}

	digits := raw[:mantissaEnd]
	fractionDigits := 0
	digitCount := 0
	dotSeen := false
	mantissaIsZero := true
	for _, c := range digits {
		switch {
		case c == '.' && !dotSeen:
			dotSeen = true
		case c >= '0' && c <= '9':
			digitCount++
			mantissaIsZero = mantissaIsZero && c == '0'
			if dotSeen {
				fractionDigits++
			}
		default:
			return 0, false
		}
	}

	exponent := 0
	if mantissaEnd < len(raw) {
		exponentRaw := raw[mantissaEnd+1:]
		negative := false
		if len(exponentRaw) > 0 && (exponentRaw[0] == '+' || exponentRaw[0] == '-') {
			negative = exponentRaw[0] == '-'
			exponentRaw = exponentRaw[1:]
		}
		if len(exponentRaw) == 0 {
			return 0, false
		}
		for len(exponentRaw) > 1 && exponentRaw[0] == '0' {
			exponentRaw = exponentRaw[1:]
		}
		for _, digit := range exponentRaw {
			if digit < '0' || digit > '9' {
				return 0, false
			}
		}
		if mantissaIsZero {
			return 0, true
		}
		if len(exponentRaw) > 3 {
			return 0, false
		}
		for _, digit := range exponentRaw {
			exponent = exponent*10 + int(digit-'0')
		}
		if exponent > 100 {
			return 0, false
		}
		if negative {
			exponent = -exponent
		}
	}

	trailingZeros := exponent - fractionDigits
	scaleReduction := 0
	if trailingZeros < 0 {
		scaleReduction = -trailingZeros
		remaining := scaleReduction
		allZeros := true
		for i := len(digits) - 1; i >= 0; i-- {
			if digits[i] == '.' {
				continue
			}
			if digits[i] != '0' {
				allZeros = false
				if remaining > 0 {
					return 0, false
				}
			}
			if remaining > 0 {
				remaining--
			}
		}
		if remaining > 0 {
			if allZeros {
				return 0, true
			}
			return 0, false
		}
	}

	maxInt := int(^uint(0) >> 1)
	parsed := 0
	digitsToAccumulate := digitCount - scaleReduction
	for _, c := range digits {
		if c == '.' {
			continue
		}
		if digitsToAccumulate <= 0 {
			break
		}
		if parsed > (maxInt-int(c-'0'))/10 {
			return 0, false
		}
		parsed = parsed*10 + int(c-'0')
		digitsToAccumulate--
	}
	if trailingZeros < 0 {
		return parsed, true
	}
	for ; trailingZeros > 0; trailingZeros-- {
		if parsed > maxInt/10 {
			return 0, false
		}
		parsed *= 10
	}
	return parsed, true
}
