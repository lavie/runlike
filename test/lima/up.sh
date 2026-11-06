#!/bin/sh
# Start (creating if missing) runlike test daemons runlike-test-1..N.
# Usage: test/lima/up.sh [-n N]   (default N=1)
# Also installs the pinned docker CLI in test/bin/ and a docker context per instance.
set -eu
V=29.9.3
N=1
[ "${1:-}" = "-n" ] && N=$2
here=$(cd "$(dirname "$0")" && pwd)
bin=$(cd "$here/.." && pwd)/bin

if ! "$bin/docker" version --format '{{.Client.Version}}' 2>/dev/null | grep -qx "$V"; then
  mkdir -p "$bin"
  tmp=$(mktemp -d)
  curl -fsSL "https://download.docker.com/mac/static/stable/aarch64/docker-$V.tgz" | tar -xz -C "$tmp"
  mv "$tmp/docker/docker" "$bin/docker"
  rm -rf "$tmp"
fi

i=1
while [ "$i" -le "$N" ]; do
  name=runlike-test-$i
  if limactl list -q 2>/dev/null | grep -qx "$name"; then
    limactl start --tty=false "$name"
  else
    limactl start --tty=false --name="$name" "$here/runlike-test.yaml"
  fi
  sock=$(limactl list "$name" --format '{{.Dir}}')/sock/docker.sock
  "$bin/docker" context rm -f "$name" >/dev/null 2>&1 || true
  "$bin/docker" context create "$name" --docker "host=unix://$sock" >/dev/null
  echo "$name: DOCKER_HOST=unix://$sock  (or: $bin/docker --context $name)"
  i=$((i + 1))
done
