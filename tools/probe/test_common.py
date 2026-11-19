"""Unit tests for the probe helpers. Run with: make test"""

from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

import common
import write_toc


class ProbeHelpersTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.wow = Path(self.tmp.name) / "World of Warcraft"
        self.account = self.wow / "_forever_" / "WTF" / "Account" / "123#1"
        self.account.mkdir(parents=True)
        self.bindings = self.account / "bindings-cache.wtf"
        self.bindings.write_text(
            "bind Q MULTIACTIONBAR1BUTTON1\n"
            "bind CTRL-ALT-SHIFT-F1 CLICK TPD1:1:0:1:OLD:LeftButton\n"
            "bind E MULTIACTIONBAR1BUTTON2\n",
            encoding="utf-8",
        )

    def tearDown(self) -> None:
        self.tmp.cleanup()

    def test_find_wow_dir(self) -> None:
        self.assertEqual(common.find_wow_dir(self.wow), self.wow.resolve())

    def test_pick_account(self) -> None:
        self.assertEqual(common.pick_account(self.wow), self.account)
        self.assertEqual(common.bindings_path(self.wow), self.bindings)

    def test_parse_preserves_user_lines(self) -> None:
        lines = common.parse_bindings(common.read_text(self.bindings))
        self.assertEqual(lines[0].key, "Q")
        self.assertEqual(lines[0].command, "MULTIACTIONBAR1BUTTON1")
        self.assertTrue(lines[1].is_bind)
        self.assertEqual(lines[2].key, "E")

    def test_strip_only_mailbox_lines(self) -> None:
        lines = common.parse_bindings(common.read_text(self.bindings))
        kept = common.strip_mailbox(lines)
        self.assertEqual(len(kept), 2)
        self.assertNotIn("TPD1", common.render_bindings(kept))

    def test_mailbox_lines(self) -> None:
        chunks = common.chunk_payload("ABCDEFGH", 3)
        self.assertEqual(chunks, ["ABC", "DEF", "GH"])
        lines = common.mailbox_lines(chunks, ["K1", "K2", "K3"], seq=7)
        self.assertEqual(
            lines[1].raw,
            "bind K2 CLICK TPD1:7:1:3:DEF:LeftButton",
        )

    def test_write_atomic_and_backup(self) -> None:
        backup = common.backup(self.bindings)
        self.assertIsNotNone(backup)
        self.assertTrue(backup.is_file())

        common.write_atomic(self.bindings, "bind A B\n")
        self.assertEqual(self.bindings.read_text(encoding="utf-8"), "bind A B\n")
        self.assertEqual(common.latest_backup(self.bindings), backup)

    def test_validate_payload(self) -> None:
        common.validate_payload("ABC123._-")
        for bad in ("", "has space", "has:colon", "has/slash"):
            with self.assertRaises(common.ProbeError):
                common.validate_payload(bad)

    def test_replace_toc_metadata(self) -> None:
        toc = "## Interface: 16001\n## Title: Probe\n\nData.lua\n"
        updated = write_toc.replace_metadata(toc, "NEW_VALUE")
        self.assertIn("## X-TP-Data: NEW_VALUE", updated)
        self.assertTrue(updated.endswith("Data.lua\n"))

        updated_again = write_toc.replace_metadata(updated, "OTHER")
        self.assertIn("## X-TP-Data: OTHER", updated_again)
        self.assertEqual(updated_again.count("## X-TP-Data: OTHER"), 1)


if __name__ == "__main__":
    unittest.main()
