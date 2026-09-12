package tools

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestShellRunsACommandInTheDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/marker.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	shell := Shell{Dir: dir}

	out, err := shell.Call(context.Background(), []byte(`{"command":"ls"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(out, "marker.txt") {
		t.Errorf("out = %q", out)
	}
	if shell.Name() != "shell" || shell.Description() == "" {
		t.Errorf("name = %q", shell.Name())
	}
	if properties, _ := shell.Parameters()["properties"].(map[string]any); properties["command"] == nil {
		t.Errorf("parameters = %#v", shell.Parameters())
	}
}

func TestShellReportsAFailedCommandToTheModel(t *testing.T) {
	out, err := Shell{}.Call(context.Background(), []byte(`{"command":"echo nope >&2; exit 3"}`))
	if err != nil {
		t.Fatalf("a non-zero exit must not fail the tool: %v", err)
	}
	if !strings.Contains(out, "nope") || !strings.Contains(out, "[exit:") {
		t.Errorf("out = %q", out)
	}
}

func TestShellReportsQuietAndStoppedCommands(t *testing.T) {
	out, err := Shell{}.Call(context.Background(), []byte(`{"command":"true"}`))
	if err != nil || out != "[no output, exit 0]" {
		t.Errorf("out = %q, err = %v", out, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err = Shell{}.Call(ctx, []byte(`{"command":"sleep 5"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(out, "[stopped after") {
		t.Errorf("out = %q", out)
	}
}

func TestShellRejectsBadArguments(t *testing.T) {
	if _, err := (Shell{}).Call(context.Background(), []byte(`{"command":1}`)); err == nil {
		t.Error("expected an error for arguments of the wrong type")
	}
	if _, err := (Shell{}).Call(context.Background(), []byte(`{}`)); err == nil {
		t.Error("expected an error for an empty command")
	}
}
