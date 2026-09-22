package sandbox

import (
	"context"
	"testing"

	"github.com/skryvets/my-agent/internal/devcontainer"
)

func TestStartPullsAnImageTheDaemonDoesNotHave(t *testing.T) {
	fake := newFakeDocker(t)
	fake.images = map[string]bool{}
	docker := newTestDocker(t, fake)

	config := devcontainer.Config{Image: "mcr.microsoft.com/devcontainers/go:1", Root: "/host/checkout"}
	if _, err := docker.Start(context.Background(), "task-1", config); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if query := fake.query("POST /images/create"); query != "fromImage=mcr.microsoft.com%2Fdevcontainers%2Fgo&tag=1" {
		t.Errorf("pull query = %q", query)
	}
}

func TestStartSkipsThePullOfAnImageTheDaemonHas(t *testing.T) {
	fake := newFakeDocker(t)
	started(t, fake)

	if fake.asked("POST /images/create") {
		t.Error("an image the daemon already has was pulled again")
	}
}

func TestPullReportsAFailure(t *testing.T) {
	cases := map[string]func(*fakeDocker){
		"inside the stream": func(f *fakeDocker) {
			f.pullStream = `{"errorDetail":{"message":"manifest unknown"},"error":"manifest unknown"}`
		},
		"as a status": func(f *fakeDocker) { f.fail = "/images/create" },
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			fake := newFakeDocker(t)
			fake.images = map[string]bool{}
			docker := newTestDocker(t, fake)
			breakIt(fake)

			if _, err := docker.Start(context.Background(), "task-1", imageConfig()); err == nil {
				t.Fatal("expected an error")
			}
			if fake.asked("POST /containers/create") {
				t.Error("a container was created from an image that did not arrive")
			}
		})
	}
}
