package agent

import "github.com/cloudwego/eino/schema"

// History is a conversation in the message format eino uses. A turn carries
// its own tool calls and tool results, so the model can follow up on what it
// already did.
type History []*schema.Message

// WithUser appends a user turn.
func (h History) WithUser(text string) History {
	return append(h, schema.UserMessage(text))
}

// WithAssistant appends an assistant turn, preceded by the tool calls and the
// tool results that produced it.
func (h History) WithAssistant(msg Message) History {
	h = append(h, msg.Steps...)
	return append(h, schema.AssistantMessage(msg.Content, nil))
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
	for drop < len(h) && h[drop].Role != schema.User {
		drop++
	}
	if drop >= len(h) {
		return nil
	}
	return h[drop:]
}
