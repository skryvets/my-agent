# my-agent

A Telegram and terminal chatbot over the OpenRouter chat completions API. No
third-party Go dependencies - `go.mod` has no `require` block, and it should
stay that way unless there is a reason the standard library cannot cover.

## Layout

```
main.go                              flag parsing and wiring, nothing else
internal/agent/                      the model
  agent.go                           Client, Model, one request
  loop.go                            the tool-calling loop
  tool.go                            the Tool interface and the registry
  toolcall.go                        reassembling streamed tool_calls
  stream.go                          server-sent events parsing
  reasoning.go                       reassembling streamed reasoning_details
  history.go                         a conversation in the API's wire format
internal/tools/                      what the agent can do in a dev container
  workspace.go                       the Workspace interface
  shell.go                           run a command
  file.go                            read a file, write a file
internal/devcontainer/               the environment a repository describes
  devcontainer.go                    Config, Load, where the file is
  jsonc.go                           JSON with comments into JSON
  lifecycle.go                       the setup commands in their three forms
  environment.go                     the workspace, the environment, variables
internal/conversation/               which conversation a call belongs to
internal/task/                       a job end to end, ending in a pull request
  task.go                            Runner, Start, the runs a restart caught
  work.go                            the stages of one run
  environment.go                     set up and release the dev container
  git.go                             git on the host, where the token is
  github.go                          the REST calls that open the request
  store.go                           the runs on disk
internal/sandbox/                    one dev container for each task
  docker.go                          the Engine API over the unix socket
  container.go                       exec in one container
  archive.go                         a file in and out as a tar
  pool.go                            bind and find the container of a task
  image.go                           pull an image, start a container, clear an earlier run
  build.go                           build an image from a Dockerfile
  workspace.go                       the Workspace the tools see
  reaper.go                          throw away what went quiet
internal/communication/telegram/     Telegram bot connector, the default
  telegram.go                        Run, options, configuration from the environment
  task.go                            the /task command
  stop.go                            the /stop command
  bot.go                             the getUpdates poll loop and its backoff
  session.go                         per-chat goroutine, commands, history
  client.go                          Bot API transport
  types.go                           Bot API wire types
  text.go                            splitting an answer to fit a message
internal/communication/terminal/     stdin and stdout connector, -cli
deploy/                              the systemd service and the script that installs it
```

## Rules

- `internal/communication/*` depends on `internal/agent`, never the reverse.
  Nothing under `internal/` imports `main`
- a connector takes the `Agent` interface (`Model() string`, `Chat(ctx, history,
  stream)`), declared in the connector that consumes it. Adding a connector is a
  new folder here plus a branch in `main.go` - do not touch `internal/agent`
- conversation state is `agent.History`. Build turns with `WithUser`,
  `WithAssistant`, `WithToolCalls`, `DropLast` and `Trim` rather than assembling
  `map[string]any` in a connector. A `Message` carries the tool turns that
  produced it in `Steps`, and `WithAssistant` puts them back, so a connector
  keeps the whole round without knowing the tool wire format
- a tool is a type in `internal/tools` that satisfies `agent.Tool`. Adding one
  is a new file there plus a line in `main.go` - `internal/agent` stays a loop
  that knows no tool by name. A tool reports a failure as text for the model,
  and returns an error only when the call itself was malformed
- a tool never touches the host. It works through `tools.Workspace`, which is
  `*sandbox.Pool`, the dev container of a task. A chat, in Telegram or with
  `-cli`, offers the model no tools. There is no host mode and no approval
  gate: the container is the boundary
- the environment of a task comes from the `devcontainer.json` of the
  repository, never from the agent. Read it with `internal/devcontainer`, and
  never put a value of the host into it, because the host holds the token
- `internal/sandbox` speaks the Docker Engine API over the unix socket with
  `net/http` and its own `DialContext`. That is what keeps `go.mod` empty of
  requirements, so do not reach for the Docker SDK. The API version is pinned
  in `docker.go`
- which conversation a call belongs to travels in the context, not in an
  argument. The task runner names it once with `conversation.WithKey`, and the
  sandbox reads it there
- a connector declares the small interface it needs (`Agent`, `Tasks`) and main
  passes the real thing in through an `Option`. That is how the bot starts a
  task without the task knowing about Telegram
- a message or a task runs on a context that `/stop` cancels. Replies go out on
  the context of the bot, so the chat can still be answered after a stop
- git and the GitHub API run in the agent process, never in the container. The
  model must not see the token, so a task clones on the host and lends the
  checkout to the container with a bind. Do not move either one inside
- every error that reaches a chat passes through `Git.hide`, because the clone
  address carries the token
- a failed turn is reported and dropped, never fatal. Only `main.go` calls
  `log.Fatal`
- one file, one concern. If a file grows past roughly 150 lines it is usually
  carrying two
- reasoning is deliberately disabled in `complete`. The reassembly code in
  `reasoning.go` is kept for when it is switched back on - do not delete it as
  dead code, and do not enable reasoning without being asked
- the README carries two mermaid diagrams, one of the package structure and one
  of a single turn. Changing either shape means updating them
- decisions about behavior are recorded in `docs/decisions/`

## Testing

`go test ./... -race` must be green before anything ships. Every package keeps
its coverage above 90%; `main.go` is wiring and is not covered.

Test files mirror source files (`session.go` / `session_test.go`). The shared
Telegram fake, an `httptest` server that records calls, lives in `fake_test.go`
alongside `fakeAgent` and `newTestBot`. The Docker fake answers on a unix
socket in `internal/sandbox/fake_test.go`. No test reaches the network.

## Deployment

An Ubuntu server with a Docker daemon, one instance only - only one process may
poll `getUpdates`. `deploy/deploy.sh` builds the binary, stops the `my-agent`
systemd service, installs the binary and `deploy/my-agent.service`, and starts
it again. The secrets are in `/etc/my-agent/env` on the server, never in the
repository. Do not give the unit a private `/tmp`: a checkout is bound into the
container by its path on the host. Without `GITHUB_TOKEN` the bot runs with
`/task` off.
