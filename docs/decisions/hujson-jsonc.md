# Decisions - the JSONC reader moves to tailscale/hujson

## Why a library at all
**Q:** Why does `internal/devcontainer` take a dependency now?
**A:** To cut the most fragile handrolled parsing in the repository. The tokenizer in `jsonc.go` had to track string literals so a `//` or `*/` inside a value was not read as a comment. `hujson` implements the same JWCC format (comments and trailing commas), and its `Standardize` is a drop-in for the old `standardize` call. The file and its tests go away, 126 lines in all.
**Source:** user (2026-09-21), from the library recommendations review

## Which library
**Q:** Which JSONC library?
**A:** [github.com/tailscale/hujson](https://github.com/tailscale/hujson), pinned at `v0.0.0-20260727124030-b80ff77dac4f`, the latest revision, since the module carries no tagged release. It is BSD-3-Clause and has no dependencies of its own. Its `go.mod` requires Go 1.26, the toolchain the module already uses.
**Source:** user (2026-09-21)

## What is deleted
**Q:** Which files go?
**A:** `internal/devcontainer/jsonc.go` and `jsonc_test.go`. `hujson.Standardize` replaces the `standardize` call in `devcontainer.go`; `encoding/json` still reads the result into `Config`.
**Source:** model (2026-09-21)

## Malformed input now errors
**Q:** The old tokenizer never returned an error. What changes?
**A:** `hujson.Standardize` reports invalid HuJSON, so a file with an unclosed comment, string or array reaches `Load` as an error carrying the file name, the same way invalid JSON already did. The old tokenizer returned partial output and left `json.Unmarshal` to fail downstream. The visible result is unchanged: the file is refused and the chat is told which file it was.
**Source:** assumed (2026-09-21)

## The rest of the project
**Q:** Does anything else move?
**A:** No. The Docker Engine API stays on `net/http` and git stays on `os/exec`, both as the recommendations review advises. `internal/devcontainer` keeps `encoding/json` for unmarshaling; only the comment and trailing-comma stripping moves to the library.
**Source:** model (2026-09-21)
