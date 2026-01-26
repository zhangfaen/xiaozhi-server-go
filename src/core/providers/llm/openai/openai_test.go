package openai

import (
	"testing"
	"xiaozhi-server-go/src/core/types"
)

func TestIsInvalidAssistantMessage(t *testing.T) {
	t.Run("assistant_empty_content_without_tool_calls", func(t *testing.T) {
		msg := types.Message{Role: "assistant"}
		if !isInvalidAssistantMessage(msg) {
			t.Fatalf("expected invalid assistant message, got valid: %+v", msg)
		}
	})

	t.Run("assistant_whitespace_content_without_tool_calls", func(t *testing.T) {
		msg := types.Message{Role: "assistant", Content: "   \n\t"}
		if !isInvalidAssistantMessage(msg) {
			t.Fatalf("expected invalid assistant message, got valid: %+v", msg)
		}
	})

	t.Run("assistant_non_empty_content_without_tool_calls", func(t *testing.T) {
		msg := types.Message{Role: "assistant", Content: "ok"}
		if isInvalidAssistantMessage(msg) {
			t.Fatalf("expected valid assistant message, got invalid: %+v", msg)
		}
	})

	t.Run("assistant_empty_content_with_tool_calls", func(t *testing.T) {
		msg := types.Message{
			Role: "assistant",
			ToolCalls: []types.ToolCall{{
				ID:   "tool_1",
				Type: "function",
				Function: types.FunctionCall{
					Name:      "local_get_time",
					Arguments: "{}",
				},
				Index: 0,
			}},
		}
		if isInvalidAssistantMessage(msg) {
			t.Fatalf("expected valid assistant message (tool_calls set), got invalid: %+v", msg)
		}
	})
}
