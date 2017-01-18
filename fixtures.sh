#!/bin/bash
docker build -t runlike_fixture dockerfiles/
docker rm -f runlike_fixture
docker run -d --name runlike_fixture runlike_fixture
