"""Offline tests for the standalone Docs and Sheets helper."""

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import textwrap
import unittest

try:
    from examples.google import docs_sheets
except ModuleNotFoundError:
    import docs_sheets


HELPER = Path(__file__).with_name("docs_sheets.py")


class DocsSheetsTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        root = Path(self.temp.name)
        self.log = root / "calls.jsonl"
        self.credential = root / "credentials.json"
        self.credential.write_text("{}", encoding="utf-8")
        self.fake = root / "gws"
        self.fake.write_text(textwrap.dedent("""\
            #!/usr/bin/env python3
            import json
            import os
            import sys
            with open(os.environ['FAKE_GWS_LOG'], 'a', encoding='utf-8') as stream:
                stream.write(json.dumps({'argv': sys.argv[1:], 'account': os.environ.get('GWS_ACCOUNT'),
                    'credentials': os.environ.get('GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE')}) + '\\n')
            if sys.argv[1:4] == ['drive', 'files', 'export']:
                sys.stdout.buffer.write(b'%PDF-1.7\\x00test')
            elif sys.argv[1:5] == ['sheets', 'spreadsheets', 'values', 'get']:
                print(json.dumps({'values': [['one', 'two']]}))
            elif sys.argv[1:5] == ['sheets', 'spreadsheets', 'values', 'append']:
                print(json.dumps({'updates': {'updatedRows': 1}}))
            elif sys.argv[1:4] == ['docs', 'documents', 'get']:
                print(json.dumps({'body': {'content': [{'paragraph': {'elements':
                    [{'textRun': {'content': 'Hello\\n'}}]}}]}}))
            """), encoding="utf-8")
        self.fake.chmod(0o755)
        self.env = dict(os.environ, GWS_ACCOUNT="reader@example.com",
                        GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE=str(self.credential),
                        GWS_BIN=str(self.fake), FAKE_GWS_LOG=str(self.log))

    def calls(self):
        return [json.loads(line) for line in self.log.read_text(encoding="utf-8").splitlines()] if self.log.exists() else []

    def cli(self, *args, env=None):
        return subprocess.run([sys.executable, str(HELPER), *args], env=env or self.env, capture_output=True)

    def test_export_binary_stdout_and_file_all_supported_mimes(self):
        for mime in sorted(docs_sheets.EXPORT_MIMES):
            with self.subTest(mime=mime):
                result = self.cli("drive-export", "file1", f"mimeType={mime}")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout, b"%PDF-1.7\x00test")
                self.assertEqual(json.loads(self.calls()[-1]["argv"][4]), {"fileId": "file1", "mimeType": mime})
        output = Path(self.temp.name) / "export.pdf"
        result = self.cli("drive-export", "file2", "mimeType=application/pdf", f"output={output}")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, b"")
        self.assertEqual(output.read_bytes(), b"%PDF-1.7\x00test")

    def test_sheets_get_default_and_explicit_range(self):
        self.assertEqual(docs_sheets.sheets_get("sheet1", env=self.env), {"values": [["one", "two"]]})
        self.assertEqual(json.loads(self.calls()[-1]["argv"][5]), {"spreadsheetId": "sheet1", "range": "Sheet1!A1:Z100"})
        result = self.cli("sheets-get", "sheet2", "range=Data!B2:C3")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(self.calls()[-1]["argv"][5]), {"spreadsheetId": "sheet2", "range": "Data!B2:C3"})

    def test_sheets_append_and_dry_run(self):
        result = self.cli("sheets-append", "sheet1", "range=Data!A1", 'values:=[["x",2]]')
        self.assertEqual(result.returncode, 0, result.stderr)
        args = self.calls()[-1]["argv"]
        self.assertEqual(json.loads(args[5]), {"spreadsheetId": "sheet1", "range": "Data!A1", "valueInputOption": "USER_ENTERED"})
        self.assertEqual(json.loads(args[7]), {"values": [["x", 2]]})
        self.assertEqual(args[8], "--format=json")
        result = self.cli("sheets-append", "sheet1", "range=Data!A1", 'values:=[["x"]]', "--dry-run")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(json.loads(result.stdout)["dry_run"])
        self.assertEqual(len(self.calls()), 1)

    def test_docs_get_and_text_extraction(self):
        self.assertEqual(docs_sheets.extract_plain_text(docs_sheets.docs_get("doc1", env=self.env)), "Hello\n")
        self.assertEqual(json.loads(self.calls()[-1]["argv"][4]), {"documentId": "doc1"})
        document = {"body": {"content": [
            {"paragraph": {"elements": [{"textRun": {"content": "Top\n"}}, {"inlineObjectElement": {}}]}},
            {"table": {"tableRows": [{"tableCells": [{"content": [
                {"paragraph": {"elements": [{"textRun": {"content": "Cell\n"}}]}}]}]}]}},
            {"tableOfContents": {"content": [{"paragraph": {"elements": [
                {"textRun": {"content": "TOC\n"}}]}}]}}]}}
        self.assertEqual(docs_sheets.extract_plain_text(document), "Top\nCell\nTOC\n")

    def test_preflight_and_secret_refusal_never_invoke_gws(self):
        cases = [
            (("docs-get", "doc1"), {"GWS_ACCOUNT": ""}, "MISSING_ACCOUNT"),
            (("docs-get", "doc1"), {"GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE": ""}, "MISSING_CREDENTIALS"),
            (("docs-get", "doc1"), {"GWS_BIN": "/missing/gws"}, "GWS_NOT_FOUND"),
            (("docs-get", "doc1", "client_secret=hidden"), {}, "SECRET_VIA_ARGV"),
            (("sheets-append", "sheet1", "range=Data!A1", 'values:=[[{"access_token":"hidden"}]]'), {}, "SECRET_VIA_ARGV"),
        ]
        for args, changes, code in cases:
            with self.subTest(code=code):
                env = dict(self.env, **changes)
                result = self.cli(*args, env=env)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(json.loads(result.stderr)["error"]["code"], code)
        self.assertEqual(self.calls(), [])

    def test_path_fallback_and_invalid_rows(self):
        env = dict(self.env)
        env.pop("GWS_BIN")
        env["PATH"] = self.temp.name + os.pathsep + env.get("PATH", "")
        self.assertEqual(self.cli("docs-get", "doc1", env=env).returncode, 0)
        with self.assertRaises(docs_sheets.HelperError):
            docs_sheets.sheets_append("sheet1", "Data!A1", {"not": "rows"}, env=self.env)


if __name__ == "__main__":
    unittest.main()
