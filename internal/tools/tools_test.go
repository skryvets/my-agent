package tools

import (
	"context"
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

func TestAllBindsEveryToolToTheWorkspace(t *testing.T) {
	kit, err := All(&fakeWorkspace{})
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	var names []string
	for _, built := range kit {
		info, err := built.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, info.Name)
	}
	if got := strings.Join(names, ","); got != "shell,read_file,write_file" {
		t.Errorf("tools = %q", got)
	}
}
