# Manual QA

A hands-on check of runlike against a real daemon. Checked on Docker
Desktop for Mac (engine 29.9.2); on a plain Linux daemon the
Desktop-specific notes below do not apply.

## Setup

    scripts/qa-setup.sh [INSTALL_DIR]   # default ~/.local/bin
    which runlike && runlike --version

The script builds runlike (`make build`), installs it, removes any old
`qa-*` objects and creates the containers below on the current docker
context. `scripts/qa-teardown.sh` removes every `qa-*` container,
network, volume and image. Base images (alpine, nginx, busybox) stay.

Start with a clean marker file; it must never appear:

    rm -f /tmp/qa-pwned

## Every container

For each container, try:

    runlike qa-x                          # one line
    runlike -p qa-x                       # one option per line
    docker inspect qa-x | runlike -s      # no image or daemon data
    runlike --shell powershell qa-x

`-s` also prints what the image sets (nginx's `--entrypoint`, `PATH`,
...) and warns `no image data`; that is expected. Warnings go to
stderr and start with `runlike: warning:`.

On Docker Desktop every command contains `--log-driver=local`: Desktop's
daemon default is `local`, which differs from Docker's `json-file`, so
runlike prints it.

**Running containers** (qa-web, qa-net, qa-mac-running, qa-health,
qa-restart) print `--detach`. Only a pinned MAC is printed
(`--mac-address`), on Docker Desktop too. With `-s` (inspect data
newer than API 1.51) a running or stopped container gets no MAC and
no warning (a known limit, see the README).

## The containers

