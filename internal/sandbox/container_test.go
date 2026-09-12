package sandbox

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestContainerRunReturnsTheOutputAndTheExitStatus(t *testing.T) {
	fake := newFakeDocker(t)
	fake.output = "README.md\ngo.mod\n"
	pool := newTestPool(t, fake, Options{})
	ctx := context.Background()

	got, err := pool.Run(ctx, "ls")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "README.md\ngo.mod\n" {
		t.Errorf("output = %q", got)
	}
	if strings.Contains(got, "exit status") {
		t.Errorf("a command that worked reported a status: %q", got)
	}

	body := fake.body("POST /containers/container-1/exec")
	if !strings.Contains(body, `"Tty":true`) {
		t.Errorf("the stream is multiplexed and needs unpacking: %s", body)
	}
	if !strings.Contains(body, `"sh","-c","ls"`) {
		t.Errorf("exec body = %s", body)
	}

	fake.exitCode = 2
	got, err = pool.Run(ctx, "false")
	if err != nil {
		t.Fatalf("a non-zero exit must not be an error: %v", err)
	}
	if !strings.Contains(got, "[exit status 2]") {
		t.Errorf("output = %q", got)
	}
}

func TestContainerReadFileUnpacksTheArchive(t *testing.T) {
	fake := newFakeDocker(t)
	fake.files["/work/go.mod"] = "module example\n"
	fake.files["/etc/hosts"] = "127.0.0.1 localhost\n"
	pool := newTestPool(t, fake, Options{})
	ctx := context.Background()

	got, err := pool.ReadFile(ctx, "go.mod")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got != "module example\n" {
		t.Errorf("content = %q", got)
	}

	// An absolute path reaches the whole container, which is the point of
	// having one: there is nothing inside worth guarding.
	got, err = pool.ReadFile(ctx, "/etc/hosts")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got != "127.0.0.1 localhost\n" {
		t.Errorf("content = %q", got)
	}

	if _, err := pool.ReadFile(ctx, "absent.txt"); err == nil {
		t.Error("expected an error for a file that is not there")
	}
}

func TestContainerReadFileReportsAnEmptyArchive(t *testing.T) {
	fake := newFakeDocker(t)
	fake.files["/work/empty"] = ""
	pool := newTestPool(t, fake, Options{})

	// A directory answers with an archive that holds no regular file.
	fake.onlyDirectories = true
	if _, err := pool.ReadFile(context.Background(), "empty"); err == nil {
		t.Error("expected an error for an archive with no file in it")
	}
}

func TestContainerWriteFileSendsATarAndMakesTheDirectory(t *testing.T) {
	fake := newFakeDocker(t)
	pool := newTestPool(t, fake, Options{})

	if err := pool.WriteFile(context.Background(), "pkg/hello.go", "package pkg\n"); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// The archive endpoint extracts into a directory that must already be
	// there, so the write makes it first.
	exec := fake.body("POST /containers/container-1/exec")
	if !strings.Contains(exec, "mkdir -p '/work/pkg'") {
		t.Errorf("the parent directory was not made: %s", exec)
	}

	sent := fake.files["/work/pkg"]
	reader := tar.NewReader(strings.NewReader(sent))
	header, err := reader.Next()
	if err != nil {
		t.Fatalf("the body is not a tar: %v", err)
	}
	if header.Name != "hello.go" {
		t.Errorf("tar entry = %q", header.Name)
	}
	content, _ := io.ReadAll(reader)
	if string(content) != "package pkg\n" {
		t.Errorf("tar content = %q", content)
	}
}

func TestContainerWriteFileReportsAFailedDirectory(t *testing.T) {
	fake := newFakeDocker(t)
	pool := newTestPool(t, fake, Options{})
	if _, err := pool.Run(context.Background(), "true"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	fake.fail = "/containers/container-1/exec"
	if err := pool.WriteFile(context.Background(), "pkg/hello.go", "x"); err == nil {
		t.Error("expected an error when the directory could not be made")
	}
}

func TestResolveReadsAPathAgainstTheWorkingDirectory(t *testing.T) {
	container := &Container{dir: workDir}

	for name, want := range map[string]string{
		"go.mod":       "/work/go.mod",
		"./pkg/a.go":   "/work/pkg/a.go",
		"/etc/hosts":   "/etc/hosts",
		"../../escape": "/escape",
	} {
		if got := container.resolve(name); got != want {
			t.Errorf("resolve(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestShellQuoteSurvivesAQuote(t *testing.T) {
	if got := shellQuote("/work/it's"); got != `'/work/it'\''s'` {
		t.Errorf("shellQuote = %s", got)
	}
}

func TestJSONBodyFallsBackWhenTheValueCannotBeEncoded(t *testing.T) {
	body, err := io.ReadAll(jsonBody(make(chan int)))
	if err != nil {
		t.Fatalf("reading the body: %v", err)
	}
	if !bytes.Equal(body, []byte("{}")) {
		t.Errorf("body = %s", body)
	}
}

func TestShortIDLeavesAShortIDAlone(t *testing.T) {
	if got := shortID("abc"); got != "abc" {
		t.Errorf("shortID = %q", got)
	}
	if got := shortID(strings.Repeat("a", 64)); len(got) != 12 {
		t.Errorf("shortID = %q", got)
	}
}
