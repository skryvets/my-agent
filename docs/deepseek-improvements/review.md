# Codebase review - complexity and unnecessary items

A read-only review of the tree as it stands. No code was changed. The goal was
to find anything that can be simplified or removed, and to say plainly where the
code already earns its place.

## Verdict

This is a lean codebase. `go vet` and `gofmt` are clean, every package is above
90% coverage, there is exactly one TODO, and the layering rules in `CLAUDE.md`
hold. There is no dead package, no unused exported API, and no speculative
abstraction. Most of the code earns its place, so the honest answer is that
there is little to cut.

## Worth doing

### 1. `task.Runner` carries the GitHub token twice

`internal/task/task.go:57` has `Token string`, and `GitHub` at
`internal/task/task.go:49` already has a `Token`. `main.go:62` and `main.go:64`
pass the same value to both. `Runner.Token` is read in exactly one place,
`internal/task/work.go:25`, to build `Git`.

Drop the field, read `r.GitHub.Token`, and remove one wiring line in `main.go`.

### 2. `jsonBody` hides a marshal failure

`internal/sandbox/container.go:97` falls back to `"{}"` when `json.Marshal`
fails, which would start an exec with `Tty` unset and silently change behavior.
The value is a constant map that cannot fail. Either inline `json.Marshal` with
an error return, as `docker.call` already does, or drop the fallback. It is the
only silent-failure path found.

### 3. Two identical container teardown triggers

`internal/sandbox/reaper.go:21` calls `Shutdown` on context cancel, and
`main.go:51` defers `Shutdown(context.WithoutCancel(ctx))` for the same signal.
The map swap makes the second call a no-op and `main.go:49` acknowledges this.
One of the two can go, since the reaper already owns the lifecycle.

## Optional, a tradeoff rather than cleanup

### 4. Commit subject naming

`internal/task/work.go:118` to `internal/task/work.go:164` adds `Runner.Plain`,
`subjectFor`, `subjectLine` and `subjectOf`, plus a second model round-trip, all
to name a commit. It is a nice touch, but `subjectOf(instruction)` alone would
delete about 45 lines and one model call. Only worth it if the better subject is
not valued.

### 5. Terminal history never trims

`internal/communication/terminal/terminal.go:50` grows the history forever,
while Telegram caps it at `internal/communication/telegram/session.go:105`. A
long `-cli` session re-sends the whole conversation each turn.
`history.Trim(historyTurns*2)` would make the two connectors consistent.

### 6. Run files are never pruned

`internal/task/store.go:69` loads every `*.json` and nothing deletes finished
runs. At a few runs a day this is harmless, but it grows without bound and the
restart cost scales with it. Low priority.

### 7. `slug` is process-global state

`internal/agent/agent.go:33` reads the environment at init. `newClient` already
exists as the injectable seam, so this is minor, but it is the only hidden
dependency in an otherwise explicitly wired codebase.

## Considered and rejected

- `internal/agent/chat.go:54` `answered` looks redundant beside
  `UnknownToolsHandler`, but `docs/decisions/eino-agent-loop.md:35` records it
  as a real eino streaming gap, so it stays
- `internal/devcontainer/jsonc.go` hand-rolls JSONC in 79 lines, but the
  no-dependency rule is explicit
- the method repetition in `internal/sandbox/workspace.go` is four two-line
  delegates, and a generic helper would be less readable
- test seams such as `Runner.WorkRoot`, `GitHost`, `GitHub.BaseURL` and
  `sandbox.Options` are only set by tests but are the right way to keep
  `go test ./...` off the network
- `internal/task/work.go` (171 lines) and `internal/task/task.go` (159 lines)
  edge past the 150-line guideline. Splitting `subjectFor`, `subjectLine` and
  `subjectOf` into a `subject.go` would satisfy it and pairs naturally with
  item 4

## Suggested order

1. item 1, then item 2, as the two concrete cleanups
2. items 4 and 7, the only places that read as over-engineered
3. the rest at leisure
