package agent

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// assistant is one answer of the model.
func assistant(content string) *schema.Message {
	return schema.AssistantMessage(content, nil)
}

var errNoReply = errors.New("the fake model has no reply left")

func TestChatStreamsTheAnswer(t *testing.T) {
	fake := &fakeModel{replies: []reply{{message: assistant("the answer is here")}}}
	client := testClient(t, fake)

	var out strings.Builder
	msg, err := client.Chat(context.Background(), History(nil).WithUser("ask"), &out)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if msg.Content != "the answer is here" {
		t.Errorf("content = %q", msg.Content)
	}
	if !strings.Contains(out.String(), "the answer is here") {
		t.Errorf("stream = %q", out.String())
	}
	if len(msg.Steps) != 0 {
		t.Errorf("steps = %#v", msg.Steps)
	}
	if client.Model() != "test-model" {
		t.Errorf("model = %q", client.Model())
	}
}

func TestChatRunsAToolAndAsksAgain(t *testing.T) {
	asked := assistant("")
	asked.ToolCalls = append(asked.ToolCalls, toolCall("call_1", "echo", `{"text":"pong"}`))
	fake := &fakeModel{replies: []reply{{message: asked}, {message: assistant("done")}}}
	client := testClient(t, fake, echoTool(t, nil))

	var out strings.Builder
	msg, err := client.Chat(context.Background(), History(nil).WithUser("say pong"), &out)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if msg.Content != "done" {
		t.Errorf("content = %q", msg.Content)
	}
	if len(msg.Steps) != 2 {
		t.Fatalf("steps = %#v", msg.Steps)
	}
	if len(msg.Steps[0].ToolCalls) != 1 || msg.Steps[1].Content != "pong" {
		t.Errorf("steps = %#v", msg.Steps)
	}
	if !strings.Contains(out.String(), "--- tool: echo") {
		t.Errorf("stream does not name the tool: %q", out.String())
	}
	if fake.times() != 2 {
		t.Errorf("the model was asked %d times, want 2", fake.times())
	}
}

func TestChatReportsAFailedToolToTheModel(t *testing.T) {
	asked := assistant("")
	asked.ToolCalls = append(asked.ToolCalls, toolCall("call_1", "echo", `{"text":"pong"}`))
	fake := &fakeModel{replies: []reply{{message: asked}, {message: assistant("I cannot")}}}
	client := testClient(t, fake, echoTool(t, errors.New("the workspace is gone")))

	msg, err := client.Chat(context.Background(), History(nil).WithUser("say pong"), io.Discard)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if msg.Content != "I cannot" {
		t.Errorf("content = %q", msg.Content)
	}
	if len(msg.Steps) != 2 || !strings.Contains(msg.Steps[1].Content, "the workspace is gone") {
		t.Errorf("steps = %#v", msg.Steps)
	}
}

func TestChatReportsAToolTheModelInvented(t *testing.T) {
	asked := assistant("")
	asked.ToolCalls = append(asked.ToolCalls, toolCall("call_1", "fly", `{}`))
	fake := &fakeModel{replies: []reply{{message: asked}, {message: assistant("I cannot")}}}
	client := testClient(t, fake, echoTool(t, nil))

	msg, err := client.Chat(context.Background(), History(nil).WithUser("fly"), io.Discard)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(msg.Steps) != 2 || !strings.Contains(msg.Steps[1].Content, `no tool named "fly"`) {
		t.Errorf("steps = %#v", msg.Steps)
	}
}

func TestChatTriesAFailedCallAgain(t *testing.T) {
	fake := &fakeModel{replies: []reply{
		{err: errors.New("rate limited")},
		{message: assistant("second time")},
	}}
	client := testClient(t, fake)

	msg, err := client.Chat(context.Background(), History(nil).WithUser("ask"), io.Discard)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if msg.Content != "second time" {
		t.Errorf("content = %q", msg.Content)
	}
	if fake.times() != 2 {
		t.Errorf("the model was asked %d times, want 2", fake.times())
	}
}

func TestChatGivesUpAfterTheRetries(t *testing.T) {
	var replies []reply
	for range maxRetries + 1 {
		replies = append(replies, reply{err: errors.New("rate limited")})
	}
	client := testClient(t, &fakeModel{replies: replies})

	if _, err := client.Chat(context.Background(), History(nil).WithUser("ask"), io.Discard); err == nil {
		t.Fatal("expected the failure to reach the caller")
	}
}

func TestChatStopsWhenTheCallerStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testClient(t, &fakeModel{replies: []reply{{message: assistant("never")}}})
	if _, err := client.Chat(ctx, History(nil).WithUser("ask"), io.Discard); err == nil {
		t.Fatal("expected the stopped run to fail")
	}
}

func TestChatCarriesTheWholeHistoryToTheModel(t *testing.T) {
	fake := &fakeModel{replies: []reply{{message: assistant("two")}}}
	client := testClient(t, fake)

	history := History(nil).WithUser("one").WithAssistant(Message{Content: "answer"}).WithUser("two")
	if _, err := client.Chat(context.Background(), history, io.Discard); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	seen := fake.history(0)
	if len(seen) != 3 || seen[0].Content != "one" || seen[2].Content != "two" {
		t.Errorf("the model saw %#v", seen)
	}
}

func TestChatReportsAnEmptyStream(t *testing.T) {
	client := testClient(t, &fakeModel{replies: []reply{{}}})

	if _, err := client.Chat(context.Background(), History(nil).WithUser("ask"), io.Discard); err == nil {
		t.Fatal("expected an error for an answer with no message")
	}
}

func TestChatReportsAStreamThatBreaks(t *testing.T) {
	broken := errors.New("the connection dropped")
	client := testClient(t, &fakeModel{replies: []reply{{streamErr: broken}}})

	_, err := client.Chat(context.Background(), History(nil).WithUser("ask"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "the connection dropped") {
		t.Fatalf("err = %v", err)
	}
}

func TestChatReportsChunksThatDoNotJoin(t *testing.T) {
	mixed := []*schema.Message{assistant("half"), schema.UserMessage("other")}
	client := testClient(t, &fakeModel{replies: []reply{{chunks: mixed}}})

	if _, err := client.Chat(context.Background(), History(nil).WithUser("ask"), io.Discard); err == nil {
		t.Fatal("expected an error for chunks of two roles")
	}
}

func TestNewBuildsAClientForOpenRouter(t *testing.T) {
	client, err := New(context.Background(), "test-key")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if client.Model() != slug {
		t.Errorf("model = %q, want %q", client.Model(), slug)
	}
}
