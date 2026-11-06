#!/bin/sh
# Stop runlike test daemons. Usage: test/lima/down.sh [-n N] [--delete]
set -eu
N=1; del=
while [ $# -gt 0 ]; do
  case $1 in -n) N=$2; shift ;; --delete) del=1 ;; esac
  shift
done
bin=$(cd "$(dirname "$0")/.." && pwd)/bin
i=1
while [ "$i" -le "$N" ]; do
  name=runlike-test-$i
  if limactl list -q 2>/dev/null | grep -qx "$name"; then
    if [ -n "$del" ]; then
      limactl delete -f "$name"
      "$bin/docker" context rm -f "$name" >/dev/null 2>&1 || true
    else
      limactl stop "$name" 2>/dev/null || true
    fi
  fi
  i=$((i + 1))
done
