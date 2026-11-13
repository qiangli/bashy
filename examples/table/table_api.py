"""Local REST client for table.bsh; no persistence or identity management."""
import argparse
import csv
import io
import ipaddress
import json
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request

GROUPS = {
    "docs": {"list": 0, "add": 1, "show": 1},
    "tables": {"list": 1, "add": 2},
    "columns": {"list": 2, "add": 3},
    "records": {"list": 2, "add": 2, "set": 2, "rm": 2},
    "views": {"list": 1},
    "export": {None: 2},
    "import": {None: 2},
}
WRITES = {("docs", "add"), ("tables", "add"), ("columns", "add"), ("records", "add"),
          ("records", "set"), ("records", "rm"), ("import", None)}
BATCH = 500
IDENT = re.compile(r"[A-Za-z_][A-Za-z0-9_]*")
DOC_ID = re.compile(r"[A-Za-z0-9_-]{1,64}")


class UsageError(Exception):
    pass


class Parser(argparse.ArgumentParser):
    def error(self, message):
        raise UsageError(message)


def parse_value(text):
    try:
        return json.loads(text)
    except ValueError:
        return text


def pair(text, what):
    col, sep, val = text.partition("=")
    if not sep or not IDENT.fullmatch(col):
        raise UsageError(what + " expects COLUMN=VALUE")
    return col, parse_value(val)


def parse(argv):
    p = Parser(prog="table", allow_abbrev=False)
    p.add_argument("words", nargs="*")
    p.add_argument("--json", action="store_true")
    p.add_argument("--dry-run", action="store_true")
    p.add_argument("--url", default=os.environ.get("TABLE_URL", ""))
    p.add_argument("--where", action="append", default=[])
    p.add_argument("--set", action="append", default=[], dest="sets")
    for name in ("fields", "sort", "workspace", "org", "columns", "type", "label", "format", "file"):
        p.add_argument("--" + name)
    p.add_argument("--limit", type=int)
    a = p.parse_args(argv)
    if not a.words or a.words[0] not in GROUPS:
        raise UsageError("unknown verb")
    a.group = a.words[0]
    rest = a.words[1:]
    actions = GROUPS[a.group]
    if None in actions:
        a.action, a.args = None, rest
    else:
        if not rest or rest[0] not in actions:
            raise UsageError(a.group + " takes " + "|".join(actions))
        a.action, a.args = rest[0], rest[1:]
    arity = actions[a.action]
    if a.group == "records" and a.action in ("set", "rm"):
        if len(a.args) < arity:
            raise UsageError("records %s requires DOC TABLE" % a.action)
        a.rows = a.args[arity:]
        a.args = a.args[:arity]
        try:
            a.rows = [int(r) for r in a.rows]
        except ValueError:
            raise UsageError("row IDs must be integers")
        if any(r < 1 for r in a.rows):
            raise UsageError("row IDs must be positive")
        if bool(a.rows) == bool(a.where):
            raise UsageError("records %s needs row IDs or --where, not both and not neither" % a.action)
    elif len(a.args) != arity:
        raise UsageError("%s %s expects %d argument(s)" % (a.group, a.action or "", arity))
    allowed = {
        ("docs", "add"): {"workspace"}, ("docs", "list"): {"org"},
        ("tables", "add"): {"columns"}, ("columns", "add"): {"type", "label"},
        ("records", "list"): {"where", "sort", "limit"}, ("records", "add"): {"sets", "fields"},
        ("records", "set"): {"where", "sets", "fields"}, ("records", "rm"): {"where"},
        ("export", None): {"format"}, ("import", None): {"file"},
    }.get((a.group, a.action), set())
    for field in ("where", "sets", "fields", "sort", "workspace", "org", "columns", "type", "label", "format", "file", "limit"):
        v = getattr(a, field)
        if v not in (None, []) and field not in allowed:
            raise UsageError("--" + field.replace("sets", "set") + " is not valid for " + " ".join(filter(None, (a.group, a.action))))
    if a.dry_run and (a.group, a.action) not in WRITES:
        raise UsageError("--dry-run is only valid for writes")
    ids = a.args
    if a.group != "docs" or a.action != "add":
        if ids and not DOC_ID.fullmatch(ids[0]):
            raise UsageError("invalid document ID")
    if a.group in ("tables", "columns", "records", "export", "import"):
        if a.group == "tables" and a.action == "list":
            pass
        elif a.group != "views" and len(ids) > 1 and not IDENT.fullmatch(ids[1]):
            raise UsageError("invalid table ID")
    if a.group == "columns" and a.action == "add" and not IDENT.fullmatch(ids[2]):
        raise UsageError("invalid column ID")
    if a.group == "docs" and a.action == "add":
        if a.workspace is None or not a.workspace.isdigit():
            raise UsageError("docs add requires --workspace ID")
    if a.limit is not None and a.limit < 1:
        raise UsageError("--limit must be positive")
    a.where_map = {}
    for w in a.where:
        col, val = pair(w, "--where")
        a.where_map.setdefault(col, []).append(val)
    a.field_map = {}
    if a.fields is not None:
        try:
            a.field_map = json.loads(a.fields)
        except ValueError:
            a.field_map = None
        if not isinstance(a.field_map, dict):
            raise UsageError("--fields expects a JSON object")
    for s in a.sets:
        col, val = pair(s, "--set")
        a.field_map[col] = val
    if a.group == "records" and a.action in ("add", "set") and not a.field_map:
        raise UsageError("records %s requires --set COLUMN=VALUE or --fields JSON" % a.action)
    if a.group == "export":
        a.format = a.format or "csv"
        if a.format not in ("csv", "json"):
            raise UsageError("--format expects csv or json")
    if a.group == "import" and not a.file:
        raise UsageError("import requires --file PATH (CSV)")
    if a.group == "columns" and a.action == "add":
        a.type = a.type or "Text"
    if a.group == "tables" and a.action == "add":
        cols = []
        for spec in filter(None, (a.columns or "").split(",")):
            cid, _, ctype = spec.partition(":")
            if not IDENT.fullmatch(cid):
                raise UsageError("--columns expects ID[:Type],...")
            cols.append({"id": cid, "fields": {"type": ctype or "Text"}})
        a.col_specs = cols
        if not IDENT.fullmatch(ids[1]):
            raise UsageError("invalid table ID")
    return a


