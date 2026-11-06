# Round-trip tests

Each case creates container A, runs the runlike library on A's real
`docker container inspect` and image inspect JSON, recreates the container
from that argv twice (B: argv executed directly, S: the POSIX-quoted line
through `/bin/sh -c`) and compares the normalized inspect output of A with
each copy. The first case of each area also runs the built `cmd/runlike`
and checks that its output equals the library's.

## Run

    test/lima/up.sh
    export PATH=$PWD/test/bin:$PATH DOCKER_CONTEXT=runlike-test-1
    go test -count=1 ./test/roundtrip/                                    # all
    go test -count=1 -run 'TestRoundTrip/networking' ./test/roundtrip/    # one area
    go test -count=1 -run 'TestRoundTrip/process/env-utf8' -v ./test/roundtrip/  # one case, with runlike's line

Without `DOCKER_CONTEXT` or `DOCKER_HOST`, the live tests skip; the
saved-data tests still run. The harness calls `test/bin/docker` (the pinned
CLI) when present, else `docker` from `PATH`. Use one run per daemon at a
time: the sweeper at start and end removes every `rt-*` object. Run areas
in parallel on separate daemons (`test/lima/up.sh -n 3`, then
`DOCKER_CONTEXT=runlike-test-2`, ...). Raise `-parallel N` (default: the
number of CPUs) if the daemon has headroom.

## Windows

The harness reads the daemon OS (`docker version`) and runs only the
cases for it: `Case.OS` is `""` (Linux) or `"windows"`; Linux-only tests
skip on a Windows daemon. Windows cases are in `cases_win_test.go` (not
`cases_windows_test.go`: a `_windows` suffix makes Go build the file only
on Windows), image `nano` (nanoserver:ltsc2022). Point the harness at a
Windows daemon of the pinned version, for example over SSH:

    export PATH=$PWD/test/bin:$PATH DOCKER_CONTEXT=runlike-win
    go test -count=1 -parallel 8 -run 'TestRoundTrip/windows' ./test/roundtrip/

Or run it on the Windows host itself: cross-build
`GOOS=windows GOARCH=amd64 go test -c -o roundtrip.test.exe ./test/roundtrip/`
and `go build -o runlike.exe ./cmd/runlike`, copy both with
`test/roundtrip/testdata`, and in that directory there set
`$env:DOCKER_HOST = "npipe:////./pipe/docker_engine"` and
`$env:RUNLIKE_EXE` to the runlike.exe path (a host without Go). There
the S copy's line runs in pwsh 7; without pwsh that step is skipped.

Each docker call over an `ssh://` context starts the remote default
shell; with Windows PowerShell 5.1 that is several seconds, so a run takes
minutes. On Windows the daemon stores `-v` binds in a random rotation of
the given order, so `normalize` sorts `HostConfig.Binds` there. Bind
sources must exist on the Windows host (`C:\ProgramData`,
`C:\Program Files`).

The PowerShell and cmd quoting is checked inside Windows by
`quote_shells_windows_test.go` (repository root), built as a test binary:

    GOOS=windows GOARCH=amd64 go test -c -o runlike.test.exe .
    GOOS=windows GOARCH=amd64 go build -o runlike.exe ./cmd/runlike
    # copy both to the Windows host, then in PowerShell there:
    .\runlike.test.exe '-test.run=TestWindowsShellsFuzz' '-test.v'
    $env:RUNLIKE_EXE = "$PWD\runlike.exe"; .\runlike.test.exe '-test.run=TestWindowsShellsDocker' '-test.v'

Quote the `-test.*` flags: Windows PowerShell splits `-test.run` at the
dot. The fuzz runs random hostile arguments through pwsh 7 (needed: from
the official MSI), Windows PowerShell 5.1, `cmd /c` and the cmd prompt
with a fake `docker.exe` that records its argv (`RUNLIKE_SHELL_N`,
`RUNLIKE_SHELL_SEED`); the docker test needs a local Windows daemon.

## Add a case

Add one file per area, `cases_<area>_test.go`, without touching shared code:

```go
package roundtrip

var _ = add("networking",
	Case{Name: "net-alias",
		Setup:    [][]string{{"network", "create", "{p}-net"}},
		Args:     []string{"--network={p}-net", "--network-alias=web", alpine, "true"},
		Teardown: [][]string{{"network", "rm", "{p}-net"}}},
)
```

- `Args` follow `docker create`: options, image, command. Set `Run: true`
  to use `docker run -d` instead, only when starting matters (it is
  slower, and adds the `runIgnore` fields). Options whose container
  cannot start here (`--gpus`, `--blkio-weight`) work as create cases.
