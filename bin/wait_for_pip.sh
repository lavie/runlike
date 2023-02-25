#!/bin/bash -eu

VER="$1"

for tries in {1..10}; do
    if curl -s https://pypi.org/pypi/runlike/json | jq '.releases | keys' | grep "$VER"; then
        exit 0;
    fi
    echo still waiting for $VER to appear...;
    sleep 2;
done

exit 1
