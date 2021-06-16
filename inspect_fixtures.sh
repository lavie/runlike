#!/bin/bash -eu


function sudocker() {
    sudo docker "$@"
}

for i in {1..5}; do
    sudocker container inspect runlike_fixture$i;
done
