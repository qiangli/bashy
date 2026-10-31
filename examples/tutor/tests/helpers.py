"""Test isolation: every test runs bashy against a temporary home + repo.

Never touches live boards, accounts or server data.
"""
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(HERE / "lib"))


class TempBashy(unittest.TestCase):
    """Sets BASHY_HOME, coord/room dirs, XDG data and a git repo in a temp dir."""

    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory(prefix="tutor-test-")
        root = Path(self._tmp.name)
        self.root = root
        self.repo = root / "repo"
        self.repo.mkdir()
        subprocess.run(["git", "init", "-q", str(self.repo)], check=True)
        self._env = dict(os.environ)
        os.environ.update({
            "BASHY_HOME": str(root / "home"),
            "BASHY_COORD_DIR": str(root / "coord"),
            "BASHY_ROOM_DIR": str(root / "room"),
            "XDG_DATA_HOME": str(root / "xdg"),
            "BASHY_HINTS": "off",
            "BASHY_SPRINT_ANNOUNCE": "0",
            "TUTOR_AGENT": "",
        })
        self._cwd = os.getcwd()
        os.chdir(self.repo)

    def tearDown(self):
        os.chdir(self._cwd)
        os.environ.clear()
        os.environ.update(self._env)
        self._tmp.cleanup()

    def tutor(self, *args, check=True, stdin=None):
        """Run the tutor dispatcher exactly as `bashy tutor` would."""
        proc = subprocess.run(["bashy", "--bashsharp", str(HERE / "tutor.bsh"), *args],
                              input=stdin, text=True, stdout=subprocess.PIPE,
                              stderr=subprocess.PIPE)
        if check and proc.returncode != 0:
            self.fail("tutor %s failed (%d): %s" % (args, proc.returncode, proc.stderr))
        return proc
