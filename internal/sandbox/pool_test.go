package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/skryvets/my-agent/internal/conversation"
)

func newTestPool(t *testing.T, fake *fakeDocker, options Options) *Pool {
	t.Helper()
	options.Socket = fake.socket

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	pool, err := New(ctx, options)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return pool
}

func TestNewReachesTheDaemonAndClearsAnEarlierRun(t *testing.T) {
	fake := newFakeDocker(t)
	fake.orphans = []string{"left-over-01"}

	newTestPool(t, fake, Options{})

	if !fake.asked("GET /version") {
		t.Error("the daemon was not reached")
	}
	if !fake.asked("DELETE /containers/left-over-01") {
		t.Errorf("the container of an earlier run was kept: %#v", fake.seen())
	}
	if fake.asked("POST /images/create") {
		t.Error("an image the daemon already has was pulled again")
	}
}

func TestNewPullsAnImageTheDaemonDoesNotHave(t *testing.T) {
	fake := newFakeDocker(t)
	fake.images = map[string]bool{}

	newTestPool(t, fake, Options{Image: "alpine:3"})

	if !fake.asked("POST /images/create") {
		t.Errorf("the image was not pulled: %#v", fake.seen())
	}
}

func TestNewReportsADaemonThatIsNotThere(t *testing.T) {
	if _, err := New(context.Background(), Options{Socket: "/nowhere/docker.sock"}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestNewReportsWhatTheDaemonRefuses(t *testing.T) {
	fake := newFakeDocker(t)
	fake.fail = "/containers/json"

	_, err := New(context.Background(), Options{Socket: fake.socket})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "the daemon says no") {
		t.Errorf("err = %v, want the message of the daemon", err)
	}
}

func TestPoolStartsOneContainerForEachConversation(t *testing.T) {
	fake := newFakeDocker(t)
	fake.output = "hello\n"
	pool := newTestPool(t, fake, Options{})

	first := conversation.WithKey(context.Background(), "chat-1")
	if _, err := pool.Run(first, "echo hello"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := pool.Run(first, "echo again"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	created := 0
	for _, request := range fake.seen() {
		if request == "POST /containers/create" {
			created++
		}
	}
	if created != 1 {
		t.Errorf("%d containers were created for one conversation, want 1", created)
	}

	second := conversation.WithKey(context.Background(), "chat-2")
	if _, err := pool.Run(second, "echo hello"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	created = 0
	for _, request := range fake.seen() {
		if request == "POST /containers/create" {
			created++
		}
	}
	if created != 2 {
		t.Errorf("a second conversation got %d containers in total, want 2", created)
	}
}

func TestPoolStartsAContainerWithoutANetwork(t *testing.T) {
	fake := newFakeDocker(t)
	pool := newTestPool(t, fake, Options{})

	if _, err := pool.Run(context.Background(), "true"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	body := fake.body("POST /containers/create")
	if !strings.Contains(body, `"NetworkMode":"none"`) {
		t.Errorf("the container can reach the network: %s", body)
	}
	if !strings.Contains(body, label) {
		t.Errorf("the container carries no label: %s", body)
	}
	if !strings.Contains(body, `"WorkingDir":"`+workDir+`"`) {
		t.Errorf("the working directory is wrong: %s", body)
	}
}

func TestPoolCloseThrowsTheContainerAway(t *testing.T) {
	fake := newFakeDocker(t)
	pool := newTestPool(t, fake, Options{})
	ctx := conversation.WithKey(context.Background(), "chat-1")

	if _, err := pool.Run(ctx, "true"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := pool.Close(ctx, "chat-1"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !fake.asked("DELETE /containers/container-1") {
		t.Errorf("the container was kept: %#v", fake.seen())
	}

	// Closing a conversation that has no container is not an error.
	if err := pool.Close(ctx, "never-started"); err != nil {
		t.Errorf("Close of an unknown conversation: %v", err)
	}

	// The next message starts a new container.
	if _, err := pool.Run(ctx, "true"); err != nil {
		t.Fatalf("Run after Close: %v", err)
	}
	created := 0
	for _, request := range fake.seen() {
		if request == "POST /containers/create" {
			created++
		}
	}
	if created != 2 {
		t.Errorf("%d containers were created, want a new one after Close", created)
	}
}

func TestPoolReapsAContainerThatWentQuiet(t *testing.T) {
	fake := newFakeDocker(t)
	pool := newTestPool(t, fake, Options{Idle: 20 * time.Millisecond})

	if _, err := pool.Run(conversation.WithKey(context.Background(), "chat-1"), "true"); err != nil {
		t.Fatalf("Run: %v", err)
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
	pool := newTestPool(t, fake, Options{})

	if _, err := pool.Run(conversation.WithKey(context.Background(), "chat-1"), "true"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	pool.Shutdown(context.Background())

	if !fake.asked("DELETE /containers/container-1") {
		t.Errorf("a container survived the shutdown: %#v", fake.seen())
	}

	// A daemon that refuses is reported and does not stop the shutdown.
	fake.fail = "/containers/container-1"
	if _, err := pool.Run(conversation.WithKey(context.Background(), "chat-2"), "true"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	pool.Shutdown(context.Background())
}

func TestPoolReportsAContainerThatWillNotStart(t *testing.T) {
	fake := newFakeDocker(t)
	pool := newTestPool(t, fake, Options{})
	fake.fail = "/containers/create"

	if _, err := pool.Run(context.Background(), "true"); err == nil {
		t.Error("expected an error from Run")
	}
	if _, err := pool.ReadFile(context.Background(), "a.txt"); err == nil {
		t.Error("expected an error from ReadFile")
	}
	if err := pool.WriteFile(context.Background(), "a.txt", "x"); err == nil {
		t.Error("expected an error from WriteFile")
	}
}

func TestAConversationWithNoNameStillGetsAContainer(t *testing.T) {
	fake := newFakeDocker(t)
	pool := newTestPool(t, fake, Options{})

	if _, err := pool.Run(context.Background(), "true"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := pool.Close(context.Background(), conversation.Default); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !fake.asked("DELETE /containers/container-1") {
		t.Errorf("an unnamed conversation got no container: %#v", fake.seen())
	}
}
