#!/bin/bash
# test/apt/test.sh DIST: test the .deb packages and the signed APT
# repository end to end, all in containers on the current docker daemon
# (DOCKER_HOST or context), so it runs on a Lima VM and in CI alike.
# DIST holds make cross output (runlike-linux-amd64, -arm64, SHA256SUMS).
#  1. In debian:stable: scripts/deb.sh builds both .debs; dpkg-deb -I/-c
#     checks them; a throwaway key (with a passphrase) signs a repository
#     built by scripts/apt-repo.sh. The key dies with the container.
#  2. busybox httpd serves that repository on a private network.
#  3. In debian:stable and ubuntu:24.04 (the daemon's architecture):
#     apt install ./runlike_*.deb; then apt-get update with a wrong key
#     must fail; with the right key, apt-get install runlike must work and
#     must not pull in docker.
set -euo pipefail
dist=$(cd "${1:?usage: test/apt/test.sh DIST}" && pwd)
root=$(cd "$(dirname "$0")/../.." && pwd)
v=0.0.0-test
id=rl-apt-$$
tmp=$(mktemp -d)
cleanup() {
	docker rm -f "$id-build" "$id-repo" "$id-client" >/dev/null 2>&1 || true
	docker network rm "$id" >/dev/null 2>&1 || true
	rm -rf "$tmp"
}
trap cleanup EXIT

