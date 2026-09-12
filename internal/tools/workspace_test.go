package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHostRunsACommandInItsDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := Host{Dir: dir}.Run(context.Background(), "ls")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out, "marker.txt") {
		t.Errorf("out = %q", out)
	}
}

func TestHostReportsAFailedCommandInTheText(t *testing.T) {
	out, err := Host{}.Run(context.Background(), "echo nope >&2; exit 3")
	if err != nil {
		t.Fatalf("a non-zero exit must not be an error: %v", err)
	}
	if !strings.Contains(out, "nope") || !strings.Contains(out, "exit status 3") {
		t.Errorf("out = %q", out)
	}
}

func TestHostReportsACancelledCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	if _, err := (Host{}).Run(ctx, "sleep 5"); err == nil {
		t.Fatal("expected an error for a command that was stopped")
	}
}

func TestHostWritesThenReads(t *testing.T) {
	dir := t.TempDir()
	host := Host{Dir: dir}

	if err := host.WriteFile(context.Background(), "pkg/hello.go", "package pkg\n"); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := host.ReadFile(context.Background(), "pkg/hello.go")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got != "package pkg\n" {
		t.Errorf("content = %q", got)
	}
}

func TestHostRefusesPathsOutsideItsDirectory(t *testing.T) {
	dir := t.TempDir()
	host := Host{Dir: dir}
	outside := filepath.Join(dir, "..", "escaped.txt")

	for _, path := range []string{"../escaped.txt", outside, ""} {
		if _, err := host.ReadFile(context.Background(), path); err == nil {
			t.Errorf("reading %q was allowed", path)
		}
		if err := host.WriteFile(context.Background(), path, "x"); err == nil {
			t.Errorf("writing %q was allowed", path)
		}
	}
}

func TestHostAcceptsAnAbsolutePathInsideItsDirectory(t *testing.T) {
	dir := t.TempDir()
	host := Host{Dir: dir}
	inside := filepath.Join(dir, "in.txt")

	if err := host.WriteFile(context.Background(), inside, "here"); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := host.ReadFile(context.Background(), inside)
	if err != nil || got != "here" {
		t.Errorf("content = %q, err = %v", got, err)
	}
}

func TestHostReportsWriteFailures(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "blocked"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (Host{Dir: dir}).WriteFile(context.Background(), "blocked/file.txt", "x"); err == nil {
		t.Error("expected an error when the parent is a file")
	}
	if _, err := (Host{Dir: dir}).ReadFile(context.Background(), "absent.txt"); err == nil {
		t.Error("expected an error for a file that is not there")
	}
}
