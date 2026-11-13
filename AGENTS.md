# Agent instructions

1. MODULE.bazel: to update `use_repo` rules and similar (derived from go.mod), ALWAYS run `bazel mod tidy`. Never edit `use_repo` lists by hand.

2. BUILD.bazel files: when code imports change, files are added, or dependencies are added, ALWAYS run `bazel run //tools:gazelle`. Do not edit BUILD.bazel files by hand.

3. Go: never use the Go toolchain directly, only bazel. If Go itself is needed (for example a `go get`), use the binary at `./bazel/bin/go` ($PATH should already be correct); it calls the Go binary for rules_go.

4. TMPDIR: when running bazel commands, prefix with `TMPDIR=/tmp` if TMPDIR points to a directory that does not exist (bazel sandboxing fails otherwise).

5. ASCII only: use only ASCII symbols in all changes done by agents. Never write em dashes or any other non ASCII characters in code, comments, docs, commit messages or PR descriptions.

6. Linter: after modifying any code, ALWAYS run `bazel run //tools:golangci-lint -- run --fix`. The `--fix` flag is optional but recommended: it auto-corrects simple linting errors.

