package tools

import (
	"strings"
	"testing"
)

func TestTruncateKeepsTheTail(t *testing.T) {
	if got := truncate("short"); got != "short" {
		t.Errorf("got %q", got)
	}

	long := strings.Repeat("a", outputLimit) + "END"
	got := truncate(long)
	if len(got) <= outputLimit {
		t.Fatalf("length = %d", len(got))
	}
	if !strings.HasSuffix(got, "END") {
		t.Error("the end of the text was cut")
	}
	if !strings.HasPrefix(got, "[3 bytes cut,") {
		t.Errorf("got %q", got[:40])
	}
}
