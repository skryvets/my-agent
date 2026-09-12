package conversation

import (
	"context"
	"testing"
)

func TestKeyOfReadsWhatWithKeyNamed(t *testing.T) {
	if got := KeyOf(WithKey(context.Background(), "chat-7")); got != "chat-7" {
		t.Errorf("key = %q", got)
	}
}

func TestKeyOfFallsBackToOneConversation(t *testing.T) {
	if got := KeyOf(context.Background()); got != Default {
		t.Errorf("a context with no key = %q, want %q", got, Default)
	}
	if got := KeyOf(WithKey(context.Background(), "")); got != Default {
		t.Errorf("an empty key = %q, want %q", got, Default)
	}
}
