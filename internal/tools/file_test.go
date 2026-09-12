package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileThenReadFile(t *testing.T) {
	dir := t.TempDir()
	write := WriteFile{Dir: dir}
	read := ReadFile{Dir: dir}

	out, err := write.Call(context.Background(), []byte(`{"path":"pkg/hello.go","content":"package pkg\n"}`))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.Contains(out, "wrote 12 bytes") {
		t.Errorf("out = %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "pkg", "hello.go")); err != nil {
		t.Fatalf("the parent directory was not created: %v", err)
	}

	got, err := read.Call(context.Background(), []byte(`{"path":"pkg/hello.go"}`))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != "package pkg\n" {
		t.Errorf("content = %q", got)
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

func TestFileToolsRefusePathsOutsideTheDirectory(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(dir, "..", "escaped.txt")

	for name, call := range map[string]func(string) error{
		"read": func(path string) error {
			_, err := ReadFile{Dir: dir}.Call(context.Background(), []byte(`{"path":"`+path+`"}`))
			return err
		},
		"write": func(path string) error {
			_, err := WriteFile{Dir: dir}.Call(context.Background(), []byte(`{"path":"`+path+`","content":"x"}`))
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := call("../escaped.txt"); err == nil {
				t.Error("a relative escape was allowed")
			}
			if err := call(outside); err == nil {
				t.Error("an absolute path outside the directory was allowed")
			}
			if err := call(""); err == nil {
				t.Error("an empty path was allowed")
			}
		})
	}
}

func TestFileToolsReportTheirErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := (ReadFile{Dir: dir}).Call(context.Background(), []byte(`{"path":"absent.txt"}`)); err == nil {
		t.Error("expected an error for a file that is not there")
	}
	if _, err := (ReadFile{Dir: dir}).Call(context.Background(), []byte(`{oops`)); err == nil {
		t.Error("expected an error for bad arguments")
	}
	if _, err := (WriteFile{Dir: dir}).Call(context.Background(), []byte(`{oops`)); err == nil {
		t.Error("expected an error for bad arguments")
	}
	// A file where a directory must go stops the write.
	if err := os.WriteFile(filepath.Join(dir, "blocked"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (WriteFile{Dir: dir}).Call(context.Background(), []byte(`{"path":"blocked/file.txt","content":"x"}`)); err == nil {
		t.Error("expected an error when the parent is a file")
	}
}

func TestReadFileAcceptsAnAbsolutePathInsideTheDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in.txt"), []byte("here"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFile{Dir: dir}.Call(context.Background(), []byte(`{"path":"`+filepath.Join(dir, "in.txt")+`"}`))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != "here" {
		t.Errorf("content = %q", got)
	}
}
