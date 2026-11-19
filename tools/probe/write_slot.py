#!/usr/bin/env python3
"""Rewrite a LoadOnDemand probe slot while the game is running."""

from __future__ import annotations

import argparse
import json
import sys
import time
from pathlib import Path

import common


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--wow", help="WoW install root")
    parser.add_argument("--slot", choices=("A", "B", "C", "D"), default="A")
    parser.add_argument("--payload", required=True, help="payload value written into Data.lua")
    parser.add_argument("--stamp", type=int, default=None, help="numeric stamp (default: current epoch)")
    parser.add_argument("--write", action="store_true", help="actually write the file (default: dry run)")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        common.validate_payload(args.payload)
        wow = common.find_wow_dir(args.wow)
        addon = f"TPProbeData{args.slot}"
        path = common.addons_dir(wow) / addon / "Data.lua"
        if not path.is_file():
            print(f"probe addon not installed: {path}", file=sys.stderr)
            print("run `make probe-install` first", file=sys.stderr)
            return 1

        stamp = args.stamp if args.stamp is not None else int(time.time())
        content = (
            "-- Generated probe payload. The host script tools/probe/write_slot.py rewrites\n"
            "-- this file while the game is running to test mid-session LoadAddOn behaviour.\n"
            "\n"
            f"{addon}_Payload = {json.dumps(args.payload)}\n"
            f"{addon}_Stamp = {stamp}\n"
        )

        print(f"slot:  {addon}")
        print(f"file:  {path}")
        print(f"stamp: {stamp}")
        print("new content:")
        print(content)

        if args.write:
            backup = common.backup(path)
            common.write_atomic(path, content)
            if backup:
                print(f"backup: {backup}")
            print("wrote")
        else:
            print("dry run; pass --write to update")
        return 0
    except common.ProbeError as error:
        print(f"error: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
