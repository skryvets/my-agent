package devcontainer

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCommandReadsEveryForm(t *testing.T) {
	cases := map[string]Command{
		`"make setup"`:                   {{"sh", "-c", "make setup"}},
		`["npm", "ci"]`:                  {{"npm", "ci"}},
		`{"b": "echo b", "a": ["true"]}`: {{"true"}, {"sh", "-c", "echo b"}},
		`"  "`:                           nil,
		`[]`:                             nil,
		`null`:                           nil,
	}
	for input, want := range cases {
		var got Command
		if err := json.Unmarshal([]byte(input), &got); err != nil {
			t.Errorf("%s: %v", input, err)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %#v, want %#v", input, got, want)
		}
	}
}

func TestCommandRefusesAnotherShape(t *testing.T) {
	for _, input := range []string{`7`, `{"a": 7}`} {
		var got Command
		if err := json.Unmarshal([]byte(input), &got); err == nil {
			t.Errorf("%s was accepted as %#v", input, got)
		}
	}
}

func TestLifecycleRunsInTheOrderOfTheSpecification(t *testing.T) {
	config := Config{
		PostStartCommand:     Command{{"start"}},
		PostCreateCommand:    Command{{"create"}},
		UpdateContentCommand: Command{{"update"}},
		OnCreateCommand:      Command{{"on"}, {"on", "again"}},
	}
	var names []string
	for _, step := range config.Lifecycle() {
		names = append(names, step.Name+":"+step.Args[len(step.Args)-1])
	}
	want := []string{
		"onCreateCommand:on", "onCreateCommand:again", "updateContentCommand:update",
		"postCreateCommand:create", "postStartCommand:start",
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("steps = %#v", names)
	}
}
