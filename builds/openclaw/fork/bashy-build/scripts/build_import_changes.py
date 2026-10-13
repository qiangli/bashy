#!/usr/bin/env python3
"""build_import_changes.py — runtime-config records for `docker import`.

`docker import` joins every --change argument with newlines and parses the
result as Dockerfile instructions (moby BuildFromConfig), so each record must
be ONE complete instruction whose words the Dockerfile parser tokenizes back
to the source config exactly. A LABEL value with spaces passed unquoted
splits into bare words and fails the import with:
  Syntax error - can't find = in "gateway". Must be of the form: name=value
(CI run 38469059131: org.opencontainers.image.description holds
"OpenClaw gateway and CLI runtime container image").

dfile_word() quotes a word with double quotes when it holds whitespace,
quotes, backslash, dollar, or hash, escaping backslash, double quote, and
dollar so the daemon's shell expansion reproduces the value byte-for-byte
(backtick needs no escape: the expansion passes it through literally).
Newlines cannot survive the newline-joined parse, so they fail closed here
instead of corrupting a record mid-list. CMD/ENTRYPOINT always use the JSON
array form, which the parser reads as JSON, never as words.

Usage:
  build_import_changes.py SOURCE_CONFIG_JSON OUT_FILE
writes the records NUL-separated (values may hold tabs/spaces; only NUL is
impossible in JSON strings, so it is the only safe separator).
"""

import argparse
import json
import sys


def dfile_word(s):
    if "\n" in s or "\r" in s:
        raise ValueError(f"docker --change cannot carry a newline: {s!r}")
    if s == "" or any(c.isspace() or c in "\"'\\$#" for c in s):
        return '"' + s.replace("\\", "\\\\").replace('"', '\\"').replace("$", "\\$") + '"'
    return s


def build_changes(cfg):
    """[str]: one complete `docker import --change` instruction each."""
    changes = []
    for e in cfg.get("Env") or []:
        k, _, v = e.partition("=")
        changes.append(f"ENV {dfile_word(k)}={dfile_word(v)}")
    if cfg.get("User"):
        changes.append(f"USER {dfile_word(cfg['User'])}")
    if cfg.get("WorkingDir"):
        changes.append(f"WORKDIR {dfile_word(cfg['WorkingDir'])}")
    if cfg.get("Entrypoint") is not None:
        changes.append(f"ENTRYPOINT {json.dumps(cfg['Entrypoint'])}")
    if cfg.get("Cmd") is not None:
        changes.append(f"CMD {json.dumps(cfg['Cmd'])}")
    for k in sorted(cfg.get("Labels") or {}):
        changes.append(f"LABEL {dfile_word(k)}={dfile_word(cfg['Labels'][k])}")
    return changes


def main(argv=None):
    p = argparse.ArgumentParser(description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("source_config", help="docker inspect .Config JSON file")
    p.add_argument("out_file", help="output file for NUL-separated records")
    args = p.parse_args(argv)
    with open(args.source_config, encoding="utf-8") as f:
        cfg = json.load(f)
    try:
        changes = build_changes(cfg)
    except ValueError as e:
        print(f"build_import_changes: FAIL: {e}", file=sys.stderr)
        return 1
    with open(args.out_file, "wb") as f:
        for c in changes:
            f.write(c.encode("utf-8") + b"\0")
    return 0


if __name__ == "__main__":
    sys.exit(main())
