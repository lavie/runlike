#!/bin/bash
# deb.sh VERSION DIST: build runlike_VERSION_amd64.deb and _arm64.deb in
# DIST from DIST/runlike-linux-amd64 and -arm64 (make cross output), and
# add them to DIST/SHA256SUMS. VERSION is the tag without v; a
# pre-release 2.1.0-rc.1 becomes the Debian version 2.1.0~rc.1, which
# sorts before 2.1.0. Needs dpkg-deb. Reproducible: file times and the
# archive come from SOURCE_DATE_EPOCH (default: 0).
set -euo pipefail
v=${1:?usage: deb.sh VERSION DIST}
dist=${2:?usage: deb.sh VERSION DIST}
[[ $v =~ ^[0-9][0-9A-Za-z.+-]*$ ]] || { echo "bad version: $v" >&2; exit 1; }
here=$(cd "$(dirname "$0")" && pwd)
export SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH:-0}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
for arch in amd64 arm64; do
	r=$tmp/$arch
	install -d "$r/DEBIAN" "$r/usr/bin" "$r/usr/share/doc/runlike"
	install -m 0755 "$dist/runlike-linux-$arch" "$r/usr/bin/runlike"
	install -m 0644 "$here/../LICENSE.txt" "$r/usr/share/doc/runlike/copyright"
	# Suggests, not Recommends: apt installs Recommends by default, and
	# docker.io would pull in a whole Docker Engine. People use
	# docker-ce-cli, Debian's docker-cli, docker.io or a remote context.
	cat >"$r/DEBIAN/control" <<-EOF
	Package: runlike
	Version: ${v//-/\~}
	Architecture: $arch
	Maintainer: Assaf Lavie <a@assaflavie.com>
	Installed-Size: $(du -sk "$r/usr" | cut -f1)
	Suggests: docker-ce-cli | docker-cli | docker.io
	Section: admin
	Priority: optional
	Homepage: https://github.com/lavie/runlike
	Description: print the docker run command that recreates a container
	 runlike reads docker inspect output and prints the docker run command
	 line, with every option, that runs another container just like it.
	EOF
	find "$r" -exec touch -h -d "@$SOURCE_DATE_EPOCH" {} +
	dpkg-deb --root-owner-group -Zxz --build "$r" "$dist/runlike_${v}_$arch.deb" >/dev/null
	echo "$dist/runlike_${v}_$arch.deb"
done
cd "$dist"
# Keep SHA256SUMS one line per file: drop old .deb lines, then add.
{ grep -v '\.deb$' SHA256SUMS || true; sha256sum runlike_*.deb; } >"$tmp/sums"
mv "$tmp/sums" SHA256SUMS
