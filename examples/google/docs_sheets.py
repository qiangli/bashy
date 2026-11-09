#!/usr/bin/env python3
"""Small, standalone Google Docs, Sheets, and Drive export CLI for gws."""

import json
import os
import shutil
import subprocess
import sys


EXPORT_MIMES = {"text/markdown", "text/csv", "text/plain", "application/pdf"}
DEFAULT_RANGE = "Sheet1!A1:Z100"
SECRET_KEYS = {
    "token", "access_token", "refresh_token", "id_token", "auth_token",
    "client_secret", "private_key", "private_key_id", "password", "passwd",
    "secret", "api_key", "apikey", "authorization", "bearer",
    "credentials", "credential", "service_account_key",
}


class HelperError(Exception):
    def __init__(self, code, message, exit_code=2):
        super().__init__(message)
        self.code = code
        self.exit_code = exit_code


def _secret_key(key):
    name = str(key).strip().lower().replace("-", "_")
    return name in SECRET_KEYS or any(part in name for part in (
        "password", "secret", "private_key", "credential", "api_key",
        "access_token", "refresh_token", "auth_token", "id_token", "bearer",
    ))


def _has_secret(value):
    if isinstance(value, dict):
        return any(_secret_key(key) or _has_secret(item) for key, item in value.items())
    if isinstance(value, list):
        return any(_has_secret(item) for item in value)
    return False


def _refuse_secret_args(args):
    for arg in args:
        key, separator, value = arg.partition(":=" if ":=" in arg else "=")
        if _secret_key(key.lstrip("-")):
            raise HelperError("SECRET_VIA_ARGV", "Refusing secret or credential field in arguments")
        if separator == ":=":
            try:
                parsed = json.loads(value)
            except json.JSONDecodeError:
                continue  # The command parser reports malformed JSON separately.
            if _has_secret(parsed):
                raise HelperError("SECRET_VIA_ARGV", "Refusing nested secret field in arguments")


def _preflight(env):
    if not env.get("GWS_ACCOUNT", "").strip():
        raise HelperError("MISSING_ACCOUNT", "GWS_ACCOUNT is required")
    credential = env.get("GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE", "").strip()
    if not credential or not os.path.isfile(credential):
        raise HelperError("MISSING_CREDENTIALS", "GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE must name a file")
    override = env.get("GWS_BIN", "").strip()
    executable = override or shutil.which("gws", path=env.get("PATH"))
    if not executable or not os.path.isfile(executable) or not os.access(executable, os.X_OK):
        raise HelperError("GWS_NOT_FOUND", "gws executable not found; set GWS_BIN or PATH", 127)
    return executable


def _run(args, env=None):
    env = dict(os.environ if env is None else env)
    executable = _preflight(env)
    result = subprocess.run([executable, *args], env=env, capture_output=True)
    if result.returncode:
        message = result.stderr.decode("utf-8", errors="replace").strip() or "gws failed"
        raise HelperError("GWS_FAILED", message, result.returncode)
    return result.stdout


def _json_result(args, env=None):
    raw = _run(args, env)
    try:
        return json.loads(raw)
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise HelperError("INVALID_GWS_RESPONSE", f"gws returned invalid JSON: {exc}") from exc


def drive_export(file_id, mime_type, output=None, env=None):
    """Return exported bytes, optionally writing them to *output*."""
    if not file_id or mime_type not in EXPORT_MIMES:
        raise HelperError("USAGE", "drive-export requires a file ID and supported mimeType")
    params = json.dumps({"fileId": file_id, "mimeType": mime_type})
    data = _run(["drive", "files", "export", "--params", params], env)
    if output is not None:
        with open(output, "wb") as stream:
            stream.write(data)
    return data


def sheets_get(spreadsheet_id, range=DEFAULT_RANGE, env=None):
    if not spreadsheet_id or not range:
        raise HelperError("USAGE", "sheets-get requires a spreadsheet ID and range")
    params = json.dumps({"spreadsheetId": spreadsheet_id, "range": range})
    return _json_result(["sheets", "spreadsheets", "values", "get", "--params", params, "--format=json"], env)


