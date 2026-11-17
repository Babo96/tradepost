#!/usr/bin/env python3
"""Write one ring slot, simulating the companion.

Example:
  python3 tools/ring/write_slot.py \
    --market eu:pve:horde \
    --items "i:2589,1234,50,2;i:4496,5000,3,1" \
    --write

The window defaults to the current 5-minute window and the slot to
window % count. The addon loads the slot for the completed window at the next
boundary, or immediately with `/bf ring now` (current window).
"""

from __future__ import annotations

import argparse
import json
import re
import sys
import time
from pathlib import Path

import gen_slots

KEY_RE = re.compile(r"^[A-Za-z0-9:]+$")


def parse_items(value: str, stamp: int) -> list[dict]:
    items = []
    for chunk in value.split(";"):
        chunk = chunk.strip()
        if not chunk:
            continue
        parts = [part.strip() for part in chunk.split(",")]
        if len(parts) < 3:
            raise ValueError(f"item needs key,price,qty: {chunk!r}")
        key = parts[0]
        if not KEY_RE.match(key):
            raise ValueError(f"invalid item key: {key!r}")
        price = int(parts[1])
        quantity = int(parts[2])
        listings = int(parts[3]) if len(parts) > 3 else 1
        item_stamp = int(parts[4]) if len(parts) > 4 else stamp
        items.append(
            {
                "k": key,
                "p": price,
                "q": quantity,
                "n": listings,
                "t": item_stamp,
            }
        )
    return items


def lua_table(items: list[dict]) -> str:
    if not items:
        return "{}"
    lines = ["{"]
    for item in items:
        lines.append(
            "    {{ k = {k}, p = {p}, q = {q}, n = {n}, t = {t} }},".format(
                k=json.dumps(item["k"]),
                p=item["p"],
                q=item["q"],
                n=item["n"],
                t=item["t"],
            )
        )
    lines.append("  }")
    return "\n".join(lines)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--wow", default=str(gen_slots.DEFAULT_WOW))
    parser.add_argument("--count", type=int, default=64)
    parser.add_argument("--window", type=int, default=None)
    parser.add_argument("--slot", type=int, default=None)
    parser.add_argument("--market", default="eu:pve:horde")
    parser.add_argument("--seq", type=int, default=1)
    parser.add_argument("--base", type=int, default=0)
    parser.add_argument("--generated", type=int, default=None)
    parser.add_argument("--items", default="i:2589,1234,50,2")
    parser.add_argument("--write", action="store_true")
    args = parser.parse_args(argv)

    if args.count < 1 or args.count > 256:
        print("count must be between 1 and 256", file=sys.stderr)
        return 2

    window = args.window if args.window is not None else int(time.time() // 300)
    slot = args.slot if args.slot is not None else window % args.count
    if slot < 0 or slot >= args.count:
        print("slot out of range", file=sys.stderr)
        return 2

    stamp = args.generated if args.generated is not None else int(time.time())
    try:
        items = parse_items(args.items, stamp)
    except ValueError as error:
        print(f"error: {error}", file=sys.stderr)
        return 2

    wow = Path(args.wow).expanduser()
    addons = gen_slots.addons_dir(wow)
    name = gen_slots.SLOT_PREFIX + f"{slot:02d}"
    path = addons / name / "Data.lua"
    if not path.parent.is_dir():
        print(f"slot not installed: {path.parent}", file=sys.stderr)
        print("run `make ring-install` first", file=sys.stderr)
        return 1

    content = f"""-- Generated live ring slot. The companion rewrites this file.
BABOFOREVER_RING = {{
  schema = 1,
  market = {json.dumps(args.market)},
  window = {window},
  generated = {stamp},
  base = {args.base},
  seq = {args.seq},
  items = {lua_table(items)},
  removed = {{}},
  pins = {{}},
}}
"""

    print(f"slot:    {name}")
    print(f"file:    {path}")
    print(f"window:  {window}")
    print(f"seq:     {args.seq}")
    print(f"items:   {len(items)}")
    print(content)

    if args.write:
        import tempfile

        fd, tmp_name = tempfile.mkstemp(prefix=".Data.lua.", suffix=".tmp", dir=path.parent)
        tmp = Path(tmp_name)
        try:
            with open(fd, "w", encoding="utf-8", newline="\n") as handle:
                handle.write(content)
            if path.is_file():
                backup = path.with_name(f"{path.name}.bfbak-{int(time.time())}")
                backup.write_text(path.read_text(encoding="utf-8"), encoding="utf-8")
            tmp.replace(path)
        finally:
            if tmp.exists():
                tmp.unlink()
        print("wrote")
    else:
        print("dry run; pass --write to update")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
