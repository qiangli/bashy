"""Local Connect client for notes.bsh; no persistence or identity management."""
import argparse
import ipaddress
import json
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request


class UsageError(Exception):
    pass


class Parser(argparse.ArgumentParser):
    def error(self, message):
        raise UsageError(message)


def parse(argv):
    p = Parser(prog="notes", allow_abbrev=False)
    p.add_argument("verb", choices=["list", "show", "add", "set", "archive", "unarchive", "rm", "search", "export"])
    p.add_argument("item", nargs="?")
    p.add_argument("--json", action="store_true")
    p.add_argument("--dry-run", action="store_true")
    p.add_argument("--url", default=os.environ.get("NOTES_URL", ""))
    for name in ("content", "tags", "visibility", "space", "pinned", "filter", "page-token", "order-by", "state"):
        p.add_argument("--" + name)
    p.add_argument("--page-size", type=int)
    p.add_argument("--format", choices=["markdown", "json"], default=None)
    a = p.parse_args(argv)
    allowed = {
        "list": {"filter", "page_token", "page_size", "order_by", "state"},
        "search": {"filter", "page_token", "page_size", "order_by", "state"},
        "add": {"content", "tags", "visibility", "space"},
        "set": {"content", "tags", "visibility", "space", "pinned"},
        "export": {"format"},
    }.get(a.verb, set())
    for field in ("content", "tags", "visibility", "space", "pinned", "filter", "page_token", "page_size", "order_by", "state", "format"):
        if getattr(a, field) is not None and field not in allowed:
            raise UsageError("--" + field.replace("_", "-") + " is not valid for " + a.verb)
    if a.verb in ("list", "add"):
        if a.item is not None:
            raise UsageError(a.verb + " does not take an ID")
    elif not a.item:
        raise UsageError(a.verb + " requires " + ("TEXT" if a.verb == "search" else "ID"))
    elif a.verb != "search":
        a.item = a.item.removeprefix("memos/")
        if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9-]{0,35}", a.item):
            raise UsageError("invalid memo ID")
    if a.dry_run and a.verb not in ("add", "set", "archive", "unarchive", "rm"):
        raise UsageError("--dry-run is only valid for writes")
    if a.verb == "add" and a.content is None:
        raise UsageError("add requires --content")
    if a.tags is not None and a.content is None:
        raise UsageError("tags are part of Markdown; supply the complete replacement --content with --tags")
    if a.tags:
        tags = a.tags.split(",")
        if any(not re.fullmatch(r"[\w/-]+", tag) for tag in tags):
            raise UsageError("--tags expects comma-separated hashtag names without #")
        a.content += "\n\n" + " ".join("#" + tag for tag in tags)
    if a.visibility is not None:
        a.visibility = a.visibility.upper()
        if a.visibility not in ("PRIVATE", "PROTECTED", "PUBLIC", "SPACE"):
            raise UsageError("invalid visibility")
    if a.state is not None:
        a.state = a.state.upper()
        if a.state not in ("NORMAL", "ARCHIVED"):
            raise UsageError("invalid state")
    if a.pinned is not None and a.pinned not in ("true", "false"):
        raise UsageError("--pinned expects true or false")
    if a.page_size is not None and not 1 <= a.page_size <= 1000:
        raise UsageError("--page-size must be between 1 and 1000")
    if a.space and not re.fullmatch(r"spaces/[A-Za-z0-9-]+", a.space):
        raise UsageError("--space expects spaces/ID (or an empty string to remove placement)")
    return a


def plan(a):
    if a.verb in ("list", "search"):
        body = {key: getattr(a, field) for field, key in (("filter", "filter"), ("page_size", "pageSize"), ("page_token", "pageToken"), ("order_by", "orderBy"), ("state", "state")) if getattr(a, field) is not None}
        if a.verb == "search":
            term = "content.contains(" + json.dumps(a.item, ensure_ascii=False) + ")"
            body["filter"] = "(" + body["filter"] + ") && " + term if body.get("filter") else term
        return "ListMemos", body
    if a.verb in ("show", "export", "rm"):
        return ("DeleteMemo" if a.verb == "rm" else "GetMemo"), {"name": "memos/" + a.item}
    memo = {field: getattr(a, field) for field in ("content", "visibility", "space") if getattr(a, field) is not None}
    if a.pinned is not None:
        memo["pinned"] = a.pinned == "true"
    if a.verb == "add":
        return "CreateMemo", {"memo": memo}
    if a.verb in ("archive", "unarchive"):
        memo["state"] = "ARCHIVED" if a.verb == "archive" else "NORMAL"
    if not memo:
        raise UsageError("set requires at least one editable field")
    mask = ",".join(memo)
    memo["name"] = "memos/" + a.item
    return "UpdateMemo", {"memo": memo, "updateMask": mask}


def local_url(value):
    try:
        u = urllib.parse.urlsplit(value)
        ip = ipaddress.ip_address(u.hostname or "")
        if u.scheme not in ("http", "https") or not ip.is_loopback or not u.port or u.username or u.password or u.query or u.fragment:
            raise ValueError()
        if any(part in (".", "..") for part in urllib.parse.unquote(u.path).split("/")):
            raise ValueError()
    except ValueError:
        raise UsageError("set NOTES_URL or --url to the running principal instance's numeric loopback URL with port (optional proxy prefix)")
    return value.rstrip("/")


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def call(url, method, body, token):
    request = urllib.request.Request(url + "/memos.api.MemoService/" + method,
        data=json.dumps(body).encode(), headers={"Content-Type": "application/json", "Connect-Protocol-Version": "1", "Authorization": "Bearer " + token})
    # Local traffic must not use environment HTTP proxies, nor follow redirects.
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    try:
        with opener.open(request, timeout=30) as response:
            return response.status, response.read()
    except urllib.error.HTTPError as error:
        return error.code, error.read()


def emit_error(code, message, out):
    print(json.dumps({"code": code, "message": message}), file=out)


def run(argv, out=sys.stdout, transport=call):
    try:
        a = parse(argv)
        method, body = plan(a)
        url = local_url(a.url)
        if a.dry_run:
            print(json.dumps({"dryRun": True, "method": "POST", "url": url + "/memos.api.MemoService/" + method, "body": body}), file=out)
            return 0
        token = os.environ.get("NOTES_TOKEN", "").strip()
        if not token or any(c.isspace() for c in token):
            emit_error("unauthenticated", "set NOTES_TOKEN to the principal's existing local app token", out)
            return 1
        status, raw = transport(url, method, body, token)
        try:
            response = json.loads(raw)
            if not isinstance(response, dict):
                raise ValueError()
        except (ValueError, UnicodeError):
            emit_error("data_loss", "local app returned a non-object JSON response", out)
            return 1
        # Preserve the app's JSON, including code/message/details on denials.
        if status >= 300 or a.json or a.verb != "export" or a.format == "json":
            out.write(raw.decode() + ("" if raw.endswith(b"\n") else "\n"))
        else:
            print(response.get("content", ""), file=out)
        return 0 if 200 <= status < 300 else 1
    except UsageError as error:
        emit_error("invalid_argument", str(error), out)
        return 2
    except (OSError, ValueError):
        emit_error("unavailable", "cannot contact the local notes API", out)
        return 1


if __name__ == "__main__":
    sys.exit(run(sys.argv[1:]))
