package agent

import "github.com/cloudwego/eino/schema"

// History is a conversation in the message format eino uses: one user turn,
// one assistant turn, and so on. The tool calls of a turn stay inside eino,
// because a chat offers no tools and a task asks one question only.
type History []*schema.Message

// WithUser appends a user turn.
func (h History) WithUser(text string) History {
	return append(h, schema.UserMessage(text))
}

// WithAssistant appends the answer of the model.
func (h History) WithAssistant(text string) History {
	return append(h, schema.AssistantMessage(text, nil))
}

// DropLast removes the newest turn, so a question the model failed to answer
// does not stay in the conversation.
func (h History) DropLast() History {
	if len(h) == 0 {
		return h
	}
	return h[:len(h)-1]
}

// Trim keeps the newest limit messages. Whole turns are dropped, so the
// history never starts on an assistant reply.
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