echo "== build the .debs and a signed repository (throwaway key)"
mkdir -p "$tmp/src/scripts" "$tmp/src/dist"
cp "$root/scripts/deb.sh" "$root/scripts/apt-repo.sh" "$tmp/src/scripts/"
cp "$root/LICENSE.txt" "$tmp/src/"
cp "$dist/runlike-linux-amd64" "$dist/runlike-linux-arm64" "$dist/SHA256SUMS" "$tmp/src/dist/"
docker create --name "$id-build" -w /src -e DEBIAN_FRONTEND=noninteractive debian:stable bash -euo pipefail -c '
	apt-get update -qq && apt-get install -y -qq --no-install-recommends dpkg-dev apt-utils gpg gpg-agent >/dev/null
	scripts/deb.sh '"$v"' dist
	for a in amd64 arm64; do
		f=dist/runlike_'"$v"'_$a.deb
		dpkg-deb -I "$f" >&2
		test "$(dpkg-deb -f "$f" Version)" = 0.0.0~test
		test "$(dpkg-deb -f "$f" Architecture)" = "$a"
		c=$(dpkg-deb -c "$f") && echo "$c" >&2
		test "$(grep -c "/usr/bin/runlike$" <<<"$c")" = 1
		grep -q -- "^-rwxr-xr-x root/root .* ./usr/bin/runlike$" <<<"$c"
		cmp <(dpkg-deb --fsys-tarfile "$f" | tar -xO ./usr/bin/runlike) dist/runlike-linux-$a
	done
	(cd dist && sha256sum --quiet --ignore-missing -c SHA256SUMS)
	test "$(grep -c "\.deb$" dist/SHA256SUMS)" = 2
	# Same input, same bytes: a republish does not change the .debs.
	cp dist/runlike_'"$v"'_amd64.deb /tmp/first.deb && scripts/deb.sh '"$v"' dist >/dev/null
	cmp /tmp/first.deb dist/runlike_'"$v"'_amd64.deb
	test "$(grep -c "\.deb$" dist/SHA256SUMS)" = 2
	export GNUPGHOME=$(mktemp -d) APT_GPG_PASSPHRASE=throwaway-test
	gpg --batch --passphrase "$APT_GPG_PASSPHRASE" --quick-gen-key "runlike test <test@invalid>" ed25519 sign never
	mkdir /out && scripts/apt-repo.sh /out/repo dist/*.deb
	gpg --batch --verify /out/repo/dists/stable/InRelease
	gpg --batch --verify /out/repo/dists/stable/Release.gpg /out/repo/dists/stable/Release
	# As in release.yml: the armored secret key (the environment secret)
	# imported into a fresh GNUPGHOME signs a second run, which keeps the
	# repository valid.
	key=$(gpg --batch --pinentry-mode loopback --passphrase "$APT_GPG_PASSPHRASE" --armor --export-secret-keys)
	export GNUPGHOME=$(mktemp -d)
	gpg --batch --import <<<"$key"
	scripts/apt-repo.sh /out/repo dist/*.deb
	gpg --batch --verify /out/repo/dists/stable/InRelease
	cp dist/*.deb /out/
	export GNUPGHOME=$(mktemp -d)
	gpg --batch --passphrase "" --quick-gen-key "wrong key <wrong@invalid>" ed25519 sign never
	gpg --batch --armor --export >/out/wrong.asc
	# Another key must not re-sign the repository.
	if APT_GPG_PASSPHRASE= scripts/apt-repo.sh /out/repo dist/*.deb 2>/tmp/e; then echo "re-signed with another key"; exit 1; fi
	grep -q "is signed by" /tmp/e
' >/dev/null
docker cp "$tmp/src/." "$id-build:/src"
docker start -a "$id-build"
docker cp "$id-build:/out" "$tmp/out"
(cd "$tmp/out/repo" && find . -type f | sort)

echo "== serve it"
docker network create "$id" >/dev/null
docker create --name "$id-repo" --network "$id" --network-alias repo busybox:stable httpd -f -p 80 -h /srv >/dev/null
docker cp "$tmp/out/repo/." "$id-repo:/srv"
docker start "$id-repo" >/dev/null

for image in debian:stable ubuntu:24.04; do
	echo "== $image"
	docker create --name "$id-client" --network "$id" -w /t -e DEBIAN_FRONTEND=noninteractive "$image" bash -euo pipefail -c '
		a=$(dpkg --print-architecture)
		no() { if "$@"; then echo "unexpected success: $*"; exit 1; fi; }  # "! cmd" ignores set -e
		check() {
			runlike --version | tee /dev/stderr | grep "^runlike " >/dev/null
			out=$(echo "[{\"Name\":\"/web\",\"Config\":{\"Image\":\"nginx:1.27\",\"Env\":[\"A=b c\"]},\"HostConfig\":{}}]" | runlike --stdin 2>/dev/null)
			test "$out" = "docker run --name=web '"'"'--env=A=b c'"'"' --detach nginx:1.27"
			test "$(dpkg-query -W -f "\${Version}" runlike)" = 0.0.0~test
			no command -v docker
		}
		apt-get update -qq
		apt-get install -y -qq ./runlike_'"$v"'_$a.deb >/dev/null
		check
		apt-get purge -y -qq runlike >/dev/null
		no command -v runlike

		install -d /etc/apt/keyrings
		src() { printf "Types: deb\nURIs: http://repo\nSuites: stable\nComponents: main\nSigned-By: /etc/apt/keyrings/runlike.asc\n" >/etc/apt/sources.list.d/runlike.sources; }
		cp wrong.asc /etc/apt/keyrings/runlike.asc && src
		if apt-get update >/tmp/u 2>&1; then cat /tmp/u; echo "wrong key accepted"; exit 1; fi
		grep -q "is not signed" /tmp/u
		echo "wrong key rejected"
		cp runlike.asc /etc/apt/keyrings/runlike.asc
		apt-get update 2>&1 | tee /tmp/u
		no grep -q "^[WE]:" /tmp/u
		apt-get install -y -qq runlike >/dev/null
		check
		echo "OK $(. /etc/os-release && echo "$PRETTY_NAME") $a"
	' >/dev/null
	mkdir -p "$tmp/t"; cp "$tmp/out/wrong.asc" "$tmp/out/repo/runlike.asc" "$tmp/out/"*.deb "$tmp/t/"
	docker cp "$tmp/t/." "$id-client:/t"
	docker start -a "$id-client"
	docker rm "$id-client" >/dev/null
done
echo "PASS"