| Container | Made with | Output should contain | Expected warnings |
|---|---|---|---|
| qa-web | nginx, `-p 18080:80 -p 127.0.0.1:18443:443`, two `-e` | `'--env=GREETING=hello world'`, `--env=MODE=qa`, both `--publish`; no nginx `--entrypoint`/`--stop-signal` (image defaults); no `--mac-address` | none |
| qa-net | two networks, static IP, aliases | `--network=qa-net-a --network-alias=api --network-alias=api-v2 --ip=172.30.10.10`, then `--network=name=qa-net-b,alias=backend`; no MAC | none |
| qa-mac-running | `--mac-address 02:42:ac:1e:0a:63`, running | `--mac-address=02:42:ac:1e:0a:63` | none |
| qa-mac-stopped | `--mac-address 02:42:ac:1e:0a:64`, started then stopped | `--mac-address=02:42:ac:1e:0a:64` | none |
| qa-volumes | named, anonymous, bind (ro), `--mount` with `readonly,volume-nocopy`, tmpfs | `--volume=qa-data:/data`, the bind with `:ro`, `--volume=/anon`, `--mount=type=volume,source=qa-opts,target=/opts,readonly,volume-nocopy`, `--tmpfs=/scratch:rw,size=64m,mode=1777` | none |
| qa-limits | `--cpus 1.5 -m 256m --memory-swap 768m`, ulimits, `--pids-limit 100` | all of them | none |
| qa-security | cap add/drop, `--read-only`, no-new-privileges, `--user 1000:1000` | `--cap-add=CAP_NET_ADMIN --cap-drop=CAP_MKNOD` (Docker stores the `CAP_` form), the rest as given | none |
| qa-health | image with its own HEALTHCHECK, overridden | `'--health-cmd=test -e /tmp \|\| exit 1'`, interval 30s, start period 10s, retries 5 | none |
| qa-restart | `--restart on-failure:3 --init --stop-signal SIGINT --stop-timeout 20` | all four | none |
| qa-hostile | labels with `$(...)`, backticks, quotes, a newline, an ESC sequence, `ち`; `-e QA_EMPTY` (no value); a hostile `$(...)` in env and command | every value single-quoted; the command is wrapped in `(LC_ALL=C; ...)` and ESC and `ち` are written as `"$(printf '\033')"` and `"$(printf '\343\201\241')"` | `--env=QA_EMPTY has no value` |
| qa-moved | `qa-moved-img:latest` rebuilt after creation (adds `ENV QA_ADDED=1`) | `--label=qa.version=1`, image `qa-moved-img:latest` | `image qa-moved-img:latest is now <id>, not the container's <id> ... adds what no flag removes: env QA_ADDED` |
| qa-host | `--network host --hostname qa-host-name` | `--hostname=qa-host-name --network=host` | none |
| qa-amd64 | `--platform linux/amd64` busybox | `--platform=linux/amd64` (only because it is not the daemon's native platform) | none |

Notes:

- `--memory-swap` equal to twice `-m` is Docker's own default and is
  not printed; qa-limits uses 768m so it shows.
- On Docker Desktop the bind source is printed as
  `/host_mnt/private/var/folders/...`: Desktop itself rewrites the path
  it stores. The copy works on the same Desktop.
- On Docker Desktop for Mac, runlike reads the pinned MAC through
  Desktop's own daemon socket
  (`~/Library/Containers/com.docker.docker/Data/docker.raw.sock`); its
  API proxy hides it once a container has started. Without that socket
  (Desktop on Linux or Windows) a started container gets the warning
  `Docker Desktop's API hides a MAC address pinned with --mac-address`.

- `runlike --shell cmd qa-hostile` refuses (`cannot write control
  character U+001B; use --shell powershell`): cmd.exe cannot quote ESC.
- `runlike -l qa-hostile` drops every label.

## Round trip

Recreate a container from runlike's output and compare. Use `docker
create` so the copy does not start (and an attached `docker run` of a
created container does not block on its command); `--no-name` avoids
the name clash:

    qa_trip() {
      new=$(eval "$(runlike --no-name "$1" | sed -e '1s/docker run/docker create/' -e 's/ --detach//')") || return
      f='.[0] | {Config: (.Config | del(.Hostname, .AttachStdout, .AttachStderr) | .Env |= sort),
                 HostConfig: (.HostConfig | del(.OomKillDisable) | .Binds |= (. // [] | sort)),
                 Nets: (.NetworkSettings.Networks | map_values({Aliases, IPAMConfig, MacAddress}))}'
      diff <(docker inspect "$1" | jq -S "$f") <(docker inspect "$new" | jq -S "$f") && echo "$1: same"
      docker rm -v "$new" >/dev/null
    }
    for c in $(docker ps -a --filter name=^qa- --format '{{.Names}}'); do qa_trip "$c"; done

Expected: every container prints `same`, except

- qa-moved: the copy has `QA_ADDED=1` from the rebuilt image (the warning
  said so).
- qa-mac-stopped on Docker Desktop: `same`, but only because Desktop
  hides the MAC on both sides. The copy has lost the pinned MAC; that
  is what the warning is about.

The filter drops what differs between `docker create` and `run -d`
(attach flags, `OomKillDisable`), the generated host name, and the
order of Env and Binds: Docker does not keep the order of `-e` and `-v`
values stable from run to run.

To check a real start instead, stop the original first (ports, static
IP and MAC are taken while it runs) and eval the plain output:

    cmd=$(runlike --no-name qa-web); docker stop qa-web; eval "$cmd"

## Hostile values

    eval "$(runlike --no-name qa-hostile | sed '1s/docker run/docker create/')"
    ls /tmp/qa-pwned       # must say: No such file or directory

Do it in both bash and zsh, and paste the `--shell powershell` output
into `pwsh` (7.3+) if you have it. Also `docker start -a qa-hostile`
prints `$(touch /tmp/qa-pwned)` literally, and `/tmp/qa-pwned` is still
absent. After that start, qa-hostile is an exited container, so on
Docker Desktop it also gets the `stopped container on Docker Desktop`
MAC warning; rerun `scripts/qa-setup.sh` for a fresh set.

## Clean up

    scripts/qa-teardown.sh
