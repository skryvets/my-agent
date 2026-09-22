# Decisions - the JSONC reader moves to tailscale/hujson

## Why a library at all
**Q:** Why does `internal/devcontainer` take a dependency now?
**A:** To cut the most fragile handrolled parsing in the repository. The tokenizer in `jsonc.go` had to track string literals so a `//` or `*/` inside a value was not read as a comment. `hujson` implements the same JWCC format (comments and trailing commas), and its `Standardize` is a drop-in for the old `standardize` call. The file and its tests go away, 126 lines in all.

## Which library
**Q:** Which JSONC library?
**A:** [github.com/tailscale/hujson](https://github.com/tailscale/hujson), pinned at `v0.0.0-20260727124030-b80ff77dac4f`, the latest revision, since the module carries no tagged release. It is BSD-3-Clause and has no dependencies of its own. Its `go.mod` requires Go 1.26, the toolchain the module already uses.

## What is deleted
**Q:** Which files go?
**A:** `internal/devcontainer/jsonc.go` and `jsonc_test.go`. `hujson.Standardize` replaces the `standardize` call in `devcontainer.go`; `encoding/json` still reads the result into `Config`.

## A trailing unclosed comment is now refused
**Q:** The old tokenizer never returned an error. What changes?
**A:** `hujson.Standardize` reports invalid HuJSON. A file that ends in an unclosed block comment used to be accepted: the old tokenizer turned `{"a": 1} /* open` into `{"a": 1} `, which `json.Unmarshal` read and `Load` returned. It is refused now, with the file name in the error. An unclosed string or array was already refused, by `json.Unmarshal` after the tokenizer returned partial output. The stricter behavior is deliberate, and `TestLoadRefusesAFileWithAnUnclosedComment` pins it.

## The rest of the project
**Q:** Does anything else move?
**A:** No. git stays on `os/exec`. The Docker Engine API stayed on `net/http` until `docker-client.md`. `internal/devcontainer` keeps `encoding/json` for unmarshaling; only the comment and trailing-comma stripping moves to the library.
