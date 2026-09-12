package telegram

import (
	"context"
	"github.com/skryvets/my-agent/internal/approval"
	"reflect"
	"testing"

	"github.com/skryvets/my-agent/internal/agent"
)

func TestParseAllowedUsers(t *testing.T) {
	allowed, err := parseAllowedUsers(" 42, 7 ,")
	if err != nil {
		t.Fatalf("parseAllowedUsers: %v", err)
	}
	if !reflect.DeepEqual(allowed, map[int64]bool{42: true, 7: true}) {
		t.Errorf("allowed = %#v", allowed)
	}
	if empty, err := parseAllowedUsers(""); err != nil || len(empty) != 0 {
		t.Errorf("empty = %#v, err = %v", empty, err)
	}
	if _, err := parseAllowedUsers("sergey"); err == nil {
		t.Error("expected error for a non-numeric id")
	}
}

func TestRunRequiresToken(t *testing.T) {
	model := fakeAgent{answer: func(agent.History) (agent.Message, error) {
		return agent.Message{}, nil
	}}

	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	if err := Run(context.Background(), model); err == nil {
		t.Fatal("expected an error without a bot token")
	}
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_ALLOWED_USERS", "nope")
	if err := Run(context.Background(), model); err == nil {
		t.Fatal("expected an error for a bad allowlist")
	}
}

func TestOptionsReachTheBot(t *testing.T) {
	box := &fakeSandbox{}
	approvals := &fakeApprovals{}
	bot := &Bot{}

	WithSandbox(box)(bot)
	WithApproval(approvals)(bot)

	if bot.sandbox != box {
		t.Error("the sandbox did not reach the bot")
	}
	if approvals.ask == nil {
		t.Error("the bot did not offer to answer the questions")
	}
}

// fakeApprovals records the handler the bot registered.
type fakeApprovals struct {
	ask approval.Ask
}

func (f *fakeApprovals) Handle(ask approval.Ask) { f.ask = ask }
