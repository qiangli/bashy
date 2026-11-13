import io
import json
import os
import tempfile
import unittest
from unittest.mock import patch
import table_api as api

BASE = "http://127.0.0.1:1234/table"


class TableAPI(unittest.TestCase):
    def invoke(self, args, status=200, response=b'{}', responses=None):
        calls = []
        queue = list(responses or [])
        def transport(*request):
            calls.append(request)
            return queue.pop(0) if queue else (status, response)
        out = io.StringIO()
        with patch.dict(os.environ, {"TABLE_URL": BASE, "TABLE_TOKEN": "existing-key"}):
            code = api.run(args, out, transport)
        return code, out.getvalue(), calls

    def test_read_verbs_map_to_rest(self):
        cases = [(["docs", "list"], "GET", "/api/orgs/current/workspaces"),
                 (["docs", "show", "d1"], "GET", "/api/docs/d1"),
                 (["tables", "list", "d1"], "GET", "/api/docs/d1/tables"),
                 (["columns", "list", "d1", "T"], "GET", "/api/docs/d1/tables/T/columns"),
                 (["records", "list", "d1", "T"], "GET", "/api/docs/d1/tables/T/records"),
                 (["views", "list", "d1"], "GET", "/api/docs/d1/tables/_grist_Views/records"),
                 (["export", "d1", "T", "--format", "json"], "GET", "/api/docs/d1/tables/T/records"),
                 (["export", "d1", "T"], "GET", "/api/docs/d1/download/csv?tableId=T")]
        for args, method, path in cases:
            with self.subTest(args=args):
                code, _, calls = self.invoke(args + ["--json"])
                self.assertEqual(code, 0)
                self.assertEqual(calls[0][1:3], (method, path))
                self.assertEqual(calls[0][4], "existing-key")

    def test_write_requests(self):
        cases = [(["docs", "add", "Notes", "--workspace", "3"], "POST", "/api/workspaces/3/docs", {"name": "Notes"}),
                 (["tables", "add", "d1", "People", "--columns", "Name,Age:Int"], "POST", "/api/docs/d1/tables",
                  {"tables": [{"id": "People", "columns": [{"id": "Name", "fields": {"type": "Text"}}, {"id": "Age", "fields": {"type": "Int"}}]}]}),
                 (["columns", "add", "d1", "T", "Qty", "--type", "Numeric", "--label", "Quantity"], "POST", "/api/docs/d1/tables/T/columns",
                  {"columns": [{"id": "Qty", "fields": {"type": "Numeric", "label": "Quantity"}}]}),
                 (["records", "add", "d1", "T", "--set", "A=1", "--set", "B=x y"], "POST", "/api/docs/d1/tables/T/records",
                  {"records": [{"fields": {"A": 1, "B": "x y"}}]}),
                 (["records", "set", "d1", "T", "4", "5", "--set", "A=true"], "PATCH", "/api/docs/d1/tables/T/records",
                  {"records": [{"id": 4, "fields": {"A": True}}, {"id": 5, "fields": {"A": True}}]}),
                 (["records", "rm", "d1", "T", "4", "5"], "POST", "/api/docs/d1/tables/T/data/delete", [4, 5])]
        for args, method, path, body in cases:
            with self.subTest(args=args):
                code, _, calls = self.invoke(args)
                self.assertEqual((code, calls[0][1:4]), (0, (method, path, body)))
                code, output, calls = self.invoke(args + ["--dry-run"])
                self.assertEqual((code, calls), (0, []))
                m = json.loads(output)
                self.assertEqual((m["dryRun"], m["method"], m["url"], m["body"]), (True, method, BASE + path, body))
                self.assertNotIn("existing-key", output)

    def test_where_filter_on_list(self):
        _, _, calls = self.invoke(["records", "list", "d1", "T", "--where", "Status=open", "--where", "Status=wip", "--where", "N=3", "--sort=-N", "--limit", "5"])
        path = calls[0][2]
        self.assertTrue(path.startswith("/api/docs/d1/tables/T/records?filter="))
        from urllib.parse import urlsplit, parse_qs
        qs = parse_qs(urlsplit(path).query)
        self.assertEqual(json.loads(qs["filter"][0]), {"Status": ["open", "wip"], "N": [3]})
        self.assertEqual((qs["sort"], qs["limit"]), (["-N"], ["5"]))

    def test_where_write_resolves_through_api(self):
        found = (200, b'{"records":[{"id":7,"fields":{}},{"id":9,"fields":{}}]}')
        code, _, calls = self.invoke(["records", "set", "d1", "T", "--where", "A=1", "--set", "B=2"], responses=[found, (200, b'{}')])
        self.assertEqual(code, 0)
        self.assertEqual(calls[0][1], "GET")
        self.assertEqual(calls[1][1:4], ("PATCH", "/api/docs/d1/tables/T/records", {"records": [{"id": 7, "fields": {"B": 2}}, {"id": 9, "fields": {"B": 2}}]}))
        code, _, calls = self.invoke(["records", "rm", "d1", "T", "--where", "A=1"], responses=[found, (200, b'null')])
        self.assertEqual(calls[1][1:4], ("POST", "/api/docs/d1/tables/T/data/delete", [7, 9]))

    def test_where_write_no_match_writes_nothing(self):
        code, output, calls = self.invoke(["records", "rm", "d1", "T", "--where", "A=1"], response=b'{"records":[]}')
        self.assertEqual((code, len(calls), json.loads(output)), (0, 1, {"matched": 0}))

    def test_where_write_dry_run_no_io(self):
        code, output, calls = self.invoke(["records", "rm", "d1", "T", "--where", "A=1", "--dry-run"])
        m = json.loads(output)
        self.assertEqual((code, calls, len(m["requests"])), (0, [], 2))
        self.assertEqual(m["requests"][0]["method"], "GET")
        self.assertEqual(m["requests"][1]["after"], "resolve")

    def test_denied_envelope_byte_preserved(self):
        raw = b'{"error":"access denied"}'
        for args in (["docs", "show", "d1"], ["records", "add", "d1", "T", "--set", "A=1"], ["records", "rm", "d1", "T", "3"],
                     ["tables", "add", "d1", "X"], ["export", "d1", "T", "--format", "json"]):
            with self.subTest(args=args):
                code, output, _ = self.invoke(args + ["--json"], 403, raw)
                self.assertEqual((code, output), (1, raw.decode() + "\n"))

    def test_denied_on_where_lookup_stops(self):
        code, output, calls = self.invoke(["records", "rm", "d1", "T", "--where", "A=1"], 403, b'{"error":"access denied"}')
        self.assertEqual((code, len(calls)), (1, 1))

    def test_csv_export(self):
        code, output, _ = self.invoke(["export", "d1", "T"], response=b"a,b\n1,2\n")
        self.assertEqual((code, output), (0, "a,b\n1,2\n"))
        _, output, _ = self.invoke(["export", "d1", "T", "--json"], response=b"a,b\n1,2\n")
        self.assertEqual(json.loads(output), {"format": "csv", "content": "a,b\n1,2\n"})

    def csv_file(self, text):
        f = tempfile.NamedTemporaryFile("w", suffix=".csv", delete=False)
        f.write(text)
        f.close()
        self.addCleanup(os.unlink, f.name)
        return f.name

    def test_import_maps_labels_and_coerces(self):
        path = self.csv_file("Full Name,Age,Ok\nAda,36,true\nBob,,false\n")
        cols = (200, json.dumps({"columns": [
            {"id": "Name", "fields": {"label": "Full Name", "type": "Text"}},
            {"id": "Age", "fields": {"label": "Age", "type": "Int"}},
            {"id": "Ok", "fields": {"label": "Ok", "type": "Bool"}}]}).encode())
        code, output, calls = self.invoke(["import", "d1", "T", "--file", path], responses=[cols, (200, b'{}')])
        self.assertEqual((code, json.loads(output)), (0, {"imported": 2}))
        self.assertEqual(calls[1][3], {"records": [{"fields": {"Name": "Ada", "Age": 36, "Ok": True}}, {"fields": {"Name": "Bob", "Age": None, "Ok": False}}]})

    def test_import_unknown_column_writes_nothing(self):
        path = self.csv_file("Nope\nx\n")
        cols = (200, b'{"columns":[{"id":"A","fields":{"label":"A","type":"Text"}}]}')
        code, output, calls = self.invoke(["import", "d1", "T", "--file", path], responses=[cols])
        self.assertEqual((code, len(calls), json.loads(output)["code"]), (2, 1, "invalid_argument"))

    def test_import_batches_and_dry_run(self):
        path = self.csv_file("A\n" + "\n".join(str(i) for i in range(1200)) + "\n")
        code, output, calls = self.invoke(["import", "d1", "T", "--file", path, "--dry-run"])
        m = json.loads(output)
        self.assertEqual((code, calls, [len(r["body"]["records"]) for r in m["requests"]]), (0, [], [500, 500, 200]))

    def test_bad_args_no_io(self):
        cases = [[], ["nope"], ["docs"], ["docs", "show"], ["docs", "add", "N"], ["docs", "list", "--dry-run"],
                 ["records", "rm", "d1", "T"], ["records", "rm", "d1", "T", "1", "--where", "A=1"],
                 ["records", "set", "d1", "T", "1"], ["records", "set", "d1", "T", "x", "--set", "A=1"],
                 ["records", "add", "d1", "T"], ["records", "add", "d1", "T", "--set", "bad"],
                 ["records", "add", "d1", "T", "--fields", "[1]"], ["records", "list", "d1", "T", "--set", "A=1"],
                 ["records", "list", "d1", "../T"], ["export", "d1", "T", "--format", "xml"], ["import", "d1", "T"],
                 ["records", "list", "d1", "T", "--limit", "0"], ["columns", "add", "d1", "T", "bad id"],
                 ["records", "list", "d1", "T", "--where", "1x=2"], ["views", "list", "d1", "extra"]]
        for args in cases:
            with self.subTest(args=args):
                code, output, calls = self.invoke(args)
                self.assertEqual((code, calls), (2, []))
                self.assertEqual(json.loads(output)["code"], "invalid_argument")

    def test_loopback_only(self):
        for url in ["https://example.invalid:443", "http://localhost:80", "http://127.0.0.1", "http://user@127.0.0.1:80", "http://127.0.0.1:80/?k=x", "http://127.0.0.1:80/../x"]:
            code, _, calls = self.invoke(["docs", "list", "--url", url])
            self.assertEqual((code, calls), (2, []))

    def test_missing_token_and_offline_dry_run(self):
        with patch.dict(os.environ, {"TABLE_TOKEN": ""}):
            out = io.StringIO()
            transport = lambda *args: self.fail("unexpected network")
            self.assertEqual(api.run(["docs", "show", "d1", "--url", "http://127.0.0.1:12"], out, transport), 1)
            self.assertEqual(json.loads(out.getvalue())["code"], "unauthenticated")
            self.assertEqual(api.run(["records", "rm", "d1", "T", "1", "--dry-run", "--url", "http://127.0.0.1:12"], io.StringIO(), transport), 0)

    def test_http_request_and_redirect_policy(self):
        class Response:
            status = 200
            def read(self): return b'{}'
            def __enter__(self): return self
            def __exit__(self, *args): pass
        with patch.object(api.urllib.request, "build_opener") as builder:
            builder.return_value.open.return_value = Response()
            self.assertEqual(api.call("http://127.0.0.1:12", "PATCH", "/api/x", {"a": 1}, "key"), (200, b'{}'))
            req = builder.return_value.open.call_args.args[0]
            self.assertEqual((req.get_method(), req.get_header("Authorization")), ("PATCH", "Bearer key"))
            self.assertEqual(builder.call_args.args[0].proxies, {})
            self.assertIsNone(builder.call_args.args[1].redirect_request(None, None, 302, "", {}, "http://example.invalid"))


if __name__ == "__main__":
    unittest.main()
