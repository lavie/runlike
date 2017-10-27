#!/bin/bash
docker build -t runlike_fixture dockerfiles/

docker network rm runlike_fixture_bridge
docker network create runlike_fixture_bridge

docker rm -f runlike_fixture1
docker run -d --name runlike_fixture1 \
    --hostname Essos \
    --expose 1000 \
    --privileged \
    -p 400:400 \
    -p 300 \
    -p 301/udp \
    -p 503:502/udp \
    -p 127.0.0.1:601:600/udp \
    --label com.example.group="one" \
    --label com.example.environment="test" \
    --restart=always \
    -v $(pwd):/workdir \
    -v /random_volume \
    runlike_fixture

docker rm -f runlike_fixture2
docker run -d --name runlike_fixture2 \
    --restart=on-failure \
    --net host \
    --label com.example.version="1" \
    runlike_fixture

docker rm -f runlike_fixture3
docker run -d --name runlike_fixture3 \
    --restart=on-failure:3 \
    --network runlike_fixture_bridge \
    runlike_fixture
