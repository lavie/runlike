#!/bin/bash
# macos-sign.sh BINARY: sign a macOS binary with the Developer ID Application
# certificate (hardened runtime, secure timestamp), notarize it, and check it.
# A bare Mach-O cannot be stapled, so Gatekeeper fetches the ticket online.
# Runs on macOS only. Fails, and leaves BINARY unusable for release, on any
# error. Reads its secrets from the environment, never from arguments:
#   MACOS_CERT_P12_BASE64  the .p12 (certificate + private key), base64
#   MACOS_CERT_PASSWORD    the .p12 password
#   ASC_KEY_ID, ASC_ISSUER_ID, ASC_KEY_P8  App Store Connect team API key
#                          (contents of AuthKey_<id>.p8) for notarytool
set -euo pipefail
bin=${1:?usage: macos-sign.sh BINARY}
: "${MACOS_CERT_P12_BASE64:?}" "${MACOS_CERT_PASSWORD:?}" "${ASC_KEY_ID:?}" "${ASC_ISSUER_ID:?}" "${ASC_KEY_P8:?}"

tmp=$(mktemp -d)
keychain=$tmp/signing.keychain-db
here=$(cd "$(dirname "$0")" && pwd)
old_keychains=()
while IFS= read -r k; do old_keychains+=("$(echo "$k" | sed -e 's/^ *"//' -e 's/"$//')"); done < <(security list-keychains -d user)
cleanup() {
	security list-keychains -d user -s "${old_keychains[@]}" || true
	security delete-keychain "$keychain" 2>/dev/null || true
	rm -rf "$tmp"
}
trap cleanup EXIT
chmod 700 "$tmp"
(umask 077
	printf '%s' "$MACOS_CERT_P12_BASE64" | base64 -D > "$tmp/cert.p12"
	printf '%s\n' "$ASC_KEY_P8" > "$tmp/key.p8")

# A throwaway keychain that only codesign may use. It joins the user's
# search list while this script runs, so codesign finds the intermediate
# certificate (Apple's Developer ID G2 CA, scripts/DeveloperIDG2CA.pem, from
# apple.com/certificateauthority) even on a clean runner.
kcpass=$(openssl rand -hex 24)
security create-keychain -p "$kcpass" "$keychain"
security set-keychain-settings -lut 3600 "$keychain"
security unlock-keychain -p "$kcpass" "$keychain"
security import "$here/DeveloperIDG2CA.pem" -k "$keychain" >/dev/null
security import "$tmp/cert.p12" -k "$keychain" -P "$MACOS_CERT_PASSWORD" -T /usr/bin/codesign >/dev/null
security set-key-partition-list -S apple-tool:,apple: -k "$kcpass" "$keychain" >/dev/null
rm -f "$tmp/cert.p12"
security list-keychains -d user -s "$keychain" "${old_keychains[@]}"
identity=$(security find-identity -v -p codesigning "$keychain" | awk -F'"' '/Developer ID Application/ {print $2; exit}')
[ -n "$identity" ] || { echo "no Developer ID Application identity in the .p12" >&2; exit 1; }
echo "signing with: $identity"

codesign --force --sign "$identity" --keychain "$keychain" --options runtime --timestamp \
	--identifier io.github.lavie.runlike "$bin"
codesign --verify --strict --verbose=2 "$bin"

ditto -c -k --keepParent "$bin" "$tmp/notarize.zip"
xcrun notarytool submit "$tmp/notarize.zip" --key "$tmp/key.p8" --key-id "$ASC_KEY_ID" \
	--issuer "$ASC_ISSUER_ID" --wait --timeout 30m --output-format json > "$tmp/submit.json"
id=$(plutil -extract id raw "$tmp/submit.json")
status=$(plutil -extract status raw "$tmp/submit.json")
echo "notarization $id: $status"
if [ "$status" != Accepted ]; then
	xcrun notarytool log "$id" --key "$tmp/key.p8" --key-id "$ASC_KEY_ID" --issuer "$ASC_ISSUER_ID" >&2 || true
	exit 1
fi
# Ask Apple's service whether this exact binary has a ticket.
codesign --verify --strict --check-notarization -R=notarized --verbose=2 "$bin"
