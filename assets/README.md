# TradePost brand assets

Original TradePost artwork, not a Blizzard texture.

| File | Use |
| --- | --- |
| `tradepost-icon.svg` | master vector mark (512×512) |
| `tradepost-icon-small.svg` | simplified cut for 32px, favicon and the game |
| `tradepost-icon-512.png` | CurseForge/WoWUI project icon, social image |
| `tradepost-icon-192.png` | Apple touch icon / large web icon |
| `tradepost-icon-32.png` | favicon |
| `tradepost-icon.ico` | Windows shortcut icon |
| `../companion/cmd/tradepost/rsrc_windows_amd64.syso` | Embedded Windows executable icon |
| `../addon/TradePost/Media/icon.tga` | in-game 32×32 TGA (`## IconTexture`) |
| `../addon/TradePost/Media/highlight.tga` | minimap hover ring |

Regenerate every derivative from the SVG sources:

```sh
make icon
```

The renderer is `tools/assets/render_icon.sh` and needs `rsvg-convert` plus
Python 3 with Pillow. `make icon` also regenerates the Windows `.syso` resource
via `rsrc`, which needs Go.
