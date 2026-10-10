#!/bin/sh
# Renders PKGBUILD and .SRCINFO for a published release, ready to push to
# the AUR repository:
#   sh packaging/aur/update.sh 0.4.0
# Needs curl and sha256sum; .SRCINFO is generated in an Arch container.
set -eu
VERSION=${1:?usage: update.sh VERSION}
DIR=$(cd "$(dirname "$0")" && pwd)
REPO=maffmeier/ulanzi-deck-configurator
RAW="https://raw.githubusercontent.com/$REPO/v$VERSION"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

curl -fsSL -o "$TMP/SHA256SUMS" "https://github.com/$REPO/releases/download/v$VERSION/SHA256SUMS"
sum_of() { awk -v f="$1" '$2 == f || $2 == "*"f { print $1 }' "$TMP/SHA256SUMS"; }
AMD64=$(sum_of ulanzi-deck-linux-amd64)
ARM64=$(sum_of ulanzi-deck-linux-arm64)
[ -n "$AMD64" ] && [ -n "$ARM64" ] || { echo "binaries missing in SHA256SUMS" >&2; exit 1; }

COMMON=""
for f in packaging/linux/70-ulanzi-deck.rules packaging/linux/ulanzi-deck.desktop packaging/linux/ulanzi-deck.png LICENSE THIRD_PARTY_NOTICES.md; do
  curl -fsSL -o "$TMP/file" "$RAW/$f"
  COMMON="$COMMON'$(sha256sum "$TMP/file" | cut -d' ' -f1)' "
done

sed -e "s/@VERSION@/$VERSION/" -e "s/@SUMS_COMMON@/${COMMON% }/" \
    -e "s/@SUM_AMD64@/$AMD64/" -e "s/@SUM_ARM64@/$ARM64/" \
    "$DIR/PKGBUILD.in" > "$DIR/PKGBUILD"

docker run --rm -v "$DIR":/pkg -w /pkg archlinux:latest sh -c \
  'useradd -m b >/dev/null && chown -R b /pkg && su b -c "makepkg --printsrcinfo" > .SRCINFO.tmp && chown -R '"$(id -u):$(id -g)"' /pkg' 
mv "$DIR/.SRCINFO.tmp" "$DIR/.SRCINFO"
echo "PKGBUILD and .SRCINFO for $VERSION written to $DIR"
