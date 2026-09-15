package devcontainer

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
)

// Command is one lifecycle property. The specification allows a string for a
// shell, an array for a program and its arguments, and an object of either.
type Command [][]string

func (c *Command) UnmarshalJSON(data []byte) error {
	var line string
	if json.Unmarshal(data, &line) == nil {
		if strings.TrimSpace(line) != "" {
			*c = Command{{"sh", "-c", line}}
		}
		return nil
	}
	var args []string
	if json.Unmarshal(data, &args) == nil {
		if len(args) > 0 {
			*c = Command{args}
		}
		return nil
	}
	var named map[string]json.RawMessage
	if err := json.Unmarshal(data, &named); err != nil {
		return errors.New("a lifecycle command is a string, an array or an object")
	}
	// The object form runs its entries at the same time in the reference
	// tool. One after the other, in a fixed order, gives the same end state
	// with a log that reads top to bottom.
	for _, name := range slices.Sorted(maps.Keys(named)) {
		var entry Command
		if err := entry.UnmarshalJSON(named[name]); err != nil {
			return err
		}
		*c = append(*c, entry...)
	}
	return nil
}

// Step is one command that finishes the setup of a new container.
type Step struct {
	Name string
	Args []string
}

// Lifecycle is every setup command, in the order the specification runs them.
func (c Config) Lifecycle() []Step {
	var steps []Step
	for _, stage := range []struct {
		name    string
		command Command
	}{
		{"onCreateCommand", c.OnCreateCommand},
		{"updateContentCommand", c.UpdateContentCommand},
		{"postCreateCommand", c.PostCreateCommand},
		{"postStartCommand", c.PostStartCommand},
	} {
		for _, args := range stage.command {
			steps = append(steps, Step{Name: stage.name, Args: args})
		}
	}
	return steps
}
