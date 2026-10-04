#!/bin/sh
# Downloads third-party assets that get embedded into the binary.
# Run inside the dev container: docker compose run --rm dev sh scripts/fetch-assets.sh
set -eu

ROOT=$(cd "$(dirname "$0")/.." && pwd)
FONTS="$ROOT/internal/infrastructure/render/fonts"
CATALOG="$ROOT/internal/infrastructure/catalog/data"
STATIC="$ROOT/internal/interfaces/web/static"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

mkdir -p "$FONTS" "$CATALOG" "$STATIC"

echo "DejaVu fonts"
curl -fsSL -o "$TMP/dejavu.tar.bz2" https://github.com/dejavu-fonts/dejavu-fonts/releases/download/version_2_37/dejavu-fonts-ttf-2.37.tar.bz2
tar -xjf "$TMP/dejavu.tar.bz2" -C "$TMP"
for f in DejaVuSans DejaVuSans-Bold DejaVuSans-Oblique DejaVuSans-BoldOblique \
         DejaVuSerif DejaVuSerif-Bold DejaVuSerif-Italic DejaVuSerif-BoldItalic \
         DejaVuSansMono DejaVuSansMono-Bold DejaVuSansMono-Oblique DejaVuSansMono-BoldOblique; do
  cp "$TMP/dejavu-fonts-ttf-2.37/ttf/$f.ttf" "$FONTS/"
done
cp "$TMP/dejavu-fonts-ttf-2.37/LICENSE" "$FONTS/LICENSE-DejaVu"

echo "Liberation fonts"
curl -fsSL -o "$TMP/liberation.tar.gz" https://github.com/liberationfonts/liberation-fonts/files/7261482/liberation-fonts-ttf-2.1.5.tar.gz
tar -xzf "$TMP/liberation.tar.gz" -C "$TMP"
for f in LiberationSans-Regular LiberationSans-Bold LiberationSans-Italic LiberationSans-BoldItalic \
         LiberationSerif-Regular LiberationSerif-Bold LiberationSerif-Italic LiberationSerif-BoldItalic; do
  cp "$TMP/liberation-fonts-ttf-2.1.5/$f.ttf" "$FONTS/"
done
cp "$TMP/liberation-fonts-ttf-2.1.5/LICENSE" "$FONTS/LICENSE-Liberation"

echo "Font Awesome Free 6.6.0"
curl -fsSL -o "$TMP/fa.zip" https://use.fontawesome.com/releases/v6.6.0/fontawesome-free-6.6.0-desktop.zip
unzip -q "$TMP/fa.zip" -d "$TMP"
FA="$TMP/fontawesome-free-6.6.0-desktop"
cp "$FA/otfs/Font Awesome 6 Free-Solid-900.otf" "$CATALOG/fa-solid.otf"
cp "$FA/otfs/Font Awesome 6 Free-Regular-400.otf" "$CATALOG/fa-regular.otf"
cp "$FA/otfs/Font Awesome 6 Brands-Regular-400.otf" "$CATALOG/fa-brands.otf"
cp "$FA/LICENSE.txt" "$CATALOG/LICENSE-FontAwesome.txt"
jq -c 'with_entries(select(.value.free | length > 0)
       | .value = {unicode: .value.unicode,
                   styles: .value.free,
                   terms: ((.value.search.terms // []) + (.value.aliases.names // []))})' \
  "$FA/metadata/icons.json" > "$CATALOG/fontawesome.json"

echo "Emoji metadata (iamcal/emoji-data)"
curl -fsSL https://raw.githubusercontent.com/iamcal/emoji-data/v15.1.2/emoji.json \
  | jq -c '[.[] | select(.has_img_twitter) |
            {unified, short_name, name: (.name // .short_name),
             category, subcategory, short_names}]' > "$CATALOG/emoji.json"

echo "Twemoji SVGs (jdecked/twemoji)"
curl -fsSL -o "$TMP/twemoji.tar.gz" https://github.com/jdecked/twemoji/archive/refs/tags/v15.1.0.tar.gz
tar -xzf "$TMP/twemoji.tar.gz" -C "$TMP" twemoji-15.1.0/assets/svg twemoji-15.1.0/LICENSE-GRAPHICS
(cd "$TMP/twemoji-15.1.0/assets/svg" && zip -q -9 -r "$CATALOG/twemoji.zip" .)
cp "$TMP/twemoji-15.1.0/LICENSE-GRAPHICS" "$CATALOG/LICENSE-Twemoji.txt"

echo "Alpine.js"
curl -fsSL -o "$STATIC/alpine.min.js" https://cdn.jsdelivr.net/npm/alpinejs@3.14.8/dist/cdn.min.js

echo "done"
