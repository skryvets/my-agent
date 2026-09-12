package telegram

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/skryvets/my-agent/internal/approval"
)

// press answers the question the bot is waiting on, once it has been sent.
// The id counts up across the whole process, so the test reads it rather than
// assuming it.
func press(t *testing.T, bot *Bot, decision string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		bot.mu.Lock()
		var open string
		for id := range bot.waiting {
			open = id
		}
		bot.mu.Unlock()

		if open != "" {
			query := &callbackQuery{ID: "q1", Data: decision + ":" + open}
			query.From.ID = 42
			bot.answer(context.Background(), query)
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("no question was ever asked")
}

func TestAskSendsButtonsAndWaitsForOne(t *testing.T) {
	fake := newFakeTelegram(t)
	fake.reply("sendMessage", `{"ok":true,"result":{"message_id":77}}`)
	bot := newTestBot(fake, nil)

	answered := make(chan bool, 1)
	go func() {
		allowed, err := bot.ask(context.Background(), "99", approval.Request{
			Tool:    "shell",
			Details: "curl https://example.com",
		})
		if err != nil {
			t.Errorf("ask: %v", err)
		}
		answered <- allowed
	}()

	asked := fake.nextSent(t)
	if !strings.Contains(asked, "May I run this?") || !strings.Contains(asked, "curl https://example.com") {
		t.Errorf("question = %q", asked)
	}

	press(t, bot, "y")

	select {
	case allowed := <-answered:
		if !allowed {
			t.Error("Approve was read as a no")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the call is still waiting after the button was pressed")
	}

	methods := fake.methods()
	if !contains(methods, "answerCallbackQuery") {
		t.Errorf("the button was left spinning: %v", methods)
	}
	if !contains(methods, "editMessageText") {
		t.Errorf("the question kept its buttons: %v", methods)
	}
	if !keyboardWasSent(t, fake) {
		t.Error("the question carried no buttons")
	}
}

func TestAskReadsDenyAsNo(t *testing.T) {
	fake := newFakeTelegram(t)
	fake.reply("sendMessage", `{"ok":true,"result":{"message_id":77}}`)
	bot := newTestBot(fake, nil)

	answered := make(chan bool, 1)
	go func() {
		allowed, _ := bot.ask(context.Background(), "99", approval.Request{Tool: "fetch"})
		answered <- allowed
	}()
	fake.nextSent(t)
	press(t, bot, "n")

	select {
	case allowed := <-answered:
		if allowed {
			t.Error("Deny was read as a yes")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the call is still waiting")
	}
}

func TestAskStopsWhenTheQuestionTimesOut(t *testing.T) {
	fake := newFakeTelegram(t)
	fake.reply("sendMessage", `{"ok":true,"result":{"message_id":77}}`)
	bot := newTestBot(fake, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	allowed, err := bot.ask(ctx, "99", approval.Request{Tool: "fetch"})
	if allowed {
		t.Error("a question nobody answered was taken for a yes")
	}
	if err == nil {
		t.Error("expected the deadline to be reported")
	}

	bot.mu.Lock()
	open := len(bot.waiting)
	bot.mu.Unlock()
	if open != 0 {
		t.Errorf("%d questions were left open", open)
	}
}

func TestAskReportsAChatItCannotReach(t *testing.T) {
	fake := newFakeTelegram(t)
	bot := newTestBot(fake, nil)

	if _, err := bot.ask(context.Background(), "not-a-chat", approval.Request{Tool: "fetch"}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestAnswerIgnoresStrangersAndStaleButtons(t *testing.T) {
	fake := newFakeTelegram(t)
	bot := newTestBot(fake, nil)
	bot.allowed = map[int64]bool{7: true}

	stranger := &callbackQuery{ID: "q1", Data: "y:1"}
	stranger.From.ID = 42
	bot.answer(context.Background(), stranger)
	if contains(fake.methods(), "answerCallbackQuery") {
		t.Error("a stranger was answered")
	}

	// A button of a question that is gone must still stop spinning.
	stale := &callbackQuery{ID: "q2", Data: "y:404"}
	stale.From.ID = 7
	bot.answer(context.Background(), stale)
	if !contains(fake.methods(), "answerCallbackQuery") {
		t.Error("a stale button was left spinning")
	}

	// Data that carries no id at all is dropped.
	fake.calls = nil
	broken := &callbackQuery{ID: "q3", Data: "nonsense"}
	broken.From.ID = 7
	bot.answer(context.Background(), broken)
	if len(fake.methods()) != 0 {
		t.Errorf("broken data reached the API: %v", fake.methods())
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// keyboardWasSent reports whether a sendMessage carried an inline keyboard
// with both buttons.
func keyboardWasSent(t *testing.T, fake *fakeTelegram) bool {
	t.Helper()
	fake.mu.Lock()
	defer fake.mu.Unlock()

	for _, call := range fake.calls {
		if call.Method != "sendMessage" {
			continue
		}
		markup, ok := call.Payload["reply_markup"]
		if !ok {
			continue
		}
		encoded, err := json.Marshal(markup)
		if err != nil {
			t.Fatal(err)
		}
		text := string(encoded)
		if strings.Contains(text, `"callback_data":"y:`) && strings.Contains(text, `"callback_data":"n:`) {
			return true
		}
	}
	return false
}
