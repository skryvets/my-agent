# my-agent

A Docker workspace for developing [my-agent](https://github.com/skryvets/my-agent), a
terminal and Telegram chatbot over the OpenRouter chat completions API.

On first start the workspace installs the Go toolchain into `~/.local/go`, clones the
repository into `~/my-agent`, downloads modules and builds the package. The home
directory is a persistent Docker volume, so the toolchain and the checkout survive a
stop and start. Changing the **Go version** parameter reinstalls the toolchain on the
next start.

## Parameters

| Parameter | Purpose |
| --- | --- |
| `repo_url` | Repository cloned into `~/my-agent`. Immutable |
| `go_version` | Go toolchain to install. Must satisfy the `go` directive in `go.mod` |
| `model` | `MY_AGENT_MODEL`, the OpenRouter model slug the agent talks to |

## Secrets

Secrets are not template parameters - they would be stored in the build. The startup
script creates `~/.config/my-agent/env` with empty placeholders and sources it from
`~/.bashrc`. Fill it in once per workspace:

```sh
export OPENROUTER_API_KEY=sk-or-...
export TELEGRAM_BOT_TOKEN=...
export TELEGRAM_ALLOWED_USERS=...
```

`OPENROUTER_API_KEY` is required - `main.go` exits without it. The Telegram variables
are only needed when running with `-telegram`.

## Apps

- **code-server** opens the checkout in the browser
- **Terminal chat** runs `go run .`, the stdin and stdout connector
- **go test -race** runs `go test ./... -race`, the gate everything ships behind
