package task

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newOrigin makes a repository on disk that a clone can be taken from, so no
// test reaches the network.
func newOrigin(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	work := filepath.Join(dir, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"-c", "user.name=test", "-c", "user.email=t@e.st", "add", "-A"},
		{"-c", "user.name=test", "-c", "user.email=t@e.st", "commit", "-m", "first"},
	} {
		cmd := exec.Command("git", append([]string{"-C", work}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return work
}

func TestGitClonesBranchesCommitsAndPushes(t *testing.T) {
	origin := newOrigin(t)
	dir := filepath.Join(t.TempDir(), "checkout")
	git := Git{Dir: dir}
	ctx := context.Background()

	// A local path stands in for the GitHub address the runner builds.
	if err := git.cloneFrom(ctx, origin, "my-agent/1"); err != nil {
		t.Fatalf("clone: %v", err)
	}

	changed, err := git.Changed(ctx)
	if err != nil {
		t.Fatalf("Changed: %v", err)
	}
	if changed {
		t.Error("a fresh checkout reported changes")
	}

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello again\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err = git.Changed(ctx)
	if err != nil {
		t.Fatalf("Changed: %v", err)
	}
	if !changed {
		t.Fatal("a written file was not seen")
	}

	if err := git.Commit(ctx, "Say hello again", "because the test asked"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := git.Push(ctx, "my-agent/1"); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// The branch must now be in the repository that was cloned.
	out, err := exec.Command("git", "-C", origin, "branch", "--list", "my-agent/1").CombinedOutput()
	if err != nil {
		t.Fatalf("git branch: %v: %s", err, out)
	}
	if !strings.Contains(string(out), "my-agent/1") {
		t.Errorf("the branch never arrived: %s", out)
	}
}

func TestGitKeepsTheTokenOutOfItsErrors(t *testing.T) {
	git := Git{Dir: filepath.Join(t.TempDir(), "checkout"), Token: "gh-secret-token"}

	// An address that does not exist, carrying the token where the GitHub
	// one carries it. git names the address it failed on, so the error
	// would repeat the token into a chat.
	err := git.cloneFrom(context.Background(), "/nowhere/gh-secret-token/repo.git", "my-agent/1")
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "gh-secret-token") {
		t.Errorf("the token was printed: %v", err)
	}
	if !strings.Contains(err.Error(), "***") {
		t.Errorf("err = %v, want the token hidden", err)
	}
}

func TestGitReportsACommitThatCannotRun(t *testing.T) {
	git := Git{Dir: filepath.Join(t.TempDir(), "nowhere")}
	ctx := context.Background()

	if _, err := git.Changed(ctx); err == nil {
		t.Error("expected an error outside a repository")
	}
	if err := git.Commit(ctx, "subject", ""); err == nil {
		t.Error("expected an error outside a repository")
	}
	if err := git.Push(ctx, "my-agent/1"); err == nil {
		t.Error("expected an error outside a repository")
	}
}
