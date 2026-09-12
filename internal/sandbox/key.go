package sandbox

import "context"

// defaultKey is the conversation a caller that set no key gets. A terminal
// chat is one conversation, so it never has to name itself.
const defaultKey = "default"

type keyHolder struct{}

// WithKey names the conversation whose container the tools work in. A
// connector that serves many conversations must set it for each one.
func WithKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, keyHolder{}, key)
}

// WithKey lets a connector name a conversation through the Pool it was given,
// without importing this package.
func (p *Pool) WithKey(ctx context.Context, key string) context.Context {
	return WithKey(ctx, key)
}

// keyOf reads the conversation out of the context.
func keyOf(ctx context.Context) string {
	if key, ok := ctx.Value(keyHolder{}).(string); ok && key != "" {
		return key
	}
	return defaultKey
}
