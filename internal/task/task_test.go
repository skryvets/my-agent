package task

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newRunner builds a runner whose repository, GitHub and container are all
// local, so the whole run happens on this machine.
func newRunner(t *testing.T, model *fakeAgent, box *fakeSandbox) (*Runner, *[]map[string]any) {
	runner, opened, _ := newRunnerWithNamer(t, model, box)
	return runner, opened
}

// newRunnerWithNamer also hands back the tool-free client that names the
// change, so a test can read what it was asked.
func newRunnerWithNamer(t *testing.T, model *fakeAgent, box *fakeSandbox) (*Runner, *[]map[string]any, *fakeAgent) {
	t.Helper()

	origin := newOriginAt(t, "skryvets", "my-agent")
	var opened []map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pulls") {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			opened = append(opened, body)
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"html_url":"https://github.com/skryvets/my-agent/pull/7"}`))
			return
		}
		w.Write([]byte(`{"default_branch":"main"}`))
	}))
	t.Cleanup(server.Close)

	model.box = box
	namer := &fakeAgent{answer: "Remove the unused import"}
	return &Runner{
		Agent:    model,
		Plain:    namer,
		Sandbox:  box,
		GitHub:   GitHub{BaseURL: server.URL, HTTP: server.Client()},
		Store:    Store{Dir: filepath.Join(t.TempDir(), "state")},
		WorkRoot: filepath.Join(t.TempDir(), "work"),
		GitHost:  origin,
	}, &opened, namer
}

// newOriginAt lays a repository out the way a git host does, so the address
// the runner builds finds it.
func newOriginAt(t *testing.T, owner, name string) string {
	t.Helper()

	root := t.TempDir()
	repo := filepath.Join(root, owner, name+".git")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"-c", "user.name=test", "-c", "user.email=t@e.st", "add", "-A"},
		{"-c", "user.name=test", "-c", "user.email=t@e.st", "commit", "-m", "first"},
	} {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return root
}

func TestStartOpensAPullRequest(t *testing.T) {
	model := &fakeAgent{
		answer: "I removed the unused import.",
		write:  writeInto(t, "main.go", "package main\n"),
	}
	box := newFakeSandbox()
	runner, opened, namer := newRunnerWithNamer(t, model, box)
	progress := &reporter{}

	err := runner.Start(context.Background(), "99", "skryvets/my-agent", "fix the lint warning", progress.report)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if len(*opened) != 1 {
		t.Fatalf("%d pull requests were opened", len(*opened))
	}
	request := (*opened)[0]
	// The title is the subject the model wrote, not the message that asked.
	if request["title"] != "Remove the unused import" || request["base"] != "main" {
		t.Errorf("pull request = %#v", request)
	}
	if head, _ := request["head"].(string); !strings.HasPrefix(head, "my-agent/") {
		t.Errorf("head = %#v", request["head"])
	}
	if body, _ := request["body"].(string); !strings.Contains(body, "I removed the unused import.") {
		t.Errorf("the body does not carry the summary: %#v", request["body"])
	}

	lines := strings.Join(progress.seen(), "\n")
	for _, want := range []string{"Cloning skryvets/my-agent", "Working on it", "Pushing my-agent/", "pull/7"} {
		if !strings.Contains(lines, want) {
			t.Errorf("the chat was never told %q: %s", want, lines)
		}
	}

	// The model worked in the container of this run, on the checkout.
	keys, told := model.seen()
	if len(keys) != 1 || !strings.HasPrefix(keys[0], "task-") {
		t.Errorf("the model worked as %#v", keys)
	}
	if !strings.Contains(told[0], "fix the lint warning") || !strings.Contains(told[0], "Do not run git") {
		t.Errorf("the model was told %q", told[0])
	}

	// Naming the change is a question of its own, with no tools behind it.
	_, named := namer.seen()
	if len(named) != 1 || !strings.Contains(named[0], "commit subject") {
		t.Errorf("the change was named by %#v", named)
	}
	bound, closed := box.seen()
	if len(bound) != 1 {
		t.Errorf("containers = %#v", bound)
	}
	if len(closed) != 1 || closed[0] != keys[0] {
		t.Errorf("the container of the run was not thrown away: %#v", closed)
	}
}

func TestStartStopsWhenNothingChanged(t *testing.T) {
	model := &fakeAgent{answer: "There was nothing to fix."}
	box := newFakeSandbox()
	runner, opened := newRunner(t, model, box)
	progress := &reporter{}

	if err := runner.Start(context.Background(), "99", "skryvets/my-agent", "fix nothing", progress.report); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if len(*opened) != 0 {
		t.Errorf("a pull request was opened with no change: %#v", *opened)
	}
	if !strings.Contains(strings.Join(progress.seen(), "\n"), "Nothing changed") {
		t.Errorf("the chat was not told: %#v", progress.seen())
	}

	runs := loadRuns(t, runner)
	if len(runs) != 1 || runs[0].State != NoChange {
		t.Errorf("runs = %#v", runs)
	}
}

func TestStartRefusesWhatItCannotDo(t *testing.T) {
	runner, _ := newRunner(t, &fakeAgent{}, newFakeSandbox())
	ctx := context.Background()

	if err := runner.Start(ctx, "99", "not-a-repo", "do something", func(string) {}); err == nil {
		t.Error("a repository that is not one was accepted")
	}
	if err := runner.Start(ctx, "99", "skryvets/my-agent", "   ", func(string) {}); err == nil {
		t.Error("an empty instruction was accepted")
	}
}

func TestStartWritesDownWhyItFailed(t *testing.T) {
	model := &fakeAgent{err: errors.New("the model is down")}
	box := newFakeSandbox()
	runner, _ := newRunner(t, model, box)

	err := runner.Start(context.Background(), "99", "skryvets/my-agent", "fix it", func(string) {})
	if err == nil {
		t.Fatal("expected an error")
	}

	runs := loadRuns(t, runner)
	if len(runs) != 1 || runs[0].State != Failed {
		t.Fatalf("runs = %#v", runs)
	}
	if !strings.Contains(runs[0].Detail, "the model is down") {
		t.Errorf("detail = %q", runs[0].Detail)
	}
	if _, closed := box.seen(); len(closed) != 1 {
		t.Errorf("the container of a failed run was kept: %#v", closed)
	}
}

func TestStartReportsAContainerThatWillNotStart(t *testing.T) {
	box := newFakeSandbox()
	box.err = errors.New("the daemon is gone")
	runner, _ := newRunner(t, &fakeAgent{}, box)

	if err := runner.Start(context.Background(), "99", "skryvets/my-agent", "fix it", func(string) {}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestStartLeavesNoCheckoutBehind(t *testing.T) {
	model := &fakeAgent{answer: "done", write: writeInto(t, "main.go", "package main\n")}
	runner, _ := newRunner(t, model, newFakeSandbox())

	if err := runner.Start(context.Background(), "99", "skryvets/my-agent", "fix it", func(string) {}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	left, err := os.ReadDir(runner.WorkRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("the checkout was kept: %#v", left)
	}
}

func TestInterruptedMarksWhatARestartCaught(t *testing.T) {
	runner, _ := newRunner(t, &fakeAgent{}, newFakeSandbox())

	// One run was under way when the process died, one had finished.
	runner.save(Run{ID: "20260101-000001-1", Chat: "99", Repo: "a/b", State: Working, Branch: "my-agent/1"})
	runner.save(Run{ID: "20260101-000002-2", Chat: "99", Repo: "a/b", State: Opened})

	lost := runner.Interrupted(context.Background())
	if len(lost) != 1 || lost[0].ID != "20260101-000001-1" {
		t.Fatalf("lost = %#v", lost)
	}
	if lost[0].State != Interrupted {
		t.Errorf("state = %q", lost[0].State)
	}

	// A second start must not report the same run again.
	if again := runner.Interrupted(context.Background()); len(again) != 0 {
		t.Errorf("the same run was reported twice: %#v", again)
	}
}

func loadRuns(t *testing.T, runner *Runner) []Run {
	t.Helper()
	runs, err := runner.Store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return runs
}

func TestStartFallsBackWhenTheModelWillNotNameTheChange(t *testing.T) {
	model := &fakeAgent{answer: "I fixed it.", write: writeInto(t, "main.go", "package main\n")}
	runner, opened, namer := newRunnerWithNamer(t, model, newFakeSandbox())
	namer.answer = "Here is the subject:\n\nFix it"

	if err := runner.Start(context.Background(), "99", "skryvets/my-agent", "fix the lint warning", func(string) {}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(*opened) != 1 {
		t.Fatalf("pull requests = %#v", *opened)
	}
	if title := (*opened)[0]["title"]; title != "fix the lint warning" {
		t.Errorf("title = %#v, want the message that asked", title)
	}
}
