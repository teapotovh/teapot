## Use comments sparingly

Follow the repository's coding style. Add comments on exported fields only. At
most 2 lines for a comment, only if necessary. Prefer one-liners.
Comments in the middle of an implementation are okay, but only if necessary to
aid understanding, use sporadically. Interfaces and methods inside them should
be well-documented. Implementations of said interfaces should just say "implements XX",
deferring to the interface documentation.

Write all comments and all text in simple English ASD-STE100.

## Code Style

Prefer simple, short variable names. All error variables (`errors.New`) MUST be
exported and commented. Keep errors short but descriptive. Use only ASD-STE100
for error messages.

Prefer simple struct abstractions. Follow the patterns already in the repository,
such as `NewXX` initializers, interfaces for mocking and abstraction. Reuse code
as much as possible, where it makes sense. If there's the need for a shared
utility which will be used by other modules, it's worth adding a small package
under `lib/`. Abstractions should be individually testable. Configuration should
be parsed in a `flags.go` file for each abstraction, and provided to the `NewXX`
call via a struct.

Avoid acronyms if possible. keep all error/log messages lowercase.

## Testing

Tests should be added only when necessary. Generally, when adding new code it's
good to provide test coverage, but only if we have meaningful examples to use.
Tests that serve the only purpose of bumping coverage numbers should be avoided.

Tests should be added by copying inputs from specifications such as RFCs,
existing open-source projects, etc, to make sure the input values are not bogous.
Then, outputs should be asserted based on the specification/expectation, not
based on the written code.

## Validating

Never run `bazel build` on more targets than necessary. Rely on the GitHub CI to
run these expensive checks. Same goes for `bazel test`. Only test packages you
directly touched or which you know will be directly affected.
