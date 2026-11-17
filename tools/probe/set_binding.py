#!/usr/bin/env python3
"""Set one binding line to a known-valid command.

Used to distinguish "LoadBindings does not re-read the file" from
"the client drops unknown binding actions". The default key is a reserved
CTRL-ALT-SHIFT combination and the default command is a harmless action bar
binding.
"""

from __future__ import annotations

import argparse
import sys

import common


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--wow", help="WoW install root")
    parser.add_argument("--account", help="account folder name under WTF/Account")
    parser.add_argument("--key", default="CTRL-ALT-SHIFT-F1", help="binding key")
    parser.add_argument("--command", default="ACTIONBUTTON1", help="valid binding command")
    parser.add_argument("--write", action="store_true", help="actually write the file (default: dry run)")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        wow = common.find_wow_dir(args.wow)
        path = common.bindings_path(wow, args.account)
        if not path.is_file():
            print(f"bindings file does not exist: {path}", file=sys.stderr)
            return 1

        lines = common.parse_bindings(common.read_text(path))
        replacement = f"bind {args.key} {args.command}"
        found = False
        old_line = None
        for index, line in enumerate(lines):
            if line.is_bind and line.key == args.key:
                old_line = line.raw
                lines[index] = common.BindingLine(
                    raw=replacement, key=args.key, command=args.command
                )
                found = True
                break

        if not found:
            lines.append(
                common.BindingLine(raw=replacement, key=args.key, command=args.command)
            )

        print(f"bindings: {path}")
        if old_line:
            print(f"  - {old_line}")
        else:
            print("  - (no existing binding)")
        print(f"  + {replacement}")

        if args.write:
            backup = common.backup(path)
            common.write_atomic(path, common.render_bindings(lines))
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
