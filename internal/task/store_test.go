package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreWritesAndReadsRuns(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "state")}

	first := Run{ID: "20260101-000001-1", Chat: "99", Repo: "a/b", State: Working, Started: time.Now().UTC()}
	second := Run{ID: "20260101-000002-2", Chat: "99", Repo: "a/b", State: Opened}
	if err := store.Save(first); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Save(second); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// A later save of the same run replaces the earlier one.
	first.State = Opened
	if err := store.Save(first); err != nil {
		t.Fatalf("Save: %v", err)
	}

	runs, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs = %#v", runs)
	}
	if runs[0].ID != first.ID || runs[0].State != Opened {
		t.Errorf("oldest run = %#v", runs[0])
	}
}

func TestStoreWithNoDirectoryKeepsNothing(t *testing.T) {
	store := Store{}
	if err := store.Save(Run{ID: "1"}); err != nil {
		t.Errorf("Save: %v", err)
	}
	runs, err := store.Load()
	if err != nil || runs != nil {
		t.Errorf("runs = %#v, err = %v", runs, err)
	}
}

func TestStoreSkipsAFileItCannotRead(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}
	if err := store.Save(Run{ID: "20260101-000001-1", State: Opened}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "20260101-000002-2.json"), []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}

	runs, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(runs) != 1 {
		t.Errorf("one unreadable file hid the rest: %#v", runs)
	}
}

func TestStoreReportsADirectoryItCannotUse(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := Store{Dir: filepath.Join(blocked, "state")}
	if err := store.Save(Run{ID: "1"}); err == nil {
		t.Error("expected an error when the directory cannot be made")
	}
	if _, err := (Store{Dir: blocked}).Load(); err != nil {
		t.Errorf("Load of a directory with no runs: %v", err)
	}
}

func TestStateKnowsWhenARunIsOver(t *testing.T) {
	for _, state := range []State{Opened, NoChange, Failed, Interrupted} {
		if !state.Done() {
			t.Errorf("%q is not final", state)
		}
	}
	for _, state := range []State{Cloning, Working, Pushing} {
		if state.Done() {
			t.Errorf("%q is final", state)
		}
	}
}

func TestAddressCarriesTheTokenOnlyOverHTTPS(t *testing.T) {
	repo := Repo{Owner: "skryvets", Name: "my-agent"}

	plain := Git{}.address(repo)
	if plain != "https://github.com/skryvets/my-agent.git" {
		t.Errorf("address = %q", plain)
	}

	withToken := Git{Token: "gh-token"}.address(repo)
	if withToken != "https://x-access-token:gh-token@github.com/skryvets/my-agent.git" {
		t.Errorf("address = %q", withToken)
	}

	// A local host is a path, and a path carries no credentials.
	local := Git{Token: "gh-token", Host: "/srv/git"}.address(repo)
	if strings.Contains(local, "gh-token") {
		t.Errorf("the token was put into a path: %q", local)
	}
}

func TestSubjectAndBodyStayReadable(t *testing.T) {
	long := strings.Repeat("a", subjectLimit*2)
	if got := subjectOf(long); len(got) != subjectLimit {
		t.Errorf("subject is %d characters: %q", len(got), got)
	}
	if got := subjectOf("first line\nsecond line"); got != "first line" {
		t.Errorf("subject = %q", got)
	}
	if got := subjectOf("  "); got == "" {
		t.Error("an empty instruction gave an empty subject")
	}

	body := body("do the thing", strings.Repeat("b", summaryLimit*2))
	if !strings.Contains(body, "do the thing") {
		t.Error("the body does not say what was asked for")
	}
	if len(body) > summaryLimit+500 {
		t.Errorf("the body is %d characters", len(body))
	}
}

func TestCheckoutFallsBackToTheTemporaryDirectory(t *testing.T) {
	runner := &Runner{}
	dir, err := runner.checkout("20260101-000001-1")
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if !strings.HasPrefix(dir, os.TempDir()) {
		t.Errorf("checkout = %q, want one under %q", dir, os.TempDir())
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("the directory was made, and git clone wants to make it itself")
	}
}

func TestSubjectLineAcceptsOnlyOneCleanLine(t *testing.T) {
	if got := subjectLine("  Fix the misspelled greeting  "); got != "Fix the misspelled greeting" {
		t.Errorf("subject = %q", got)
	}
	if got := subjectLine("`Fix the greeting`"); got != "Fix the greeting" {
		t.Errorf("a quoted line = %q", got)
	}

	for name, answer := range map[string]string{
		"two lines":   "Fix the greeting\n\nand here is why",
		"a lead-in":   "Here is the subject:",
		"a long line": strings.Repeat("a", subjectLimit+1),
		"nothing":     "   ",
	} {
		if got := subjectLine(answer); got != "" {
			t.Errorf("%s gave the subject %q", name, got)
		}
	}
}
