#!/bin/sh
# Runs runlike 1.6.3 from test/compat/.venv (see test/compat/README.md).
exec "$(dirname "$0")/.venv/bin/runlike" "$@"
