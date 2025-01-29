#!/bin/bash -eu

VER="$1"

echo "Waiting for runlike version $VER to appear on PyPI..."
for tries in {1..15}; do
    echo "Attempt $tries/15..."
    if curl -sf https://pypi.org/pypi/runlike/json | jq -e --arg ver "$VER" '.releases | has($ver)' > /dev/null; then
        echo "Version $VER found on PyPI!"
        exit 0
    fi
    echo "Version $VER not found yet, waiting 5 seconds..."
    sleep 5
done

echo "ERROR: Timed out waiting for version $VER to appear on PyPI"
exit 1
