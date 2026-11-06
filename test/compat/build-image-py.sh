#!/bin/sh
# Build image runlike-compat:py from runlike 1.6.3 (git tag v1.6.3; PyPI stops
# at 1.5.4) on the daemon in DOCKER_CONTEXT. Local only: never push it.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
ctx=$(mktemp -d)
trap 'rm -rf "$ctx"' EXIT
mkdir "$ctx/src"
git -C "$here/../.." archive v1.6.3 runlike pyproject.toml README.md LICENSE.txt | tar -x -C "$ctx/src"
cp "$here/Dockerfile.py" "$ctx/Dockerfile"
docker build -q -t runlike-compat:py "$ctx"
