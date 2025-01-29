#!/bin/bash -eu

VER="$1"

echo "Waiting for runlike version $VER to appear on PyPI..."
for tries in {1..30}; do
    echo "Attempt $tries/30..."
    if curl -sf https://pypi.org/pypi/runlike/json | jq -e --arg ver "$VER" '.releases | has($ver)' > /dev/null; then
        echo "Version $VER found in PyPI index, verifying installability..."
        if pip install --dry-run runlike==$VER >/dev/null 2>&1; then
            echo "Version $VER is installable from PyPI!"
            exit 0
        else
            echo "Version $VER found but not yet installable, waiting..."
        fi
    fi
    echo "Version $VER not ready yet, waiting 10 seconds..."
    sleep 10
done

echo "ERROR: Timed out waiting for version $VER to be available on PyPI"
exit 1
