#!/usr/bin/env python3
"""Rewrite the X-BF-Data field of a probe addon TOC."""

from __future__ import annotations

import argparse
import sys

import common


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--wow", help="WoW install root")
    parser.add_argument("--addon", default="BFProbeDataA", help="probe addon folder name")
    parser.add_argument("--payload", required=True, help="new X-BF-Data value")
    parser.add_argument("--write", action="store_true", help="actually write the file (default: dry run)")
    return parser


def replace_metadata(text: str, payload: str) -> str:
    lines = text.splitlines()
    replacement = f"## X-BF-Data: {payload}"
    found = False
    for index, line in enumerate(lines):
        if line.lower().startswith("## x-bf-data:"):
            lines[index] = replacement
            found = True
            break

    if not found:
        insert_at = len(lines)
        for index, line in enumerate(lines):
            if line.strip() and not line.lstrip().startswith("#"):
                insert_at = index
                break
        lines.insert(insert_at, replacement)

    return "\n".join(lines) + "\n"


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        common.validate_payload(args.payload)
        wow = common.find_wow_dir(args.wow)
        path = common.addons_dir(wow) / args.addon / f"{args.addon}.toc"
        if not path.is_file():
            print(f"probe addon not installed: {path}", file=sys.stderr)
            print("run `make probe-install` first", file=sys.stderr)
            return 1

        old = common.read_text(path)
        new = replace_metadata(old, args.payload)

        print(f"addon: {args.addon}")
        print(f"file:  {path}")
        print("changed lines:")
        for old_line, new_line in zip(old.splitlines(), new.splitlines()):
            if old_line != new_line:
                print(f"  - {old_line}")
                print(f"  + {new_line}")

        if args.write:
            backup = common.backup(path)
            common.write_atomic(path, new)
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
