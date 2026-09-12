# my-agent

A terminal and Telegram chatbot over the OpenRouter chat completions API. No
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
internal/tools/                      what the agent can do
  workspace.go                       the Workspace interface and the host
  shell.go                           run a command
  file.go                            read a file, write a file
  fetch.go                           get a URL
internal/approval/                   the person in the loop
  approval.go                        the Broker between a tool and a person
  policy.go                          what runs without a question
  guard.go                           a tool that asks first
internal/conversation/               which conversation a call belongs to
internal/task/                       a job end to end, ending in a pull request
  task.go                            Runner, Start, the runs a restart caught
  work.go                            the stages of one run
  git.go                             git on the host, where the token is
  github.go                          the REST calls that open the request
  store.go                           the runs on disk
internal/sandbox/                    one container for each conversation
  docker.go                          the Engine API over the unix socket
  container.go                       exec in one container
  archive.go                         a file in and out as a tar
  pool.go                            find or start the container of a chat
  workspace.go                       the Workspace the tools see
  reaper.go                          throw away what went quiet
  image.go                           pull, start, clear an earlier run
internal/communication/terminal/     stdin and stdout connector
internal/communication/telegram/     Telegram bot connector
  telegram.go                        Run, options, configuration from the environment
  approval.go                        the Approve and Deny buttons
  task.go                            the /task command
  bot.go                             the getUpdates poll loop and its backoff
  session.go                         per-chat goroutine, commands, history
  client.go                          Bot API transport
  types.go                           Bot API wire types
  text.go                            splitting an answer to fit a message
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
- a tool never touches the host directly. It works through `tools.Workspace`,
  which is `tools.Host` in a terminal chat and `*sandbox.Pool` with `-sandbox`.
  A tool must not be able to tell the two apart
- `internal/sandbox` speaks the Docker Engine API over the unix socket with
  `net/http` and its own `DialContext`. That is what keeps `go.mod` empty of
  requirements, so do not reach for the Docker SDK. The API version is pinned
  in `docker.go`
- which conversation a call belongs to travels in the context, not in an
  argument. A connector names it once with `conversation.WithKey`, and the
  sandbox and the approval broker both read it there
- a tool the policy does not name waits for a person. Widen `approval.Default`
  rather than working around the guard, and keep anything that leaves the
  sandbox out of it. A refused call is reported to the model as text, so it
  tries something else instead of asking again
- a connector declares the small interface it needs (`Sandbox`, `Approvals`,
  `Tasks`) and main passes the real thing in through an `Option`. That is how
  the bot answers the questions of the tools without the tools knowing about
  Telegram
- git and the GitHub API run in the agent process, never in the container. The
  model must not see the token and must not need a network, so a task clones on
  the host and lends the checkout to the container with a bind. Do not move
  either one inside
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

## Testing

`go test ./... -race` must be green before anything ships. Every package keeps
its coverage above 90%; `main.go` is wiring and is not covered.

Test files mirror source files (`session.go` / `session_test.go`). The shared
Telegram fake, an `httptest` server that records calls, lives in `fake_test.go`
alongside `fakeAgent` and `newTestBot`. No test reaches the network.

## Deployment

Railway worker service, single replica - only one instance may poll
`getUpdates`. Railpack builds the root package as `out` and `railway.json`
starts it, so `main.go` stays at the repository root.
