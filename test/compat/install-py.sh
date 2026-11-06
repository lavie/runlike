#!/bin/sh
# Install runlike 1.6.3 (git tag v1.6.3; PyPI stops at 1.5.4) into
# test/compat/.venv, for runlike-py.sh.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
src=$(mktemp -d)
trap 'rm -rf "$src"' EXIT
git -C "$here/../.." archive v1.6.3 runlike pyproject.toml README.md LICENSE.txt | tar -x -C "$src"
rm -rf "$here/.venv"
/usr/bin/python3 -m venv "$here/.venv"
"$here/.venv/bin/pip" install -q --disable-pip-version-check "$src"
