package sandbox

import (
	"context"
	"strings"
	"testing"

	"github.com/skryvets/my-agent/internal/devcontainer"
)

func TestBindPullsAnImageTheDaemonDoesNotHave(t *testing.T) {
	fake := newFakeDocker(t)
	fake.images = map[string]bool{}
	pool := newTestPool(t, fake, Options{})

	config := devcontainer.Config{Image: "mcr.microsoft.com/devcontainers/go:1", Root: "/host/checkout"}
	if err := pool.Bind(context.Background(), "task-1", config); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if query := fake.query("POST /images/create"); query != "fromImage=mcr.microsoft.com%2Fdevcontainers%2Fgo&tag=1" {
		t.Errorf("pull query = %q", query)
	}
}

func TestBindSkipsThePullOfAnImageTheDaemonHas(t *testing.T) {
	fake := newFakeDocker(t)
	bound(t, fake, "task-1")

	if fake.asked("POST /images/create") {
		t.Error("an image the daemon already has was pulled again")
	}
}

func TestPullReportsAFailure(t *testing.T) {
	cases := map[string]func(*fakeDocker){
		"inside the stream": func(f *fakeDocker) { f.pullStream = `{"error":"manifest unknown"}` },
		"as a status":       func(f *fakeDocker) { f.fail = "/images/create" },
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			fake := newFakeDocker(t)
			fake.images = map[string]bool{}
			pool := newTestPool(t, fake, Options{})
			breakIt(fake)

			if err := pool.Bind(context.Background(), "task-1", imageConfig()); err == nil {
				t.Fatal("expected an error")
			}
			if fake.asked("POST /containers/create") {
				t.Error("a container was created from an image that did not arrive")
			}
		})
	}
}

func TestSplitReferenceNamesTheTag(t *testing.T) {
	cases := map[string][2]string{
		"golang":                      {"golang", "latest"},
		"golang:1.26":                 {"golang", "1.26"},
		"localhost:5000/team/app":     {"localhost:5000/team/app", "latest"},
		"localhost:5000/team/app:dev": {"localhost:5000/team/app", "dev"},
		"alpine@sha256:abc":           {"alpine", "sha256:abc"},
	}
	for reference, want := range cases {
		name, tag := splitReference(reference)
		if name != want[0] || tag != want[1] {
			t.Errorf("splitReference(%q) = %q, %q", reference, name, tag)
		}
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
