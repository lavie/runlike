#!/bin/bash
docker build -t runlike_fixture dockerfiles/
docker rm -f runlike_fixture
docker run -d --name runlike_fixture \
    -p 400:400 \
    -v $(pwd):/workdir \
    -v /random_volume \
    runlike_fixture
