# runlike

`runlike <container>` prints the `docker run` command that recreates an
existing container. Public project: github.com/lavie/runlike. Version 2
is a Go rewrite maintained by AI agents; the maintainer (Assaf Lavie)
approves releases.

If `AGENTS.local.md` exists next to this file, read it too: it holds
the maintainer's private working rules and machine setup. It is not in
git and must never be committed.

## Rules

- One pinned Docker CLI version (now v29.9.3, `spec/v29.9.3/`). Its
  official `docker run` reference and real CLI behaviour are the only
  source of truth for what runlike prints. The pin moves forward on
  purpose; every option gets a row in `spec/<pin>/coverage.json`.
- runlike's own command line stays backward compatible with 1.x
  (`test/compat` enforces it). Output may change when that makes it
  correct.
- Every option is tested by a round trip against a real Docker daemon
  (`test/roundtrip`): create a container, recreate it from runlike's
  output, compare `docker inspect`.
- The Python 1.x package is frozen at 1.6.3 (branch `python-1.x`).
- Releases happen only on the maintainer's explicit word: he merges a
  pull request that adds `release-notes/vX.Y.Z.md` (RELEASING.md).

## Design rulings

- Design: the library turns `docker inspect` JSON into an argument
  list; the daemon OS is an input, shell quoting (POSIX default, new
  `--shell powershell|cmd`) is the last step. The CLI gets the JSON by
  running `docker container inspect`, as today, so contexts and
  `DOCKER_HOST` keep working; the image ships the docker CLI.
