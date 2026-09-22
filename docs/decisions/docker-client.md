# Decisions - the sandbox moves to moby/moby/client

## Why a library at all
**Q:** `eino-agent-loop.md` kept the Engine API on `net/http`. Why does it move now?
**A:** The user reversed that decision to cut complexity and to rely on libraries instead of hand-rolled code. The request transport, the error parsing of the daemon, the wire types written as maps, the reference split for a pull and the query of a build go away. `internal/sandbox` went from 733 lines to 614 without its tests.
**Source:** user (2026-09-21)

## Which library
**Q:** Which Docker library?
**A:** [github.com/moby/moby/client](https://pkg.go.dev/github.com/moby/moby/client) v0.6.0, with the types of [github.com/moby/moby/api](https://pkg.go.dev/github.com/moby/moby/api) v1.56.0. It is the official Go client of the Docker Engine. `github.com/docker/docker/client` is its old path and stops at v28.5.2+incompatible. The module brings OpenTelemetry, containerd/errdefs and distribution/reference as indirect dependencies.
**Source:** model (2026-09-21)

## The API version
**Q:** Does the client negotiate the API version?
**A:** No. `client.WithAPIVersion` pins `v1.43`, as before, so the client sends no ping first and the requests keep the same paths.
**Source:** model (2026-09-21)

## What stays
**Q:** What does the package still own?
**A:** The pool, the reaper, the tar of a single file for the archive endpoint, the tar of the build context, and `readProgress`, which reads the build stream for the image id in `aux` and keeps the tail of the log for a failed build. A pull reads its stream with `ImagePullResponse.Wait`.
**Source:** model (2026-09-21)

## Exec
**Q:** How does an exec read its output now?
**A:** `ExecAttach` with a TTY. The client takes over the connection after `101 UPGRADED`, and the raw stream is read until the command ends. The fake answers an exec start the same way.
**Source:** model (2026-09-21)

## Build arguments
**Q:** Why does `devcontainer.Config.BuildArgs` return pointers?
**A:** `ImageBuildOptions.BuildArgs` is a `map[string]*string`, because the Engine API tells an empty argument from an absent one. `internal/devcontainer` stays on the standard library.
**Source:** model (2026-09-21)

## A write over a directory
**Q:** Does a write change?
**A:** `CopyToContainer` sends `noOverwriteDirNonDir=true` by default, so a file cannot replace a directory of the same name. The raw request did not send it. The stricter behavior is kept.
**Source:** model (2026-09-21)
