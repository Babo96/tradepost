#!/usr/bin/env python3
"""Write or inspect the Babo Forever bindings mailbox.

The script is deliberately conservative:

* it only removes lines whose command starts with ``BFD1:`` (or ``CLICK BFD1:``)
* it preserves every other line byte-for-byte
* it makes a timestamped backup before writing
* ``--write`` is required to change anything

Examples
--------
List accounts and current mailbox lines::

    python3 tools/probe/rewrite_bindings.py --show

Write a four-chunk mailbox payload while the game is running::

    python3 tools/probe/rewrite_bindings.py --payload TEST123 --keys 4 --write

Restore the newest backup::

    python3 tools/probe/rewrite_bindings.py --restore
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

import common


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--wow", help="WoW install root (default: /games/World of Warcraft or $WOW_DIR)")
    parser.add_argument("--account", help="account folder name under WTF/Account")
    parser.add_argument("--payload", help="ASCII payload to place in the mailbox")
    parser.add_argument("--keys", type=int, default=4, help="number of reserved keys to use (default: 4)")
    parser.add_argument("--chunk-size", type=int, default=200, help="characters per chunk (default: 200)")
    parser.add_argument("--seq", type=int, default=1, help="mailbox sequence number (default: 1)")
    parser.add_argument("--mode", choices=("click", "raw"), default="click", help="CLICK action or raw command")
    parser.add_argument("--write", action="store_true", help="actually write the file (default: dry run)")
    parser.add_argument("--clear", action="store_true", help="remove mailbox lines and exit")
    parser.add_argument("--restore", action="store_true", help="restore the newest backup and exit")
    parser.add_argument("--show", action="store_true", help="show the current mailbox lines")
    parser.add_argument("--list-accounts", action="store_true", help="list accounts and exit")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)

    try:
        wow = common.find_wow_dir(args.wow)
        accounts = common.account_dirs(wow)
        if args.list_accounts:
            print(f"WoW: {wow}")
            for account in accounts:
                print(f"  {account.name}  ({account})")
            return 0

        path = common.bindings_path(wow, args.account)
        print(f"WoW:      {wow}")
        print(f"account:  {path.parent.name}")
        print(f"bindings: {path}")

        if args.restore:
            backup = common.latest_backup(path)
            if backup is None:
                print("no backup found", file=sys.stderr)
                return 1
            common.write_atomic(path, common.read_text(backup))
            print(f"restored {backup.name} -> {path.name}")
            return 0

        if not path.is_file():
            print(f"bindings file does not exist: {path}", file=sys.stderr)
            return 1

        lines = common.parse_bindings(common.read_text(path))

        if args.clear:
            kept = common.strip_mailbox(lines)
            removed = len(lines) - len(kept)
            print(f"mailbox lines found: {removed}")
            if args.write:
                common.backup(path)
                common.write_atomic(path, common.render_bindings(kept))
                print("cleared")
            else:
                print("dry run; pass --write to clear")
            return 0

        if args.payload:
            common.validate_payload(args.payload)
            available = common.default_keys()
            if args.keys < 1 or args.keys > len(available):
                print(f"--keys must be between 1 and {len(available)}", file=sys.stderr)
                return 2
            keys = available[: args.keys]
            chunks = common.chunk_payload(args.payload, args.chunk_size)
            new_lines = common.mailbox_lines(
                chunks, keys, tag="BFD1", seq=args.seq, mode=args.mode
            )
            kept = common.strip_mailbox(lines)
            result = kept + new_lines

            print(f"payload: {len(args.payload)} chars, {len(chunks)} chunk(s), {len(keys)} key(s), seq={args.seq}")
            for line in new_lines:
                print(f"  + {line.raw}")

            if args.write:
                backup = common.backup(path)
                common.write_atomic(path, common.render_bindings(result))
                if backup:
                    print(f"backup: {backup}")
                print(f"wrote {path}")
            else:
                print("dry run; pass --write to update the bindings file")
            return 0

        # Default: show mailbox lines.
        mailbox = [line for line in lines if line.is_bind and common.is_mailbox_command(line.command or "")]
        if not mailbox:
            print("no BFD1 mailbox lines")
        else:
            print(f"BFD1 mailbox lines: {len(mailbox)}")
            for line in mailbox:
                print(f"  {line.raw}")
        return 0

    except common.ProbeError as error:
        print(f"error: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
