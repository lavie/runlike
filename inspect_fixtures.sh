#!/bin/bash -eu


function sudocker() {
    sudo docker "$@"
}

for i in {1..6}; do
    sudocker container inspect runlike_fixture$i;
done
