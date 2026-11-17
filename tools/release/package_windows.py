#!/usr/bin/env python3
"""Package the Windows x64 companion release zip.

The zip contains bfc.exe, the addon sources, and a setup.cmd so a user can
extract it and run one file. Addon folders keep the repository layout because
`bfc install` expects <source>/addon and <source>/data-addon.
"""

from __future__ import annotations

import argparse
import shutil
import sys
import zipfile
from pathlib import Path

README = """Babo Forever Companion {version} (Windows x64)
===========================================

1. Extract this zip somewhere permanent, for example C:\\BaboForever.
2. Close World of Warcraft while the addon is installed.
3. Double-click setup.cmd, or run the commands below in this folder.

Manual setup:

  bfc.exe enroll --server https://babo-forever.b4b0.de --market eu:pve:horde --count 64
  bfc.exe install --count 64
  bfc.exe run

`enroll` finds the WoW install automatically. If it cannot, pass the game
folder explicitly:

  bfc.exe enroll --wow "C:\\Program Files (x86)\\World of Warcraft" ...

The install step writes:

  <WoW>\\Interface\\AddOns\\BaboForever
  <WoW>\\Interface\\AddOns\\BaboForever_Data
  <WoW>\\Interface\\AddOns\\BaboForever_LiveData
  <WoW>\\Interface\\AddOns\\BaboForever_Data_00 .. 63

Then start the game and run /reload. Hover items for prices; /bf browse opens
the in-game browser.

Config:    %AppData%\\babo-forever\\config.json
Dashboard: http://127.0.0.1:8787

`bfc run` keeps running in the console. Close the window or press Ctrl+C to
stop. Data uploads happen after an auction house scan and /bf export.
"""

SETUP = """@echo off
setlocal
cd /d "%~dp0"

echo Babo Forever Companion setup
echo.

bfc.exe enroll --server https://babo-forever.b4b0.de --market eu:pve:horde --count 64
if errorlevel 1 goto error

bfc.exe install --source "%~dp0" --count 64
if errorlevel 1 goto error

echo.
echo Installed. Starting the companion. Close this window or press Ctrl+C to stop.
echo Dashboard: http://127.0.0.1:8787
echo.
bfc.exe run
exit /b 0

:error
echo.
echo Setup failed. Fix the error above and run setup.cmd again.
pause
exit /b 1
"""


def copy_tree(src: Path, dst: Path) -> None:
    if not src.is_dir():
        raise SystemExit(f"missing source directory: {src}")
    shutil.copytree(src, dst)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", default="0.1.0")
    parser.add_argument("--binary", default="dist/bfc.exe")
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

    shutil.copy2(binary, stage / "bfc.exe")
    copy_tree(root / "addon" / "BaboForever", stage / "addon" / "BaboForever")
    copy_tree(root / "data-addon" / "BaboForever_Data", stage / "data-addon" / "BaboForever_Data")
    copy_tree(root / "data-addon" / "BaboForever_LiveData", stage / "data-addon" / "BaboForever_LiveData")
    (stage / "README.txt").write_text(README.format(version=args.version), encoding="utf-8", newline="\r\n")
    (stage / "setup.cmd").write_text(SETUP, encoding="utf-8", newline="\r\n")

    archive = out / f"babo-forever-companion-{args.version}-windows-amd64.zip"
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
