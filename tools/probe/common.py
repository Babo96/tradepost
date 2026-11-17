"""Shared helpers for the phase 0 probe scripts.

Only the Python standard library is used. Every write is atomic and backed up;
the scripts never touch a line that is not part of the Babo Forever mailbox.
"""

from __future__ import annotations

import datetime as _dt
import os
import re
import shutil
import tempfile
from dataclasses import dataclass
from pathlib import Path

DEFAULT_WOW_DIR = Path("/games/World of Warcraft")
FOREVER_DIR = "_forever_"
ADDONS_DIR = Path("Interface") / "AddOns"
WTF_DIR = Path("WTF")
BINDINGS_FILE = "bindings-cache.wtf"
SAVED_VARIABLES_DIR = "SavedVariables"

# A binding line looks like: bind CTRL-ALT-SHIFT-F1 CLICK BFD1:...:LeftButton
BIND_RE = re.compile(r"^bind\s+(\S+)\s+(.+)$")
SAFE_PAYLOAD_RE = re.compile(r"^[A-Za-z0-9._-]+$")


class ProbeError(RuntimeError):
    """A user-facing probe error."""


@dataclass
class BindingLine:
    raw: str
    key: str | None = None
    command: str | None = None

    @property
    def is_bind(self) -> bool:
        return self.key is not None and self.command is not None


def find_wow_dir(explicit: str | os.PathLike[str] | None = None) -> Path:
    """Locate the WoW install containing the _forever_ game folder."""
    candidates = []
    if explicit:
        candidates.append(Path(explicit))
    if os.environ.get("WOW_DIR"):
        candidates.append(Path(os.environ["WOW_DIR"]))
    candidates.append(DEFAULT_WOW_DIR)

    for candidate in candidates:
        candidate = candidate.expanduser()
        if (candidate / FOREVER_DIR).is_dir():
            return candidate.resolve()

    tried = ", ".join(str(c) for c in candidates)
    raise ProbeError(
        f"could not find a WoW install with {FOREVER_DIR!r} in: {tried}\n"
        "Pass --wow /path/to/World of Warcraft or set WOW_DIR."
    )


def forever_dir(wow_dir: Path) -> Path:
    return wow_dir / FOREVER_DIR


def addons_dir(wow_dir: Path) -> Path:
    return forever_dir(wow_dir) / ADDONS_DIR


def account_dirs(wow_dir: Path) -> list[Path]:
    """Return account directories, newest bindings file first."""
    root = forever_dir(wow_dir) / WTF_DIR / "Account"
    if not root.is_dir():
        raise ProbeError(f"account directory not found: {root}")

    accounts = []
    for entry in root.iterdir():
        if not entry.is_dir() or entry.name == SAVED_VARIABLES_DIR:
            continue
        bindings = entry / BINDINGS_FILE
        savedvars = entry / SAVED_VARIABLES_DIR
        if bindings.is_file() or savedvars.is_dir():
            accounts.append(entry)

    if not accounts:
        raise ProbeError(f"no account directories found under {root}")

    def sort_key(path: Path) -> float:
        bindings = path / BINDINGS_FILE
        try:
            return bindings.stat().st_mtime
        except OSError:
            return 0.0

    return sorted(accounts, key=sort_key, reverse=True)


def pick_account(wow_dir: Path, account: str | None = None) -> Path:
    accounts = account_dirs(wow_dir)
    if account:
        for candidate in accounts:
            if candidate.name == account:
                return candidate
        raise ProbeError(f"account {account!r} not found; available: {', '.join(a.name for a in accounts)}")
    return accounts[0]


def bindings_path(wow_dir: Path, account: str | None = None) -> Path:
    return pick_account(wow_dir, account) / BINDINGS_FILE


def read_text(path: Path) -> str:
    return path.read_text(encoding="utf-8", errors="replace")


