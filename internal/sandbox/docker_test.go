package sandbox

import (
	"context"
	"strings"
	"testing"
)

func TestNewReachesTheDaemonAndClearsAnEarlierRun(t *testing.T) {
	fake := newFakeDocker(t)
	fake.orphans = []string{"left-over-01"}

	newTestDocker(t, fake)

	if !fake.asked("GET /containers/json") {
		t.Error("the daemon was not reached")
	}
	if !fake.asked("DELETE /containers/left-over-01") {
		t.Errorf("the container of an earlier run was kept: %#v", fake.seen())
	}
	if fake.asked("POST /images/create") || fake.asked("POST /containers/create") {
		t.Errorf("something started before it was asked for: %#v", fake.seen())
	}
	if query := fake.query("GET /containers/json"); !strings.Contains(query, label) {
		t.Errorf("the sweep did not look for the label: %q", query)
	}
}

func TestNewReportsADaemonThatIsNotThere(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix:///nowhere/docker.sock")
	if _, err := New(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestNewReportsWhatTheDaemonRefuses(t *testing.T) {
	fake := newFakeDocker(t)
	fake.fail = "/containers/json"
	t.Setenv("DOCKER_HOST", "unix://"+fake.socket)

	_, err := New(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "the daemon says no") {
		t.Errorf("err = %v, want the message of the daemon", err)
	}
}

func TestSweepReportsAContainerItCouldNotRemove(t *testing.T) {
	fake := newFakeDocker(t)
	docker := newTestDocker(t, fake)

	fake.orphans = []string{"stuck-01"}
	fake.fail = "/containers/stuck-01"
	if err := docker.Sweep(context.Background()); err == nil {
		t.Error("expected an error")
	}
}

func TestStartRunsTheDevContainerOnTheCheckout(t *testing.T) {
	fake := newFakeDocker(t)
	box := started(t, fake)

	body := fake.body("POST /containers/create")
	for _, want := range []string{
		`"Binds":["/host/checkout:/workspaces/checkout"]`,
		`"WorkingDir":"/workspaces/checkout"`,
		`"Image":"` + testImage + `"`,
		`"Entrypoint":["sh","-c",`,
		`"` + label + `":"task-1"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the container was created without %s: %s", want, body)
		}
	}
	if strings.Contains(body, `"NetworkMode":"none"`) {
		t.Errorf("the container was cut off the network: %s", body)
	}

	if _, err := box.Run(context.Background(), "ls"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !fake.asked("POST /containers/container-1/exec") {
		t.Errorf("the command did not run in the started container: %#v", fake.seen())
	}
}

func TestStartSetsTheUserAndTheEnvironment(t *testing.T) {
	fake := newFakeDocker(t)
	fake.env = []string{"PATH=/usr/bin"}
	docker := newTestDocker(t, fake)

	config := imageConfig()
	config.ContainerUser = "root"
	config.ContainerEnv = map[string]string{"GOFLAGS": "-mod=mod"}
	config.RemoteUser = "vscode"
	config.RemoteEnv = map[string]string{"PATH": "${containerEnv:PATH}:/go/bin"}
	box, err := docker.Start(context.Background(), "task-1", config)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	create := fake.body("POST /containers/create")
	if !strings.Contains(create, `"Env":["GOFLAGS=-mod=mod"]`) || !strings.Contains(create, `"User":"root"`) {
		t.Errorf("create = %s", create)
	}

	if _, _, err := box.Exec(context.Background(), []string{"go", "env"}); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	exec := fake.body("POST /containers/container-1/exec")
	if !strings.Contains(exec, `"User":"vscode"`) || !strings.Contains(exec, `"Env":["PATH=/usr/bin:/go/bin"]`) {
		t.Errorf("exec = %s", exec)
	}
}

func TestStartReportsAContainerThatWillNotStart(t *testing.T) {
	for _, failing := range []string{"/containers/create", "/containers/container-1/start", "/containers/container-1/json"} {
		t.Run(failing, func(t *testing.T) {
			fake := newFakeDocker(t)
			docker := newTestDocker(t, fake)
			fake.fail = failing

			config := imageConfig()
			config.RemoteEnv = map[string]string{"A": "b"}
			if _, err := docker.Start(context.Background(), "task-1", config); err == nil {
				t.Fatal("expected an error")
			}
			if failing != "/containers/create" && !fake.asked("DELETE /containers/container-1") {
				t.Errorf("a container that failed to start was kept: %#v", fake.seen())
			}
		})
	}
}

func TestRemoveThrowsTheContainerAway(t *testing.T) {
	fake := newFakeDocker(t)
	box := started(t, fake)

	if err := box.Remove(context.Background()); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !fake.asked("DELETE /containers/container-1") {
		t.Errorf("the container was kept: %#v", fake.seen())
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
