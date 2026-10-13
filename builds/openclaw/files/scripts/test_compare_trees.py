#!/usr/bin/env python3
"""Tests for compare_trees.py: dangling symlinks compare as links."""

import os
import shutil
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
SCRIPT = os.path.join(HERE, "compare_trees.py")

# Dangling pnpm links of the shape that broke `diff -r` with exit 2 in
# CI run 38464949834 (No such file or directory).
DANGLING = [
    ("node_modules/.pnpm/node_modules/openclaw-control-ui", "../../missing-target"),
    ("node_modules/@openclaw/ai/node_modules/@anthropic-ai+sdk", "nonexistent-sdk"),
    ("node_modules/@openclaw/ai/node_modules/openai", "../nowhere-openai"),
]


def build_tree(root):
    os.makedirs(os.path.join(root, "dist"))
    with open(os.path.join(root, "openclaw.mjs"), "w") as f:
        f.write("export {};\n")
    with open(os.path.join(root, "dist", "index.js"), "w") as f:
        f.write("console.log('ship');\n")
    for rel, target in DANGLING:
        full = os.path.join(root, rel)
        os.makedirs(os.path.dirname(full), exist_ok=True)
        os.symlink(target, full)


def run(a, b):
    return subprocess.run(
        [sys.executable, SCRIPT, a, b], capture_output=True, text=True
    )


class CompareTrees(unittest.TestCase):
    def setUp(self):
        self.d = tempfile.mkdtemp()
        self.a = os.path.join(self.d, "a")
        self.b = os.path.join(self.d, "b")
        build_tree(self.a)
        shutil.copytree(self.a, self.b, symlinks=True)

    def tearDown(self):
        shutil.rmtree(self.d, ignore_errors=True)

    def test_identical_trees_with_dangling_links_compare_equal(self):
        # The CI failure: plain `diff -r` exits 2 here; the link-aware
        # comparison must exit 0.
        for rel, _ in DANGLING:
            self.assertTrue(
                os.path.islink(os.path.join(self.a, rel)),
                "%s must stay a link in the fixture" % rel,
            )
            self.assertFalse(
                os.path.exists(os.path.join(self.a, rel)),
                "%s must dangle in the fixture" % rel,
            )
        p = run(self.a, self.b)
        self.assertEqual(p.returncode, 0, p.stdout + p.stderr)

    def test_changed_file_bytes_fail(self):
        with open(os.path.join(self.b, "dist", "index.js"), "w") as f:
            f.write("console.log('tampered');\n")
        p = run(self.a, self.b)
        self.assertEqual(p.returncode, 1, p.stdout + p.stderr)
        self.assertIn("dist/index.js", p.stdout)

    def test_changed_link_target_fails(self):
        rel, _ = DANGLING[0]
        os.remove(os.path.join(self.b, rel))
        os.symlink("../../other-target", os.path.join(self.b, rel))
        p = run(self.a, self.b)
        self.assertEqual(p.returncode, 1, p.stdout + p.stderr)
        self.assertIn(rel, p.stdout)

    def test_missing_and_extra_paths_fail(self):
        os.remove(os.path.join(self.b, "openclaw.mjs"))
        with open(os.path.join(self.b, "extra.js"), "w") as f:
            f.write("x = 1;\n")
        p = run(self.a, self.b)
        self.assertEqual(p.returncode, 1, p.stdout + p.stderr)
        self.assertIn("openclaw.mjs", p.stdout)
        self.assertIn("extra.js", p.stdout)

    def test_file_replaced_by_symlink_fails(self):
        target = os.path.join(self.b, "dist", "index.js")
        os.remove(target)
        os.symlink("nowhere", target)
        p = run(self.a, self.b)
        self.assertEqual(p.returncode, 1, p.stdout + p.stderr)

    def test_missing_tree_is_an_error(self):
        p = run(self.a, os.path.join(self.d, "no-such-tree"))
        self.assertEqual(p.returncode, 2, p.stdout + p.stderr)


if __name__ == "__main__":
    unittest.main()
