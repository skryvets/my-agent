package tools

import (
	"context"
	"strings"
	"testing"
)

func TestReadFileAsksTheWorkspace(t *testing.T) {
	workspace := &fakeWorkspace{output: "package pkg\n"}

	got, err := call(t, build(t, ReadFile, workspace), `{"path":"pkg/hello.go"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if workspace.path != "pkg/hello.go" {
		t.Errorf("path = %q", workspace.path)
	}
	if got != "package pkg\n" {
		t.Errorf("content = %q", got)
	}
}

func TestWriteFileAsksTheWorkspace(t *testing.T) {
	workspace := &fakeWorkspace{}

	out, err := call(t, build(t, WriteFile, workspace), `{"path":"hello.go","content":"package pkg\n"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if workspace.path != "hello.go" || workspace.content != "package pkg\n" {
		t.Errorf("wrote %q to %q", workspace.content, workspace.path)
	}
	if !strings.Contains(out, "wrote 12 bytes") {
		t.Errorf("out = %q", out)
	}
}

func TestFileToolsCutLongContent(t *testing.T) {
	workspace := &fakeWorkspace{output: strings.Repeat("a", outputLimit*2)}

	got, err := call(t, build(t, ReadFile, workspace), `{"path":"big.txt"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(got, "bytes cut") {
		t.Error("long content was not cut")
	}
}

func TestFileToolsDescribeThemselves(t *testing.T) {
	read := build(t, ReadFile, &fakeWorkspace{})
	write := build(t, WriteFile, &fakeWorkspace{})

	readInfo, err := read.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	writeInfo, err := write.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if readInfo.Name != "read_file" || writeInfo.Name != "write_file" {
		t.Errorf("names = %q %q", readInfo.Name, writeInfo.Name)
	}
	if readInfo.Desc == "" || writeInfo.Desc == "" {
		t.Error("a tool has no description")
	}
	if properties(t, read)["content"] {
		t.Error("read_file asks for content")
	}
	if !properties(t, write)["path"] || !properties(t, write)["content"] {
		t.Errorf("write_file asks for %#v", properties(t, write))
	}
}

func TestFileToolsReportTheirErrors(t *testing.T) {
	read := build(t, ReadFile, &fakeWorkspace{err: errWorkspace})
	write := build(t, WriteFile, &fakeWorkspace{err: errWorkspace})

	if _, err := call(t, read, `{"path":"a.txt"}`); err == nil {
		t.Error("expected the workspace failure")
	}
	if _, err := call(t, write, `{"path":"a.txt","content":"x"}`); err == nil {
		t.Error("expected the workspace failure")
	}
	if _, err := call(t, read, `{oops`); err == nil {
		t.Error("expected an error for bad arguments")
	}
	if _, err := call(t, write, `{oops`); err == nil {
		t.Error("expected an error for bad arguments")
	}
}
