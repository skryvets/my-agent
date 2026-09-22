package sandbox

import (
	"archive/tar"
	"context"
	"io"
	"strings"
	"testing"
)

func TestContainerRunReturnsTheOutputAndTheExitStatus(t *testing.T) {
	fake := newFakeDocker(t)
	fake.output = "README.md\ngo.mod\n"
	box := started(t, fake)
	ctx := context.Background()

	got, err := box.Run(ctx, "ls")
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
	got, err = box.Run(ctx, "false")
	if err != nil {
		t.Fatalf("a non-zero exit must not be an error: %v", err)
	}
	if !strings.Contains(got, "[exit status 2]") {
		t.Errorf("output = %q", got)
	}

	output, code, err := box.Exec(ctx, []string{"false"})
	if err != nil || code != 2 || strings.Contains(output, "exit status") {
		t.Errorf("Exec = %q, %d, %v, want the code apart from the output", output, code, err)
	}
}

func TestContainerRunReportsAnExecThatFails(t *testing.T) {
	for _, failing := range []string{"/containers/container-1/exec", "/exec/exec-1/start", "/exec/exec-1/json"} {
		t.Run(failing, func(t *testing.T) {
			fake := newFakeDocker(t)
			box := started(t, fake)
			ctx := context.Background()
			fake.fail = failing
			if _, err := box.Run(ctx, "ls"); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestContainerReadFileUnpacksTheArchive(t *testing.T) {
	fake := newFakeDocker(t)
	fake.files["/workspaces/checkout/go.mod"] = "module example\n"
	fake.files["/etc/hosts"] = "127.0.0.1 localhost\n"
	box := started(t, fake)
	ctx := context.Background()

	got, err := box.ReadFile(ctx, "go.mod")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got != "module example\n" {
		t.Errorf("content = %q", got)
	}

	// An absolute path reaches the whole container, which is the point of
	// having one: there is nothing inside worth guarding.
	got, err = box.ReadFile(ctx, "/etc/hosts")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got != "127.0.0.1 localhost\n" {
		t.Errorf("content = %q", got)
	}

	if _, err := box.ReadFile(ctx, "absent.txt"); err == nil {
		t.Error("expected an error for a file that is not there")
	}
}

func TestContainerReadFileReportsAnEmptyArchive(t *testing.T) {
	fake := newFakeDocker(t)
	fake.files["/workspaces/checkout/empty"] = ""
	box := started(t, fake)
	ctx := context.Background()

	// A directory answers with an archive that holds no regular file.
	fake.onlyDirectories = true
	if _, err := box.ReadFile(ctx, "empty"); err == nil {
		t.Error("expected an error for an archive with no file in it")
	}
}

func TestContainerWriteFileSendsATarAndMakesTheDirectory(t *testing.T) {
	fake := newFakeDocker(t)
	box := started(t, fake)
	ctx := context.Background()

	if err := box.WriteFile(ctx, "pkg/hello.go", "package pkg\n"); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// The archive endpoint extracts into a directory that must already be
	// there, so the write makes it first.
	exec := fake.body("POST /containers/container-1/exec")
	if !strings.Contains(exec, "mkdir -p '/workspaces/checkout/pkg'") {
		t.Errorf("the parent directory was not made: %s", exec)
	}

	sent := fake.files["/workspaces/checkout/pkg"]
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
	box := started(t, fake)
	ctx := context.Background()

	fake.fail = "/containers/container-1/exec"
	if err := box.WriteFile(ctx, "pkg/hello.go", "x"); err == nil {
		t.Error("expected an error when the directory could not be made")
	}
}

func TestResolveReadsAPathAgainstTheWorkingDirectory(t *testing.T) {
	container := &Container{dir: "/work"}

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
