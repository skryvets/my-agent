package sandbox

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/skryvets/my-agent/internal/devcontainer"
)

const testImage = "golang:1.26"

// fakeDocker is an Engine API daemon that answers over a unix socket and
// records what it was asked, so no test needs a real Docker.
type fakeDocker struct {
	socket string

	mu       sync.Mutex
	requests []string
	bodies   map[string]string
	queries  map[string]string

	// images is what the daemon already has, so a pull is only asked for
	// when it is missing.
	images map[string]bool
	// orphans is what a previous run left behind.
	orphans []string
	// output is what the next exec prints, and exitCode is how it ends.
	output   string
	exitCode int
	// hang keeps an exec running until the client closes the connection.
	hang bool
	// files answers an archive request, by absolute path.
	files map[string]string
	// fail makes exactly one path answer with a 500.
	fail string
	// onlyDirectories answers a file read with an archive that holds no
	// regular file, which is what reading a directory gives.
	onlyDirectories bool
	// pullStream and buildStream are the progress the daemon streams back.
	pullStream  string
	buildStream string
	// env is what the container reports it was started with.
	env []string
}

func newFakeDocker(t *testing.T) *fakeDocker {
	t.Helper()

	// A unix socket path is capped near 104 bytes, which the directory of a
	// test with a long name can pass.
	dir, err := os.MkdirTemp("", "sb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	fake := &fakeDocker{
		socket:      filepath.Join(dir, "d.sock"),
		bodies:      map[string]string{},
		queries:     map[string]string{},
		images:      map[string]bool{testImage: true},
		files:       map[string]string{},
		pullStream:  `{"status":"Pulling"}` + "\n",
		buildStream: `{"stream":"Step 1/1 : FROM golang\n"}` + "\n" + `{"aux":{"ID":"sha256:built"}}` + "\n",
	}

	listener, err := net.Listen("unix", fake.socket)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(fake.serve))
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	return fake
}

func (f *fakeDocker) serve(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/"+apiVersion)
	body, _ := io.ReadAll(r.Body)

	f.mu.Lock()
	f.requests = append(f.requests, r.Method+" "+path)
	f.bodies[r.Method+" "+path] = string(body)
	f.queries[r.Method+" "+path] = r.URL.RawQuery
	failing := f.fail
	f.mu.Unlock()

	if failing != "" && path == failing {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message":"the daemon says no"}`)
		return
	}

	switch {
	case path == "/containers/json":
		f.writeOrphans(w)
	case strings.HasPrefix(path, "/images/") && strings.HasSuffix(path, "/json"):
		f.writeImage(w, path)
	case path == "/images/create":
		fmt.Fprint(w, f.stream(&f.pullStream))
	case path == "/build":
		fmt.Fprint(w, f.stream(&f.buildStream))
	case path == "/containers/create":
		fmt.Fprint(w, `{"Id":"container-1"}`)
	case strings.HasSuffix(path, "/start") && strings.HasPrefix(path, "/containers/"):
		w.WriteHeader(http.StatusNoContent)
	case strings.HasPrefix(path, "/containers/") && strings.HasSuffix(path, "/json"):
		f.mu.Lock()
		fmt.Fprintf(w, `{"Config":{"Env":["%s"]}}`, strings.Join(f.env, `","`))
		f.mu.Unlock()
	case strings.HasSuffix(path, "/exec"):
		fmt.Fprint(w, `{"Id":"exec-1"}`)
	case path == "/exec/exec-1/start":
		f.writeOutput(w)
	case path == "/exec/exec-1/json":
		f.writeExit(w)
	case strings.HasSuffix(path, "/archive"):
		f.archive(w, r)
	case r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"message":"no route for %s"}`, path)
	}
}

func (f *fakeDocker) stream(field *string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return *field
}

func (f *fakeDocker) writeOrphans(w http.ResponseWriter) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := make([]string, 0, len(f.orphans))
	for _, id := range f.orphans {
		ids = append(ids, `{"Id":"`+id+`"}`)
	}
	fmt.Fprint(w, "["+strings.Join(ids, ",")+"]")
}

func (f *fakeDocker) writeImage(w http.ResponseWriter, path string) {
	name := strings.TrimSuffix(strings.TrimPrefix(path, "/images/"), "/json")
	f.mu.Lock()
	known := f.images[name]
	f.mu.Unlock()
	if !known {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"no such image"}`)
		return
	}
	fmt.Fprint(w, `{"Id":"image-1"}`)
}

// writeOutput answers an exec start the way the daemon does: it takes over
// the connection and streams the output raw until the command ends.
func (f *fakeDocker) writeOutput(w http.ResponseWriter) {
	f.mu.Lock()
	output, hang := f.output, f.hang
	f.mu.Unlock()

	conn, buffered, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	fmt.Fprint(buffered, "HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n"+output)
	buffered.Flush()
	if hang {
		io.Copy(io.Discard, conn)
	}
}

func (f *fakeDocker) writeExit(w http.ResponseWriter) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fmt.Fprintf(w, `{"ExitCode":%d,"Running":false}`, f.exitCode)
}

// archive answers a file read with a tar, and stores what a write extracts.
func (f *fakeDocker) archive(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")

	if r.Method == http.MethodPut {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.files[path] = f.bodies["PUT "+strings.TrimPrefix(r.URL.Path, "/"+apiVersion)]
		return
	}

	f.mu.Lock()
	content, ok := f.files[path]
	f.mu.Unlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"no such file"}`)
		return
	}

	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	writer.WriteHeader(&tar.Header{Name: "dir/", Typeflag: tar.TypeDir, Mode: 0o755})
	if !f.onlyDirectories {
		writer.WriteHeader(&tar.Header{
			Name:     filepath.Base(path),
			Mode:     0o644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		})
		io.WriteString(writer, content)
	}
	writer.Close()

	stat := base64.StdEncoding.EncodeToString([]byte(`{"name":"` + filepath.Base(path) + `"}`))
	w.Header().Set("X-Docker-Container-Path-Stat", stat)
	w.Header().Set("Content-Type", "application/x-tar")
	w.Write(archive.Bytes())
}

func (f *fakeDocker) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

func (f *fakeDocker) asked(want string) bool {
	for _, request := range f.seen() {
		if strings.Contains(request, want) {
			return true
		}
	}
	return false
}

func (f *fakeDocker) count(want string) int {
	count := 0
	for _, request := range f.seen() {
		if request == want {
			count++
		}
	}
	return count
}

func (f *fakeDocker) body(request string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[request]
}

func (f *fakeDocker) query(request string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.queries[request]
}

func newTestDocker(t *testing.T, fake *fakeDocker) *Docker {
	t.Helper()
	t.Setenv("DOCKER_HOST", "unix://"+fake.socket)

	docker, err := New(context.Background())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return docker
}

// imageConfig is a dev container that names an image, on a checkout called
// checkout, so its workspace is /workspaces/checkout.
func imageConfig() devcontainer.Config {
	return devcontainer.Config{Image: testImage, Root: "/host/checkout"}
}

// started is the container of one task on the fake daemon.
func started(t *testing.T, fake *fakeDocker) *Container {
	t.Helper()
	container, err := newTestDocker(t, fake).Start(context.Background(), "task-1", imageConfig())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return container
}
