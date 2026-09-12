package task

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// State is how far one run got.
type State string

const (
	Cloning     State = "cloning"
	Working     State = "working"
	Pushing     State = "pushing"
	Opened      State = "opened"
	NoChange    State = "no change"
	Failed      State = "failed"
	Interrupted State = "interrupted"
)

// Done reports whether a state is the last one of a run.
func (s State) Done() bool {
	return s == Opened || s == NoChange || s == Failed || s == Interrupted
}

// Run is one coding task, from the message that asked for it to the pull
// request it opened. It is written to disk at every step, so a restart knows
// what was under way.
type Run struct {
	ID          string    `json:"id"`
	Chat        string    `json:"chat"`
	Repo        string    `json:"repo"`
	Instruction string    `json:"instruction"`
	Branch      string    `json:"branch"`
	State       State     `json:"state"`
	Detail      string    `json:"detail"`
	Started     time.Time `json:"started"`
	Ended       time.Time `json:"ended,omitzero"`
}

// Store keeps the runs on disk. Railway restarts a worker whenever it
// redeploys, and a run that was under way must not simply vanish.
type Store struct {
	// Dir holds one file for each run. An empty Dir keeps nothing.
	Dir string
}

// Save writes one run, replacing what was there.
func (s Store) Save(run Run) error {
	if s.Dir == "" {
		return nil
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}

	content, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.Dir, run.ID+".json"), content, 0o644)
}

// Load reads every run, oldest first.
func (s Store) Load() ([]Run, error) {
	if s.Dir == "" {
		return nil, nil
	}
	names, err := filepath.Glob(filepath.Join(s.Dir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(names)

	var runs []Run
	for _, name := range names {
		content, err := os.ReadFile(name)
		if err != nil {
			return nil, err
		}
		var run Run
		if err := json.Unmarshal(content, &run); err != nil {
			// One unreadable file must not hide the rest.
			continue
		}
		runs = append(runs, run)
	}
	return runs, nil
}
