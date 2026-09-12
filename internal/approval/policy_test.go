package approval

import (
	"encoding/json"
	"testing"
)

func TestDefaultPolicyLetsReadingAndBuildingThrough(t *testing.T) {
	policy := Default()

	allowed := map[string]string{
		"a plain listing":  `{"command":"ls -la"}`,
		"a build":          `{"command":"go build ./..."}`,
		"a test run":       `{"command":"go test ./... -race"}`,
		"a pipe of two":    `{"command":"ls | head -3"}`,
		"a chain of three": `{"command":"go vet ./... && go test ./... && git status"}`,
		"a search":         `{"command":"grep -rn tool_calls internal"}`,
	}
	for name, args := range allowed {
		if !policy.Allows("shell", json.RawMessage(args)) {
			t.Errorf("%s was gated: %s", name, args)
		}
	}
}

func TestDefaultPolicyGatesWhatItDoesNotKnow(t *testing.T) {
	policy := Default()

	gated := map[string]string{
		"an unknown program":        `{"command":"curl https://example.com"}`,
		"an unknown one at the end": `{"command":"ls && curl https://example.com"}`,
		"a substitution":            `{"command":"echo $(curl https://example.com)"}`,
		"a backtick":                "{\"command\":\"echo `curl https://example.com`\"}",
		"a script":                  `{"command":"./deploy.sh"}`,
		"an environment prefix":     `{"command":"GOFLAGS=-mod=mod go build"}`,
		"an empty command":          `{"command":""}`,
		"bad arguments":             `{oops`,
	}
	for name, args := range gated {
		if policy.Allows("shell", json.RawMessage(args)) {
			t.Errorf("%s was allowed: %s", name, args)
		}
	}
}

func TestDefaultPolicyFreesTheWorkspaceToolsAndGatesTheNetwork(t *testing.T) {
	policy := Default()

	for _, tool := range []string{"read_file", "write_file"} {
		if !policy.Allows(tool, json.RawMessage(`{"path":"a.txt"}`)) {
			t.Errorf("%s was gated", tool)
		}
	}
	// fetch leaves the sandbox, so it always waits for a person.
	if policy.Allows("fetch", json.RawMessage(`{"url":"https://example.com"}`)) {
		t.Error("fetch was allowed without asking")
	}
	// A tool nobody named is gated, so a new one is safe by default.
	if policy.Allows("deploy", json.RawMessage(`{}`)) {
		t.Error("an unknown tool was allowed")
	}
}
