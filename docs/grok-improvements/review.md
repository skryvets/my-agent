# Codebase review - complexity and unnecessary items

A read-only review of the tree as it stands. No code was changed. The goal was
to find anything that can be simplified or removed, and to say plainly where the
code already earns its place.

## Verdict

This is a lean codebase. About 2.9k production lines, one concern per file, and
almost no dead code. There is no leftover host mode, approval gate, or fetch
tool. The layering rules in `CLAUDE.md` hold. Most of the code earns its place,
so the honest answer is that there is little to cut. The real cuts are small.

## Worth doing

### 1. The idle reaper is weakly justified

Every `/task` does `Bind` then `defer release` to `Close`. Crash leftovers are
already cleared by `sweepOrphans` on startup, and `main.go:51` already
`Shutdown`s on exit. That leaves `internal/sandbox/reaper.go`, `lastUse`,
`DefaultIdle`, and `Options.Idle` serving a case that does not happen.

Dropping them is the biggest sandbox win. Keep orphan sweep and shutdown.

There is also a second teardown trigger: `reaper.go:21` calls `Shutdown` on
context cancel, and `main.go:51` defers it for the same signal. The map swap
makes the second call a no-op. If the reaper stays, one of the two can go.

### 2. Export surface is wider than any caller

Nothing outside the defining package uses `sandbox.Container`, `DefaultIdle`,
`DefaultSocket`, or `conversation.Default`. Unexport them.

### 3. `task.Runner` carries the GitHub token twice

`internal/task/task.go:58` has `Token string`, and `GitHub` at
`internal/task/task.go:49` already has a `Token`. `main.go:62` and `main.go:64`
pass the same value to both. `Runner.Token` is read in exactly one place,
`internal/task/work.go:25`, to build `Git`.

Drop the field, read `r.GitHub.Token`, and remove one wiring line in `main.go`.

### 4. `sandbox/workspace.go` is a pass-through file

Four identical lookup-and-delegate methods. Fold them into `pool.go`. Same for
`internal/tools/tools.go` if one less file is wanted: `truncate` can live next
to `Shell`.

## Optional, a tradeoff rather than cleanup

### 5. Commit subject naming

`internal/task/work.go:118` to `internal/task/work.go:164` adds `Runner.Plain`,
`subjectFor`, `subjectLine` and `subjectOf`, plus a second model round-trip, all
to name a commit. `subjectOf(instruction)` already exists as fallback. Dropping
this also simplifies the `main.go` wiring. Only worth it if the better subject
is not valued.

### 6. Dedicated file tools

The model can `cat` and write via `shell`. Removing `read_file` and `write_file`
would delete `internal/tools/file.go`, `internal/sandbox/archive.go`, and two
`Workspace` methods. Do not do this: quoting and truncation get worse.

### 7. Telegram discards the stream

Telegram and `/task` pass `io.Discard` into `Chat`, so streaming and tool
announcements only show in `-cli`. Not dead, just unused in the default
connector.

## Considered and rejected

- `internal/conversation` as its own 26-line package keeps `task` off `sandbox`
- two `agent.New` clients, because a chat offers the model no tools
- duplicate `Agent` interfaces in telegram, terminal and task: connectors
  declare what they need
- the custom Docker HTTP client and the JSONC parser: the stdlib budget is
  explicit
- the Telegram split of session, stop, task and text
- `answered()` filling missing tool results: an eino quirk, recorded in
  `docs/decisions/eino-agent-loop.md`
- test seams such as `Runner.WorkRoot`, `GitHost`, `GitHub.BaseURL` and
  `sandbox.Options` are only set by tests but keep `go test ./...` off the
  network
- `session.go`, `task.go` and `work.go` sit just over the 150-line guideline.
  Splitting them further would add files, not remove complexity

## Nits

- `IsRetryAble` is deprecated in favor of `ShouldRetry` (staticcheck SA1019)

## Suggested order

1. item 1, then items 2 and 3, as the concrete cleanups
2. item 4, a file merge with no behavior change
3. item 5 only if the extra model call is not valued
4. the rest at leisure
