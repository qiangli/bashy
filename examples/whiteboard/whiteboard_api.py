"""Local whiteboard client for whiteboard.bsh; no persistence or identity management."""
import argparse
import base64
import ipaddress
import json
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request

BOARD_ID = r"[A-Za-z0-9][A-Za-z0-9_-]{0,63}"
SCENE_ID = r"[0-9a-f]{16}"


class UsageError(Exception):
    pass


class Parser(argparse.ArgumentParser):
    def error(self, message):
        raise UsageError(message)


def parse(argv):
    p = Parser(prog="whiteboard", allow_abbrev=False)
    p.add_argument("verb", choices=["boards", "new", "show", "set", "lock", "rm",
                                    "share", "save", "load", "upload", "download", "export"])
    p.add_argument("item", nargs="?")
    p.add_argument("--json", action="store_true")
    p.add_argument("--dry-run", action="store_true")
    p.add_argument("--url", default=os.environ.get("WHITEBOARD_URL", ""))
    for name in ("note", "file", "output", "scene", "format"):
        p.add_argument("--" + name)
    a = p.parse_args(argv)
    allowed = {
        "new": {"note"},
        "set": {"note"},
        "save": {"file"},
        "load": {"output"},
        "upload": {"file"},
        "download": {"output"},
        "export": {"format", "scene", "output"},
    }.get(a.verb, set())
    for field in ("note", "file", "output", "scene", "format"):
        if getattr(a, field) is not None and field not in allowed:
            raise UsageError("--" + field + " is not valid for " + a.verb)
    if a.verb == "boards":
        if a.item is not None:
            raise UsageError("boards does not take an ID")
    elif a.verb == "new":
        if a.item is not None and not re.fullmatch(BOARD_ID, a.item):
            raise UsageError("invalid board ID")
    elif a.verb == "save":
        if a.item is not None:
            raise UsageError("save takes --file, not an ID")
        if a.file is None:
            raise UsageError("save requires --file")
    elif a.verb == "upload":
        if not a.item:
            raise UsageError("upload requires a remote PATH")
        check_asset_path(a.item)
        if a.file is None:
            raise UsageError("upload requires --file")
    elif a.verb in ("show", "set", "lock", "rm", "share", "export"):
        if not a.item:
            raise UsageError(a.verb + " requires a board ID")
        if not re.fullmatch(BOARD_ID, a.item):
            raise UsageError("invalid board ID")
    elif a.verb in ("load", "download"):
        if not a.item:
            raise UsageError(a.verb + " requires an ID or PATH")
        if a.verb == "load" and not re.fullmatch(SCENE_ID, a.item):
            raise UsageError("invalid scene ID")
        if a.verb == "download":
            check_asset_path(a.item)
    if a.dry_run and a.verb not in ("new", "set", "lock", "rm", "save", "upload"):
        raise UsageError("--dry-run is only valid for writes")
    if a.verb == "set" and a.note is None:
        raise UsageError("set requires --note")
    if a.verb == "export":
        if a.format is not None and a.format != "json":
            raise UsageError(a.format + " export is a browser canvas render with no server API; only --format json is supported")
        if a.scene is not None and not re.fullmatch(SCENE_ID, a.scene):
            raise UsageError("invalid scene ID")
    if a.verb in ("load", "download") and a.json and not a.output:
        raise UsageError("--json requires --output for " + a.verb)
    return a


def check_asset_path(path):
    if not path or path.startswith("/") or "//" in path:
        raise UsageError("invalid asset PATH")
    parts = path.split("/")
    if any(part in ("", ".", "..") for part in parts):
        raise UsageError("invalid asset PATH")


def plan(a):
    if a.verb == "boards":
        return [("GET", "/api/board", None, None)]
    if a.verb == "show":
        return [("GET", "/api/state?id=" + urllib.parse.quote(a.item, safe=""), None, None)]
    if a.verb in ("new", "set", "lock", "rm", "share"):
        action = {"new": "add", "set": "set", "lock": "close", "rm": "remove", "share": "share"}[a.verb]
        body = {"action": action}
        if a.verb == "new":
            if a.item:
                body["board"] = a.item
            if a.note is not None:
                body["body"] = a.note
        else:
            body["id"] = a.item
            if a.verb == "set":
                body["body"] = a.note
        return [("POST", "/api/action", body, None)]
    if a.verb == "save":
        return [("POST", "/api/v2/post", None, ("file", a.file))]
    if a.verb == "load":
        return [("GET", "/api/v2/" + a.item, None, ("raw", None))]
    if a.verb == "upload":
        return [("POST", "/files/" + a.item, None, ("file", a.file))]
    if a.verb == "download":
        return [("GET", "/files/" + a.item, None, ("raw", None))]
    if a.verb == "export":
        steps = [("GET", "/api/state?id=" + urllib.parse.quote(a.item, safe=""), None, None)]
        if a.scene is not None:
            steps.append(("GET", "/api/v2/" + a.scene, None, ("raw", None)))
        return steps
    raise UsageError("unknown verb")