def sheets_append(spreadsheet_id, range, values, dry_run=False, env=None):
    if not spreadsheet_id or not range or not isinstance(values, list) or not all(isinstance(row, list) for row in values):
        raise HelperError("USAGE", "sheets-append requires a spreadsheet ID, range, and JSON array of rows")
    if _has_secret(values):
        raise HelperError("SECRET_VIA_ARGV", "Refusing nested secret field in values")
    params = {"spreadsheetId": spreadsheet_id, "range": range, "valueInputOption": "USER_ENTERED"}
    body = {"values": values}
    args = ["sheets", "spreadsheets", "values", "append", "--params", json.dumps(params), "--json", json.dumps(body), "--format=json"]
    if dry_run:
        _preflight(dict(os.environ if env is None else env))
        return {"dry_run": True, "command": args}
    return _json_result(args, env)


def docs_get(document_id, env=None):
    if not document_id:
        raise HelperError("USAGE", "docs-get requires a document ID")
    params = json.dumps({"documentId": document_id})
    return _json_result(["docs", "documents", "get", "--params", params, "--format=json"], env)


def extract_plain_text(document):
    """Collect text runs from paragraphs, tables, and table-of-contents elements."""
    def walk(elements):
        for element in elements:
            paragraph = element.get("paragraph")
            if paragraph:
                for part in paragraph.get("elements", []):
                    yield part.get("textRun", {}).get("content", "")
            table = element.get("table")
            if table:
                for row in table.get("tableRows", []):
                    for cell in row.get("tableCells", []):
                        yield from walk(cell.get("content", []))
            toc = element.get("tableOfContents")
            if toc:
                yield from walk(toc.get("content", []))

    return "".join(walk(document.get("body", {}).get("content", [])))


def _parse_options(args, allowed, required=()):
    options = {}
    for arg in args:
        key, sep, value = arg.partition("=")
        if not sep or key not in allowed or key in options or not value:
            raise HelperError("USAGE", f"Unexpected or invalid argument: {arg}")
        options[key] = value
    if any(key not in options for key in required):
        raise HelperError("USAGE", f"Required option: {', '.join(required)}")
    return options


def main(argv=None):
    args = list(sys.argv[1:] if argv is None else argv)
    try:
        _refuse_secret_args(args)
        if len(args) < 2:
            raise HelperError("USAGE", "Expected a command and ID")
        command, identifier, *rest = args
        if command == "drive-export":
            opts = _parse_options(rest, {"mimeType", "output"}, {"mimeType"})
            data = drive_export(identifier, opts["mimeType"], opts.get("output"))
            if "output" not in opts:
                sys.stdout.buffer.write(data)
        elif command == "sheets-get":
            opts = _parse_options(rest, {"range"})
            print(json.dumps(sheets_get(identifier, opts.get("range", DEFAULT_RANGE))))
        elif command == "sheets-append":
            dry_run = "--dry-run" in rest
            opts = _parse_options([arg for arg in rest if arg != "--dry-run"], {"range", "values:"}, {"range", "values:"})
            try:
                values = json.loads(opts["values:"])
            except json.JSONDecodeError as exc:
                raise HelperError("INVALID_JSON_ARG", f"Invalid values JSON: {exc}") from exc
            print(json.dumps(sheets_append(identifier, opts["range"], values, dry_run=dry_run)))
        elif command == "docs-get":
            if rest:
                raise HelperError("USAGE", "docs-get accepts only a document ID")
            print(json.dumps(docs_get(identifier)))
        else:
            raise HelperError("USAGE", f"Unknown command: {command}")
        return 0
    except HelperError as exc:
        print(json.dumps({"error": {"code": exc.code, "message": str(exc)}}), file=sys.stderr)
        return exc.exit_code
    except OSError as exc:
        print(json.dumps({"error": {"code": "IO_ERROR", "message": str(exc)}}), file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
