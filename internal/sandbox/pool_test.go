package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/skryvets/my-agent/internal/conversation"
	"github.com/skryvets/my-agent/internal/devcontainer"
)

func TestNewReachesTheDaemonAndClearsAnEarlierRun(t *testing.T) {
	fake := newFakeDocker(t)
	fake.orphans = []string{"left-over-01"}

	newTestPool(t, fake, Options{})

	if !fake.asked("GET /containers/json") {
		t.Error("the daemon was not reached")
	}
	if !fake.asked("DELETE /containers/left-over-01") {
		t.Errorf("the container of an earlier run was kept: %#v", fake.seen())
	}
	if fake.asked("POST /images/create") || fake.asked("POST /containers/create") {
		t.Errorf("the pool started something before it was asked: %#v", fake.seen())
	}
}

func TestNewReportsADaemonThatIsNotThere(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix:///nowhere/docker.sock")
	if _, err := New(context.Background(), Options{}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestNewReportsWhatTheDaemonRefuses(t *testing.T) {
	fake := newFakeDocker(t)
	fake.fail = "/containers/json"
	t.Setenv("DOCKER_HOST", "unix://"+fake.socket)

	_, err := New(context.Background(), Options{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "the daemon says no") {
		t.Errorf("err = %v, want the message of the daemon", err)
	}
}

func TestBindStartsTheDevContainerOnTheCheckout(t *testing.T) {
	fake := newFakeDocker(t)
	pool, ctx := bound(t, fake, "task-1")

	body := fake.body("POST /containers/create")
	for _, want := range []string{
		`"Binds":["/host/checkout:/workspaces/checkout"]`,
		`"WorkingDir":"/workspaces/checkout"`,
		`"Image":"` + testImage + `"`,
		`"Entrypoint":["sh","-c",`,
		label,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the container was created without %s: %s", want, body)
		}
	}
	if strings.Contains(body, `"NetworkMode":"none"`) {
		t.Errorf("the container was cut off the network: %s", body)
	}

	if _, err := pool.Run(ctx, "ls"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if created := fake.count("POST /containers/create"); created != 1 {
		t.Errorf("%d containers were created, want the bound one to be reused", created)
	}

	if err := pool.Bind(context.Background(), "task-1", imageConfig()); err == nil {
		t.Error("a second container was bound to the same conversation")
	}
}

func TestBindKeepsConversationsApart(t *testing.T) {
	fake := newFakeDocker(t)
	pool := newTestPool(t, fake, Options{})

	for _, key := range []string{"task-1", "task-2"} {
		if err := pool.Bind(context.Background(), key, imageConfig()); err != nil {
			t.Fatalf("Bind %s: %v", key, err)
		}
	}
	if created := fake.count("POST /containers/create"); created != 2 {
		t.Errorf("%d containers were created, want one for each conversation", created)
	}
}

func TestPoolRefusesAConversationWithNoContainer(t *testing.T) {
	fake := newFakeDocker(t)
	pool := newTestPool(t, fake, Options{})
	ctx := conversation.WithKey(context.Background(), "chat-1")

	if _, err := pool.Run(ctx, "true"); err == nil {
		t.Error("expected an error from Run")
	}
	if _, err := pool.ReadFile(ctx, "a.txt"); err == nil {
		t.Error("expected an error from ReadFile")
	}
	if err := pool.WriteFile(ctx, "a.txt", "x"); err == nil {
		t.Error("expected an error from WriteFile")
	}
	if _, _, err := pool.Exec(ctx, []string{"true"}); err == nil {
		t.Error("expected an error from Exec")
	}
	if fake.asked("POST /containers/create") {
		t.Error("a conversation with no dev container got a container")
	}
}

func TestBindSetsTheUserAndTheEnvironment(t *testing.T) {
	fake := newFakeDocker(t)
	fake.env = []string{"PATH=/usr/bin"}
	pool := newTestPool(t, fake, Options{})

	config := imageConfig()
	config.ContainerUser = "root"
	config.ContainerEnv = map[string]string{"GOFLAGS": "-mod=mod"}
	config.RemoteUser = "vscode"
	config.RemoteEnv = map[string]string{"PATH": "${containerEnv:PATH}:/go/bin"}
	if err := pool.Bind(context.Background(), "task-1", config); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	create := fake.body("POST /containers/create")
	if !strings.Contains(create, `"Env":["GOFLAGS=-mod=mod"]`) || !strings.Contains(create, `"User":"root"`) {
		t.Errorf("create = %s", create)
	}

	if _, _, err := pool.Exec(conversation.WithKey(context.Background(), "task-1"), []string{"go", "env"}); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	exec := fake.body("POST /containers/container-1/exec")
	if !strings.Contains(exec, `"User":"vscode"`) || !strings.Contains(exec, `"Env":["PATH=/usr/bin:/go/bin"]`) {
		t.Errorf("exec = %s", exec)
	}
}

func TestBindReportsAContainerThatWillNotStart(t *testing.T) {
	for _, failing := range []string{"/containers/create", "/containers/container-1/start", "/containers/container-1/json"} {
		t.Run(failing, func(t *testing.T) {
			fake := newFakeDocker(t)
			pool := newTestPool(t, fake, Options{})
			fake.fail = failing

			config := imageConfig()
			config.RemoteEnv = map[string]string{"A": "b"}
			if err := pool.Bind(context.Background(), "task-1", config); err == nil {
				t.Fatal("expected an error")
			}
			if failing != "/containers/create" && !fake.asked("DELETE /containers/container-1") {
				t.Errorf("a container that failed to start was kept: %#v", fake.seen())
			}
			fake.fail = ""
			if err := pool.Bind(context.Background(), "task-1", imageConfig()); err != nil {
				t.Errorf("the failed bind held on to the conversation: %v", err)
			}
		})
	}
}

func TestPoolCloseThrowsTheContainerAway(t *testing.T) {
	fake := newFakeDocker(t)
	pool, ctx := bound(t, fake, "task-1")

	if err := pool.Close(ctx, "task-1"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !fake.asked("DELETE /containers/container-1") {
		t.Errorf("the container was kept: %#v", fake.seen())
	}
	if _, err := pool.Run(ctx, "true"); err == nil {
		t.Error("a closed conversation still reached a container")
	}

	// Closing a conversation that has no container is not an error.
	if err := pool.Close(ctx, "never-started"); err != nil {
		t.Errorf("Close of an unknown conversation: %v", err)
	}
}

func TestPoolReapsAContainerThatWentQuiet(t *testing.T) {
	fake := newFakeDocker(t)
	pool := newTestPool(t, fake, Options{Idle: 20 * time.Millisecond})

	if err := pool.Bind(context.Background(), "task-1", imageConfig()); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fake.asked("DELETE /containers/container-1") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Errorf("the idle container was not reaped: %#v", fake.seen())
}

func TestPoolShutdownRemovesEverythingItStarted(t *testing.T) {
	fake := newFakeDocker(t)
	pool, _ := bound(t, fake, "task-1")

	pool.Shutdown(context.Background())
	if !fake.asked("DELETE /containers/container-1") {
		t.Errorf("a container survived the shutdown: %#v", fake.seen())
	}

	// A daemon that refuses is reported and does not stop the shutdown.
	if err := pool.Bind(context.Background(), "task-2", devcontainer.Config{Image: testImage, Root: "/host/other"}); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	fake.fail = "/containers/container-1"
	pool.Shutdown(context.Background())
}
