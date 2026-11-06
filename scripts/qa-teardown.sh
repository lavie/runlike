#!/usr/bin/env bash
# Remove every qa-* container, network, volume and image made by qa-setup.sh
# on the current docker context. Base images (alpine, nginx, busybox) stay.
set -euo pipefail

rm_all() { # rm_all <docker subcommand...> -- reads names on stdin
	xargs -r "$@" >/dev/null 2>&1 || true
}
docker ps -aq --filter 'name=^qa-' | rm_all docker rm -f -v
docker network ls -q --filter 'name=^qa-' | rm_all docker network rm
docker volume ls -q --filter 'name=^qa-' | rm_all docker volume rm
docker images --format '{{.Repository}}:{{.Tag}}' | { grep '^qa-' || true; } | rm_all docker rmi -f
docker images -q --filter 'dangling=true' --filter 'label=runlike-qa' | rm_all docker rmi -f
echo "qa-* containers, networks, volumes and images removed"
