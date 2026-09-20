# Library placements for handrolled implementations

Reviewed on 2026-09-20. This document records recommendations only. No application code was changed.

The project has already moved two handrolled subsystems to libraries, the agent loop to eino and the Telegram transport to go-telegram/bot, each documented in `docs/decisions/`. That work set the rule: one library per concern, and the standard library below `internal/agent`. This document surveys what remains handrolled and where a library could take over, weighed against that rule.

## Clear wins

### 1. JSONC parser -> `tailscale/hujson`

[`jsonc.go`](../internal/devcontainer/jsonc.go) is a handrolled tokenizer that strips C-style comments and trailing commas from `devcontainer.json` before [`encoding/json`](../internal/devcontainer/devcontainer.go) reads it. The scanner has to track string literals so a `//` or `*/` inside a value is not mistaken for a comment. This is the most fragile handrolled parsing in the repository.

`github.com/tailscale/hujson` implements exactly this format (JSON with comments and trailing commas) and exposes [`hujson.Standardize`](https://pkg.go.dev/github.com/tailscale/hujson#Standardize), a drop-in for the `standardize` call:

```go
data, err := hujson.Standardize(data) // replaces standardize(data)
```

It is BSD-3-Clause, has no dependencies, and requires Go 1.26, the same toolchain the module already uses. Roughly 79 lines plus tests go away.

### 2. GitHub REST client -> `google/go-github`

[`github.go`](../internal/task/github.go) handrolls the REST call, the `X-GitHub-Api-Version` header, bearer auth, and error parsing for two endpoints: reading the default branch and opening a pull request.

`github.com/google/go-github/v89` covers both with typed methods and adds rate-limit handling and typed errors for free. It pins the same `2022-11-28` API version already hardcoded in the file. The cost is one transitive dependency (`go-querystring`). Worth it the moment a third endpoint appears; marginal at two.

## Deliberately kept

### Docker Engine API -> Docker SDK

The roughly 530 lines across [`docker.go`](../internal/sandbox/docker.go), [`container.go`](../internal/sandbox/container.go), [`image.go`](../internal/sandbox/image.go), [`archive.go`](../internal/sandbox/archive.go) and [`build.go`](../internal/sandbox/build.go) are the obvious candidate for the Docker SDK. This is already decided: `docs/decisions/eino-agent-loop.md` states the Engine API stays on `net/http` and its own `DialContext`. Listed here only so it stays on record.

### git via `os/exec` -> `go-git`

[`git.go`](../internal/task/git.go) shells out to `git`. `go-git` is a known source of subtle divergence from real git in auth, credential helpers, and shallow-clone edge cases, and this code deliberately runs git on the host where the token lives. Shelling out is the defensible choice, not a gap.

## Feature gap, not a replacement

### Terminal -> readline

[`terminal.go`](../internal/communication/terminal/terminal.go) reads with a plain `bufio.Scanner`, so `-cli` has no line editing, history, or tab completion. This is a missing feature, not handrolled code to delete.

If line editing is wanted, do not use `chzyer/readline`, which is unmaintained (last release 2019, 121 open issues). The maintained fork is `github.com/ergochat/readline`. Two things make it non-trivial: the model streams its answer to stdout while readline owns the terminal and redraws the prompt, and the current tests drive the session with non-TTY input, which readline does not accept. Both need a line-mode fallback, which is the `bufio.Scanner` path kept anyway. Rank this below the two clear wins.

## Marginal or standard-library

| Location | Recommendation | Reason |
| --- | --- | --- |
| [`expandWith`](../internal/devcontainer/environment.go) | Consider `os.Expand` with a mapper | The `${...}` scanner is handrolled, but the `containerEnv:NAME:default` syntax and leaving unknown variables in place stay in the mapper. Saves little |
| [`splitReference`](../internal/sandbox/image.go) | Leave as is | `distribution/reference` would cover it, but it folds into the Docker SDK decision above |
| [`splitMessage`](../internal/communication/telegram/text.go) | Leave as is | Thirty standard-library lines; no library is a better fit |
| [`store.go`](../internal/task/store.go) | Leave as is | One JSON file per run is simple and restart-friendly; an embedded database is more machinery than it needs |

## Suggested order

Adopt `hujson` first: it removes fragile parsing for the smallest dependency cost. Adopt `go-github` only when the GitHub surface grows. Leave the Docker, git, and terminal paths as they are unless a requirement changes.
