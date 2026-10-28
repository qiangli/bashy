#!/usr/bin/env python3
"""Restore a board snapshot into a fresh data directory without overwriting work."""

import json
import os
from pathlib import Path
import re
import sys


def restore(kind: str, data_arg: str, backup_arg: str) -> Path:
    if kind not in {"whiteboard", "brainstorm"}:
        raise ValueError("expected whiteboard or brainstorm")
    data = Path(data_arg).expanduser().absolute()
    backup = Path(backup_arg).expanduser().absolute()
    if not backup.is_dir() or backup.is_symlink():
        raise ValueError("backup must be a real directory")
    if backup == data or backup in data.parents or data == backup.parent:
        raise ValueError("restore into a separate, fresh data directory")

    folder = "boards" if kind == "whiteboard" else "sessions"
    target = data / folder
    if data.exists():
        if data.is_symlink() or not data.is_dir() or any(data.iterdir()):
            raise ValueError("restore data directory must be absent or empty")
    files = list(backup.iterdir())
    if not files:
        raise ValueError("backup is empty")
    if any(not p.is_file() or p.is_symlink() for p in files):
        raise ValueError("backup contains a non-regular file")

    saved = []
    for source in files:
        if kind == "brainstorm" and source.name == "board.json":
            destination = data / "board.json"
        elif source.suffix == ".json" and re.fullmatch(r"[A-Za-z0-9_-]{1,64}", source.stem):
            destination = target / source.name
        else:
            raise ValueError(f"unsupported backup entry: {source.name}")
        raw = source.read_bytes()
        record = json.loads(raw)
        if not isinstance(record, dict) or record.get("version") not in ({1} if kind == "whiteboard" else {1, 2}):
            raise ValueError(f"invalid saved {kind} version: {source.name}")
        if destination.parent == target and record.get("name") != source.stem:
            raise ValueError(f"saved name and filename differ: {source.name}")
        capability = "teacher" if source.name == "board.json" else "facilitator"
        if not isinstance(record.get(capability), str) or not re.fullmatch(r"[0-9a-fA-F]{48}", record[capability]):
            raise ValueError(f"saved facilitator capability missing: {source.name}")
        saved.append((destination, raw))

    # All input is checked before creating the destination. A new directory is
    # required so a restore cannot silently replace newer group work.
    if not data.exists():
        data.mkdir(mode=0o700, parents=True)
    target.mkdir(mode=0o700)
    for destination, raw in saved:
        fd = os.open(destination, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "wb") as output:
            output.write(raw)
            output.flush()
            os.fsync(output.fileno())
        if destination.read_bytes() != raw:
            raise OSError(f"restore verification failed: {destination.name}")
    return data


if __name__ == "__main__":
    if len(sys.argv) != 4:
        sys.exit("usage: restore-board.py whiteboard|brainstorm DATA BACKUP")
    try:
        print(restore(*sys.argv[1:]))
    except (OSError, ValueError, json.JSONDecodeError) as exc:
        sys.exit(f"restore refused: {exc}")
