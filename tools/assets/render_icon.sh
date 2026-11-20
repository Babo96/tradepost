#!/usr/bin/env bash
# Regenerate every TradePost brand asset from the SVG sources.
#
# Sources:
#   assets/tradepost-icon.svg        full mark, 512x512 viewBox
#   assets/tradepost-icon-small.svg  simplified mark for 32px/favicon/TGA
#
# Outputs:
#   assets/tradepost-icon-{32,64,180,192,512}.png
#   assets/tradepost-icon.ico
#   addon/TradePost/Media/icon.tga        32x32 uncompressed RGBA
#   addon/TradePost/Media/highlight.tga   soft white hover ring
#   server/internal/api/web/icon.svg
#   server/internal/api/web/icon-{32,192,512}.png
#
# Requires rsvg-convert and Python 3 with Pillow.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

src="assets/tradepost-icon.svg"
small="assets/tradepost-icon-small.svg"
web="server/internal/api/web"
media="addon/TradePost/Media"

for bin in rsvg-convert python3; do
  command -v "$bin" >/dev/null 2>&1 || {
    echo "missing required tool: $bin" >&2
    exit 1
  }
done
for file in "$src" "$small"; do
  test -f "$file" || {
    echo "missing source: $file" >&2
    exit 1
  }
done

mkdir -p assets "$web" "$media"

for size in 64 180 192 512; do
  rsvg-convert -w "$size" -h "$size" -o "assets/tradepost-icon-${size}.png" "$src"
done

# The small mark is the one used at favicon/header/TGA sizes.
rsvg-convert -w 32 -h 32 -o assets/tradepost-icon-32.png "$small"

cp "$small" "$web/icon.svg"
cp assets/tradepost-icon-32.png "$web/icon-32.png"
cp assets/tradepost-icon-192.png "$web/icon-192.png"
cp assets/tradepost-icon-512.png "$web/icon-512.png"

python3 - <<'PY'
from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter

root = Path(".")
media = root / "addon/TradePost/Media"
media.mkdir(parents=True, exist_ok=True)

# 32x32 uncompressed 32-bit RGBA TGA for the game.
icon = Image.open("assets/tradepost-icon-32.png").convert("RGBA")
if icon.size != (32, 32):
    icon = icon.resize((32, 32), Image.LANCZOS)
icon.save(media / "icon.tga", format="TGA")

# Windows/shortcut icon with several embedded sizes.
big = Image.open("assets/tradepost-icon-512.png").convert("RGBA")
big.save("assets/tradepost-icon.ico", format="ICO",
         sizes=[(16, 16), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)])

# Soft white ring drawn from scratch; used with an ADD blend on hover.
scale = 128
ring = Image.new("RGBA", (scale, scale), (255, 255, 255, 0))
draw = ImageDraw.Draw(ring)
draw.ellipse((10, 10, scale - 10, scale - 10),
             outline=(255, 255, 255, 210), width=5)
ring = ring.filter(ImageFilter.GaussianBlur(1.4)).resize((32, 32), Image.LANCZOS)
ring.save(media / "highlight.tga", format="TGA")

for path in (media / "icon.tga", media / "highlight.tga"):
    data = path.read_bytes()
    if data[2] != 2 or data[16] != 32 or (data[17] & 0x0F) != 8:
        raise SystemExit(f"{path} is not an uncompressed 32-bit RGBA TGA")
    if int.from_bytes(data[12:14], "little") != 32 or int.from_bytes(data[14:16], "little") != 32:
        raise SystemExit(f"{path} is not 32x32")

print("wrote icon assets")
PY

echo "done: assets/tradepost-icon.*, addon/TradePost/Media/*.tga, $web/icon.*"
