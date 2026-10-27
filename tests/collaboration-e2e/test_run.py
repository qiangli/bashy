"""Offline Go selection checks for the combined collaboration gate."""
import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock


SPEC = importlib.util.spec_from_file_location("collaboration_run", Path(__file__).with_name("run.py"))
runner = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(runner)


class OfflineGoTests(unittest.TestCase):
    def test_cached_toolchain_is_used_for_offline_build(self):
        with tempfile.TemporaryDirectory() as temp:
            cache = Path(temp)
            local = cache / "local-go"
            local.touch(mode=0o755)
            cached = cache / "golang.org/toolchain@v0.0.1-go1.27.2.darwin-arm64/bin/go"
            cached.parent.mkdir(parents=True)
            cached.touch(mode=0o755)
            calls = []

            def fake_command(argv, **kwargs):
                calls.append((argv, kwargs))
                self.assertEqual({key: kwargs["env"][key] for key in
                                  ("GOTOOLCHAIN", "GOPROXY", "GOSUMDB")},
                                 {"GOTOOLCHAIN": "local", "GOPROXY": "off", "GOSUMDB": "off"})
                if argv[1:] == ["env", "GOMODCACHE"]:
                    return subprocess.CompletedProcess(argv, 0, str(cache) + "\n")
                if argv[1:] == ["version"]:
                    version = "go1.26.0" if argv[0] == str(local) else "go1.27.2"
                    return subprocess.CompletedProcess(argv, 0, f"go version {version} darwin/arm64\n")
                return subprocess.CompletedProcess(argv, 0)

            with mock.patch.object(runner.shutil, "which", return_value=str(local)), \
                    mock.patch.object(runner, "command", side_effect=fake_command):
                selected = runner.offline_go()
                self.assertEqual(selected, cached)
                runner.build("whiteboard", cache / "output", selected)
            self.assertEqual(calls[-1][0][0], str(cached))
            self.assertEqual(calls[-1][0][1], "build")

    def test_missing_usable_offline_toolchain_blocks(self):
        with tempfile.TemporaryDirectory() as temp:
            local = Path(temp) / "local-go"
            local.touch(mode=0o755)

            def fake_command(argv, **kwargs):
                if argv[1:] == ["env", "GOMODCACHE"]:
                    return subprocess.CompletedProcess(argv, 0, temp + "\n")
                return subprocess.CompletedProcess(argv, 0, "go version go1.26.0 darwin/arm64\n")

            with mock.patch.object(runner.shutil, "which", return_value=str(local)), \
                    mock.patch.object(runner, "command", side_effect=fake_command):
                with self.assertRaisesRegex(runner.Blocked, "no usable local or cached toolchain"):
                    runner.offline_go()


if __name__ == "__main__":
    unittest.main()
