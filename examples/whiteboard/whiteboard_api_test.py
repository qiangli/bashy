import base64
import io
import json
import os
import unittest
from unittest.mock import patch
import whiteboard_api as api


class WhiteboardAPI(unittest.TestCase):
    def invoke(self, args, status=200, responses=None):
        calls = []
        if responses is None:
            bodies = [b'{}']
        elif isinstance(responses, bytes):
            bodies = [responses]
        else:
            bodies = list(responses)
        def transport(*request):
            calls.append(request)
            return status, bodies.pop(0) if bodies else b'{}'
        out = io.StringIO()
        with patch.dict(os.environ, {"WHITEBOARD_URL": "http://127.0.0.1:1234/whiteboard",
                                     "WHITEBOARD_TOKEN": ""}):
            code = api.run(args, out, transport)
        return code, out.getvalue(), calls

    def test_all_verbs(self):
        scene = "0123456789abcdef"
        tmp = os.path.join(os.environ.get("TMPDIR", "/tmp"), "wb-scene.bin")
        with open(tmp, "wb") as handle:
            handle.write(b"scene-bytes")
        self.addCleanup(lambda: os.path.exists(tmp) and os.unlink(tmp))
        cases = [(["boards"], "GET", "/api/board"),
                 (["new"], "POST", "/api/action"),
                 (["new", "my-board", "--note", "hi"], "POST", "/api/action"),
                 (["show", "my-board"], "GET", "/api/state"),
                 (["set", "my-board", "--note", "hi"], "POST", "/api/action"),
                 (["lock", "my-board"], "POST", "/api/action"),
                 (["rm", "my-board"], "POST", "/api/action"),
                 (["share", "my-board"], "POST", "/api/action"),
                 (["save", "--file", tmp], "POST", "/api/v2/post"),
                 (["load", scene, "--output", tmp + ".out"], "GET", "/api/v2/" + scene),
                 (["upload", "rooms/r/f", "--file", tmp], "POST", "/files/rooms/r/f"),
                 (["download", "rooms/r/f", "--output", tmp + ".out"], "GET", "/files/rooms/r/f"),
                 (["export", "my-board"], "GET", "/api/state")]
        for args, method, path in cases:
            with self.subTest(args=args):
                code, output, calls = self.invoke(args + ["--json"] if args[0] not in ("load", "download") else args)
                self.assertEqual(code, 0)
                self.assertTrue(calls[0][0].startswith("http://127.0.0.1:1234/whiteboard" + path))
                # transport receives (url, method, body, data, token)
                _url, got_method, _body, _data, _token = calls[0]
                self.assertEqual(got_method, method)

    def test_action_bodies(self):
        _, _, calls = self.invoke(["new", "my-board", "--note", "hello"])
        self.assertEqual(calls[0][2], {"action": "add", "board": "my-board", "body": "hello"})
        _, _, calls = self.invoke(["new"])
        self.assertEqual(calls[0][2], {"action": "add"})
        _, _, calls = self.invoke(["set", "my-board", "--note", "hello"])
        self.assertEqual(calls[0][2], {"action": "set", "id": "my-board", "body": "hello"})
        _, _, calls = self.invoke(["lock", "my-board"])
        self.assertEqual(calls[0][2], {"action": "close", "id": "my-board"})
        _, _, calls = self.invoke(["rm", "my-board"])
        self.assertEqual(calls[0][2], {"action": "remove", "id": "my-board"})
        _, _, calls = self.invoke(["share", "my-board"])
        self.assertEqual(calls[0][2], {"action": "share", "id": "my-board"})

    def test_write_manifests_no_io(self):
        tmp = os.path.join(os.environ.get("TMPDIR", "/tmp"), "wb-no-read.bin")
        for args in (["new", "b"], ["new", "b", "--note", "hi"], ["set", "b", "--note", "hi"],
                     ["lock", "b"], ["rm", "b"], ["save", "--file", tmp], ["upload", "r/f", "--file", tmp]):
            with self.subTest(args=args):
                code, output, calls = self.invoke(args + ["--dry-run", "--json"])
                self.assertEqual((code, calls), (0, []))
                manifest = json.loads(output)
                self.assertTrue(manifest["dryRun"])
                self.assertIn(manifest["method"], ("POST",))
                self.assertTrue(manifest["url"].startswith("http://127.0.0.1:1234/whiteboard/"))
                self.assertNotIn("existing-token", output)

    def test_save_upload_send_file_bytes(self):
        tmp = os.path.join(os.environ.get("TMPDIR", "/tmp"), "wb-payload.bin")
        with open(tmp, "wb") as handle:
            handle.write(b"\x00\x01payload")
        self.addCleanup(lambda: os.path.exists(tmp) and os.unlink(tmp))
        _, _, calls = self.invoke(["save", "--file", tmp], responses=[b'{"id":"abc"}'])
        self.assertEqual(calls[0][3], b"\x00\x01payload")
        _, _, calls = self.invoke(["upload", "rooms/r/f", "--file", tmp], responses=[b''])
        self.assertEqual(calls[0][3], b"\x00\x01payload")

    def test_load_download_bytes_and_json(self):
        tmp = os.path.join(os.environ.get("TMPDIR", "/tmp"), "wb-dl.bin")
        self.addCleanup(lambda: os.path.exists(tmp) and os.unlink(tmp))
        out = io.StringIO()
        out.buffer = io.BytesIO()
        with patch.dict(os.environ, {"WHITEBOARD_URL": "http://127.0.0.1:1234/whiteboard", "WHITEBOARD_TOKEN": ""}):
            code = api.run(["load", "0123456789abcdef"], out, lambda *r: (200, b"raw-scene"))
        self.assertEqual(code, 0)
        self.assertEqual(out.buffer.getvalue(), b"raw-scene")
        code, output, _ = self.invoke(["download", "rooms/r/f", "--output", tmp, "--json"], responses=[b"bytes!"])
        self.assertEqual(code, 0)
        self.assertEqual(json.loads(output), {"id": "rooms/r/f", "bytes": 6})
        with open(tmp, "rb") as handle:
            self.assertEqual(handle.read(), b"bytes!")

    def test_export_board_and_scene(self):
        board = b'{"id":"b","name":"b","revision":2,"locked":false}'
        scene = b"scene-bytes"
        code, output, calls = self.invoke(["export", "b"], responses=[board])
        self.assertEqual(code, 0)
        self.assertEqual(json.loads(output), {"board": json.loads(board)})
        self.assertEqual(len(calls), 1)
        code, output, calls = self.invoke(["export", "b", "--scene", "0123456789abcdef"],
                                          responses=[board, scene])
        self.assertEqual(code, 0)
        document = json.loads(output)
        self.assertEqual(document["board"], json.loads(board))
        self.assertEqual(document["scene"]["id"], "0123456789abcdef")
        self.assertEqual(base64.b64decode(document["scene"]["data"]), scene)
        self.assertEqual(len(calls), 2)

    def test_bad_args_no_io(self):
        cases = [["show"], ["show", "../x"], ["rm", "a/b"], ["set", "b"], ["new", "has space"],
                 ["set", "b", "--note", "x", "--format", "json"], ["boards", "extra"], ["save"],
                 ["save", "extra", "--file", "f"], ["upload", "r/f"], ["upload", "../x", "--file", "f"],
                 ["upload", "/abs", "--file", "f"], ["download"], ["download", "../x"],
                 ["load"], ["load", "ZZZ"], ["load", "b", "--json"], ["export"],
                 ["export", "b", "--format", "png"], ["export", "b", "--format", "svg"],
                 ["export", "b", "--scene", "nope"], ["boards", "--dry-run"], ["show", "b", "--dry-run"],
                 ["share", "b", "--dry-run"], ["load", "0123456789abcdef", "--dry-run"],
                 ["download", "r/f", "--dry-run"], ["export", "b", "--dry-run"],
                 ["list", "x"], ["show", "b", "--unknown"]]
        for args in cases:
            with self.subTest(args=args):
                code, output, calls = self.invoke(args)
                self.assertEqual((code, calls), (2, []))
                self.assertEqual(json.loads(output)["code"], "invalid_argument")
        # png refusal records the page-only reason.
        code, output, _ = self.invoke(["export", "b", "--format", "png"])
        self.assertIn("no server API", output)

    def test_loopback_only(self):
        for url in ["https://example.invalid:443", "http://localhost:80", "http://127.0.0.1",
                    "http://user@127.0.0.1:80", "http://127.0.0.1:80/?token=x", "http://127.0.0.1:80/../x"]:
            code, _, calls = self.invoke(["boards", "--url", url])
            self.assertEqual((code, calls), (2, []))
        self.assertEqual(api.local_url("http://[::1]:1234"), "http://[::1]:1234")

    def test_denied_result_matches_page(self):
        # The page shows a 404 for a missing board; the verb passes it through.
        code, output, _ = self.invoke(["rm", "gone"], 404, b'board not found\n')
        self.assertEqual(code, 1)
        self.assertEqual(json.loads(output), {"code": "not_found", "message": "board not found"})
        code, output, _ = self.invoke(["share", "gone"], 404, b'board not found\n')
        self.assertEqual(code, 1)
        self.assertEqual(json.loads(output)["code"], "not_found")

    def test_success_envelope_byte_preserved(self):
        raw = b'{"running":true,"boards":[]}'
        code, output, calls = self.invoke(["boards", "--json"], responses=[raw])
        self.assertEqual((code, output), (0, raw.decode() + "\n"))

    def test_missing_url_and_offline_dry_run(self):
        with patch.dict(os.environ, {"WHITEBOARD_URL": ""}):
            out = io.StringIO()
            transport = lambda *args: self.fail("unexpected network")
            self.assertEqual(api.run(["show", "b"], out, transport), 2)
            self.assertEqual(json.loads(out.getvalue())["code"], "invalid_argument")
            self.assertEqual(api.run(["rm", "b", "--dry-run", "--url", "http://127.0.0.1:12"], io.StringIO(), transport), 0)

    def test_non_json_response(self):
        code, output, _ = self.invoke(["show", "b"], 502, [b'<html>failed</html>'])
        self.assertEqual((code, json.loads(output)["code"]), (1, "unavailable"))
        code, output, _ = self.invoke(["show", "b"], 200, [b'<html>ok</html>'])
        self.assertEqual((code, json.loads(output)["code"]), (1, "data_loss"))

    def test_http_request_and_redirect_policy(self):
        class Response:
            status = 200
            def read(self): return b'{}'
            def __enter__(self): return self
            def __exit__(self, *args): pass
        with patch.object(api.urllib.request, "build_opener") as builder:
            builder.return_value.open.return_value = Response()
            self.assertEqual(api.call("http://127.0.0.1:12/api/board", "GET", None, None, ""), (200, b'{}'))
            req = builder.return_value.open.call_args.args[0]
            self.assertEqual(req.get_method(), "GET")
            self.assertIsNone(req.get_header("Authorization"))
            self.assertEqual(builder.call_args.args[0].proxies, {})
            self.assertIsNone(builder.call_args.args[1].redirect_request(None, None, 302, "", {}, "http://example.invalid"))
        with patch.object(api.urllib.request, "build_opener") as builder:
            builder.return_value.open.return_value = Response()
            api.call("http://127.0.0.1:12/api/action", "POST", {"action": "list"}, None, "tok")
            req = builder.return_value.open.call_args.args[0]
            self.assertEqual(req.get_header("Authorization"), "Bearer tok")
            self.assertEqual(req.get_header("Content-type"), "application/json")
        with patch.object(api.urllib.request, "build_opener") as builder:
            builder.return_value.open.return_value = Response()
            api.call("http://127.0.0.1:12/api/v2/post", "POST", None, b"\x00", "")
            req = builder.return_value.open.call_args.args[0]
            self.assertEqual(req.get_header("Content-type"), "application/octet-stream")

    def test_token_whitespace_rejected(self):
        with patch.dict(os.environ, {"WHITEBOARD_URL": "http://127.0.0.1:12", "WHITEBOARD_TOKEN": "bad token"}):
            out = io.StringIO()
            self.assertEqual(api.run(["boards"], out, lambda *a: self.fail("unexpected network")), 2)
            self.assertEqual(json.loads(out.getvalue())["code"], "invalid_argument")


if __name__ == "__main__":
    unittest.main()
