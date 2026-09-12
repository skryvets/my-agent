package approval

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/skryvets/my-agent/internal/agent"
)

// detailLimit keeps the question short enough to read on a phone.
const detailLimit = 400

// Guard is a tool that asks a person first, unless the policy allows the call
// by itself. The model sees the same tool either way.
type Guard struct {
	Tool   agent.Tool
	Policy Policy
	Broker *Broker
}

// Guarded wraps every tool in a Guard, so main names the policy once.
func Guarded(broker *Broker, policy Policy, tools ...agent.Tool) []agent.Tool {
	guarded := make([]agent.Tool, 0, len(tools))
	for _, tool := range tools {
		guarded = append(guarded, Guard{Tool: tool, Policy: policy, Broker: broker})
	}
	return guarded
}

func (g Guard) Name() string               { return g.Tool.Name() }
func (g Guard) Description() string        { return g.Tool.Description() }
func (g Guard) Parameters() map[string]any { return g.Tool.Parameters() }

func (g Guard) Call(ctx context.Context, args json.RawMessage) (string, error) {
	if g.Policy.Allows(g.Tool.Name(), args) {
		return g.Tool.Call(ctx, args)
	}

	allowed, err := g.Broker.Allowed(ctx, Request{Tool: g.Tool.Name(), Details: details(args)})
	if err != nil {
		return "", err
	}
	if !allowed {
		// The model reads this and picks another way, instead of asking for
		// the same call again.
		return "The person refused this call. Do not repeat it. " +
			"Tell them what you wanted to do and why, or try something they allow.", nil
	}
	return g.Tool.Call(ctx, args)
}

// details is the argument line a person reads before deciding.
func details(args json.RawMessage) string {
	var fields map[string]any
	if err := json.Unmarshal(args, &fields); err != nil {
		return string(args)
	}

	var written []string
	for _, name := range []string{"command", "url", "path"} {
		if value, ok := fields[name].(string); ok {
			written = append(written, value)
		}
	}
	line := strings.Join(written, " ")
	if line == "" {
		line = string(args)
	}
	if len(line) > detailLimit {
		line = line[:detailLimit] + "..."
	}
	return line
}
