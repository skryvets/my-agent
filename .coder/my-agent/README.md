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
`~/.bashrc` and `~/.profile`. Fill it in once per workspace:

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

## GitHub access

The repository is private, so the template declares `data "coder_external_auth" "github"`
and workspace creation blocks until you have authenticated. OAuth authorization alone is
not enough - the Coder GitHub App must also be *installed* on the account, at
<https://github.com/apps/coder/installations/new>, or the token comes back 403.

## Template metadata

The name, icon and description shown in the templates list are deployment
metadata, not part of `main.tf`, and `coder templates push` leaves them alone.
Set them once with:

```sh
coder templates edit my-agent \
  --display-name "My Agent" \
  --icon "/icon/go.svg" \
  --description "Go 1.26 chatbot over the OpenRouter API, terminal and Telegram"
```