def q(value):
    return urllib.parse.quote(value, safe="")


def records_path(a):
    return "/api/docs/%s/tables/%s/records" % (q(a.args[0]), q(a.args[1]))


def with_query(path, params):
    return path + ("?" + urllib.parse.urlencode(params) if params else "")


def list_params(a):
    params = []
    if a.where_map:
        params.append(("filter", json.dumps(a.where_map, ensure_ascii=False)))
    if getattr(a, "sort", None):
        params.append(("sort", a.sort))
    if getattr(a, "limit", None):
        params.append(("limit", str(a.limit)))
    return params


def plan(a):
    """First request of the verb: (method, path, body)."""
    g, act, x = a.group, a.action, a.args
    if g == "docs":
        if act == "list":
            return "GET", "/api/orgs/%s/workspaces" % q(a.org or "current"), None
        if act == "show":
            return "GET", "/api/docs/" + q(x[0]), None
        return "POST", "/api/workspaces/%s/docs" % a.workspace, {"name": x[0]}
    if g == "tables":
        if act == "list":
            return "GET", "/api/docs/%s/tables" % q(x[0]), None
        return "POST", "/api/docs/%s/tables" % q(x[0]), {"tables": [{"id": x[1], "columns": a.col_specs}]}
    if g == "columns":
        base = "/api/docs/%s/tables/%s/columns" % (q(x[0]), q(x[1]))
        if act == "list":
            return "GET", base, None
        fields = {"type": a.type}
        if a.label:
            fields["label"] = a.label
        return "POST", base, {"columns": [{"id": x[2], "fields": fields}]}
    if g == "views":
        return "GET", "/api/docs/%s/tables/_grist_Views/records" % q(x[0]), None
    if g == "export":
        if a.format == "csv":
            return "GET", with_query("/api/docs/%s/download/csv" % q(x[0]), [("tableId", x[1])]), None
        return "GET", records_path(a), None
    if g == "import":
        return "POST", records_path(a), None  # body is built from the file
    if act == "list":
        return "GET", with_query(records_path(a), list_params(a)), None
    if act == "add":
        return "POST", records_path(a), {"records": [{"fields": a.field_map}]}
    if a.rows:
        return write_step(a, a.rows)
    return "GET", with_query(records_path(a), list_params(a)), None  # resolve --where first


