package tools

import (
	"context"
	"strings"
	"testing"
)

func TestShellPassesTheCommandToTheWorkspace(t *testing.T) {
	workspace := &fakeWorkspace{output: "marker.txt\n"}
	shell := build(t, Shell, workspace)

	out, err := call(t, shell, `{"command":"ls"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if workspace.command != "ls" {
		t.Errorf("command = %q", workspace.command)
	}
	if out != "marker.txt\n" {
		t.Errorf("out = %q", out)
	}
}

func TestShellDescribesItself(t *testing.T) {
	shell := build(t, Shell, &fakeWorkspace{})

	info, err := shell.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "shell" || info.Desc == "" {
		t.Errorf("name = %q, description = %q", info.Name, info.Desc)
	}
	if !properties(t, shell)["command"] {
		t.Error("shell does not ask for a command")
	}
}

func TestShellReportsAQuietCommand(t *testing.T) {
	out, err := call(t, build(t, Shell, &fakeWorkspace{}), `{"command":"true"}`)
	if err != nil || out != "[no output, exit 0]" {
		t.Errorf("out = %q, err = %v", out, err)
	}
}

func TestShellCutsLongOutput(t *testing.T) {
	workspace := &fakeWorkspace{output: strings.Repeat("a", outputLimit*2)}

	out, err := call(t, build(t, Shell, workspace), `{"command":"cat big"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, "bytes cut") {
		t.Error("long output was not cut")
	}
}

func TestShellReportsAStoppedCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	shell := build(t, Shell, &fakeWorkspace{err: errWorkspace})
	out, err := shell.InvokableRun(ctx, `{"command":"sleep 5"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, "[stopped after") {
		t.Errorf("out = %q", out)
	}
}

func TestShellReportsAWorkspaceFailure(t *testing.T) {
	_, err := call(t, build(t, Shell, &fakeWorkspace{err: errWorkspace}), `{"command":"ls"}`)
	if err == nil {
		t.Fatal("expected an error when the workspace itself failed")
	}
}

func TestShellRejectsBadArguments(t *testing.T) {
	shell := build(t, Shell, &fakeWorkspace{})
	if _, err := call(t, shell, `{"command":1}`); err == nil {
		t.Error("expected an error for arguments of the wrong type")
	}
	if _, err := call(t, shell, `{}`); err == nil {
		t.Error("expected an error for an empty command")
	}
}
