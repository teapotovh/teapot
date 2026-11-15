# goldap

This package encodes and decodes LDAP messages.

## Fuzzing

There are two fuzz targets in `fuzz_test.go`:

- `FuzzReadLDAPMessage`
- `FuzzReadFilter`

Run one of them for 30 seconds:

```bash
mkdir -p /tmp/fuzzcache

TMPDIR=/tmp bazel test //lib/ldapsrv/goldap:goldap_test \
  --test_arg=-test.run=^$ \
  --test_arg=-test.fuzz=FuzzReadLDAPMessage \
  --test_arg=-test.fuzztime=30s \
  --test_arg=-test.fuzzcachedir=/tmp/fuzzcache \
  --test_output=errors
```

Change `FuzzReadLDAPMessage` to `FuzzReadFilter` to run the filter decoder.

Bazel needs `-test.fuzzcachedir`. `go test` sets this automatically, but Bazel
calls the test binary directly.

A plain `bazel test` only runs the seed corpus. That is what CI does. Use the
command above when you want to fuzz for real. Add `--nocache_test_results` if
Bazel returns a cached result.

When a fuzzer finds a crash, commit the generated input under
`testdata/fuzz/<target>/`. The normal test run will replay it.

The DN parser has a fuzz target in `service/bottin/store`:

```bash
mkdir -p /tmp/fuzzcache

TMPDIR=/tmp bazel test //service/bottin/store:store_test \
  --test_arg=-test.run=^$ \
  --test_arg=-test.fuzz=FuzzParseDN \
  --test_arg=-test.fuzztime=30s \
  --test_arg=-test.fuzzcachedir=/tmp/fuzzcache \
  --test_output=errors
```
