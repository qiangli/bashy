"""Focused tests for `klatch serve`: pinned fetch, safe extract, reach choice, unit shape.

No network, host or real service: a fake release opener, a recording runner and
a stubbed admin setup stand in for them.
"""
import argparse
import hashlib
import io
import json
import sys
import tarfile
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import serve  # noqa: E402


def tgz(files, links=()):
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w:gz") as tar:
        for name, body in files.items():
            info = tarfile.TarInfo(name)
            info.size = len(body)
            info.mode = 0o755 if name.endswith(".sh") or name.endswith("klatch") else 0o644
            tar.addfile(info, io.BytesIO(body))
        for name, target in links:
            info = tarfile.TarInfo(name)
            info.type = tarfile.SYMTYPE
            info.linkname = target
            tar.addfile(info)
    return buf.getvalue()


SERVER = tgz({"./klatch": b"bin", "./transform/transform_icopy.sh": b"#!/bin/sh\n"})
WEB = tgz({"index.html": b"<html>", "assets/app.js": b"js"})
SHA = {name: hashlib.sha256(body).hexdigest() for name, body in (("server", SERVER), ("web", WEB))}
SERVER_URL = "https://example.invalid/r/klatch-v9-test-arm64.tar.gz"
WEB_URL = "https://example.invalid/r/klatch-web-v9.tar.gz"


class FakeResponse(io.BytesIO):
    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False


def opener(url, timeout=None):
    return FakeResponse({SERVER_URL: SERVER, WEB_URL: WEB}[url])


class Runner:
    def __init__(self, binary="/pinned/klatch"):
        self.calls, self.binary, self.secrets = [], binary, {}

    def __call__(self, *args, stdin=None, check=True):
        self.calls.append((args, stdin))
        if args[:2] == ("tools", "show"):
            return json.dumps({"cli": {"binary": self.binary, "managed": {
                "version": "9-bashy.1",
                "assets": {"test/arm64": {"url": SERVER_URL, "sha256": SHA["server"]}},
                "notes": "web bundle. Web asset URL: %s (sha256 %s)." % (WEB_URL, SHA["web"])}}})
        if args[:2] == ("secret", "show"):
            return self.secrets.get(args[2], "")
        if args[:2] == ("secret", "gen"):
            return "generated\n"
        if args[:2] == ("secret", "set"):
            self.secrets[args[2]] = stdin
        return ""

    def verbs(self):
        return [" ".join(a[:3]) for a, _ in self.calls]


def args(data, **kw):
    base = dict(data=str(data), port=7199, lan=False, route=None, domain=None,
                min_free=0.0, admin_ref="klatch/admin", url_ref="klatch/url")
    base.update(kw)
    return argparse.Namespace(**base)


class ServeTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.data = Path(self.tmp.name) / "klatch"
        self.setups = []
        serve.platform_key = lambda: "test/arm64"

    def tearDown(self):
        self.tmp.cleanup()

    def run_serve(self, runner=None, **kw):
        runner = runner or Runner()
        out = serve.serve(args(self.data, **kw), runner=runner, opener=opener,
                          healthy=lambda base: True,
                          setup=lambda *a: self.setups.append(a))
        return out, runner

    def test_loopback_default_installs_signed_pin_and_writes_unit(self):
        out, r = self.run_serve()
        self.assertEqual(out["reach"], "loopback")
        self.assertEqual(out["url"], "http://127.0.0.1:7199")
        self.assertIn("tools install klatch", r.verbs())
        unit = next(stdin for a, stdin in r.calls if a[:2] == ("service", "add"))
        self.assertIn('"127.0.0.1:7199"', unit)
        self.assertIn("/pinned/klatch", unit)
        self.assertTrue((self.data / "web" / "index.html").exists())
        self.assertTrue((self.data / "transform" / "transform_icopy.sh").exists())
        self.assertFalse((self.data / "transform" / "klatch").exists())
        self.assertEqual(r.secrets["klatch/url"], "http://127.0.0.1:7199\n")
        self.assertEqual(r.secrets["klatch/admin"], "generated\n")
        self.assertEqual(self.setups[0][0], "http://127.0.0.1:7199")

    def test_lan_binds_all_interfaces_and_reports_lan_address(self):
        out, r = self.run_serve(lan=True)
        unit = next(stdin for a, stdin in r.calls if a[:2] == ("service", "add"))
        self.assertIn('"0.0.0.0:7199"', unit)
        self.assertEqual(out["reach"], "lan")
        self.assertRegex(out["url"], r"^http://\d+\.\d+\.\d+\.\d+:7199$")
        self.assertNotIn("outpost", " ".join(r.verbs()))

    def test_route_uses_owner_outpost_app_and_stays_loopback(self):
        out, r = self.run_serve(route="chat")
        self.assertIn(("outpost", "apps", "add", "chat", "--url", "http://127.0.0.1:7199"),
                      [a for a, _ in r.calls])
        unit = next(stdin for a, stdin in r.calls if a[:2] == ("service", "add"))
        self.assertIn('"127.0.0.1:7199"', unit)
        self.assertEqual(out["url"], "outpost-app:chat")

    def test_existing_admin_secret_is_kept(self):
        r = Runner()
        r.secrets["klatch/admin"] = "kept\n"
        self.run_serve(runner=r)
        self.assertNotIn("secret gen", r.verbs())

    def test_sha_mismatch_aborts_before_service(self):
        r = Runner()
        bad = dict(SHA, web="0" * 64)
        orig = Runner.__call__

        def call(self, *a, stdin=None, check=True):
            if a[:2] == ("tools", "show"):
                return json.dumps({"cli": {"binary": "/pinned/klatch", "managed": {
                    "assets": {"test/arm64": {"url": SERVER_URL, "sha256": bad["server"]}},
                    "notes": "Web asset URL: %s (sha256 %s)." % (WEB_URL, bad["web"])}}})
            return orig(self, *a, stdin=stdin, check=check)
        r.__class__ = type("BadRunner", (Runner,), {"__call__": call})
        with self.assertRaises(SystemExit):
            self.run_serve(runner=r)
        self.assertNotIn("service add -f", r.verbs())

    def test_unpinned_binary_path_aborts(self):
        with self.assertRaises(SystemExit):
            self.run_serve(runner=Runner(binary="klatch"))

    def test_free_space_floor(self):
        with self.assertRaises(SystemExit):
            serve.check_space(self.data, 1e9)

    def test_extract_refuses_links_and_escapes(self):
        with self.assertRaises(SystemExit):
            serve.extract(tgz({"ok.txt": b"x"}, links=[("evil", "/etc/passwd")]), self.data / "x")
        with self.assertRaises(SystemExit):
            serve.extract(tgz({"../escape.txt": b"x"}), self.data / "y")


if __name__ == "__main__":
    unittest.main()
