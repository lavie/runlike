#!/bin/bash
docker build -t runlike_fixture dockerfiles/
docker rm -f runlike_fixture1
docker run -d --name runlike_fixture1 \
    --hostname Essos \
    --expose 1000 \
    -p 400:400 \
    -p 300 \
    -p 301/udp \
    -p 503:502/udp \
    -p 127.0.0.1:601:600/udp \
    --restart=always \
    -v $(pwd):/workdir \
    -v /random_volume \
    runlike_fixture

docker rm -f runlike_fixture2
docker run -d --name runlike_fixture2 \
    --restart=on-failure \
    runlike_fixture

docker rm -f runlike_fixture3
docker run -d --name runlike_fixture3 \
    --restart=on-failure:3 \
    runlike_fixture
