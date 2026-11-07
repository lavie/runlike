#!/bin/bash
# apt-repo.sh REPO DEB...: add the .deb files to the static APT repository
# in REPO (the checkout of lavie/apt), rebuild its indexes and sign them.
#   REPO/pool/main/r/runlike/*.deb     every version ever added
#   REPO/dists/stable/main/binary-{amd64,arm64}/Packages{,.gz}
#   REPO/dists/stable/{Release,InRelease,Release.gpg}
#   REPO/runlike.asc                   the public signing key
# Signs with the one secret key in $GNUPGHOME; its passphrase comes from
# $APT_GPG_PASSPHRASE (may be empty). Needs apt-ftparchive (apt-utils) and gpg.
set -euo pipefail
repo=${1:?usage: apt-repo.sh REPO DEB...}
shift
[ $# -gt 0 ] || { echo "usage: apt-repo.sh REPO DEB..." >&2; exit 1; }
fpr=$(gpg --batch --with-colons --list-secret-keys | awk -F: '$1 == "fpr" {print $10; exit}')
n=$(gpg --batch --with-colons --list-secret-keys | grep -c '^sec:' || true)
[ "$n" = 1 ] || { echo "need exactly one secret key in GNUPGHOME, found $n" >&2; exit 1; }
# Users trust the key in runlike.asc: a different key would break every
# apt-get update. To rotate the key on purpose, delete runlike.asc first.
if [ -f "$repo/runlike.asc" ]; then
	old=$(gpg --batch --with-colons --show-keys "$repo/runlike.asc" | awk -F: '$1 == "fpr" {print $10; exit}')
	[ "$old" = "$fpr" ] || { echo "$repo is signed by $old, not by this key $fpr" >&2; exit 1; }
fi

pool=$repo/pool/main/r/runlike
mkdir -p "$pool"
cp "$@" "$pool/"
cd "$repo"
for arch in amd64 arm64; do
	d=dists/stable/main/binary-$arch
	mkdir -p "$d"
	apt-ftparchive --arch "$arch" packages pool/main >"$d/Packages"
	gzip -9nkf "$d/Packages"
done
s=dists/stable
rm -f "$s/Release" "$s/InRelease" "$s/Release.gpg"
apt-ftparchive \
	-o APT::FTPArchive::Release::Origin=runlike \
	-o APT::FTPArchive::Release::Label=runlike \
	-o APT::FTPArchive::Release::Suite=stable \
	-o APT::FTPArchive::Release::Codename=stable \
	-o APT::FTPArchive::Release::Architectures="amd64 arm64" \
	-o APT::FTPArchive::Release::Components=main \
	-o APT::FTPArchive::Release::Description="runlike, github.com/lavie/runlike" \
	release "$s" >Release.tmp
mv Release.tmp "$s/Release"
sign() { gpg --batch --yes --pinentry-mode loopback --passphrase-fd 3 --local-user "$fpr" "$@" 3<<<"${APT_GPG_PASSPHRASE:-}"; }
sign --clearsign -o "$s/InRelease" "$s/Release"
sign --armor --detach-sign -o "$s/Release.gpg" "$s/Release"
gpg --batch --armor --export "$fpr" >runlike.asc
touch .nojekyll # GitHub Pages: serve the files as they are
