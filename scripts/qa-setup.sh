#!/usr/bin/env bash
# Build runlike, install it, and create the qa-* containers that
# docs/manual-qa.md walks through, on the current docker context.
#
#   scripts/qa-setup.sh [INSTALL_DIR]     (default: ~/.local/bin)
#
# Idempotent: it first runs qa-teardown.sh. Remove everything with
# scripts/qa-teardown.sh.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
dest=${1:-$HOME/.local/bin}

make -C "$root" build
mkdir -p "$dest"
install -m 0755 "$root/build/runlike" "$dest/runlike"
echo "installed $dest/runlike ($("$dest/runlike" --version))"

"$root/scripts/qa-teardown.sh"

A=alpine:3.20
for img in "$A" nginx:alpine; do docker pull -q "$img" >/dev/null; done
sleep_cmd=(sleep 86400)

# Plain web server: ports and environment.
docker run -d --name qa-web -p 18080:80 -p 127.0.0.1:18443:443/tcp \
	-e 'GREETING=hello world' -e MODE=qa nginx:alpine >/dev/null

# Two user networks: static IP, aliases on both.
docker network create --subnet 172.30.10.0/24 qa-net-a >/dev/null
docker network create qa-net-b >/dev/null
docker run -d --name qa-net \
	--network name=qa-net-a,ip=172.30.10.10,alias=api,alias=api-v2 \
	--network name=qa-net-b,alias=backend \
	"$A" "${sleep_cmd[@]}" >/dev/null

# Pinned MAC, running and stopped.
docker run -d --name qa-mac-running --network qa-net-a \
	--mac-address 02:42:ac:1e:0a:63 "$A" "${sleep_cmd[@]}" >/dev/null
# Started, then stopped: Docker Desktop hides the pinned MAC of an exited
# container (not of a never-started one), and runlike warns.
docker run -d --name qa-mac-stopped --network qa-net-a \
	--mac-address 02:42:ac:1e:0a:64 "$A" "${sleep_cmd[@]}" >/dev/null
docker stop -t0 qa-mac-stopped >/dev/null

# Volumes: named, anonymous, bind, --mount with options, tmpfs.
bind_dir=${TMPDIR:-/tmp}/runlike-qa-bind
mkdir -p "$bind_dir"
docker create --name qa-volumes \
	-v qa-data:/data -v /anon -v "$bind_dir":/bind:ro \
	--mount type=volume,src=qa-opts,dst=/opts,readonly,volume-nocopy \
	--tmpfs /scratch:rw,size=64m,mode=1777 \
	"$A" "${sleep_cmd[@]}" >/dev/null

# Resource limits. (--memory-swap at 2x -m is Docker's default and not printed.)
docker create --name qa-limits --cpus 1.5 -m 256m --memory-swap 768m \
	--ulimit nofile=1024:2048 --ulimit nproc=512 --pids-limit 100 \
	"$A" "${sleep_cmd[@]}" >/dev/null

# Security.
docker create --name qa-security --cap-add NET_ADMIN --cap-drop MKNOD \
	--read-only --security-opt no-new-privileges --user 1000:1000 \
	"$A" "${sleep_cmd[@]}" >/dev/null

# Health check that overrides the image's own.
docker build -q -t qa-health-img:latest --label runlike-qa - >/dev/null <<DOCKERFILE
FROM $A
HEALTHCHECK --interval=1m CMD true
DOCKERFILE
docker run -d --name qa-health \
	--health-cmd 'test -e /tmp || exit 1' --health-interval 30s \
	--health-retries 5 --health-start-period 10s \
	qa-health-img:latest "${sleep_cmd[@]}" >/dev/null

# Restart policy, init, stop signal and timeout.
docker run -d --name qa-restart --restart on-failure:3 --init \
	--stop-signal SIGINT --stop-timeout 20 "$A" "${sleep_cmd[@]}" >/dev/null

# Hostile values: running runlike's output must never create /tmp/qa-pwned.
# QA_EMPTY is unset here, so Docker stores the bare name.
env -u QA_EMPTY docker create --name qa-hostile \
	--label 'qa.dollar=$(touch /tmp/qa-pwned)' \
	--label 'qa.backtick=`touch /tmp/qa-pwned`' \
	--label "qa.quotes=it's \"quoted\"; touch /tmp/qa-pwned" \
	--label $'qa.newline=line1\ntouch /tmp/qa-pwned' \
	--label $'qa.esc=\e[31mred\e[0m' \
	--label 'qa.utf8=ち' \
	-e QA_EMPTY -e 'QA_CMD=$(touch /tmp/qa-pwned)' \
	"$A" sh -c 'echo "$1"' qa '$(touch /tmp/qa-pwned)' >/dev/null

# Image tag rebuilt after the container was created.
docker build -q -t qa-moved-img:latest --label runlike-qa --label qa.version=1 - >/dev/null <<DOCKERFILE
FROM $A
DOCKERFILE
docker create --name qa-moved qa-moved-img:latest "${sleep_cmd[@]}" >/dev/null
docker build -q -t qa-moved-img:latest --label runlike-qa --label qa.version=2 - >/dev/null <<DOCKERFILE
FROM $A
ENV QA_ADDED=1
DOCKERFILE

# Host network with a host name.
docker create --name qa-host --network host --hostname qa-host-name \
	"$A" "${sleep_cmd[@]}" >/dev/null

# Foreign platform. busybox is used only here, so pulling the other
# platform cannot move a tag the other containers use.
docker create --name qa-amd64 --platform linux/amd64 busybox \
	"${sleep_cmd[@]}" >/dev/null

docker ps -a --filter 'name=^qa-' --format 'table {{.Names}}\t{{.Status}}\t{{.Image}}'
