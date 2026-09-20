# Codebase simplification evaluation

Reviewed on 2026-09-19. This document records recommendations only. No application code was changed during the evaluation

The codebase contains 2,878 production Go lines across 34 files, plus 3,847 test lines across 31 files. Package boundaries are clear. The strongest simplification opportunities are capabilities that current production paths do not use and overlapping container lifecycle management

## Priority issue: the GitHub token reaches the checkout

[`Git.address`](../../internal/task/git.go) embeds the GitHub token in the clone URL. Git persists that URL as `remote.origin.url` in `.git/config`. [`Pool.start`](../../internal/sandbox/image.go) mounts the entire checkout into the container, including `.git`, so the model's tools can read the token despite the intended boundary

The persistence was reproduced with an isolated local repository, a fake token, and a Git URL rewrite. No real credentials or remote service were used

Recommended direction: keep clone and push authentication outside the mounted checkout and store a credential-free origin URL. Error redaction alone does not prevent the file exposure

## 1. Remove tool-history replay between calls

[`Client.Chat`](../../internal/agent/chat.go) collects tool messages, reconstructs missing tool results with `answered()`, and returns them as `Message.Steps`. [`History.WithAssistant`](../../internal/agent/history.go) replays those steps on later calls

Current production callers do not need this behavior:

- [`Runner.think`](../../internal/task/work.go) starts a fresh history, runs one task, and uses only `answer.Content`
- The Telegram and terminal connectors preserve conversation history, but their agent has no tools
- Commit naming starts another fresh history and does not consume the task's steps

For the current single-turn task design, remove `Message.Steps`, `answered()`, and replay-specific history handling. Keep eino's tool history within a running task and preserve normal chat history trimming

This is the strongest simplification opportunity. It changes the internal agent contract and its tests and documentation. If tasks later support follow-up conversations, reconsider preserving their tool history at that point

## 2. Give container cleanup one clear owner

Container cleanup currently has several overlapping paths:

- [`Runner.work`](../../internal/task/work.go) defers release of each task's container
- [`Pool.sweepOrphans`](../../internal/sandbox/image.go) removes containers left by a previous process
- [`main`](../../main.go) defers pool shutdown
- [`Pool.reap`](../../internal/sandbox/reaper.go) removes idle containers and also invokes shutdown when the application context ends

Consider removing inactivity-based reaping and its timestamps, ticker, and idle configuration. Keep task-scoped release, startup orphan cleanup, and one coordinated shutdown path. If tasks need a runtime limit, express it as an explicit task deadline

The current idle tracking updates `lastUse` before an operation starts, rather than tracking whether it is still running. A lifecycle command running for 30 minutes can therefore be mistaken for an idle container. Cleanup should also coordinate with active task goroutines during shutdown

Removing the reaper changes the documented timeout behavior. This is a lifecycle design decision, not a mechanical deletion

## 3. Remove tool announcements nobody sees

[`announce`](../../internal/agent/chat.go) formats tool names and arguments into the output stream. Every production task passes `io.Discard`, while the terminal client has no tools

Remove `announce()` and its calls unless task tool activity will actually be exposed to a user. Preserve answer streaming for the terminal

## 4. Consider dropping the separate commit-title model call

[`subjectFor`](../../internal/task/work.go) makes a separate model request to name the change, validates its response with `subjectLine()`, and falls back to `subjectOf()`

Using `subjectOf()` directly would remove:

- The additional model request for each changed task
- `subjectFor()` and `subjectLine()`
- The task runner's `Plain` dependency and its fallback to the tool-enabled agent

The tradeoff is less polished commit and pull request titles, since they would be derived from the user's instruction rather than the model's summary. Keep the separate request if title quality justifies that behavior

## 5. Small, low-risk simplifications

| Location | Recommendation | Reason |
| --- | --- | --- |
| [`Store.Load`](../../internal/task/store.go) | Remove `sort.Strings(names)` and the unused import | `filepath.Glob` already returns sorted matches |
| [`jsonBody`](../../internal/sandbox/container.go) | Replace the generic helper with handling appropriate to its fixed exec-start payload | Its only production caller supplies a serializable payload, while the silent `{}` fallback and impossible-input test add unnecessary behavior |
| [`Runner.Interrupted`](../../internal/task/task.go) | Remove its unused context argument from the method and consuming interface | The implementation does not use it |

## Keep the useful boundaries

Retain the small consumer-defined interfaces, separate file tools, retry handling, and restart recovery. They serve current requirements without excessive abstraction

The three direct module dependencies are used by production code. The indirect dependencies should not be removed merely because this repository does not import them directly

## Validation and limits

- `go test ./... -race` passed
- `go vet ./...` passed
- `go build -o /dev/null .` completed with a sandbox warning about writing Go's module cache metadata
- The repository was unchanged after the evaluation
- The credential-persistence check used a temporary local repository and a fake token
- Live Docker, Telegram, OpenRouter, and GitHub integration behavior was not exercised

Recommended order: address credential persistence first, then remove unused replay and announcement behavior, apply the small deletions, and separately decide on container deadlines and commit-title generation
