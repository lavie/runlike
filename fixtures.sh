#!/bin/bash

function sudocker() {
    sudo docker "$@"
}

sudocker build -t runlike_fixture dockerfiles/

sudocker network rm runlike_fixture_bridge
sudocker network create runlike_fixture_bridge

sudocker rm -f runlike_fixture1
sudocker run -d --name runlike_fixture1 \
    --hostname Essos \
    --expose 1000 \
    --expose 1000/udp \
    --privileged \
    --cap-add=CHOWN \
    -p 400:400 \
    -p 300 \
    -p 301/udp \
    -p 503:502/udp \
    -p 127.0.0.1:601:600/udp \
    -t \
    --dns=8.8.8.8 --dns=8.8.4.4 \
    --user daemon \
    --device=/dev/null:/dev/null:r \
    --label='com.example.notescaped=$KEEP_DOLLAR' \
    --label='com.example.environment=test' \
    --add-host hostname2:127.0.0.2 \
    --add-host hostname3:127.0.0.3 \
    --log-driver=fluentd \
    --log-opt fluentd-async-connect=true \
    --log-opt tag=docker.runlike \
    --restart=always \
    --runtime=runc \
    --env "FOO=thing=\"quoted value with 'spaces' and 'single quotes'\"" \
    --env SET_WITHOUT_VALUE \
    -v $(pwd):/workdir \
    -v /random_volume \
    --workdir=/workdir \
    runlike_fixture

sudocker rm -f runlike_fixture2
sudocker run -d --name runlike_fixture2 \
    --restart=on-failure \
    --net host \
    --pid host \
    --device=/dev/null:/dev/null \
    --label='com.example.version=1' \
    runlike_fixture \
    /bin/bash sleep.sh

sudocker rm -f runlike_fixture3
sudocker run -d --name runlike_fixture3 \
    --restart=on-failure:3 \
    --network runlike_fixture_bridge \
    --log-opt mode=non-blocking \
    --log-opt max-buffer-size=4m \
    runlike_fixture \
    bash -c 'bash sleep.sh'

sudocker rm -f runlike_fixture4
sudocker run -d --name runlike_fixture4 \
    --restart= \
    --mac-address=6a:00:01:ad:d9:e0 \
    runlike_fixture \
    bash -c "bash 'sleep.sh'"

sudocker rm -f runlike_fixture5
sudocker run -d --name runlike_fixture5 \
    --link runlike_fixture4:alias_of4 \
    --link runlike_fixture1 \
    runlike_fixture


