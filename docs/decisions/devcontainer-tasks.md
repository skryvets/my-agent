# Decisions - every task in the dev container of its repository

## Default connector
**Q:** Which connector starts when no flag is given?
**A:** The Telegram bot. A new `-cli` flag starts the terminal chat instead. The `-telegram` flag is removed.
**Source:** user (2026-09-14)

## Host mode
**Q:** Can a tool run on the host, outside Docker?
**A:** No. `tools.Host`, `-sandbox` and `-workdir` are removed. A tool runs only in a Docker container.
**Source:** user (2026-09-14)

## Where the image comes from
**Q:** Which file in the repository defines the image of a task?
**A:** The dev container file of the repository, found in the order the specification gives: `.devcontainer/devcontainer.json`, `.devcontainer.json`, then `.devcontainer/<folder>/devcontainer.json`. The root `Dockerfile` stays for production and the agent does not read it. The `-image` flag is removed. A repository with no dev container file cannot run a task, and the task stops with a message that says so.
**Source:** user (2026-09-14)

## A plain message
**Q:** What does a message that is not `/task` do?
**A:** The model answers with text only. There are no tools and no container. `-cli` works the same way. `/reset` only clears the conversation.
**Source:** user (2026-09-14)

## Network
**Q:** Does a task container get a network?
**A:** Yes, the default Docker network. The GitHub token still stays in the agent process and never enters the container.
**Source:** user (2026-09-14)

## fetch
**Q:** What happens to the `fetch` tool, which ran in the agent process?
**A:** It is deleted. The model uses `curl` or an equivalent in the container.
**Source:** user (2026-09-14)

## Approval
**Q:** Does a tool call wait for a person?
**A:** No. `internal/approval`, the Approve and Deny buttons, the `[y/N]` prompt, `-approval` and their tests are deleted.
**Source:** user (2026-09-14)

## /stop
**Q:** What does `/stop` stop?
**A:** The current answer or `/task` of that chat only. The queued messages of the chat are dropped, the task container is removed, and the run is recorded with the state `stopped`. The bot and the other chats continue. A branch that was already pushed stays on GitHub.
**Source:** user (2026-09-14)

## -state
**Q:** Is `-state` kept?
**A:** Yes. A restart still tells a chat about a task it stopped.
**Source:** user (2026-09-14)

## When Docker is needed
**Q:** Does the bot need Docker to start?
**A:** Only when `/task` is on, which is when `GITHUB_TOKEN` is set. Then a Docker daemon that does not answer stops the program. Without a token the bot answers plain messages and needs no Docker.
**Source:** assumed default

## Deployment
**Q:** Where does the bot run, and where does a developer work on it?
**A:** The bot runs on a machine with a Docker daemon. The repository carries no configuration for a hosting platform or for a remote development platform. A developer works in the dev container of the repository, `.devcontainer/devcontainer.json`.
**Source:** user (2026-09-14)

## Supported dev container properties
**Q:** How much of the dev container specification does the agent implement?
**A:** `image`, `build.dockerfile`, `build.context`, `build.args`, `build.target`, `workspaceFolder`, `containerEnv`, `remoteEnv`, `containerUser`, `remoteUser`, and the lifecycle commands `onCreateCommand`, `updateContentCommand`, `postCreateCommand` and `postStartCommand` in that order. The variables `${localWorkspaceFolder}`, `${localWorkspaceFolderBasename}`, `${containerWorkspaceFolder}`, `${containerWorkspaceFolderBasename}` and `${containerEnv:NAME}` are replaced. `${localEnv:NAME}` is replaced with its default only, never with the value on the host, because the host holds the GitHub token. `dockerComposeFile` stops the task. `features`, `initializeCommand`, `workspaceMount`, `mounts` and `runArgs` are skipped, and the chat is told which ones. A lifecycle command in the object form runs its entries one after the other, in the order of their names, instead of at the same time. A lifecycle command that fails stops the task.
**Source:** assumed (mid-build)

## Image build
**Q:** How is a `build.dockerfile` image built?
**A:** Through `POST /build` of the Engine API with the context as a tar and the classic builder, which is the default of the API. The context is sent whole, because `.dockerignore` is not read yet. The image is not tagged, and the container starts from the ID the build returns.
**Source:** assumed (mid-build)

## File ownership in the bind mount
**Q:** How can a non-root `remoteUser` write to a checkout the host cloned?
**A:** The agent makes the checkout writable for every user after the clone, and the container makes what it wrote writable for every user before it is removed, so the host can delete the checkout. The reference tool changes the UID of the user in the image instead (`updateRemoteUserUID`), which the agent does not do. The files are shared only with the container of the task, so the wider permissions are accepted.
**Source:** user (2026-09-14)

## features
**Q:** Does the agent install the `features` of a dev container?
**A:** Not for now. The agent skips `features` and tells the chat.
**Source:** user (2026-09-14)

## .dockerignore
**Q:** Must the build read `.dockerignore` before it sends the context?
**A:** Not for now. The whole context is sent, and a TODO in `internal/sandbox/build.go` marks the gap.
**Source:** user (2026-09-14)
