// Package conversation says which conversation a call belongs to. The answer
// travels in the context, so one shared agent can serve many tasks and each
// one still reaches its own container.
package conversation

import "context"

// Default is the conversation a caller that named none belongs to. A terminal
// chat is one conversation, so it never has to name itself.
const Default = "default"

type holder struct{}

// WithKey names the conversation of every call made with the returned context.
// A connector that serves many conversations sets it for each one.
func WithKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, holder{}, key)
}

// KeyOf reads the conversation out of the context.
func KeyOf(ctx context.Context) string {
	if key, ok := ctx.Value(holder{}).(string); ok && key != "" {
		return key
	}
	return Default
}