def write_step(a, rows):
    if a.action == "rm":
        return "POST", "/api/docs/%s/tables/%s/data/delete" % (q(a.args[0]), q(a.args[1])), rows
    return "PATCH", records_path(a), {"records": [{"id": r, "fields": a.field_map} for r in rows]}


def read_csv(path):
    try:
        with open(path, newline="", encoding="utf-8-sig") as f:
            rows = list(csv.reader(f))
    except (OSError, UnicodeError, csv.Error) as error:
        raise UsageError("cannot read CSV: %s" % error.__class__.__name__)
    if not rows or not any(c.strip() for c in rows[0]):
        raise UsageError("CSV needs a header row")
    header = [h.strip() for h in rows[0]]
    if len(set(header)) != len(header):
        raise UsageError("CSV header has duplicate columns")
    return header, [r for r in rows[1:] if any(c != "" for c in r)]


def coerce(text, ctype):
    if text == "":
        return "" if ctype in ("Text", "Choice", None) else None
    try:
        if ctype == "Int":
            return int(text)
        if ctype in ("Numeric", "Any"):
            return float(text) if ctype == "Numeric" else text
        if ctype == "Bool":
            return {"true": True, "false": False, "1": True, "0": False}[text.lower()]
    except (ValueError, KeyError):
        pass
    return text


def import_batches(a, header, rows, columns=None):
    """columns: {id,label,type} list from the app, or None for a dry run."""
    names = header
    types = [None] * len(header)
    if columns is not None:
        by = {}
        for c in columns:
            by[c["id"]] = c
            by.setdefault(c["fields"].get("label"), c)
        missing = [h for h in header if h not in by]
        if missing:
            raise UsageError("CSV columns not in table: " + ", ".join(missing))
        names = [by[h]["id"] for h in header]
        types = [by[h]["fields"].get("type") for h in header]
    for i in range(0, len(rows), BATCH):
        yield {"records": [{"fields": {n: coerce(c, t) for n, t, c in zip(names, types, row + [""] * (len(header) - len(row)))}}
                           for row in rows[i:i + BATCH]]}


def local_url(value):
    try:
        u = urllib.parse.urlsplit(value)
        ip = ipaddress.ip_address(u.hostname or "")
        if u.scheme not in ("http", "https") or not ip.is_loopback or not u.port or u.username or u.password or u.query or u.fragment:
            raise ValueError()
        if any(part in (".", "..") for part in urllib.parse.unquote(u.path).split("/")):
            raise ValueError()
    except ValueError:
        raise UsageError("set TABLE_URL or --url to the running principal instance's numeric loopback URL with port (optional proxy prefix)")
    return value.rstrip("/")


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def call(url, method, path, body, token):
    data = None if body is None else json.dumps(body).encode()
    headers = {"Authorization": "Bearer " + token, "Accept": "application/json, text/csv"}
    if data is not None:
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(url + path, data=data, method=method, headers=headers)
    # Local traffic must not use environment HTTP proxies, nor follow redirects.
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    try:
        with opener.open(request, timeout=30) as response:
            return response.status, response.read()
    except urllib.error.HTTPError as error:
        return error.code, error.read()


