#!/usr/bin/env python3
"""Compare two directory trees without dereferencing symlinks.

`diff -r` follows symlinks, so two identical pnpm trees fail the comparison
with exit 2 on dangling links (e.g. node_modules/.pnpm/node_modules/* whose
targets only exist at install time): "No such file or directory". This script
instead builds a manifest of (path, type, sha256-or-link-target) for each
tree using lstat only — a dangling link is never resolved — and fails only on
real differences: missing/extra paths, type changes, changed file bytes, or
changed link targets.

Usage: compare_trees.py <tree-a> <tree-b>
Exit 0 when the trees are identical, 1 when they differ, 2 on usage/IO error.
"""

import hashlib
import os
import sys

CHUNK = 65536


def sha256_of(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(CHUNK), b""):
            h.update(chunk)
    return h.hexdigest()


def manifest(root):
    """Map relpath -> record string. Never follows symlinks."""
    entries = {}
    stack = [root]
    while stack:
        cur = stack.pop()
        with os.scandir(cur) as it:
            for e in it:
                rel = os.path.relpath(e.path, root)
                try:
                    if e.is_symlink():
                        entries[rel] = "symlink:" + os.readlink(e.path)
                    elif e.is_dir(follow_symlinks=False):
                        entries[rel] = "dir"
                        stack.append(e.path)
                    elif e.is_file(follow_symlinks=False):
                        entries[rel] = "file:sha256:" + sha256_of(e.path)
                    else:
                        st = e.stat(follow_symlinks=False)
                        entries[rel] = "other:%s:%s" % (
                            oct(st.st_mode & 0o7777),
                            ",".join(
                                n
                                for n in ("fifo", "socket", "chr", "blk")
                                if getattr(os, "S_IS" + n.upper())(st.st_mode)
                            ),
                        )
                except OSError as exc:
                    raise OSError("cannot record %s: %s" % (e.path, exc))
    return entries


def compare(a, b):
    ma, mb = manifest(a), manifest(b)
    problems = []
    for path in sorted(set(ma) | set(mb)):
        if path not in ma:
            problems.append("only in %s: %s" % (b, path))
        elif path not in mb:
            problems.append("only in %s: %s" % (a, path))
        elif ma[path] != mb[path]:
            problems.append("differs: %s (%s != %s)" % (path, ma[path], mb[path]))
    return problems


def main(argv):
    if len(argv) != 3:
        print("usage: compare_trees.py <tree-a> <tree-b>", file=sys.stderr)
        return 2
    for t in argv[1:]:
        if not os.path.isdir(t):
            print("compare_trees: not a directory: %s" % t, file=sys.stderr)
            return 2
    try:
        problems = compare(argv[1], argv[2])
    except OSError as exc:
        print("compare_trees: %s" % exc, file=sys.stderr)
        return 2
    if problems:
        for p in problems:
            print(p)
        print(
            "compare_trees: %d difference(s) between %s and %s"
            % (len(problems), argv[1], argv[2]),
            file=sys.stderr,
        )
        return 1
    print("compare_trees: %s and %s are identical" % (argv[1], argv[2]))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
