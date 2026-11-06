    "See this docker container? I wish I could run another one just like it,
    but I'll be damned if I'm going to type all those command-line switches manually!"

This is what `runlike` does. You give it a docker container, it outputs the command line necessary to run another one just like it, along with all those pesky options (ports, links, volumes, ...). It's a real time saver for those that normally deploy their docker containers via some CM tool like Ansible/Chef and then find themselves needing to manually re-run some container.

runlike 2.0 is a rewrite in Go. The command line is the same as 1.x; the
output is more complete and safe to run. It covers every `docker run`
option of Docker CLI v29.9.3 (see [Supported options](#supported-options)).

# Install

- **Prebuilt binaries** for Linux, macOS (Apple Silicon) and Windows: download
  the binary for your system from the
  [releases page](https://github.com/lavie/runlike/releases)
  (`runlike-linux-amd64`, `runlike-linux-arm64`, `runlike-darwin-arm64`,
  `runlike-windows-amd64.exe`, `runlike-windows-arm64.exe`; checksums in
  `SHA256SUMS`), rename it to `runlike` (or `runlike.exe`), make it
  executable, and put it on your `PATH`. runlike calls the `docker` command, so the
  Docker CLI must be installed too.
- **With Go** (1.24 or later):

  ```
  go install github.com/lavie/runlike/v2/cmd/runlike@latest
  ```

- **Docker image**, nothing to install: [assaflavie/runlike](https://hub.docker.com/r/assaflavie/runlike/)
  ships runlike with the Docker CLI. Mount the Docker socket:

  ```
  docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
      assaflavie/runlike YOUR-CONTAINER
  ```

  Or as an alias in `~/.profile` or `~/.bashrc`:

  ```
  alias runlike="docker run --rm -v /var/run/docker.sock:/var/run/docker.sock assaflavie/runlike"
  ```

- **Homebrew**: planned for later.

# Usage

    runlike [OPTIONS] CONTAINER

This prints the `docker run` command that creates a container just like
`CONTAINER`. runlike runs `docker container inspect` (and `docker image
inspect` for its image) through your `docker` command, so Docker contexts,
`DOCKER_CONTEXT` and `DOCKER_HOST` work as usual.

| Option | What it does |
|---|---|
| `-p`, `--pretty` | One option per line |
| `--no-name` | Leave out `--name`, so the copy does not collide with the original |
| `-l`, `--no-labels` | Leave out `--label` |
| `--use-volume-id` | Keep the generated names of anonymous volumes, so the copy reuses their data |
| `-s`, `--stdin` | Read `docker inspect` JSON from stdin instead of asking Docker |
| `--shell SHELL` | Quote for `posix` (default: sh, bash, zsh), `powershell` (7.3 or later) or `cmd` |
| `--help` | Show the options |

For example:

    $ docker run -d --name web --restart=unless-stopped -p 8080:80 \
        -e 'GREETING=hello world' -v data:/data --network appnet \
        --network-alias web --memory 256m --label app=demo \
        alpine:3.20 sleep 3600

    $ runlike -p web
    docker run \
    	--name=web \
    	'--env=GREETING=hello world' \
    	--detach \
    	--network=appnet \
    	--network-alias=web \
    	--publish=8080:80 \
    	--volume=data:/data \
    	--memory=256m \
    	--restart=unless-stopped \
    	--label=app=demo \
    	alpine:3.20 \
    	sleep 3600

## Running the output

To run the copy in one step, use `eval` with double quotes:

    eval "$(runlike --no-name web)"

Do not use a bare `$(runlike web)`, as the 1.x README suggested. The shell
splits that output on spaces and does not remove quotes, so a value such as
`GREETING=hello world` reaches Docker as two broken arguments, and the copy
runs with wrong settings or fails. `eval` makes the shell parse the line the
way it would if you typed it.

runlike quotes every value it prints, including values that someone with
control over the container set (environment, labels, commands). The quoting
is fuzz-tested so that running the line never runs anything else. In
PowerShell, run `runlike --shell powershell web` and paste the line; in
`cmd.exe`, use `--shell cmd`.

From `docker inspect` output, for example a saved file:

    docker inspect web | runlike --stdin

## Warnings

When the command cannot recreate something exactly, runlike prints the
command anyway, and a line starting with `runlike: warning:` on stderr that
says what is off and what to do. Warnings never end up in the command, so
`eval "$(runlike X)"` still works. The common ones:

- **MAC address on a further network.** Docker says whether a MAC address was
  pinned only for a container's first network. For a started container on
  several networks, runlike prints no MAC for the others and names them; add
  `mac-address=...` to that network's `--network` if you had pinned one.
- **Networks left out.** Some network settings cannot be given to `docker run`.
  The warning lists the `docker network connect` commands to run after
  creating the copy.
- **Image moved.** The copy runs the image named by the container's tag. If
  that tag now points to a different image, runlike compares with the new
  image and names what it adds that no flag removes.
- **Image not found.** If the tag is gone, runlike compares with the
  container's original image; `docker run` will pull the tag. If neither
  exists, there is no image data (see `--stdin` below).
- **No image data** (`--stdin`, or the image is gone): see
  [Known limits](#known-limits).
- The items under [Known limits](#known-limits) below.

# What changed from 1.x

The command line is unchanged: the same options, arguments and exit codes,
plus the new `--shell`. Scripts that call runlike keep working. The output
changed where 1.x was wrong or incomplete:

- **Every value is quoted safely.** 1.x quoted some values and not others, so
  spaces, quotes or `$` in a value could break the command or change it.
- **Options are printed as `--flag=value`**, one argument each (`--env=...`,
  `--publish=...`), instead of a mix of short and long forms.
- **Defaults are left out.** A value is printed only when it differs from
  Docker's built-in default (log driver `json-file`, runtime `runc`, 64 MB
  shared memory, ...) or from the image's own value (environment, labels,
  working directory, user, entrypoint, health check, ...).
- **Daemon defaults are printed when they differ.** If your daemon is
  configured with another default (Docker Desktop's `local` log driver, for
  example), runlike prints the value, so the command makes the same container
  on any daemon. An explicit `json-file` on such a daemon is printed too.
- **Named volumes are always kept**, and mounts keep the form they were made
  with (`-v` or `--mount`, with their options). Anonymous volumes the image
  declares are no longer printed ([#138](https://github.com/lavie/runlike/issues/138)).
- **All networks.** The copy joins every network of the original, with
  aliases, static IPs and per-network settings, not only the first.
- **MAC addresses.** runlike prints a MAC address only when you pinned one
  (`--mac-address`), running or stopped, so a copy never duplicates the
  original's random MAC. 1.x lost a pinned MAC with current Docker, which
  no longer returns it by default. For further networks Docker does not say
  (see [Known limits](#known-limits)). On Docker Desktop for Mac, runlike reads the
  pinned MAC through Desktop's own daemon socket, which its API proxy does
  not hide.
- **Entrypoint arguments are kept.** 1.x dropped every entrypoint element but
  the first ([#141](https://github.com/lavie/runlike/issues/141)).
- **A moved or deleted image no longer breaks runlike.** It warns and carries
  on (see [Warnings](#warnings)).
- **Auto-generated host names are left out**, and ports published with `-P`
  come back as `--publish-all`, not as the random ports Docker picked.
- **Many more options**: resource limits, health checks, security options,
  namespaces, tmpfs, ulimits, sysctls and more. 1.x supported 17 of the 103
  options fully and 18 partly.

# Supported options

<!-- BEGIN OPTIONS (tools/gen-options.py) -->
runlike 2.0 covers all 103 options of `docker run` in Docker CLI v29.9.3:

| Status | Options |
|---|---|
| Reproduced | 91 |
| Reproduced, tested on saved data (Windows-only, or kernel features the test machines lack) | 4 |
| Folded into other options | 3: `--env-file`, `--label-file`, `--use-api-socket` |
| Not recoverable (client only) | 5: `--detach-keys`, `--help`, `--pull`, `--quiet`, `--sig-proxy` |

Every option, with notes: [docs/options.md](docs/options.md).
<!-- END OPTIONS -->

**Client-only options** change only what the `docker` client does while it
runs. Docker does not store them, so no tool can read them back:

- `--detach-keys`: the key sequence for detaching from an attached container.
- `--help`: prints usage.
- `--pull`: whether the client pulls the image before creating the container.
- `--quiet`: silences the client's pull output.
- `--sig-proxy`: whether the attached client forwards signals to the container.

**Folded options** are expanded by the client before Docker sees them, and
come back in their expanded form:

- `--env-file` and `--label-file`: the client reads the file, and its entries
  come back as `--env` and `--label`.
- `--use-api-socket`: Docker stores a socket mount and an environment
  variable; runlike recognises the pair and prints `--use-api-socket` back.

# Known limits

A few things can't be copied exactly: Docker doesn't keep them, or
`docker run` has no flag for them. Here is what to watch for (runlike
says so on stderr where it can tell):

- **Seccomp profiles.** `--security-opt seccomp=FILE` is stored as the
  profile's JSON, not the file name. runlike prints it with a warning: save
  the JSON to a file and pass that file instead.
- **Exec-form health checks.** A health check written as a JSON list in a
  Dockerfile (`HEALTHCHECK CMD ["curl", "-f", "..."]`) has no `docker run`
  flag. When the container's check differs from the image's, runlike prints
  the same words in shell form (`--health-cmd`), which runs them through
  `/bin/sh`.
- **`-e NAME` without a value.** If the variable was empty in the original
  environment, Docker stores `NAME` alone. `docker run -e NAME` copies `NAME`
  from your environment when it is set there, so runlike warns.
- **The same environment variable set twice.** Only the last value counts;
  runlike prints both and warns.
- **MAC address on a further network.** Docker reports whether a MAC
  address was pinned only for a container's first network. For a started
  container's further networks (and for any network in `--stdin` data newer
  than API 1.51) runlike cannot tell a pinned MAC from a generated one, so
  it prints none and does not warn. If you pinned one there, add
  `mac-address=` to that network's `--network` yourself. The same holds for
  the first network when the container joined it by network ID
  (`--network <ID>`) and has since started: Docker then reports no pinned
  MAC for it either, so add `--mac-address` yourself.
- **Upper case on extra networks.** `docker run` lowercases the long
  `--network=name=...,alias=...` form, so upper case in such a network's name
  or driver options is lost (with a warning).
- **`--stdin` mode** has no image data and no daemon to ask: values the image
  already sets are printed too, an entrypoint cleared with `--entrypoint=`
  cannot be told from none, containers named in `container:` targets are
  printed by ID, and only Docker's built-in defaults are left out.
- **Hyper-V isolation** (Windows containers) is tested only on saved inspect
  data: our Windows test machine runs process isolation.

# The Python version (1.x)

The Python package `runlike` on PyPI is frozen at 1.6.3 and gets no more
releases. Its code lives on the
[`python-1.x`](https://github.com/lavie/runlike/tree/python-1.x) branch.
PyPI itself stops at 1.5.4 (1.6.x was never uploaded there); install 1.6.3 from that branch. If you depend on it, fork that
branch or move to runlike 2.0, which takes the same command line.

# Use as a Go library

The package `github.com/lavie/runlike/v2` turns `docker inspect` JSON into
an argument list. Quoting for a shell is a separate, last step.

```go
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/lavie/runlike/v2"
)

func main() {
	containerJSON, err := exec.Command("docker", "container", "inspect", os.Args[1]).Output()
	if err != nil {
		log.Fatal(err)
	}
	// Image: `docker image inspect` output; Daemon: the daemon's defaults.
	// Both may be left empty, as with --stdin.
	in := runlike.Inspect{Container: containerJSON}
	cmd, err := runlike.Args(in, runlike.Daemon{}, runlike.Options{NoName: true})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%q\n", cmd.Argv()) // ["docker" "run" ...], ready for exec.Command

	line, err := runlike.Format(cmd, runlike.FormatOptions{Shell: runlike.Posix, Pretty: true})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(line)
	for _, w := range cmd.Warnings {
		// Warnings quote container data: %q keeps escape codes off the terminal.
		fmt.Fprintf(os.Stderr, "warning: %q\n", w)
	}
}
```

`runlike.Options` holds the command-line switches. `runlike.Daemon` holds
the daemon's defaults, platform, host name and container names, which the
command-line tool reads from `docker info` and `docker version`; one value
serves every container on that daemon. `runlike.Inspect` holds one
container's data: its inspect JSON, its image's, and the MAC pinned on its
first network (inspect it with `DOCKER_API_VERSION=1.51`,
`runlike.MACAPIVersion`). See the package documentation for each field.

# Development

The source of truth is the `docker run` reference and behaviour of one
pinned Docker CLI release, now v29.9.3, kept in
[spec/v29.9.3/](spec/v29.9.3/). Tests run against real Docker daemons:

- `go test ./...`: unit tests and saved-data tests; no Docker needed.
- Round-trip tests ([test/roundtrip/README.md](test/roundtrip/README.md)):
  create a container, run runlike on it, create the copy from runlike's
  output, and compare both containers' `docker inspect` output.
- Compatibility suite ([test/compat/README.md](test/compat/README.md)):
  checks that runlike's own command line behaves like 1.x.
- Linux daemons run in [Lima](https://lima-vm.io/) VMs
  (`test/lima/up.sh`); Windows containers are tested on a Windows Server VM.

See [CONTRIBUTING.md](CONTRIBUTING.md). runlike 2.0 is written and
maintained by AI coding agents; a human maintainer approves every release.

# License

[LICENSE.txt](LICENSE.txt)