def local_url(value):
    try:
        u = urllib.parse.urlsplit(value)
        ip = ipaddress.ip_address(u.hostname or "")
        if u.scheme not in ("http", "https") or not ip.is_loopback or not u.port or u.username or u.password or u.query or u.fragment:
            raise ValueError()
        if any(part in (".", "..") for part in urllib.parse.unquote(u.path).split("/")):
            raise ValueError()
    except ValueError:
        raise UsageError("set WHITEBOARD_URL or --url to the running principal instance's numeric loopback URL with port (optional proxy prefix)")
    return value.rstrip("/")


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def call(url, method, body, data, token):
    headers = {}
    if data is not None:
        headers["Content-Type"] = "application/octet-stream"
    elif body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = "Bearer " + token
    request = urllib.request.Request(url, data=data, headers=headers, method=method)
    # Local traffic must not use environment HTTP proxies, nor follow redirects.
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    try:
        with opener.open(request, timeout=30) as response:
            return response.status, response.read()
    except urllib.error.HTTPError as error:
        return error.code, error.read()


def http_status_code(status):
    if status == 400:
        return "invalid_argument"
    if status == 404:
        return "not_found"
    if 500 <= status < 600:
        return "unavailable"
    return "unknown"


def emit_error(code, message, out):
    print(json.dumps({"code": code, "message": message}), file=out)


def decode_object(raw):
    try:
        response = json.loads(raw)
        if not isinstance(response, dict):
            raise ValueError()
        return response
    except (ValueError, UnicodeError):
        return None


def run(argv, out=sys.stdout, transport=call):
    try:
        a = parse(argv)
        steps = plan(a)
        base = local_url(a.url)
        token = os.environ.get("WHITEBOARD_TOKEN", "").strip()
        if token and any(c.isspace() for c in token):
            raise UsageError("WHITEBOARD_TOKEN must not contain whitespace")
        if a.dry_run:
            manifests = []
            for method, path, body, raw in steps:
                entry = {"dryRun": True, "method": method, "url": base + path}
                if body is not None:
                    entry["body"] = body
                if raw is not None:
                    entry["file"] = raw[1]
                manifests.append(entry)
            print(json.dumps(manifests[0] if len(manifests) == 1 else manifests), file=out)
            return 0
        results = []
        for method, path, body, raw in steps:
            payload = None
            if raw and raw[0] == "file":
                with open(raw[1], "rb") as handle:
                    payload = handle.read()
            status, content = transport(base + path, method, body, payload, token)
            results.append((status, content))
        if a.verb in ("load", "download"):
            status, content = results[0]
            if not 200 <= status < 300:
                emit_error(http_status_code(status), content.decode("utf-8", "replace").strip() or "request failed", out)
                return 1
            if a.json:
                print(json.dumps({"id": a.item, "bytes": len(content)}), file=out)
            if a.output:
                with open(a.output, "wb") as handle:
                    handle.write(content)
            else:
                buffer = getattr(out, "buffer", None) or sys.stdout.buffer
                buffer.write(content)
            return 0
        if a.verb == "export":
            status, content = results[0]
            if not 200 <= status < 300:
                emit_error(http_status_code(status), content.decode("utf-8", "replace").strip() or "request failed", out)
                return 1
            board = decode_object(content)
            if board is None:
                emit_error("data_loss", "local app returned a non-object JSON response", out)
                return 1
            document = {"board": board}
            if len(results) > 1:
                status, scene = results[1]
                if not 200 <= status < 300:
                    emit_error(http_status_code(status), scene.decode("utf-8", "replace").strip() or "request failed", out)
                    return 1
                document["scene"] = {"id": a.scene, "encoding": "base64",
                                     "data": base64.b64encode(scene).decode("ascii")}
            text = json.dumps(document)
            if a.output:
                with open(a.output, "w", encoding="utf-8") as handle:
                    handle.write(text + "\n")
            else:
                print(text, file=out)
            return 0
        status, content = results[0]
        if not 200 <= status < 300:
            emit_error(http_status_code(status), content.decode("utf-8", "replace").strip() or "request failed", out)
            return 1
        response = decode_object(content)
        if response is None:
            emit_error("data_loss", "local app returned a non-object JSON response", out)
            return 1
        # Preserve the app's JSON, including error details on denials.
        out.write(content.decode() + ("" if content.endswith(b"\n") else "\n"))
        return 0
    except UsageError as error:
        emit_error("invalid_argument", str(error), out)
        return 2
    except (OSError, ValueError):
        emit_error("unavailable", "cannot contact the local whiteboard API", out)
        return 1


if __name__ == "__main__":
    sys.exit(run(sys.argv[1:]))
