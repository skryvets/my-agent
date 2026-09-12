package tools

import (
	"context"
	"strings"
	"testing"
)

func TestShellPassesTheCommandToTheWorkspace(t *testing.T) {
	workspace := &fakeWorkspace{output: "marker.txt\n"}
	shell := Shell{Workspace: workspace}

	out, err := shell.Call(context.Background(), []byte(`{"command":"ls"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if workspace.command != "ls" {
		t.Errorf("command = %q", workspace.command)
	}
	if out != "marker.txt\n" {
		t.Errorf("out = %q", out)
	}
	if shell.Name() != "shell" || shell.Description() == "" {
		t.Errorf("name = %q", shell.Name())
	}
	if properties, _ := shell.Parameters()["properties"].(map[string]any); properties["command"] == nil {
		t.Errorf("parameters = %#v", shell.Parameters())
	}
}

func TestShellReportsAQuietCommand(t *testing.T) {
	out, err := Shell{Workspace: &fakeWorkspace{}}.Call(context.Background(), []byte(`{"command":"true"}`))
	if err != nil || out != "[no output, exit 0]" {
		t.Errorf("out = %q, err = %v", out, err)
	}
}

func TestShellCutsLongOutput(t *testing.T) {
	workspace := &fakeWorkspace{output: strings.Repeat("a", outputLimit*2)}

	out, err := Shell{Workspace: workspace}.Call(context.Background(), []byte(`{"command":"cat big"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(out, "bytes cut") {
		t.Error("long output was not cut")
	}
}

func TestShellReportsAStoppedCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	out, err := Shell{Workspace: &fakeWorkspace{err: errWorkspace}}.Call(ctx, []byte(`{"command":"sleep 5"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(out, "[stopped after") {
		t.Errorf("out = %q", out)
	}
}

func TestShellReportsAWorkspaceFailure(t *testing.T) {
	_, err := Shell{Workspace: &fakeWorkspace{err: errWorkspace}}.Call(context.Background(), []byte(`{"command":"ls"}`))
	if err == nil {
		t.Fatal("expected an error when the workspace itself failed")
	}
}

func TestShellRejectsBadArguments(t *testing.T) {
	shell := Shell{Workspace: &fakeWorkspace{}}
	if _, err := shell.Call(context.Background(), []byte(`{"command":1}`)); err == nil {
		t.Error("expected an error for arguments of the wrong type")
	}
	if _, err := shell.Call(context.Background(), []byte(`{}`)); err == nil {
		t.Error("expected an error for an empty command")
	}
}
