package apicompat

import "testing"

func TestAnthropicEventToResponses_TextEmitsContentPart(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "claude-sonnet-4-5"

	var types []string
	feed := func(evt *AnthropicStreamEvent) {
		for _, out := range AnthropicEventToResponsesEvents(evt, state) {
			types = append(types, out.Type)
		}
	}

	idx := 0
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1", Model: "claude-sonnet-4-5"}})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "text_delta", Text: "Hel"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "text_delta", Text: "lo"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	posOf := func(target string) int {
		for i, eventType := range types {
			if eventType == target {
				return i
			}
		}
		return -1
	}

	partAdded := posOf("response.content_part.added")
	firstDelta := posOf("response.output_text.delta")
	if partAdded < 0 || firstDelta < 0 || partAdded > firstDelta {
		t.Fatalf("invalid content part event ordering: %v", types)
	}
	if posOf("response.content_part.done") < 0 {
		t.Fatalf("response.content_part.done was not emitted: %v", types)
	}
}

func TestAnthropicEventToResponses_DoneEventsCarryFullText(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	idx := 0
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1"}})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "text_delta", Text: "Hello "}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "text_delta", Text: "world"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx})

	const want = "Hello world"
	var sawTextDone, sawPartDone bool
	for _, event := range events {
		switch event.Type {
		case "response.output_text.done":
			sawTextDone = event.Text == want
		case "response.content_part.done":
			sawPartDone = event.Part != nil && event.Part.Text == want
		}
	}
	if !sawTextDone || !sawPartDone {
		t.Fatalf("done events did not carry full text: text=%v part=%v", sawTextDone, sawPartDone)
	}
}

func TestAnthropicEventToResponses_CompletedCarriesOutput(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "claude-sonnet-4-5"
	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	idx := 0
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1"}})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx, ContentBlock: &AnthropicContentBlock{Type: "text"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "text_delta", Text: "4826"}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	for i := range events {
		if events[i].Type != "response.completed" || events[i].Response == nil {
			continue
		}
		output := events[i].Response.Output
		if len(output) == 0 || output[0].Type != "message" || len(output[0].Content) == 0 || output[0].Content[0].Text != "4826" {
			t.Fatalf("terminal output is incomplete: %+v", output)
		}
		return
	}
	t.Fatal("response.completed was not emitted")
}

func TestAnthropicEventToResponses_ToolCallCompletedCarriesArguments(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	var events []ResponsesStreamEvent
	feed := func(evt *AnthropicStreamEvent) {
		events = append(events, AnthropicEventToResponsesEvents(evt, state)...)
	}

	idx := 0
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1"}})
	feed(&AnthropicStreamEvent{Type: "content_block_start", Index: &idx, ContentBlock: &AnthropicContentBlock{Type: "tool_use", ID: "toolu_1", Name: "get_weather"}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "input_json_delta", PartialJSON: `{"city":`}})
	feed(&AnthropicStreamEvent{Type: "content_block_delta", Index: &idx, Delta: &AnthropicDelta{Type: "input_json_delta", PartialJSON: `"SH"}`}})
	feed(&AnthropicStreamEvent{Type: "content_block_stop", Index: &idx})
	feed(&AnthropicStreamEvent{Type: "message_stop"})

	for i := range events {
		if events[i].Type != "response.completed" || events[i].Response == nil || len(events[i].Response.Output) == 0 {
			continue
		}
		call := events[i].Response.Output[0]
		if call.Type != "function_call" || call.Name != "get_weather" || call.Arguments != `{"city":"SH"}` {
			t.Fatalf("terminal function call is incomplete: %+v", call)
		}
		return
	}
	t.Fatal("response.completed carries no output")
}
