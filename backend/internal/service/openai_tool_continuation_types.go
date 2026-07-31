package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

func isCodexToolCallContextItemType(typ string) bool {
	switch strings.TrimSpace(typ) {
	case "tool_call",
		"function_call",
		"local_shell_call",
		"tool_search_call",
		"custom_tool_call",
		"mcp_tool_call":
		return true
	default:
		return false
	}
}

func isCodexToolCallOutputItemType(typ string) bool {
	switch strings.TrimSpace(typ) {
	case "function_call_output",
		"tool_search_output",
		"custom_tool_call_output",
		"mcp_tool_call_output":
		return true
	default:
		return false
	}
}

type ToolCallOutputContextCoverage struct {
	HasFunctionCallOutput   bool
	ContextCoversAllCallIDs bool
}

func AnalyzeToolCallOutputContextCoverageBytes(body []byte) ToolCallOutputContextCoverage {
	coverage := ToolCallOutputContextCoverage{}
	if len(body) == 0 {
		return coverage
	}
	input := parseRawJSONView(body).Get("input")
	if !input.IsArray() {
		return coverage
	}

	missingCallID := false
	var outputCallIDs map[string]struct{}
	var contextIDs map[string]struct{}
	input.ForEach(func(_, item gjson.Result) bool {
		if !item.IsObject() {
			return true
		}
		itemType := item.Get("type").String()
		switch {
		case isCodexToolCallOutputItemType(itemType):
			coverage.HasFunctionCallOutput = true
			callID := strings.TrimSpace(item.Get("call_id").String())
			if callID == "" {
				missingCallID = true
				return true
			}
			if outputCallIDs == nil {
				outputCallIDs = make(map[string]struct{})
			}
			outputCallIDs[callID] = struct{}{}
		case isCodexToolCallContextItemType(itemType):
			callID := strings.TrimSpace(item.Get("call_id").String())
			if callID == "" {
				return true
			}
			if contextIDs == nil {
				contextIDs = make(map[string]struct{})
			}
			contextIDs[callID] = struct{}{}
		case itemType == "item_reference":
			idValue := strings.TrimSpace(item.Get("id").String())
			if idValue == "" {
				return true
			}
			if contextIDs == nil {
				contextIDs = make(map[string]struct{})
			}
			contextIDs[idValue] = struct{}{}
		}
		return true
	})

	if !coverage.HasFunctionCallOutput || missingCallID {
		return coverage
	}
	for callID := range outputCallIDs {
		if _, ok := contextIDs[callID]; !ok {
			return coverage
		}
	}
	coverage.ContextCoversAllCallIDs = true
	return coverage
}
