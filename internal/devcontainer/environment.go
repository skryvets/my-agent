package devcontainer

import (
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Workspace is where the checkout is mounted in the container. The default is
// the one the reference tool uses.
func (c Config) Workspace() string {
	if c.WorkspaceFolder == "" {
		return path.Join("/workspaces", filepath.Base(c.Root))
	}
	return c.expandWith(c.WorkspaceFolder, nil, "")
}

// ContainerEnvironment is containerEnv in the form Docker takes.
func (c Config) ContainerEnvironment() []string {
	return c.pairs(c.ContainerEnv, nil)
}

// RemoteEnvironment is remoteEnv in the form Docker takes. It may read the
// environment the container already has, which is given in the same form.
func (c Config) RemoteEnvironment(container []string) []string {
	known := make(map[string]string, len(container))
	for _, pair := range container {
		name, value, _ := strings.Cut(pair, "=")
		known[name] = value
	}
	return c.pairs(c.RemoteEnv, known)
}

func (c Config) pairs(variables, container map[string]string) []string {
	if len(variables) == 0 {
		return nil
	}
	pairs := make([]string, 0, len(variables))
	for name, value := range variables {
		pairs = append(pairs, name+"="+c.expand(value, container))
	}
	slices.Sort(pairs)
	return pairs
}

func (c Config) expand(text string, container map[string]string) string {
	return c.expandWith(text, container, c.Workspace())
}

// expandWith replaces ${...} variables. An empty workspace leaves the
// workspace variables in place, because the workspace folder itself is being
// read.
func (c Config) expandWith(text string, container map[string]string, workspace string) string {
	var out strings.Builder
	for {
		start := strings.Index(text, "${")
		if start < 0 {
			break
		}
		end := strings.IndexByte(text[start:], '}')
		if end < 0 {
			break
		}
		out.WriteString(text[:start])
		name := text[start+2 : start+end]
		out.WriteString(c.variable(name, container, workspace))
		text = text[start+end+1:]
	}
	out.WriteString(text)
	return out.String()
}

func (c Config) variable(name string, container map[string]string, workspace string) string {
	kind, rest, _ := strings.Cut(name, ":")
	switch {
	case kind == "localWorkspaceFolder":
		return c.Root
	case kind == "localWorkspaceFolderBasename":
		return filepath.Base(c.Root)
	case kind == "containerWorkspaceFolder" && workspace != "":
		return workspace
	case kind == "containerWorkspaceFolderBasename" && workspace != "":
		return path.Base(workspace)
	case kind == "localEnv":
		// The host holds the GitHub token, so no value of the host crosses
		// into the container. Only the default written in the file does.
		_, fallback, _ := strings.Cut(rest, ":")
		return fallback
	case kind == "containerEnv":
		variable, fallback, _ := strings.Cut(rest, ":")
		if value, ok := container[variable]; ok {
			return value
		}
		return fallback
	}
	return "${" + name + "}"
}