- `{p}` is the case's unique prefix `rt-<area>-<case>-<random>`. Name
  every network and volume `{p}-...` so the sweeper finds leftovers.
  Containers are named `{p}-a`, `{p}-b`, `{p}-s` and labelled
  `runlike.roundtrip=1`; the harness removes them (with their anonymous
  volumes) before `Teardown`, also when the case fails.
- Images: `alpine` (alpine:3.20) and `fixture` (alpine with ENTRYPOINT
  `["/bin/sh","-c"]`, CMD `["echo img"]`, ENV `IMG_ENV=img`, LABEL
  `img.label=img`) are ready, and `nginx` (nginx:alpine). Cases run in
  parallel: never pull, tag or commit a shared name in a case (a pull
  with `--platform` moves the tag to a foreign image under every other
  case). Pull a shared image once in `setup()`, or pull by digest and
  tag it `{p}-...` (see `platformCase`).
- `{blk}` is a whole disk of the daemon's host (`/dev/vda` on Lima);
  without one the case skips. `Refused: "<daemon message>"` skips when
  the daemon refuses the option (say where else it is tested).
- `Opts` sets runlike's switches (`NoName`, `NoLabels`, `UseVolumeID`).
- Case names: lowercase `[a-z0-9.-]`, unique in the area.
- Do not name other objects `{p}-a...`, `{p}-b...` or `{p}-s...` (say
  `{p}-arc` or `{p}-bnet`): `normalize` replaces each container's name
  with `{NAME}` by plain substring, so in A `{p}-anet` becomes
  `{NAME}net` while in B it stays, and the case fails for no real
  reason. Fixing it needs a boundary-aware replace in `harness_test.go`.
- A case whose argv Docker itself rejects is not a case: check
  `docker create` accepts it first.

## Step cases

When A must change after `docker create` (start and stop, `docker
network connect`), or the case needs daemon data the CLI would put in
`runlike.Inspect.ConfigMACAddress` or `runlike.Daemon` (`HostName`,
`ContainerNames`), use `addSteps(stepCase{...})` from
`cases_networking_test.go` (`TestSteps`). It also checks expected
options (`want`, prefix match; `notWant`) and warnings (`warn`,
substring), and with `connect: true` runs the `docker network connect`
commands from the warnings on B before comparing. Containers are
`rt-step-<case>-<random>-a`/`-b`.

## Ignored fields

Normalization is in `harness_test.go` (`alwaysIgnore`, `runIgnore`,
`normalize`). Before comparing, the harness replaces each container's own
ID, 12-character ID prefix and name with `{ID}`, `{ID12}` and `{NAME}`
(this makes Id, Name, an auto-generated Hostname, the
resolv.conf/hostname/hosts/log paths and DNS names comparable without
ignoring them), replaces anonymous volume names in Mounts with `{ANON}`,
and sorts `Config.Env` (the daemon returns it in random order for the same
argv). Then it drops:

| Field | Reason |
|---|---|
| `Created` | creation timestamp |
| `State` | run state: status, PID, times, exit code |
| `GraphDriver.Data` | overlay2 layer paths with random IDs |
| `NetworkSettings.SandboxID`, `.SandboxKey` | network namespace, assigned at start |
| `NetworkSettings.Networks.*.EndpointID` | assigned at start |
| `NetworkSettings.Networks.*.IPAddress`, `.GlobalIPv6Address` | assigned at start; a requested IP is in `IPAMConfig` |
| Run cases only: `Networks.*.MacAddress` | random MAC at start (test `--mac-address` as a create case) |
| Run cases only: `NetworkSettings.Ports` | host ports picked at start; the request is in `HostConfig.PortBindings` |
| Run cases only: `RestartCount`, `ExecIDs` | counts so far |

Every other field must match. A case may ignore more with
`Ignore: []Ignore{{"Config.Labels", "--no-labels drops them by design"}}`
(dot path, `*` matches any key or index; the reason is required). Ignore
as little as possible: an ignored field cannot catch a bug. Prefer a
substitution or sort in `normalize` when a value is random but its
presence matters.

## Saved-data cases

For options the test machines cannot exercise (Windows-only, missing
hardware), save the inspect JSON of a real container as
`test/roundtrip/testdata/<name>.json` (optionally `<name>.image.json`) and
register:

```go
var _ = addSaved(SavedCase{Name: "win-isolation", Want: []string{"--isolation=hyperv"}})
```

`TestSaved` checks that each `Want` is a whole argument in runlike's argv
and that `sh` turns the quoted line back into exactly that argv. It needs
no daemon. Do not hand-write the JSON: save it from a real daemon and name
the source (Docker version, OS) in the case's comment.
