#!/bin/sh
# Builds an unsigned universal Ulanzi Deck.app inside a .dmg (macOS only):
#   sh packaging/macos/build-dmg.sh 0.4.0 dist/ulanzi-deck-darwin-arm64 dist/ulanzi-deck-darwin-amd64
set -eu
VERSION=${1:?usage: build-dmg.sh VERSION ARM64_BINARY AMD64_BINARY}
ARM64=${2:?}
AMD64=${3:?}
DIR=$(cd "$(dirname "$0")" && pwd)
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

APP="$WORK/root/Ulanzi Deck.app"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
lipo -create -output "$APP/Contents/MacOS/ulanzi-deck" "$ARM64" "$AMD64"
chmod 755 "$APP/Contents/MacOS/ulanzi-deck"
sed "s/@VERSION@/$VERSION/g" "$DIR/Info.plist" > "$APP/Contents/Info.plist"

ICONSET="$WORK/ulanzi-deck.iconset"
mkdir "$ICONSET"
for size in 16 32 128 256; do
  sips -z $size $size "$DIR/../linux/ulanzi-deck.png" --out "$ICONSET/icon_${size}x${size}.png" >/dev/null
  double=$((size * 2))
  sips -z $double $double "$DIR/../linux/ulanzi-deck.png" --out "$ICONSET/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/ulanzi-deck.icns"

# Ad-hoc signature: required for arm64 binaries after lipo, no identity needed.
codesign --force --deep --sign - "$APP"

ln -s /Applications "$WORK/root/Applications"
OUT="dist/ulanzi-deck-$VERSION-macos-universal.dmg"
mkdir -p dist
hdiutil create -volname "Ulanzi Deck" -srcfolder "$WORK/root" -format UDZO -ov "$OUT"
ls -l "$OUT"
