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

Avoid acronyms if possible. keep all error/log messages lowercase. Error handling
should always wrap errors in most cases. Avoid not wrapping returned errors only
in the few places where a wrap truly doesn't make sense.

You MUST NOT use repated strings/number constants throughout the code without
defining an (potentially exported) constant for them. We should always use
constants for values where it makes sense to do so.

Type assertions should always be commented. Look in the repository for examples.

Changes to `.proto` files should always contain comments for every type and every field.

Never return an interface when a concrete type that implements said interface can be returned.

## Metrics

Add metrics to each new module when it makes sense. Follow the existing
`metrics.go` files as examples. Keep the collectors in a `metrics` struct. In
`metrics.go`, use the collector fields directly. From other files, publish
values only through helper methods on the `metrics` struct. Do not add helper
methods that are only used in `metrics.go`. Do not call `Observe`, `Inc`, `Add`,
or other methods on the Prometheus collectors from other files. Do not add
separate `newMetrics` constructors. Keep the `Metrics` method in the
`metrics.go` file.

## Testing

Tests should be added only when necessary. Generally, when adding new code it's
good to provide test coverage, but only if we have meaningful examples to use.
Tests that serve the only purpose of bumping coverage numbers should be avoided.

Tests should be added by copying inputs from specifications such as RFCs,
existing open-source projects, etc, to make sure the input values are not bogous.
Then, outputs should be asserted based on the specification/expectation, not
based on the written code.

When writing code, always keep testing in mind. Most code should take interfaces
as inputs for 'external systems' such as filesystems, databases, etc. Each package
should declare its own *public* interfaces that its abstractions will consume.
Each package, should also type assert that the concrete implementation type
expected to be used implements the requested interface (i.e., a `DB` type should
be implemented by `*pgx.Conn`).

## Validating

Never run `bazel build` on more targets than necessary. Rely on the GitHub CI to
run these expensive checks. Same goes for `bazel test`. Only test packages you
directly touched or which you know will be directly affected.
