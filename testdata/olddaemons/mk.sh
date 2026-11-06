#!/bin/bash
set -eux
docker pull -q alpine:3.20 >/dev/null
docker network create --subnet 172.30.0.0/24 appnet >/dev/null
docker volume create vol1 >/dev/null
docker create --name full -p 8080:80 -p 127.0.0.1:5353:53/udp --expose 9000 \
  -v vol1:/data -v /tmp:/host:ro --tmpfs /run:size=1m -e A=1 -e 'B=x y' -l app=web \
  --health-cmd 'wget -q -O- localhost' --health-interval 30s --health-retries 3 \
  --restart on-failure:3 --device /dev/null:/dev/xnull --mac-address 02:42:ac:11:00:99 \
  --memory 64m --cpus 0.5 --cpu-shares 512 --pids-limit 100 --ulimit nofile=1024:2048 \
  --cap-add NET_ADMIN --hostname h1 --dns 1.1.1.1 --add-host x:1.2.3.4 \
  --log-opt max-size=1m -w /data --user 1000 alpine:3.20 sleep 1000 >/dev/null
docker run -d --name net --network appnet --network-alias api --ip 172.30.0.10 \
  --restart unless-stopped -p 9090:90 alpine:3.20 sleep 1000 >/dev/null
docker create --name peer --network container:net alpine:3.20 true >/dev/null
docker run -d --name stopped --mac-address 02:42:ac:11:00:42 --restart always -v /data alpine:3.20 sleep 1000 >/dev/null
docker stop -t 0 stopped >/dev/null
docker create --name minimal alpine:3.20 >/dev/null
mkdir -p /tmp/out
for c in full net peer stopped minimal; do docker container inspect $c > /tmp/out/$c.container.json; done
docker image inspect alpine:3.20 > /tmp/out/alpine.image.json
docker version --format '{{.Server.Version}}' > /tmp/out/VERSION
