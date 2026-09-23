// Package agent talks to a reasoning model through the OpenRouter chat
// completions API. The eino agent development kit does the work: it streams
// the reply, reassembles the tool calls, runs them, retries a failed call and
// asks the model again.
package agent

import (
	"context"
	"os"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

// baseURL is the OpenRouter endpoint the OpenAI client speaks to.
const baseURL = "https://openrouter.ai/api/v1"

const (
	// maxRounds caps one Chat call, so a model that keeps asking for tools
	// cannot run forever.
	maxRounds = 10

	// maxRetries is how often eino tries one failed model call again. It
	// waits longer after each failure and adds jitter.
	maxRetries = 3
)

// slug is the OpenRouter model every client answers with. An empty
// MY_AGENT_MODEL leaves the choice to OpenRouter.
var slug = os.Getenv("MY_AGENT_MODEL")

// Client answers a conversation with one model.
type Client struct {
	model  string
	runner *adk.Runner
}

// New returns a client for slug that offers the given tools.
func New(ctx context.Context, apiKey string, tools ...tool.BaseTool) (*Client, error) {
	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   slug,
		// Reasoning is off on purpose. Switching it back on is this
		// field and nothing else.
		ExtraFields: map[string]any{"reasoning": map[string]any{"enabled": true}},
	})
	if err != nil {
		return nil, err
	}
	return newClient(ctx, slug, chatModel, tools...)
}

// newClient builds the eino agent over any chat model, which is how a test
// answers without the network.
func newClient(ctx context.Context, name string, chatModel model.BaseChatModel, tools ...tool.BaseTool) (*Client, error) {
	worker, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "my-agent",
		Description:   "answers a conversation and works in a dev container",
		Model:         chatModel,
		MaxIterations: maxRounds,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools:               reportFailures(tools),
				UnknownToolsHandler: unknownTool,
			},
		},
		ModelRetryConfig: &adk.ModelRetryConfig{
			MaxRetries:  maxRetries,
			IsRetryAble: retryable,
		},
	})
	if err != nil {
		return nil, err
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: worker, EnableStreaming: true})
	return &Client{model: name, runner: runner}, nil
}

// Model reports the model slug answers come from.
func (c *Client) Model() string { return c.model }
