package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShareCheckoutLetsAnyUserWrite(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "cmd")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sub, "main.go")
	script := filepath.Join(dir, "run.sh")
	for name, mode := range map[string]os.FileMode{file: 0o644, script: 0o755} {
		if err := os.WriteFile(name, []byte("x"), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(name, mode); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("/nowhere", filepath.Join(dir, "dangling")); err != nil {
		t.Fatal(err)
	}

	if err := shareCheckout(dir); err != nil {
		t.Fatalf("shareCheckout: %v", err)
	}
	for name, want := range map[string]os.FileMode{sub: 0o777, file: 0o666, script: 0o777} {
		info, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s = %o, want %o", filepath.Base(name), got, want)
		}
	}

	if err := shareCheckout(filepath.Join(dir, "absent")); err == nil {
		t.Error("expected an error for a checkout that is not there")
	}
}

func TestTailKeepsTheEndOfTheOutput(t *testing.T) {
	if got := tail("  short\n"); got != "short" {
		t.Errorf("tail = %q", got)
	}
	got := tail(strings.Repeat("a", setupTail) + "END")
	if !strings.HasPrefix(got, "...") || !strings.HasSuffix(got, "END") || len(got) != setupTail+3 {
		t.Errorf("tail has %d bytes: %.20q", len(got), got)
	}
}
