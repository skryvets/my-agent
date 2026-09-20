# my-agent

A small agent that talks to a model through the [OpenRouter](https://openrouter.ai) chat completions API, runs the tools the model asks for, and streams the answer as it arrives.

One message from a phone - "fix the lint warning in my-agent" - clones the
repository, changes it in its own dev container, runs the tests, pushes a
branch and answers with a link to the pull request.

The agentic work runs on [eino](https://github.com/cloudwego/eino), the agent
development kit of CloudWeGo. eino parses the server-sent events, joins the
streamed `tool_calls` fragments, runs a round of tools, tries a failed model
call again and asks the model until it answers in words. The agent supplies the
tools, the history and the stream.

The rest is the standard library, and it shows four things that are easy to get
right only once:

- reading `devcontainer.json`, which is JSON with comments, with nothing but `encoding/json`
- driving the Docker Engine API over its unix socket with nothing but `net/http`, to build or pull that dev container and run the tools inside it
- keeping the GitHub token out of the container, so the model never sees it
- stopping a run from the phone half way through, without losing the chat

It runs as a Telegram bot, or in the terminal with `-cli`.

```mermaid
flowchart LR
    main["main.go<br/>flag parsing, wiring"]

    subgraph comm ["internal/communication"]
        tg["telegram<br/>bot, session, client"]
        term["terminal<br/>stdin, stdout"]
    end

    subgraph core ["internal/agent"]
        chat["chat.go<br/>Client.Chat"]
        eino["agent.go<br/>eino Runner<br/>failure.go<br/>tools, retries"]
    end

    subgraph kit ["internal/tools"]
        sh["shell, read_file<br/>write_file"]
    end

    subgraph job ["internal/task"]
        runner["Runner<br/>clone, set up, work, push, open"]
    end

    subgraph spec ["internal/devcontainer"]
        dc["Load<br/>devcontainer.json"]
    end

    subgraph box ["internal/sandbox"]
        pool["Pool<br/>one container per task"]
    end

    api(["OpenRouter<br/>chat completions"])
    dock(["Docker Engine<br/>unix socket"])

    main -->|default| tg
    main -->|"-cli"| term
    main -->|eino tools| eino
    tg -->|Agent interface| chat
    term -->|Agent interface| chat
    chat -->|history| eino
    eino -->|"POST, stream: true"| api
    api -->|server-sent events| eino
    eino -->|events| chat
    eino -->|tool calls| sh
    sh -->|Workspace| pool
    sh -->|results| eino
    tg -->|"/task, /stop"| runner
    runner -->|reads| dc
    runner -->|bind the checkout| pool
    runner --> chat
    pool -->|"build, pull, exec"| dock
    runner --> gh(["GitHub<br/>git, REST"])
```

A connector depends on the agent, never the other way round.

## Requirements

- Go 1.26 or newer
- An OpenRouter API key
- A Telegram bot token, for the default mode
- A Docker daemon and a GitHub token, for `/task`

## Usage

```sh
export OPENROUTER_API_KEY=sk-or-...
go run . -cli
```

Optionally set `MY_AGENT_MODEL` to override the model, for example:

```sh
export MY_AGENT_MODEL=openai/gpt-4o
go run . -cli
```

Type a message at the `you>` prompt and press enter. The whole conversation is sent back on every turn. Ctrl-C or Ctrl-D exits.

The answer streams in as it arrives. A failed turn prints the error and drops the unanswered message, leaving the session alive.

A chat, in the terminal or in Telegram, offers the model no tools: it answers
in words. Work on a repository goes through `/task`, in the dev container of
that repository.

Reasoning is off: every request sends `"reasoning": {"enabled": false}`, in the `ExtraFields` of the chat model in `internal/agent/agent.go`. The model returns an answer and no thinking. That one field switches reasoning back on, and eino joins the reasoning chunks and replays them on the next turn.

## Tools

In a task the model does not only answer, it acts. Three tools ship today:

| Tool | What it does |
| --- | --- |
| `shell` | runs a command line with `sh -c` and returns the combined output |
| `read_file` | returns the text of a file |
| `write_file` | replaces the content of a file and creates the parent directories |

Each tool is an eino `tool.InvokableTool`. `utils.InferTool` reads the
arguments schema from a Go struct with `jsonschema` tags, so no schema is
written by hand.

One turn can take several rounds. eino sends the tool schemas with the request;
if the model answers with `tool_calls` instead of words, eino runs the calls,
appends one `tool` turn for each of them, and asks the model again. It stops on
a plain answer, or after ten rounds. Independent calls of one round run at the
same time.

A tool that fails reports the failure to the model as text, so a command that
exits non-zero is an answer the model can read and work around, not a broken
turn. `agent.New` wraps every tool for that. A name the model invented gets the
same treatment. Output is cut to the last 8000 bytes, because the whole result
goes back into the history on every later turn.

## Retries

One model call that fails is tried again, up to three times, with the
exponential backoff and the jitter of eino: 100 ms, then double each time, to a
10 s ceiling.

Two failures get no second attempt. A call the caller stopped gets none,
because `/stop` must end a run at once. A request the server refused on its own
terms gets none either - a wrong key, a model that does not exist, a malformed
request - because it fails the same way every time. Too many requests, a server
failure and a broken connection are all tried again.

After the last attempt the failure reaches the chat, the turn is dropped with
`DropLast` and the conversation stays alive.

A tool never touches the host. It works through a `Workspace`, which is the dev
container of the task the call belongs to. No tool call waits for a person: the
container is the boundary.

## The dev container

A task works in the environment the repository describes for itself, with the
[dev container standard](https://containers.dev). The agent looks for the file
in the order the specification gives:

1. `.devcontainer/devcontainer.json`
2. `.devcontainer.json`
3. `.devcontainer/<folder>/devcontainer.json`

A repository without one cannot run a task, and the chat is told so. A root
`Dockerfile` is for production, and the agent never reads it.

What the agent does with the file:

| Property | What happens |
| --- | --- |
| `image` | pulled, unless the daemon already has it |
| `build.dockerfile`, `build.context`, `build.args`, `build.target` | built through `POST /build`, with the context sent as a tar |
| `workspaceFolder` | where the checkout is mounted, `/workspaces/<repository>` by default |
| `containerEnv`, `containerUser` | set on the container |
| `remoteEnv`, `remoteUser` | set on every command the tools run |
| `onCreateCommand`, `updateContentCommand`, `postCreateCommand`, `postStartCommand` | run in that order before the model starts. One that fails stops the task |

`dockerComposeFile` stops the task. `features`, `initializeCommand`,
`workspaceMount`, `mounts` and `runArgs` are skipped, and the chat is told
which ones. `${localEnv:NAME}` is replaced with its default only, never with
the value on the host, because the host holds `GITHUB_TOKEN`.

The container of a task:

| When | What happens |
| --- | --- |
| the task sets up | the container starts with the checkout mounted and the default Docker network |
| the task ends, fails or is stopped | the container is removed |
| 30 minutes without a command | a reaper removes it |
| the agent stops | every container it started is removed |
| the agent starts | the containers of an earlier run are removed |

`internal/sandbox` speaks the Docker Engine API itself, over the unix socket,
with `net/http` and its own `DialContext`, rather than with the Docker SDK. The
calls it makes: pull or build an image, create and start a
container, create and start an exec, and put or get a tar through the archive
endpoint, because the API has no endpoint that takes a plain file.

This repository describes itself in `.devcontainer/devcontainer.json`, with
`golang:1.26`.

## A task, end to end

`/task owner/name what to change` is the whole point of the project. From a
phone:

```
/task skryvets/my-agent fix the lint warning in internal/agent
```

The bot answers as it goes, because a run takes minutes:

```
Cloning skryvets/my-agent
Starting the dev container
Working on it
I removed the unused import in chat.go and go vet is clean.
Pushing my-agent/20260912-134544-1
Pull request open: https://github.com/skryvets/my-agent/pull/8
```

What happens, and where:

| Step | Where it runs |
| --- | --- |
| clone the repository | the agent process, with the token |
| read `devcontainer.json` | the agent process |
| build or pull the image, lend it the checkout | Docker, through a bind mount |
| run the setup commands | inside the container |
| read, change and test the code | the model, inside the container |
| name the change | the model, with no tools |
| commit and push the branch | the agent process, with the token |
| open the pull request | the agent process, GitHub REST |

**git and the GitHub API never run inside the container.** The model therefore
never sees the token: it works on a checkout the agent lends it through a bind
mount, and the two steps that publish the change are made by code rather than
by the model. The container has the default network, so the setup and the
tests can download what they need.

`GITHUB_TOKEN` needs write access to the repository. Without it `/task` is off,
and the bot needs no Docker.

`/stop` ends a run where it is. The model call or the command under way is
cancelled, the container is removed, the run is recorded as `stopped`, and the
messages that wait in that chat are dropped. A branch that was already pushed
stays on GitHub.

Every run is written to `-state` (`state` by default) at each step, so a
restart knows what was under way. A run it caught in the middle is marked and
reported to its chat, with the branch it reached:

```
A restart stopped the task on skryvets/my-agent (fix the lint warning).
It reached the state "working" on the branch my-agent/20260912-134544-1.
Send it again to start over.
```

## Telegram bot

Talk to [@BotFather](https://t.me/BotFather), send `/newbot`, and keep the token it gives you.

```sh
export OPENROUTER_API_KEY=sk-or-...
export TELEGRAM_BOT_TOKEN=123456:ABC-DEF...
export TELEGRAM_ALLOWED_USERS=11111111,22222222
export GITHUB_TOKEN=github_pat_...
go run .
```

The bot long-polls `getUpdates` for `message` updates, keeps one conversation per chat, and answers each message with the model. Commands:

- `/start`, `/help` - what the bot does
- `/task owner/name what to change` - change a repository in its dev container and open a pull request
- `/stop` - stop the answer or the task under way in that chat, and drop the messages that wait
- `/reset` - forget the conversation in that chat

Details worth knowing:

- `TELEGRAM_ALLOWED_USERS` is a comma-separated list of numeric Telegram user ids. Anyone can find a bot by its username, so without an allowlist strangers spend your OpenRouter credits. Ask [@userinfobot](https://t.me/userinfobot) for your id
- each chat is served by its own goroutine, so a slow answer in one chat does not block another. Messages inside one chat are answered in order, and a sender who runs ahead of the queue is told to wait
- `/stop` is read as it arrives, not in the queue of the chat, because the queue waits behind the very message it has to stop
- history is capped at the last 10 turns per chat and lives in memory only, so a restart clears it
- answers longer than Telegram's 4096 character limit are split on the last blank line, newline or space that fits
- a `429` from Telegram is retried after the `retry_after` it returns, other poll failures back off up to a minute
- Ctrl-C or `SIGTERM` stops the bot

## Deploying

The bot has no HTTP server, so it needs no port. `/task` needs a Docker daemon
the agent can reach at `/var/run/docker.sock`, so the bot runs on a machine
with Docker: a VM or a server, not a platform that gives a service no socket.

On an Ubuntu server with Go, git and Docker Engine, run the script from the
checkout:

```sh
git pull
./deploy/deploy.sh
```

The first run writes `/etc/my-agent/env` with empty variables and stops. Fill
in the variables above, then run the script again. Each run then:

1. builds the binary, so a build that fails leaves the running bot alone
2. creates the system user `my-agent` once
3. stops the `my-agent` service, which ends the running tasks and removes their containers
4. installs the binary to `/usr/local/bin/my-agent` and `deploy/my-agent.service` to `/etc/systemd/system/`
5. enables and starts the service, and prints the log if it does not stay up

The service runs as `my-agent` in the `docker` group, restarts when it exits,
and keeps its runs in `/var/lib/my-agent/state`. Follow it with
`journalctl -u my-agent -f`. Only one instance may poll `getUpdates` at a time,
so run the service on one server only. Do not add `PrivateTmp=` to the unit,
because the checkouts are in `/tmp` and the daemon must see them there.

The daemon resolves the bind mount of a checkout on its own host. When the
agent itself runs in a container with the socket mounted, set `TMPDIR` to a
directory that is mounted at the same path inside and outside it.

## Configuration

The model is set by the `MY_AGENT_MODEL` environment variable. The agent reads it at startup; if unset the slug is the empty string and OpenRouter uses its own default.

## How a turn works

```mermaid
sequenceDiagram
    autonumber
    actor user as User
    participant conn as Connector
    participant out as stream writer
    participant agent as agent.Client
    participant eino as eino Runner
    participant kit as internal/tools
    participant api as OpenRouter

    user->>conn: message
    conn->>conn: history.WithUser(text)
    conn->>agent: Chat(ctx, history, stream)
    agent->>eino: Run(ctx, history)

    loop until the model answers in words, up to ten rounds
        eino->>api: POST /chat/completions, tools
        api-->>eino: server-sent events
        note over eino,api: a failed call is tried again,<br/>up to three times, with backoff
        eino-->>agent: event, a message or a stream of chunks
        agent->>out: content as it arrives
        alt the model asked for tools
            eino->>kit: run the calls
            kit-->>eino: one tool turn for each call
            agent->>out: the name of each tool
        end
    end

    eino-->>agent: the answer in words
    agent-->>conn: Message{Content, Steps}
    conn->>conn: history.WithAssistant(msg)
    conn-->>user: answer
```

In a chat the client offers no tools, so the loop ends on the first round. In a
task the client offers the three tools, and the task runner is the caller.

The `stream` writer is where the reply appears while it is still arriving. The
terminal passes `os.Stdout`, so the answer types itself out. Telegram cannot
edit a message per token, so it passes `io.Discard` and sends the finished
answer in one go.

`Steps` carries the tool calls and the tool results of the round.
`WithAssistant` puts them into the history in front of the answer, so the next
turn replays what the agent did and the connector never has to know the
`tool_calls` wire format.

A turn the model failed to answer is dropped with `DropLast`, so a broken turn
never poisons the history. Reasoning is off, in the `ExtraFields` of the chat
model; eino joins the reasoning chunks itself when it is switched back on.

## Layout

```
main.go                              flag parsing and wiring
internal/agent/                      the model: the eino agent over OpenRouter, and the history
internal/tools/                      what the agent can do in a dev container: shell, files
internal/devcontainer/               the environment a repository describes for itself
internal/conversation/               which conversation a call belongs to
internal/task/                       a job end to end: clone, set up, work, push, open a pull request
internal/sandbox/                    a Docker container for each task
internal/communication/telegram/     Telegram bot connector, the default
internal/communication/terminal/     stdin and stdout connector, with -cli
```

A connector depends on the agent, never the other way round. Each one takes an
`Agent` interface - `Model() string` and `Chat(ctx, history, stream)` - so a new
connector is a new folder under `internal/communication` and a branch in
`main.go`. `CLAUDE.md` has the file-by-file breakdown.
