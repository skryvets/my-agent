package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudwego/eino-ext/components/model/openai"
)

// refused is the failure the client reports when the server turns a request
// down on its own terms.
func refused(code int, message string) error {
	return &openai.APIError{HTTPStatusCode: code, HTTPStatus: http.StatusText(code), Message: message}
}

func TestChatDoesNotTryARefusedRequestAgain(t *testing.T) {
	fake := &fakeModel{replies: []reply{{err: refused(http.StatusUnauthorized, "bad key")}}}
	client := testClient(t, fake)

	_, err := client.Chat(context.Background(), History(nil).WithUser("ask"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "node path") {
		t.Errorf("the node path reached the caller: %v", err)
	}
	if fake.times() != 1 {
		t.Errorf("the model was asked %d times, want 1", fake.times())
	}
}

func TestChatTriesTooManyRequestsAgain(t *testing.T) {
	fake := &fakeModel{replies: []reply{
		{err: refused(http.StatusTooManyRequests, "slow down")},
		{message: assistant("here it is")},
	}}
	client := testClient(t, fake)

	answer, err := client.Chat(context.Background(), History(nil).WithUser("ask"), io.Discard)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if answer != "here it is" || fake.times() != 2 {
		t.Errorf("answer = %q after %d calls", answer, fake.times())
	}
}

func TestChatTriesAServerFailureAgain(t *testing.T) {
	fake := &fakeModel{replies: []reply{
		{err: refused(http.StatusBadGateway, "bad gateway")},
		{message: assistant("here it is")},
	}}
	client := testClient(t, fake)

	if _, err := client.Chat(context.Background(), History(nil).WithUser("ask"), io.Discard); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if fake.times() != 2 {
		t.Errorf("the model was asked %d times, want 2", fake.times())
	}
}

func TestChatSaysPlainlyWhenTheRoundsRunOut(t *testing.T) {
	var replies []reply
	for i := range maxRounds {
		asked := assistant("")
		asked.ToolCalls = append(asked.ToolCalls, toolCall(fmt.Sprintf("call_%d", i), "echo", `{"text":"again"}`))
		replies = append(replies, reply{message: asked})
	}
	fake := &fakeModel{replies: replies}
	client := testClient(t, fake, echoTool(t, nil))

	_, err := client.Chat(context.Background(), History(nil).WithUser("loop"), io.Discard)
	if err == nil || err.Error() != "the model asked for tools 50 times without an answer" {
		t.Errorf("err = %v", err)
	}
}

func TestPlainKeepsAnErrorItDoesNotKnow(t *testing.T) {
	own := errors.New("something else")
	if got := plain(own); got != own {
		t.Errorf("got %v, want %v", got, own)
	}
}
