package agent

// History is a conversation in the wire format the API expects. The assistant
// turns carry their reasoning_details, so the model can follow up on its own
// thinking on a later turn.
type History []map[string]any

// WithUser appends a user turn.
func (h History) WithUser(text string) History {
	return append(h, map[string]any{"role": "user", "content": text})
}

// WithAssistant appends an assistant turn, preceded by the tool calls and tool
// results that produced it.
func (h History) WithAssistant(msg Message) History {
	h = append(h, msg.Steps...)
	return append(h, map[string]any{
		"role":              "assistant",
		"content":           msg.Content,
		"reasoning_details": msg.ReasoningDetails,
	})
}

// WithToolCalls appends the assistant turn that asked for tools. The results
// follow it as tool turns, one for each call.
func (h History) WithToolCalls(msg Message) History {
	return append(h, map[string]any{
		"role":              "assistant",
		"content":           msg.Content,
		"reasoning_details": msg.ReasoningDetails,
		"tool_calls":        wireCalls(msg.ToolCalls),
	})
}

// DropLast removes the newest turn, so a question the model failed to answer
// does not stay in the conversation.
func (h History) DropLast() History {
	if len(h) == 0 {
		return h
	}
	return h[:len(h)-1]
}

// Trim keeps the newest limit messages. Whole turns are dropped so the history
// never starts on an assistant reply, and never on a tool result whose call is
// already gone.
func (h History) Trim(limit int) History {
	if len(h) <= limit {
		return h
	}
	drop := len(h) - limit
	for drop < len(h) && h[drop]["role"] != "user" {
		drop++
	}
	if drop >= len(h) {
		return nil
	}
	return h[drop:]
}
