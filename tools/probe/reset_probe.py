#!/usr/bin/env python3
"""Remove probe bindings from bindings-cache.wtf.

Removes reserved CTRL-ALT-SHIFT-* lines and mailbox lines. User bindings are
preserved. A backup is made before writing.
"""

from __future__ import annotations

import argparse
import sys

import common


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--wow", help="WoW install root")
    parser.add_argument("--account", help="account folder name under WTF/Account")
    parser.add_argument("--write", action="store_true", help="actually write the file")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        wow = common.find_wow_dir(args.wow)
        path = common.bindings_path(wow, args.account)
        lines = common.parse_bindings(common.read_text(path))

        kept = []
        removed = []
        for line in lines:
            if line.is_bind and (
                (line.key or "").startswith("CTRL-ALT-SHIFT-")
                or common.is_mailbox_command(line.command or "")
            ):
                removed.append(line.raw)
            else:
                kept.append(line)

        print(f"bindings: {path}")
        print(f"probe lines removed: {len(removed)}")
        for raw in removed:
            print(f"  - {raw}")

        if args.write:
            if removed:
                backup = common.backup(path)
                common.write_atomic(path, common.render_bindings(kept))
                if backup:
                    print(f"backup: {backup}")
                print("wrote")
            else:
                print("nothing to do")
        else:
            print("dry run; pass --write to clean")
        return 0
    except common.ProbeError as error:
        print(f"error: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
