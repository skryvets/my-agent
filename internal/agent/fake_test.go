package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

// fakeModel answers with a scripted reply for each call, so the agent can be
// tested without the network. A reply with no message is a failure.
type fakeModel struct {
	mu      sync.Mutex
	replies []reply
	seen    []History
	calls   int
}

// reply is one scripted answer of fakeModel. chunks, when it is set, is the
// stream the model sends instead of the message cut in two.
type reply struct {
	message *schema.Message
	err     error

	chunks    []*schema.Message
	streamErr error
}

func (f *fakeModel) next(input []*schema.Message) (reply, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.seen = append(f.seen, append(History(nil), input...))
	if f.calls >= len(f.replies) {
		return reply{}, errNoReply
	}
	answer := f.replies[f.calls]
	f.calls++
	return answer, answer.err
}

func (f *fakeModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	answer, err := f.next(input)
	if err != nil {
		return nil, err
	}
	return answer.message, nil
}

// Stream cuts the reply into one chunk for each rune of the content, so the
// test covers the path that joins the chunks again.
func (f *fakeModel) Stream(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	answer, err := f.next(input)
	if err != nil {
		return nil, err
	}
	if answer.streamErr != nil {
		reader, writer := schema.Pipe[*schema.Message](1)
		writer.Send(nil, answer.streamErr)
		writer.Close()
		return reader, nil
	}
	if answer.chunks != nil || answer.message == nil {
		return schema.StreamReaderFromArray(answer.chunks), nil
	}
	return schema.StreamReaderFromArray(chunksOf(answer.message)), nil
}

func (f *fakeModel) history(index int) History {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seen[index]
}

func (f *fakeModel) times() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func chunksOf(message *schema.Message) []*schema.Message {
	if message.Content == "" {
		return []*schema.Message{message}
	}
	half := len(message.Content) / 2
	head := &schema.Message{Role: message.Role, Content: message.Content[:half]}
	tail := *message
	tail.Content = message.Content[half:]
	return []*schema.Message{head, &tail}
}

// toolCall is one call the model asks for.
func toolCall(id, name, arguments string) schema.ToolCall {
	return schema.ToolCall{
		ID:       id,
		Type:     "function",
		Function: schema.FunctionCall{Name: name, Arguments: arguments},
	}
}

type echoArgs struct {
	Text string `json:"text" jsonschema:"required,description=What to say back"`
}

// echoTool answers with what it was given, or with the error the test set.
func echoTool(t *testing.T, err error) tool.BaseTool {
	t.Helper()
	built, buildErr := utils.InferTool("echo", "Say the text back",
		func(_ context.Context, in echoArgs) (string, error) {
			if err != nil {
				return "", err
			}
			return in.Text, nil
		})
	if buildErr != nil {
		t.Fatalf("build the tool: %v", buildErr)
	}
	return built
}

// testClient wires a client over a fake model.
func testClient(t *testing.T, fake *fakeModel, tools ...tool.BaseTool) *Client {
	t.Helper()
	client, err := newClient(context.Background(), "test-model", fake, tools...)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	return client
}
