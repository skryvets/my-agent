package devcontainer

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFile(t *testing.T, root, name, content string) string {
	t.Helper()
	file := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestLoadReadsTheFileWithCommentsAndTrailingCommas(t *testing.T) {
	root := t.TempDir()
	file := writeFile(t, root, ".devcontainer/devcontainer.json", `{
		// the toolchain
		"image": "mcr.microsoft.com/devcontainers/go:1",
		"containerEnv": {"GOFLAGS": "-mod=mod",},
		"remoteUser": "vscode",
		"postCreateCommand": "go mod download",
	}`)

	config, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if config.Image != "mcr.microsoft.com/devcontainers/go:1" || config.RemoteUser != "vscode" {
		t.Errorf("config = %#v", config)
	}
	if config.ContainerEnv["GOFLAGS"] != "-mod=mod" {
		t.Errorf("containerEnv = %#v", config.ContainerEnv)
	}
	if config.Root != root || config.File != file {
		t.Errorf("root = %q, file = %q", config.Root, config.File)
	}
	if !reflect.DeepEqual(config.PostCreateCommand, Command{{"sh", "-c", "go mod download"}}) {
		t.Errorf("postCreateCommand = %#v", config.PostCreateCommand)
	}
}

func TestLoadLooksInTheOrderOfTheSpecification(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".devcontainer/python/devcontainer.json", `{"image": "nested"}`)
	config, err := Load(root)
	if err != nil || config.Image != "nested" {
		t.Fatalf("nested: %#v, %v", config, err)
	}

	writeFile(t, root, ".devcontainer.json", `{"image": "root"}`)
	if config, _ := Load(root); config.Image != "root" {
		t.Errorf("image = %q, want the file at the root before a nested one", config.Image)
	}

	writeFile(t, root, ".devcontainer/devcontainer.json", `{"image": "folder"}`)
	if config, _ := Load(root); config.Image != "folder" {
		t.Errorf("image = %q, want .devcontainer/devcontainer.json first", config.Image)
	}
}

func TestLoadReportsARepositoryWithNoFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".devcontainer", "devcontainer.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); !errors.Is(err, ErrMissing) {
		t.Errorf("err = %v, want ErrMissing", err)
	}
}

func TestLoadRefusesWhatItCannotRun(t *testing.T) {
	cases := map[string]string{
		"not json":      `{"image": `,
		"no image":      `{"name": "empty"}`,
		"compose":       `{"dockerComposeFile": "compose.yml", "service": "app"}`,
		"bad lifecycle": `{"image": "x", "postCreateCommand": 7}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, ".devcontainer.json", content)
			_, err := Load(root)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), ".devcontainer.json") {
				t.Errorf("err = %v, want the name of the file", err)
			}
		})
	}
}

func TestLoadReportsAFileItCannotRead(t *testing.T) {
	root := t.TempDir()
	file := writeFile(t, root, ".devcontainer.json", `{"image": "x"}`)
	if err := os.Chmod(file, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(file, 0o644) })
	if _, err := os.ReadFile(file); err == nil {
		t.Skip("the file is still readable, which happens as root")
	}
	if _, err := Load(root); err == nil {
		t.Error("expected an error")
	}
}

func TestSkippedNamesOnlyWhatIsSet(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".devcontainer.json", `{
		"image": "x",
		"features": {"ghcr.io/devcontainers/features/node:1": {}},
		"initializeCommand": "echo host",
		"mounts": [],
		"runArgs": ["--privileged"]
	}`)
	config, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := config.Skipped(); !reflect.DeepEqual(got, []string{"features", "initializeCommand", "runArgs"}) {
		t.Errorf("skipped = %#v", got)
	}
}

func TestBuildPathsAreRelativeToTheFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".devcontainer/devcontainer.json", `{
		"build": {"dockerfile": "Dockerfile", "context": "..", "args": {"VARIANT": "${localEnv:VARIANT:bookworm}"}}
	}`)
	t.Setenv("VARIANT", "from-the-host")

	config, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := config.BuildContext(); got != root {
		t.Errorf("context = %q, want %q", got, root)
	}
	if got := config.Dockerfile(); got != filepath.Join(root, ".devcontainer", "Dockerfile") {
		t.Errorf("dockerfile = %q", got)
	}
	if got := config.BuildArgs(); got["VARIANT"] != "bookworm" {
		t.Errorf("args = %#v, want the default and never the host value", got)
	}
	if (Config{}).BuildArgs() != nil {
		t.Error("a build with no args has args")
	}
}

func TestLoadKeepsCommentMarkersInsideStrings(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".devcontainer.json", `{
		"image": "x",
		"remoteEnv": {"URL": "https://example.com/a//b", "GLOB": "/* keep */"},
		"postCreateCommand": "echo \"// not a comment\""
	}`)

	config, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if config.RemoteEnv["URL"] != "https://example.com/a//b" || config.RemoteEnv["GLOB"] != "/* keep */" {
		t.Errorf("remoteEnv = %#v", config.RemoteEnv)
	}
	if !reflect.DeepEqual(config.PostCreateCommand, Command{{"sh", "-c", `echo "// not a comment"`}}) {
		t.Errorf("postCreateCommand = %#v", config.PostCreateCommand)
	}
}

func TestLoadRefusesAFileWithAnUnclosedComment(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".devcontainer.json", `{"image": "x"} /* open`)
	_, err := Load(root)
	if err == nil || !strings.Contains(err.Error(), ".devcontainer.json") {
		t.Errorf("err = %v, want the name of the file", err)
	}
}
