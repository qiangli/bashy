import io
import json
import os
import unittest
from unittest.mock import patch
import notes_api as api


class NotesAPI(unittest.TestCase):
    def invoke(self, args, status=200, response=b'{}'):
        calls = []
        def transport(*request):
            calls.append(request)
            return status, response
        out = io.StringIO()
        with patch.dict(os.environ, {"NOTES_URL": "http://127.0.0.1:1234/notes", "NOTES_TOKEN": "existing-token"}):
            code = api.run(args, out, transport)
        return code, out.getvalue(), calls

    def test_all_verbs(self):
        cases = [(["list"], "ListMemos"), (["show", "abc"], "GetMemo"),
                 (["add", "--content", "hello"], "CreateMemo"),
                 (["set", "abc", "--visibility", "private"], "UpdateMemo"),
                 (["archive", "abc"], "UpdateMemo"), (["unarchive", "abc"], "UpdateMemo"),
                 (["rm", "abc"], "DeleteMemo"), (["search", "hello"], "ListMemos"),
                 (["export", "abc", "--format", "json"], "GetMemo")]
        for args, method in cases:
            with self.subTest(args=args):
                code, output, calls = self.invoke(args + ["--json"])
                self.assertEqual(code, 0)
                self.assertEqual(json.loads(output), {})
                self.assertEqual(calls[0][1], method)
                self.assertEqual(calls[0][3], "existing-token")

    def test_write_manifests_no_io(self):
        for args in (["add", "--content", "hi"], ["set", "abc", "--content", "new"], ["archive", "abc"], ["unarchive", "abc"], ["rm", "abc"]):
            code, output, calls = self.invoke(args + ["--dry-run", "--json"])
            self.assertEqual((code, calls), (0, []))
            manifest = json.loads(output)
            self.assertTrue(manifest["dryRun"])
            self.assertEqual(manifest["method"], "POST")
            self.assertNotIn("existing-token", output)
            self.assertIn("/notes/memos.api.MemoService/", manifest["url"])

    def test_content_tags_mask(self):
        _, _, calls = self.invoke(["set", "memos/abc", "--content", "new\ntext", "--tags", "work,work/project", "--visibility", "public", "--pinned", "false"])
        self.assertEqual(calls[0][2], {"memo": {"name": "memos/abc", "content": "new\ntext\n\n#work #work/project", "visibility": "PUBLIC", "pinned": False}, "updateMask": "content,visibility,pinned"})

    def test_archive_states(self):
        for verb, state in [("archive", "ARCHIVED"), ("unarchive", "NORMAL")]:
            _, _, calls = self.invoke([verb, "abc"])
            self.assertEqual(calls[0][2], {"memo": {"name": "memos/abc", "state": state}, "updateMask": "state"})

    def test_search_escaping_and_pagination(self):
        term = 'x") || true || ("'
        _, _, calls = self.invoke(["search", term, "--filter", '"work" in tags', "--page-token", "next", "--page-size", "20", "--state", "archived"])
        self.assertEqual(calls[0][2], {"filter": '("work" in tags) && content.contains(' + json.dumps(term) + ')', "pageToken": "next", "pageSize": 20, "state": "ARCHIVED"})

    def test_bad_args_no_io(self):
        cases = [["show"], ["rm", "../abc"], ["add"], ["set", "abc"], ["set", "abc", "--tags", "work"], ["list", "--dry-run"], ["rm", "abc", "--content", "ignored"], ["list", "--page-size", "0"], ["list", "--page-size", "bad"], ["add", "--content", "hi", "--visibility", "world"], ["list", "--unknown"]]
        for args in cases:
            with self.subTest(args=args):
                code, output, calls = self.invoke(args)
                self.assertEqual((code, calls), (2, []))
                self.assertEqual(json.loads(output)["code"], "invalid_argument")

    def test_loopback_only(self):
        for url in ["https://example.invalid:443", "http://localhost:80", "http://127.0.0.1", "http://user@127.0.0.1:80", "http://127.0.0.1:80/?token=x", "http://127.0.0.1:80/../x"]:
            code, _, calls = self.invoke(["list", "--url", url])
            self.assertEqual((code, calls), (2, []))
        self.assertEqual(api.local_url("http://[::1]:1234"), "http://[::1]:1234")

    def test_denied_envelope_byte_preserved(self):
        raw = b'{"code":"permission_denied","message":"permission denied","details":[]}'
        for verb in ("show", "archive", "rm", "export"):
            code, output, _ = self.invoke([verb, "abc", "--json"], 403, raw)
            self.assertEqual(code, 1)
            self.assertEqual(output, raw.decode() + "\n")

    def test_success_envelope_byte_preserved(self):
        raw = b'{ "memos": [], "nextPageToken": "opaque" }'
        code, output, _ = self.invoke(["list", "--json"], response=raw)
        self.assertEqual((code, output), (0, raw.decode() + "\n"))

    def test_export(self):
        raw = b'{"name":"memos/abc","content":"# Title\\nBody"}'
        self.assertEqual(self.invoke(["export", "abc", "--format", "markdown"], response=raw)[1], "# Title\nBody\n")
        self.assertEqual(self.invoke(["export", "abc", "--format", "markdown", "--json"], response=raw)[1], raw.decode() + "\n")

    def test_missing_token_and_offline_dry_run(self):
        with patch.dict(os.environ, {"NOTES_TOKEN": ""}):
            out = io.StringIO()
            transport = lambda *args: self.fail("unexpected network")
            self.assertEqual(api.run(["show", "abc", "--url", "http://127.0.0.1:12"], out, transport), 1)
            self.assertEqual(json.loads(out.getvalue())["code"], "unauthenticated")
            self.assertEqual(api.run(["rm", "abc", "--dry-run", "--url", "http://127.0.0.1:12"], io.StringIO(), transport), 0)

    def test_non_json_response(self):
        code, output, _ = self.invoke(["show", "abc"], 502, b'<html>failed</html>')
        self.assertEqual((code, json.loads(output)["code"]), (1, "data_loss"))

    def test_http_request_and_redirect_policy(self):
        class Response:
            status = 200
            def read(self): return b'{}'
            def __enter__(self): return self
            def __exit__(self, *args): pass
        with patch.object(api.urllib.request, "build_opener") as builder:
            builder.return_value.open.return_value = Response()
            self.assertEqual(api.call("http://127.0.0.1:12", "GetMemo", {"name": "memos/a"}, "token"), (200, b'{}'))
            req = builder.return_value.open.call_args.args[0]
            self.assertEqual(req.get_method(), "POST")
            self.assertEqual(req.get_header("Authorization"), "Bearer token")
            self.assertEqual(req.get_header("Connect-protocol-version"), "1")
            self.assertEqual(builder.call_args.args[0].proxies, {})
            self.assertIsNone(builder.call_args.args[1].redirect_request(None, None, 302, "", {}, "http://example.invalid"))

if __name__ == "__main__":
    unittest.main()
