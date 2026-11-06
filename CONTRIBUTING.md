# Contributing

runlike 2.0 is written and maintained mostly by AI coding agents, with a
human maintainer who approves every release. These rules apply to agents and
people alike.

## The spec pin

What runlike prints follows one pinned Docker CLI release, now **v29.9.3**:
its `docker run` reference ([spec/v29.9.3/container_run.md](spec/v29.9.3/container_run.md))
and the behaviour of that CLI and daemon. When the reference and real Docker
disagree, Docker wins; note it in the coverage table.

- [spec/v29.9.3/coverage.json](spec/v29.9.3/coverage.json) lists every
  `docker run` option with its status. Change it when an option's status or
  behaviour changes, then regenerate the docs:

  ```
  python3 tools/gen-options.py
  ```

  It rewrites [docs/options.md](docs/options.md) and the summary in
  [README.md](README.md) between the `OPTIONS` markers. Do not edit those by hand.
- The pin moves forward on purpose, in its own change: a new
  `spec/vX.Y.Z/` directory, the new CLI in `test/lima/up.sh`, and the gaps
  found filed as issues.

## Rules

- runlike's own command line stays backward compatible with 1.x. Its output
  may change when the change makes it correct.
- A value is left out only when it equals Docker's built-in default (and, in
  CLI mode, the daemon's own default) or the image's value.
- Never fail because an image is gone: warn on stderr instead.
- Every printed value goes through the shell quoting in `quote.go`. Users run
  the output with `eval`, on values the container controls.
- A new or changed option needs a round-trip test case.

## Tests

Run before every commit:

```
go vet ./...
go test ./...
```

This runs the unit tests and the saved-data tests; no Docker needed. Then,
for changes to option handling, the tests against real Docker:

```
test/lima/up.sh                       # Linux daemon in a Lima VM, plus the pinned CLI in test/bin
export PATH=$PWD/test/bin:$PATH DOCKER_CONTEXT=runlike-test-1
go test -count=1 ./test/roundtrip/    # round trip: container -> runlike -> copy, compare inspect
go build -o build/runlike ./cmd/runlike
RUNLIKE_BIN=$PWD/build/runlike go test -count=1 ./test/compat/   # runlike's command line, as in 1.x
```

- [test/roundtrip/README.md](test/roundtrip/README.md): how to add a case,
  step cases, saved-data cases, the fields the comparison ignores.
- [test/compat/README.md](test/compat/README.md): setup of the 1.x
  reference and how to test any runlike build.
- Quoting changes: also fuzz each dialect you touched, for example
  `go test -run '^$' -fuzz FuzzFormatPosix -fuzztime 2m .` (targets: `grep -n 'func Fuzz' *_test.go`).
- Test on a Lima VM (real Linux kernel), not Docker Desktop: Docker Desktop
  drops or rejects several resource options.
- Windows containers need an x64 Windows host with Docker; options it cannot
  run are tested with saved inspect data (see the round-trip README).

## Releases

A release is a pull request that adds `release-notes/vX.Y.Z.md`; the
maintainer's merge publishes it. See [RELEASING.md](RELEASING.md). Never
create tags by hand.
