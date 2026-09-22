// Package devcontainer reads the dev container file of a repository, the
// standard way a project says what its development environment is:
// https://containers.dev/implementors/json_reference/
package devcontainer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tailscale/hujson"
)

// ErrMissing is a repository that says nothing about its environment.
var ErrMissing = errors.New("the repository has no .devcontainer/devcontainer.json or .devcontainer.json")

// Config is the part of devcontainer.json that a task can act on.
type Config struct {
	Image                string            `json:"image"`
	Build                Build             `json:"build"`
	WorkspaceFolder      string            `json:"workspaceFolder"`
	ContainerEnv         map[string]string `json:"containerEnv"`
	RemoteEnv            map[string]string `json:"remoteEnv"`
	ContainerUser        string            `json:"containerUser"`
	RemoteUser           string            `json:"remoteUser"`
	OnCreateCommand      Command           `json:"onCreateCommand"`
	UpdateContentCommand Command           `json:"updateContentCommand"`
	PostCreateCommand    Command           `json:"postCreateCommand"`
	PostStartCommand     Command           `json:"postStartCommand"`

	DockerComposeFile json.RawMessage `json:"dockerComposeFile"`
	Features          json.RawMessage `json:"features"`
	InitializeCommand json.RawMessage `json:"initializeCommand"`
	WorkspaceMount    json.RawMessage `json:"workspaceMount"`
	Mounts            json.RawMessage `json:"mounts"`
	RunArgs           json.RawMessage `json:"runArgs"`

	Root string `json:"-"`
	File string `json:"-"`
}

// Build is the Dockerfile the image comes from, when there is no image name.
type Build struct {
	Dockerfile string            `json:"dockerfile"`
	Context    string            `json:"context"`
	Args       map[string]string `json:"args"`
	Target     string            `json:"target"`
}

// Load reads the dev container file of the repository at root.
func Load(root string) (Config, error) {
	file, err := find(root)
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return Config{}, err
	}
	name, _ := filepath.Rel(root, file)

	data, err = hujson.Standardize(data)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %v", name, err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, fmt.Errorf("%s: %v", name, err)
	}
	config.Root, config.File = root, file

	if present(config.DockerComposeFile) {
		return Config{}, fmt.Errorf("%s: dockerComposeFile is not supported, name an image or a build.dockerfile", name)
	}
	if config.Image == "" && config.Build.Dockerfile == "" {
		return Config{}, fmt.Errorf("%s names no image and no build.dockerfile", name)
	}
	return config, nil
}

// find looks in the order the specification gives.
func find(root string) (string, error) {
	for _, name := range []string{".devcontainer/devcontainer.json", ".devcontainer.json"} {
		if file := filepath.Join(root, name); isFile(file) {
			return file, nil
		}
	}
	nested, _ := filepath.Glob(filepath.Join(root, ".devcontainer", "*", "devcontainer.json"))
	for _, file := range nested {
		if isFile(file) {
			return file, nil
		}
	}
	return "", ErrMissing
}

func isFile(name string) bool {
	info, err := os.Stat(name)
	return err == nil && info.Mode().IsRegular()
}

// Skipped names the properties the file sets that a task does not act on.
func (c Config) Skipped() []string {
	var skipped []string
	for _, property := range []struct {
		name  string
		value json.RawMessage
	}{
		{"features", c.Features},
		{"initializeCommand", c.InitializeCommand},
		{"workspaceMount", c.WorkspaceMount},
		{"mounts", c.Mounts},
		{"runArgs", c.RunArgs},
	} {
		if present(property.value) {
			skipped = append(skipped, property.name)
		}
	}
	return skipped
}

func present(value json.RawMessage) bool {
	switch strings.TrimSpace(string(value)) {
	case "", "null", "{}", "[]", `""`:
		return false
	}
	return true
}

// BuildContext is the host directory the image is built from.
func (c Config) BuildContext() string {
	return filepath.Join(filepath.Dir(c.File), c.Build.Context)
}

// Dockerfile is the host path of the Dockerfile.
func (c Config) Dockerfile() string {
	return filepath.Join(filepath.Dir(c.File), c.Build.Dockerfile)
}

// BuildArgs are the build arguments with their variables replaced. A value is
// a pointer because the Engine API tells an empty argument from an absent one.
func (c Config) BuildArgs() map[string]*string {
	if len(c.Build.Args) == 0 {
		return nil
	}
	args := make(map[string]*string, len(c.Build.Args))
	for name, value := range c.Build.Args {
		expanded := c.expand(value, nil)
		args[name] = &expanded
	}
	return args
}
