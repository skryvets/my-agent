package telegram

import (
	"context"
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
	model := fakeAgent{answer: func(agent.History) (string, error) {
		return "", nil
	}}

	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	if err := Run(context.Background(), model, nil); err == nil {
		t.Fatal("expected an error without a bot token")
	}
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("TELEGRAM_ALLOWED_USERS", "nope")
	if err := Run(context.Background(), model, nil); err == nil {
		t.Fatal("expected an error for a bad allowlist")
	}
}
