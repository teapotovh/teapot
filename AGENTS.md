## Characters

Only use ASCII characters for any output. Only ever write in plain English
ASD-STE100. You MUST NOT use any unicode characters in the code or comments or
any type of output committed to Git.

## Sub-Agents

When starting sub-agents, always pick the cheapest model. If free models (non
'OpenCode Go' ones) are available and not rate-limited, you MUST use those. For
simple correction and iteration tasks, always spawn sub agents with the smallest
model available on lowest effort.

## Bazel instructions

To update the `MODULE.bazel` on `go.mod` changes, you MUST use `bazel mod tidy`.
Never perform edits by hand.

To update `BUILD.bazel` files when changing imports, you MUST use
`bazel run //tools:gazelle`. Never perform edits by hand.

After performing any code changes, ensure that the linter is passing by running
`bazel run //tools:golangci-lint -- run --fix`. The `--fix` flag is optional but
recommended: it auto-corrects simple linting errors.

## Go

You must avoid running the `go` binary directly. If you absolutley have to, for
example to run `go get`, make sure the binary you're running is the one aliased
to `bazel/bin/go`, which runs the `rules_go`'s Go version.

## TMPDIR

You may find the `TMPDIR` points to a non-existing directory. Feel free to use `TMPDIR=/tmp` instead.
