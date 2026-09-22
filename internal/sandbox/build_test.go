package sandbox

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skryvets/my-agent/internal/devcontainer"
)

// buildConfig is a repository whose dev container builds .devcontainer/Dockerfile
// with the repository as its context.
func buildConfig(t *testing.T) devcontainer.Config {
	t.Helper()
	root := filepath.Join(t.TempDir(), "checkout")
	for name, content := range map[string]string{
		"README.md":                "hello\n",
		".devcontainer/Dockerfile": "FROM golang:1.26\n",
	} {
		file := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("README.md", filepath.Join(root, "LINK.md")); err != nil {
		t.Fatal(err)
	}
	return devcontainer.Config{
		Root: root,
		File: filepath.Join(root, ".devcontainer", "devcontainer.json"),
		Build: devcontainer.Build{
			Dockerfile: "Dockerfile",
			Context:    "..",
			Args:       map[string]string{"VARIANT": "bookworm"},
			Target:     "dev",
		},
	}
}

func TestBindBuildsTheDockerfile(t *testing.T) {
	fake := newFakeDocker(t)
	pool := newTestPool(t, fake, Options{})

	if err := pool.Bind(context.Background(), "task-1", buildConfig(t)); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	query, err := url.ParseQuery(fake.query("POST /build"))
	if err != nil {
		t.Fatal(err)
	}
	if query.Get("dockerfile") != ".devcontainer/Dockerfile" || query.Get("target") != "dev" {
		t.Errorf("build query = %#v", query)
	}
	if query.Get("buildargs") != `{"VARIANT":"bookworm"}` {
		t.Errorf("buildargs = %q", query.Get("buildargs"))
	}

	sent := map[string]string{}
	reader := tar.NewReader(strings.NewReader(fake.body("POST /build")))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("the context is not a tar: %v", err)
		}
		content, _ := io.ReadAll(reader)
		sent[header.Name] = string(content) + header.Linkname
	}
	if sent["README.md"] != "hello\n" || sent[".devcontainer/Dockerfile"] != "FROM golang:1.26\n" || sent["LINK.md"] != "README.md" {
		t.Errorf("context = %#v", sent)
	}

	if create := fake.body("POST /containers/create"); !strings.Contains(create, `"Image":"sha256:built"`) {
		t.Errorf("the container did not start from the built image: %s", create)
	}
	if fake.asked("POST /images/create") {
		t.Error("a built image was pulled too")
	}
}

func TestBuildRefusesADockerfileOutsideTheContext(t *testing.T) {
	fake := newFakeDocker(t)
	pool := newTestPool(t, fake, Options{})

	config := buildConfig(t)
	config.Build.Context = "sub"
	if err := pool.Bind(context.Background(), "task-1", config); err == nil {
		t.Fatal("expected an error")
	}
	if fake.asked("POST /build") {
		t.Error("the build was sent anyway")
	}
}

func TestBuildReportsAFailure(t *testing.T) {
	cases := map[string]struct {
		breakIt func(*fakeDocker, *devcontainer.Config)
		want    string
	}{
		"a failed step": {
			breakIt: func(f *fakeDocker, _ *devcontainer.Config) {
				f.buildStream = `{"stream":"RUN make\n"}` + "\n" + `{"errorDetail":{"message":"exit code 2"},"error":"exit code 2"}`
			},
			want: "RUN make\nexit code 2",
		},
		"no image": {
			breakIt: func(f *fakeDocker, _ *devcontainer.Config) { f.buildStream = `{"stream":"done\n"}` },
			want:    "without an image",
		},
		"not json": {
			breakIt: func(f *fakeDocker, _ *devcontainer.Config) { f.buildStream = `{oops` },
			want:    "invalid character",
		},
		"a refused request": {
			breakIt: func(f *fakeDocker, _ *devcontainer.Config) { f.fail = "/build" },
			want:    "the daemon says no",
		},
		"a context that is gone": {
			breakIt: func(_ *fakeDocker, c *devcontainer.Config) { os.RemoveAll(c.Root) },
			want:    "",
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			fake := newFakeDocker(t)
			pool := newTestPool(t, fake, Options{})
			config := buildConfig(t)
			test.breakIt(fake, &config)

			err := pool.Bind(context.Background(), "task-1", config)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("err = %v, want %q", err, test.want)
			}
		})
	}
}

func TestReadProgressKeepsOnlyTheTailOfTheLog(t *testing.T) {
	long := strings.Repeat("a", progressTail*2)
	stream := `{"stream":"` + long + `"}` + "\n" + `{"stream":"END"}` + "\n" + `{"errorDetail":{"message":"failed"}}`
	_, err := readProgress(strings.NewReader(stream))
	if err == nil {
		t.Fatal("expected an error")
	}
	if len(err.Error()) > progressTail+len("\nfailed") || !strings.Contains(err.Error(), "END\nfailed") {
		t.Errorf("err has %d bytes: %.80q", len(err.Error()), err.Error())
	}
}
