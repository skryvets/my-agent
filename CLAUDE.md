# my-agent

A Telegram and terminal chatbot over the OpenRouter chat completions API. The
agentic work runs on [eino](https://github.com/cloudwego/eino): eino streams
the reply, reassembles the tool calls, runs them, retries a failed call and
asks the model again. The Telegram connector runs on
[go-telegram/bot](https://github.com/go-telegram/bot), which polls and speaks
the Bot API. The dev containers run on
[moby/moby/client](https://github.com/moby/moby/tree/master/client), the Docker
Engine client. The GitHub calls run on
[google/go-github](https://github.com/google/go-github). `go.mod` requires eino,
the eino OpenAI model, go-telegram/bot, moby/moby/client with its api module,
google/go-github, and tailscale/hujson, and nothing else. Everything below
`internal/devcontainer` stays on the standard library, except the JSONC reader,
which is hujson. `internal/task` stays on the standard library except
`github.go`, which calls GitHub through go-github.

## Layout

```
main.go                              flag parsing and wiring, nothing else
internal/agent/                      the model
  agent.go                           Client, New, the eino agent and its runner
  chat.go                            one turn, read from the eino event stream
  failure.go                         what to retry, what to tell the model
  history.go                         a conversation as eino schema.Message
internal/tools/                      what the agent can do in a dev container
  tools.go                           All, the list of tools, and the output cap
  workspace.go                       the Workspace interface
  shell.go                           run a command
  file.go                            read a file, write a file
internal/devcontainer/               the environment a repository describes
  devcontainer.go                    Config, Load, where the file is
  lifecycle.go                       the setup commands in their three forms
  environment.go                     the workspace, the environment, variables
internal/session/                    one conversation, the same in every connector
  session.go                         Session, Handle, the history and the commands
  task.go                            the /task command
internal/task/                       a job end to end, ending in a pull request
  task.go                            Runner, Start, the runs a restart caught
  work.go                            the stages of one run, the worker agent
  subject.go                         naming the change, the body of the request
  environment.go                     read, set up and release the dev container
  git.go                             git on the host, where the token is
  github.go                          the GitHub calls that open the request
  store.go                           the runs on disk
internal/sandbox/                    one dev container for each task
  docker.go                          Docker, the client, Sweep, reading a build stream
  image.go                           Start: pull an image, create and start a container
  build.go                           build an image from a Dockerfile
  container.go                       Container, exec in it, the Workspace the tools see
  archive.go                         a file in and out as a tar
internal/communication/telegram/     Telegram bot connector, the default
  telegram.go                        Run, the go-telegram bot, configuration from the environment
  chat.go                            per-chat goroutine and queue, one session for each chat
  stop.go                            the /stop command
  text.go                            splitting an answer to fit a message
internal/communication/terminal/     stdin and stdout connector, -cli
deploy/                              the systemd service and the script that installs it
```

## Rules

- `internal/communication/*` depends on `internal/agent`, never the reverse.
  Nothing under `internal/` imports `main`
- a connector reads messages and sends replies, nothing more. Everything a
  person can say - a chat turn, `/help`, `/history`, `/clear`, `/task` - is answered by
  `session.Session`, so the terminal and Telegram behave the same. Adding a
  connector is a new folder here that makes one `Session` for each person it
  talks to, plus a branch in `main.go` - do not touch `internal/agent`
- `session` declares the `Agent` interface (`Model() string`, `Chat(ctx,
  history, stream)`) and the `Tasks` interface it needs, and main passes the
  real thing in. A nil `Tasks` turns `/task` off
- conversation state is `agent.History`, a slice of eino `*schema.Message`,
  owned by the `Session`. Build turns with `WithUser`, `WithAssistant`,
  `DropLast` and `Trim` rather than assembling messages. `Chat` returns the
  answer as text: the tool calls of a turn stay inside eino, because a chat
  offers no tools and a task asks one question only
- a tool is a constructor in `internal/tools` that returns an eino
  `tool.BaseTool`, built with `utils.InferTool` from an arguments struct with
  `jsonschema` tags. Adding one is a new file there plus a name in
  `tools.All` - `internal/agent` knows no tool by name. A tool returns an ordinary error;
  `agent.New` wraps every tool so the failure reaches the model as text
- do not write a JSON Schema by hand. `utils.InferTool` reads it from the
  arguments struct. A `description` in a `jsonschema` tag must carry no comma,
  because a comma separates the options of the tag
- a tool never touches the host. It works through `tools.Workspace`, which is
  `*sandbox.Container`, the dev container of one task. A chat, in Telegram or
  with `-cli`, offers the model no tools. There is no host mode and no
  approval gate: the container is the boundary
- every run gets its own container, its own tools bound to that container,
  and its own agent, built by `Runner.Worker`. Nothing is shared between two
  runs, so nothing has to say which run a call belongs to. The run removes
  its container when it ends, and `Docker.Sweep` removes whatever is left at
  start and at exit
- the environment of a task comes from the `devcontainer.json` of the
  repository, never from the agent. Read it with `internal/devcontainer`, and
  never put a value of the host into it, because the host holds the token
- `internal/sandbox` speaks the Docker Engine API over the unix socket through
  `moby/moby/client`. Do not write a raw Engine API request next to it. The API
  version is pinned in `docker.go`
- in Telegram a message or a task runs on a context that `/stop` cancels.
  Replies go out on the context of the bot, so the chat can still be answered
  after a stop. A `Session` keeps quiet about a turn its context ended,
  because the stop was answered already
- the poll loop, its backoff and its `retry_after` belong to go-telegram/bot.
  The bot registers one default handler and runs it with
  `WithNotAsyncHandlers`, so the messages of a chat reach its queue in order.
  `dispatch` only queues, so the poll loop never waits for an answer
- git and the GitHub API run in the agent process, never in the container. The
  model must not see the token, so a task clones on the host and lends the
  checkout to the container with a bind. Do not move either one inside
- every error that reaches a chat passes through `Git.hide`, because the clone
  address carries the token
- a failed turn is reported and dropped, never fatal. Only `main.go` calls
  `log.Fatal`
- one file, one concern. If a file grows past roughly 150 lines it is usually
  carrying two
- reasoning is deliberately disabled, in the `ExtraFields` of the chat model in
  `agent.go`. eino joins the reasoning chunks itself, so switching reasoning
  back on is that one field. Do not switch it on without being asked
- one failed model call is tried again, up to `maxRetries` times, with the
  exponential backoff and the jitter of eino. `retryable` refuses a second
  attempt to a run the caller stopped, because `/stop` must end a run at once,
  and to a request the server refused with a 4xx other than 429
- an error that leaves `Chat` passes through `plain`, because eino wraps a
  failure in a node path and that text goes straight into a chat
- the README carries two mermaid diagrams, one of the package structure and one
  of a single turn. Changing either shape means updating them
- decisions about behavior are recorded in `docs/decisions/`

## Testing

`go test ./... -race` must be green before anything ships. Every package keeps
its coverage above 90%; `main.go` is wiring and is not covered.

Test files mirror source files (`session.go` / `session_test.go`). The shared
Telegram fake, an `httptest` server that records the multipart form of each
call, lives in `fake_test.go` alongside `fakeAgent` and `newTestBot`, which
points `bot.New` at the fake with `WithServerURL` and `WithSkipGetMe`. The Docker fake answers on a unix
socket in `internal/sandbox/fake_test.go`, and takes over the connection of an
exec start the way the daemon does. `internal/agent/fake_test.go` holds
`fakeModel`, a scripted `model.BaseChatModel` that `newClient` takes in place
of OpenRouter. `internal/session/fake_test.go` holds the `fakeAgent` and
`fakeTasks` a session is tested against; the connector tests only check that a
message reaches a session and a reply comes back. No test reaches the network.

## Deployment

An Ubuntu server with a Docker daemon, one instance only - only one process may
poll `getUpdates`. `deploy/deploy.sh` builds the binary, stops the `my-agent`
systemd service, installs the binary and `deploy/my-agent.service`, and starts
it again. The secrets are in `/etc/my-agent/env` on the server, never in the
repository. Do not give the unit a private `/tmp`: a checkout is bound into the
container by its path on the host. Without `GITHUB_TOKEN` the bot runs with
`/task` off.
