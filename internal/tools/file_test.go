package tools

import (
	"context"
	"strings"
	"testing"
)

func TestReadFileAsksTheWorkspace(t *testing.T) {
	workspace := &fakeWorkspace{output: "package pkg\n"}
	read := ReadFile{Workspace: workspace}

	got, err := read.Call(context.Background(), []byte(`{"path":"pkg/hello.go"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
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
	write := WriteFile{Workspace: workspace}

	out, err := write.Call(context.Background(), []byte(`{"path":"hello.go","content":"package pkg\n"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
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

	got, err := ReadFile{Workspace: workspace}.Call(context.Background(), []byte(`{"path":"big.txt"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(got, "bytes cut") {
		t.Error("long content was not cut")
	}
}

func TestFileToolsDescribeThemselves(t *testing.T) {
	read := ReadFile{}
	write := WriteFile{}
	if read.Name() != "read_file" || write.Name() != "write_file" {
		t.Errorf("names = %q %q", read.Name(), write.Name())
	}
	if read.Description() == "" || write.Description() == "" {
		t.Error("a tool has no description")
	}
	readProperties, _ := read.Parameters()["properties"].(map[string]any)
	if readProperties["content"] != nil {
		t.Errorf("read_file asks for content: %#v", read.Parameters())
	}
	writeProperties, _ := write.Parameters()["properties"].(map[string]any)
	if writeProperties["content"] == nil {
		t.Errorf("write_file does not ask for content: %#v", write.Parameters())
	}
	if required, _ := write.Parameters()["required"].([]string); len(required) != 2 {
		t.Errorf("required = %#v", write.Parameters()["required"])
	}
}

func TestFileToolsReportTheirErrors(t *testing.T) {
	broken := &fakeWorkspace{err: errWorkspace}

	if _, err := (ReadFile{Workspace: broken}).Call(context.Background(), []byte(`{"path":"a.txt"}`)); err == nil {
		t.Error("expected the workspace failure")
	}
	if _, err := (WriteFile{Workspace: broken}).Call(context.Background(), []byte(`{"path":"a.txt","content":"x"}`)); err == nil {
		t.Error("expected the workspace failure")
	}
	if _, err := (ReadFile{}).Call(context.Background(), []byte(`{oops`)); err == nil {
		t.Error("expected an error for bad arguments")
	}
	if _, err := (WriteFile{}).Call(context.Background(), []byte(`{oops`)); err == nil {
		t.Error("expected an error for bad arguments")
	}
}