- Leave a value out only when it equals Docker v29.9.3's built-in
  default (log driver json-file, runtime runc, 64 MB shm, ipc private,
  ...) AND, in CLI mode, this daemon's own default (`docker info`).
  So a daemon-configured default (Docker Desktop's `local`) is
  printed, and an explicit json-file on such a daemon is printed too.
  In `--stdin` mode only the built-in default counts.
- Image comparison uses the image the copy will run: inspect
  `Config.Image` (the tag); if it resolves to a different ID than the
  container's image, warn on stderr; if the tag no longer resolves,
  use the container's image ID; if neither exists, use no image data
  and warn. Never fail because an image is gone.
- MAC addresses: print only pinned MACs. First network: API 1.51
  `Config.MacAddress` (moby's DesiredMacAddress) for any state; absent
  = not pinned, no warning. On Docker Desktop read it through
  `docker.raw.sock` when that socket serves the same daemon (`docker
  info` ID); the proxy hides it. Further networks: no API version
  reports the pinned MAC once started, so print none (a created
  container's endpoint MACs are the pinned ones). `--stdin`: API <=
  1.51 data is authoritative; newer data of a started container gets
  no MAC. No warning in either case: it would fire on nearly every
  multi-network container; README "Known limits" documents it
  (maintainer's decision, 2026-11-06).
- Round-trip tests use `docker create`, not `run`, where starting is
  not needed: options whose container cannot start on our machines
  (`--gpus`, `--blkio-weight`, ...) still round-trip.
- Option rulings (orchestrator, 2026-11-06): print `--cidfile` when
  set; print `--use-api-socket` back when its socket mount and
  `DOCKER_CONFIG` pair are present, instead of the expanded pair;
  print `--platform` only when the image platform differs from the
  daemon's native one; print `container:` targets (network, ipc, pid)
  by name in CLI mode, by ID otherwise; `--umask` is stored as a decimal number, print
  it in octal; MAC: see above.
- Fuzz the quoting from day one: users run `$(runlike x)` on
  container-controlled values.
- Release as Go module `github.com/lavie/runlike/v2`, version 2.0.0.
  The last Python release lives on branch `python-1.x`; `master`
  drops Python.
- Test daemons: Lima VMs with a real Linux kernel, not Docker Desktop.
  Coverage table statuses: reproduced, folded, client-only,
  saved-data-only (Windows-only or hardware the VMs lack).
- Out of scope for 2.0: docker-compose output, `podman inspect` input.
- Windows containers need an x64 Windows host: Apple Silicon VMs
  cannot run them.


## Code layout

- `runlike.go`: public API (`Args`, `Inspect`, `Daemon`, `Options`, `Command`); `defaults.go`: the pin's built-in defaults, the build
  pipeline, image comparison helpers. One file per option area:
  `networking.go`, `storage.go`, `resources.go`, `security.go`,
  `runtime.go`, `process.go`, `health.go`, `logging.go`, `metadata.go`.
  Output order is the `areas` slice in `runlike.go`.
- `quote.go`: `Format` for posix, powershell and cmd. Security
  sensitive: users `eval` the output. Fuzzed (`FuzzFormatPosix`,
  `FuzzFormatCmd`).
- `cmd/runlike`: the click-compatible CLI; it shells out to `docker`.
- `spec/`: pinned reference and coverage table. `dashboard/`: progress
  page generator. `test/`: compat, roundtrip, Lima test daemons.
- `docs/`: the GitHub Pages site (`index.html`, `assets/`: one CSS, one
  JS, the share image), served from master's `docs/` folder, next to
  `docs/options.md` and `docs/manual-qa.md`. The hero example is real
  runlike output; a custom domain later goes in `docs/CNAME`.

## Python 1.x (frozen)

Branch `python-1.x` at tag v1.6.3 holds the last Python release; its
README says it is frozen. PyPI and Docker Hub stop at 1.5.4: 1.6.x was
never uploaded (GitHub has release v1.6.3). test/compat installs 1.6.3
from the tag (`test/compat/install-py.sh`) as the reference.

## Build and tests

- `go vet ./... && go test ./...` (live suites skip without a daemon). New options need round-trip cases; values a Linux daemon cannot store need saved-data cases built from real inspect output.
- `make build|test|fuzz|lint|cross` (Makefile). `cross` writes static
binaries for linux/amd64, linux/arm64, darwin/arm64, windows/amd64,
windows/arm64 and SHA256SUMS to `dist/`; the version comes from `git describe` via
`-X main.version`. Live suites: `test/roundtrip/README.md`,
`test/compat/README.md` (Lima daemons, `test/lima/up.sh`). The
Dockerfile builds the binary into `docker:29.9.3-cli`, both pinned by
digest.

## Release

See RELEASING.md. No AI runs in GitHub Actions; agents have no write
access to lavie/runlike and open pull requests from a fork.

- `.github/workflows/ci.yml` only tests, with a read-only token and no
  secrets: pull requests (fork runs need the maintainer's approval),
  pushes to master, a nightly long fuzz, and release.yml's call. Matrix:
  ubuntu-24.04 and -arm with Docker Engine 29.9.3 (unit, old-daemon data,
  fuzz smoke, round trip, compat), windows-2022 with Docker 29.9.3 static
  binaries (`go test ./...` incl. Windows round trip and pwsh/cmd shells),
  windows-11-arm (build, unit, quoting), macos-15 (build, unit, CLI, POSIX
  fuzz).
- A release is a merged pull request that adds `release-notes/vX.Y.Z.md`
  (the GitHub release text). Before opening it, commit the notes and run
  `scripts/release-check.py release-notes/vX.Y.Z.md` (name, version greater
  than the latest tag, one pending release, notes name the version).
- `.github/workflows/release.yml` runs on every push to master: the same
  check on the pushed commit (`--commit`), the full CI on that commit, then
  in environment `release` (master only, secret `DOCKERHUB_TOKEN`): tag,
  binaries, GitHub release, image `assaflavie/runlike:X.Y.Z` and `:latest`
  for the newest non-prerelease. A version whose release failed before
  tagging stays pending and ships with the next push to master; delete
  its notes file to abandon it. A manual run reruns a pending version on
  the master head, or republishes an existing tag. Never publish to PyPI.

## Working here

- Known bugs and planned work are beads (`bd list`).
- Commit finished work on the current branch.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:46cd31e7 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` for the full workflow.

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

Use `bd` for all task tracking, not markdown TODO lists.
<!-- END BEADS INTEGRATION -->