def emit_error(code, message, out):
    print(json.dumps({"code": code, "message": message}), file=out)


def decode(raw):
    try:
        return json.loads(raw)
    except (ValueError, UnicodeError):
        return None


def run(argv, out=sys.stdout, transport=call):
    try:
        a = parse(argv)
        method, path, body = plan(a)
        url = local_url(a.url)
        if a.group == "import":
            header, rows = read_csv(a.file)
            batches = list(import_batches(a, header, rows))
            reqs = [{"method": "POST", "url": url + path, "body": b} for b in batches]
        elif a.group == "records" and a.action in ("set", "rm") and not a.rows:
            wm, wp, wb = write_step(a, ["<matched row IDs>"])
            reqs = [{"method": "GET", "url": url + path, "body": None},
                    {"method": wm, "url": url + wp, "body": wb, "after": "resolve"}]
        else:
            reqs = [{"method": method, "url": url + path, "body": body}]
        if a.dry_run:
            result = {"dryRun": True, **reqs[0]} if len(reqs) == 1 else {"dryRun": True, "requests": reqs}
            if a.group == "import":
                result["note"] = "CSV headers resolve to column IDs or labels at run time"
            print(json.dumps(result), file=out)
            return 0
        token = os.environ.get("TABLE_TOKEN", "").strip()
        if not token or any(c.isspace() for c in token):
            emit_error("unauthenticated", "set TABLE_TOKEN to the principal's existing local API key", out)
            return 1
        return execute(a, url, method, path, body, token, out, transport)
    except UsageError as error:
        emit_error("invalid_argument", str(error), out)
        return 2
    except (OSError, ValueError):
        emit_error("unavailable", "cannot contact the local table API", out)
        return 1


def finish(raw, status, out):
    out.write(raw.decode() + ("" if raw.endswith(b"\n") else "\n"))
    return 0 if 200 <= status < 300 else 1


def execute(a, url, method, path, body, token, out, transport):
    if a.group == "import":
        header, rows = read_csv(a.file)
        status, raw = transport(url, "GET", "/api/docs/%s/tables/%s/columns" % (q(a.args[0]), q(a.args[1])), None, token)
        cols = decode(raw)
        if status >= 300 or not isinstance(cols, dict):
            return finish(raw, status if status >= 300 else 502, out)
        added = 0
        for batch in import_batches(a, header, rows, cols.get("columns", [])):
            status, raw = transport(url, "POST", path, batch, token)
            if status >= 300:
                return finish(raw, status, out)
            added += len(batch["records"])
        print(json.dumps({"imported": added}), file=out)
        return 0
    status, raw = transport(url, method, path, body, token)
    if status >= 300:
        return finish(raw, status, out)
    parsed = decode(raw)
    if a.group == "records" and a.action in ("set", "rm") and not a.rows:
        if not isinstance(parsed, dict) or not isinstance(parsed.get("records"), list):
            emit_error("data_loss", "local app returned an unexpected record list", out)
            return 1
        ids = [r["id"] for r in parsed["records"]]
        if not ids:
            print(json.dumps({"matched": 0}), file=out)
            return 0
        m, p, b = write_step(a, ids)
        status, raw = transport(url, m, p, b, token)
        return finish(raw, status, out)
    if a.group == "export" and a.format == "csv":
        text = raw.decode("utf-8", "replace")
        print(json.dumps({"format": "csv", "content": text}) if a.json else text.rstrip("\n"), file=out)
        return 0
    if parsed is None and raw.strip() not in (b"", b"null"):
        emit_error("data_loss", "local app returned a non-JSON response", out)
        return 1
    return finish(raw if raw.strip() else b"null", status, out)


if __name__ == "__main__":
    sys.exit(run(sys.argv[1:]))