def write_atomic(path: Path, text: str) -> None:
    """Write text next to path, fsync, then replace it atomically."""
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, tmp_name = tempfile.mkstemp(prefix=f".{path.name}.", suffix=".tmp", dir=path.parent)
    tmp = Path(tmp_name)
    try:
        with os.fdopen(fd, "w", encoding="utf-8", newline="\n") as handle:
            handle.write(text)
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(tmp, path)
    except Exception:
        try:
            tmp.unlink()
        except OSError:
            pass
        raise


def backup(path: Path, tag: str = "bfbak") -> Path | None:
    if not path.is_file():
        return None
    stamp = _dt.datetime.now().strftime("%Y%m%d-%H%M%S")
    target = path.with_name(f"{path.name}.{tag}-{stamp}")
    shutil.copy2(path, target)
    _prune_backups(path, tag)
    return target


def _prune_backups(path: Path, tag: str, keep: int = 10) -> None:
    pattern = f"{path.name}.{tag}-*"
    backups = sorted(path.parent.glob(pattern))
    for old in backups[:-keep]:
        try:
            old.unlink()
        except OSError:
            pass


def latest_backup(path: Path, tag: str = "bfbak") -> Path | None:
    backups = sorted(path.parent.glob(f"{path.name}.{tag}-*"))
    return backups[-1] if backups else None


def parse_bindings(text: str) -> list[BindingLine]:
    lines: list[BindingLine] = []
    for raw in text.splitlines():
        match = BIND_RE.match(raw.strip())
        if match:
            lines.append(BindingLine(raw=raw, key=match.group(1), command=match.group(2)))
        else:
            lines.append(BindingLine(raw=raw))
    return lines


def render_bindings(lines: list[BindingLine]) -> str:
    body = "\n".join(line.raw for line in lines)
    if not body.endswith("\n"):
        body += "\n"
    return body


def is_mailbox_command(command: str, tag: str = "BFD1") -> bool:
    command = command.strip()
    return command.startswith(tag + ":") or command.startswith(f"CLICK {tag}:")


def strip_mailbox(lines: list[BindingLine], tag: str = "BFD1") -> list[BindingLine]:
    return [
        line
        for line in lines
        if not (line.is_bind and is_mailbox_command(line.command or "", tag))
    ]


def chunk_payload(payload: str, chunk_size: int) -> list[str]:
    if chunk_size < 1:
        raise ProbeError("chunk size must be at least 1")
    return [payload[i : i + chunk_size] for i in range(0, len(payload), chunk_size)]


def mailbox_lines(
    chunks: list[str],
    keys: list[str],
    tag: str = "BFD1",
    seq: int = 1,
    mode: str = "click",
) -> list[BindingLine]:
    if len(chunks) > len(keys):
        raise ProbeError(
            f"payload needs {len(chunks)} chunks but only {len(keys)} keys are available; "
            "reduce --chunk-size or increase --keys"
        )
    total = len(chunks)
    lines: list[BindingLine] = []
    for index, chunk in enumerate(chunks):
        key = keys[index]
        if mode == "raw":
            command = f"{tag}:{seq}:{index}:{total}:{chunk}"
        else:
            command = f"CLICK {tag}:{seq}:{index}:{total}:{chunk}:LeftButton"
        lines.append(BindingLine(raw=f"bind {key} {command}", key=key, command=command))
    return lines


def validate_payload(payload: str) -> None:
    if not payload:
        raise ProbeError("payload must not be empty")
    if not SAFE_PAYLOAD_RE.match(payload):
        raise ProbeError(
            "payload may only contain letters, digits, dot, underscore and dash "
            "(the binding parser treats other characters as syntax)"
        )


def default_keys() -> list[str]:
    letters = list("ABCDEGHJKLMNPQRSUVWXYZ")
    keys = [f"CTRL-ALT-SHIFT-F{i}" for i in range(1, 13)]
    keys += [f"CTRL-ALT-SHIFT-{i}" for i in range(0, 10)]
    keys += [f"CTRL-ALT-SHIFT-{letter}" for letter in letters]
    return keys
