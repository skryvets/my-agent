package devcontainer

import (
	"reflect"
	"testing"
)

func TestWorkspaceDefaultsToTheNameOfTheCheckout(t *testing.T) {
	config := Config{Root: "/tmp/run-1/my-agent"}
	if got := config.Workspace(); got != "/workspaces/my-agent" {
		t.Errorf("workspace = %q", got)
	}

	config.WorkspaceFolder = "/src/${localWorkspaceFolderBasename}/${containerWorkspaceFolder}"
	if got := config.Workspace(); got != "/src/my-agent/${containerWorkspaceFolder}" {
		t.Errorf("workspace = %q", got)
	}
}

func TestContainerEnvironmentReplacesItsVariables(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "gh-secret-token")
	config := Config{
		Root: "/tmp/run-1/my-agent",
		ContainerEnv: map[string]string{
			"PROJECT": "${containerWorkspaceFolder}",
			"NAME":    "${containerWorkspaceFolderBasename}",
			"HOST":    "${localWorkspaceFolder}",
			"TOKEN":   "${localEnv:GITHUB_TOKEN}",
			"MODE":    "${localEnv:MODE:dev}",
			"ODD":     "${unknown} and ${unclosed",
		},
	}
	want := []string{
		"HOST=/tmp/run-1/my-agent",
		"MODE=dev",
		"NAME=my-agent",
		"ODD=${unknown} and ${unclosed",
		"PROJECT=/workspaces/my-agent",
		"TOKEN=",
	}
	if got := config.ContainerEnvironment(); !reflect.DeepEqual(got, want) {
		t.Errorf("env = %#v", got)
	}
	if (Config{}).ContainerEnvironment() != nil {
		t.Error("no containerEnv gave an environment")
	}
}

func TestRemoteEnvironmentReadsTheContainer(t *testing.T) {
	config := Config{RemoteEnv: map[string]string{
		"PATH":  "${containerEnv:PATH}:/go/bin",
		"SHELL": "${containerEnv:SHELL:/bin/sh}",
	}}
	got := config.RemoteEnvironment([]string{"PATH=/usr/bin", "HOME=/root"})
	want := []string{"PATH=/usr/bin:/go/bin", "SHELL=/bin/sh"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("env = %#v", got)
	}
}
