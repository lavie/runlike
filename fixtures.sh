#!/bin/bash
docker build -t runlike_fixture dockerfiles/
docker rm -f runlike_fixture
docker run -d --name runlike_fixture \
    --expose 1000 \
    -p 400:400 \
    -p 300 \
    -p 301/udp \
    -p 503:502/udp \
    -p 127.0.0.1:601:600/udp \
    -v $(pwd):/workdir \
    -v /random_volume \
    runlike_fixture
