package agent

// History is a conversation in the wire format the API expects. The assistant
// turns carry their reasoning_details, so the model can follow up on its own
// thinking on a later turn.
type History []map[string]any

// WithUser appends a user turn.
func (h History) WithUser(text string) History {
	return append(h, map[string]any{"role": "user", "content": text})
}

// WithAssistant appends an assistant turn.
func (h History) WithAssistant(msg Message) History {
	return append(h, map[string]any{
		"role":              "assistant",
		"content":           msg.Content,
		"reasoning_details": msg.ReasoningDetails,
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
// never starts on an assistant reply.
func (h History) Trim(limit int) History {
	if len(h) <= limit {
		return h
	}
	drop := len(h) - limit
	if drop%2 != 0 {
		drop++
	}
	if drop >= len(h) {
		return nil
	}
	return h[drop:]
}
