#!/usr/bin/env python3
"""Package the Windows x64 companion release zip.

The zip contains tradepost.exe, the addon sources, and a setup.cmd so a user can
extract it and run one file. Addon folders keep the repository layout because
`tradepost install` expects <source>/addon and <source>/data-addon.
"""

from __future__ import annotations

import argparse
import shutil
import sys
import zipfile
from pathlib import Path

README = """TradePost Companion {version} (Windows x64)
===========================================

1. Extract this zip somewhere permanent, for example C:\\TradePost.
2. Close World of Warcraft while the addon is installed.
3. Run tradepost.exe (or setup.cmd).

The companion starts in the Windows notification area. Left-click the tray icon
to open the dashboard; right-click for Open dashboard / Sync now / Quit. It
opens a local setup wizard at http://127.0.0.1:8787/setup. Select the WoW
folder, server URL, market and ring slots there. No command-line setup is
required.

Starting tradepost.exe again while it is running opens the dashboard instead of
creating a second tray icon.

If you prefer the command line:

  tradepost.exe enroll --server https://tradepost.b4b0.de --market eu:pve:horde --count 64
  tradepost.exe install --count 64
  tradepost.exe run

The install step writes:

  <WoW>\\Interface\\AddOns\\TradePost
  <WoW>\\Interface\\AddOns\\TradePost_Data
  <WoW>\\Interface\\AddOns\\TradePost_LiveData
  <WoW>\\Interface\\AddOns\\TradePost_Data_00 .. 63

Then start the game and run /reload. Hover items for prices; /tradepost browse opens
the in-game browser.

Config:    %AppData%\\tradepost\\config.json
Log:       %AppData%\\tradepost\\companion.log
Dashboard: http://127.0.0.1:8787

The tray icon keeps running in the background; no terminal window is shown.
Data uploads happen after an auction house scan and /tradepost export.
"""

SETUP = """@echo off
setlocal
cd /d "%~dp0"

echo TradePost Companion
echo.
echo Starting TradePost in the notification area.
echo Left-click the tray icon to open the dashboard.
echo.
start "" "%~dp0tradepost.exe"
exit /b 0
"""


def copy_tree(src: Path, dst: Path) -> None:
    if not src.is_dir():
        raise SystemExit(f"missing source directory: {src}")
    shutil.copytree(src, dst)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", default="0.1.0")
    parser.add_argument("--binary", default="dist/tradepost.exe")
    parser.add_argument("--root", default=".")
    parser.add_argument("--out", default="dist")
    args = parser.parse_args(argv)

    root = Path(args.root).resolve()
    binary = Path(args.binary)
    if not binary.is_absolute():
        binary = (root / binary).resolve()
    if not binary.is_file():
        raise SystemExit(f"missing binary: {binary} (run make companion-build-windows)")

    out = (root / args.out).resolve()
    out.mkdir(parents=True, exist_ok=True)
    stage = out / "windows-stage"
    shutil.rmtree(stage, ignore_errors=True)
    stage.mkdir(parents=True)

    shutil.copy2(binary, stage / "tradepost.exe")
    copy_tree(root / "addon" / "TradePost", stage / "addon" / "TradePost")
    copy_tree(root / "data-addon" / "TradePost_Data", stage / "data-addon" / "TradePost_Data")
    copy_tree(root / "data-addon" / "TradePost_LiveData", stage / "data-addon" / "TradePost_LiveData")
    (stage / "README.txt").write_text(README.format(version=args.version), encoding="utf-8", newline="\r\n")
    (stage / "setup.cmd").write_text(SETUP, encoding="utf-8", newline="\r\n")

    archive = out / f"tradepost-companion-{args.version}-windows-amd64.zip"
    archive.unlink(missing_ok=True)
    with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as zf:
        for path in sorted(stage.rglob("*")):
            if path.is_file():
                zf.write(path, path.relative_to(stage))
    shutil.rmtree(stage, ignore_errors=True)

    size = archive.stat().st_size
    print(f"packaged {archive} ({size} bytes)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
