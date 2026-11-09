#!/usr/bin/env python3
"""Standalone Gmail thread helpers wrapping the `gws` CLI.

This module is intentionally independent of ``google.bsh``: it shells out to
the Tier-6 managed external ``gws`` binary (``googleworkspace/cli``) using the
same preflight rules (explicit ``GWS_ACCOUNT`` + ``GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE``,
secret-in-argv refusal) and the same ``--params`` / ``--json`` / ``--format=json``
invocation shape.

Verbs (CLI and callable)::

    gmail-thread-get <thread-id> [format=full]
    gmail-reply <message-id> body=TEXT [cc=ADDR] [--dry-run]
    gmail-attachment-get <message-id> <attachment-id> [output=<path>]

Callable entry points: :func:`gmail_thread_get`, :func:`gmail_reply`,
:func:`gmail_attachment_get` (with ``get_thread``, ``reply_to_message`` and
``get_attachment`` aliases).
"""

import base64
import json
import os
import shutil
import subprocess
import sys
from email.message import EmailMessage

MAX_ATTACHMENT_BYTES = 26214400

SAFE_TOKEN_KEYS = (
    "pagetoken",
    "nextpagetoken",
    "synctoken",
    "nextsynctoken",
)

SECRET_EXACT_KEYS = (
    "token",
    "access_token",
    "accesstoken",
    "refresh_token",
    "refreshtoken",
    "id_token",
    "idtoken",
    "auth_token",
    "authtoken",
    "client_secret",
    "clientsecret",
    "private_key",
    "privatekey",
    "private_key_id",
    "password",
    "passwd",
    "pwd",
    "secret",
    "api_key",
    "apikey",
    "authorization",
    "bearer",
    "credentials",
    "credential",
    "service_account_key",
)

SECRET_KEY_SUBSTRINGS = (
    "password",
    "passwd",
    "secret",
    "apikey",
    "api_key",
    "private_key",
    "privatekey",
    "credential",
    "bearer",
    "access_token",
    "refresh_token",
    "auth_token",
    "id_token",
)


class GmailThreadsError(Exception):
    """Raised by callable helpers; rendered as JSON on stderr by the CLI."""

    def __init__(self, code, message, exit_code=2, extra=None):
        super().__init__(message)
        self.code = code
        self.message = message
        self.exit_code = exit_code
        self.extra = dict(extra or {})


def fail(code, message, exit_code=2, extra=None):
    raise GmailThreadsError(code, message, exit_code=exit_code, extra=extra)


def is_secret_key_name(key):
    low = str(key).strip().lower().replace("-", "_")
    if low in SAFE_TOKEN_KEYS:
        return False
    if low in SECRET_EXACT_KEYS:
        return True
    return any(sub in low for sub in SECRET_KEY_SUBSTRINGS)


def contains_secret_key(value):
    if isinstance(value, dict):
        for k, v in value.items():
            if is_secret_key_name(k) or contains_secret_key(v):
                return True
    elif isinstance(value, list):
        return any(contains_secret_key(item) for item in value)
    return False


def arg_looks_secret(arg):
    if arg.startswith("--"):
        flag_part = arg[2:].split("=", 1)[0].strip()
        if is_secret_key_name(flag_part):
            return True
    for sep in (":=", "="):
        if sep in arg and not arg.startswith("-"):
            key, raw_val = arg.split(sep, 1)
            if is_secret_key_name(key):
                return True
            if sep == ":=":
                try:
                    parsed = json.loads(raw_val)
                except Exception:
                    parsed = None
                if contains_secret_key(parsed):
                    return True
            break
    return False


def refuse_secrets_in_argv(argv):
    for arg in argv:
        if arg_looks_secret(arg):
            fail(
                "SECRET_VIA_ARGV",
                "Refusing secret or credential field in command arguments; "
                "use GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE.",
                exit_code=2,
            )


def find_gws_bin():
    override = os.environ.get("GWS_BIN", "").strip()
    if override:
        if os.path.isfile(override) and os.access(override, os.X_OK):
            return override
        fail("GWS_NOT_FOUND", "GWS_BIN is not an executable file: %s" % override, exit_code=127)
    found = shutil.which("gws")
    if found:
        return found
    fail(
        "GWS_NOT_FOUND",
        "gws (Google Workspace CLI) was not found on PATH. Install gws or set GWS_BIN.",
        exit_code=127,
    )


def require_preflight():
    """Return ``(gws_bin, account, credentials_file)`` or raise."""
    gws_bin = find_gws_bin()
    account = os.environ.get("GWS_ACCOUNT", "").strip()
    if not account:
        fail(
            "MISSING_ACCOUNT",
            "GWS_ACCOUNT is required; no ambient account fallback is permitted.",
            exit_code=2,
        )
    creds_file = os.environ.get("GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE", "").strip()
    if not creds_file:
        fail(
            "MISSING_CREDENTIALS",
            "GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE is required.",
            exit_code=2,
        )
    expanded = os.path.expanduser(creds_file)
    if not os.path.isfile(expanded):
        fail(
            "MISSING_CREDENTIALS",
            "Credentials file does not exist or is not a regular file: %s" % creds_file,
            exit_code=2,
        )
    return gws_bin, account, expanded


def build_gws_env(account, creds_file):
    env = dict(os.environ)
    for ambient in ("GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_WORKSPACE_CLI_TOKEN"):
        env.pop(ambient, None)
    env["GWS_ACCOUNT"] = account
    env["GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE"] = creds_file
    return env


def run_gws_json(gws_bin, account, creds_file, argv):
    cmd = [gws_bin] + list(argv)
    if "--format=json" not in cmd and not any(a.startswith("--format") for a in cmd):
        cmd.append("--format=json")
    proc = subprocess.run(
        cmd, env=build_gws_env(account, creds_file), capture_output=True, text=True
    )
    if proc.returncode != 0:
        fail(
            "GWS_CALL_FAILED",
            "gws exited with status %d" % proc.returncode,
            exit_code=proc.returncode or 1,
            extra={"stderr": (proc.stderr or "").strip(), "argv": argv},
        )
    try:
        return json.loads(proc.stdout) if proc.stdout.strip() else {}
    except Exception as exc:
        fail("GWS_BAD_JSON", "gws did not return JSON: %s" % exc, exit_code=1)


def message_headers(msg_obj):
    headers = {}
    payload = msg_obj.get("payload") if isinstance(msg_obj, dict) else None
    if isinstance(payload, dict):
        for entry in payload.get("headers") or []:
            if isinstance(entry, dict) and entry.get("name"):
                headers[str(entry["name"]).lower()] = entry.get("value", "")
    return headers


def summarize_thread(thread_obj):
    """Summarize a ``threads.get`` response into id/from/subject/date/snippet rows."""
    messages = []
    raw_messages = thread_obj.get("messages") if isinstance(thread_obj, dict) else None
    for msg in raw_messages or []:
        if not isinstance(msg, dict):
            continue
        headers = message_headers(msg)
        messages.append({
            "id": msg.get("id", ""),
            "from": headers.get("from", ""),
            "subject": headers.get("subject", ""),
            "date": headers.get("date", ""),
            "snippet": msg.get("snippet", ""),
        })
    return {
        "id": thread_obj.get("id", "") if isinstance(thread_obj, dict) else "",
        "messageCount": len(messages),
        "messages": messages,
    }


def gmail_thread_get(thread_id, format="full"):  # noqa: A002 - 'format' matches gws params
    """Fetch a Gmail thread and return its summary. Raises GmailThreadsError."""
    if thread_id is None or not str(thread_id).strip():
        fail("USAGE", "gmail-thread-get requires <thread-id>")
    gws_bin, account, creds_file = require_preflight()
    params = {"userId": "me", "id": str(thread_id), "format": str(format)}
    thread_obj = run_gws_json(
        gws_bin, account, creds_file,
        ["gmail", "users", "threads", "get", "--params", json.dumps(params)],
    )
    summary = summarize_thread(thread_obj if isinstance(thread_obj, dict) else {})
    summary["account"] = account
    return summary


def get_thread(thread_id, format="full"):  # noqa: A002
    """Alias of :func:`gmail_thread_get`."""
    return gmail_thread_get(thread_id, format=format)


def build_reply_raw(to_addr, subject, body, cc=None, in_reply_to=None, references=None):
    msg = EmailMessage()
    msg["To"] = to_addr
    msg["Subject"] = subject
    if cc:
        msg["Cc"] = cc
    if in_reply_to:
        msg["In-Reply-To"] = in_reply_to
    if references:
        msg["References"] = references
    msg.set_content(body or "")
    return base64.urlsafe_b64encode(msg.as_bytes()).decode("ascii").rstrip("=")


def gmail_reply(message_id, body, cc=None, dry_run=False):
    """Reply to a Gmail message, preserving threading. Raises GmailThreadsError."""
    if message_id is None or not str(message_id).strip():
        fail("USAGE", "gmail-reply requires <message-id>")
    if body is None or not str(body):
        fail("USAGE", "gmail-reply requires body=TEXT")
    gws_bin, account, creds_file = require_preflight()

    original = run_gws_json(
        gws_bin, account, creds_file,
        ["gmail", "users", "messages", "get", "--params",
         json.dumps({"userId": "me", "id": str(message_id), "format": "full"})],
    )
    if not isinstance(original, dict):
        fail("GWS_BAD_JSON", "gws messages.get did not return an object.", exit_code=1)
    headers = message_headers(original)
    thread_id = original.get("threadId", "")

    orig_subject = headers.get("subject", "")
    if orig_subject.lower().startswith("re:"):
        subject = orig_subject
    elif orig_subject:
        subject = "Re: " + orig_subject
    else:
        subject = "Re: (no subject)"

    to_addr = headers.get("reply-to", "") or headers.get("from", "")
    if not to_addr:
        fail("MISSING_REPLY_RECIPIENT", "Original message has no From or Reply-To header.")

    message_id_hdr = headers.get("message-id", "") or headers.get("messageid", "")
    references_hdr = headers.get("references", "")
    if message_id_hdr and references_hdr:
        references = references_hdr + " " + message_id_hdr
    else:
        references = references_hdr or message_id_hdr or ""

    raw = build_reply_raw(
        to_addr=to_addr,
        subject=subject,
        body=str(body),
        cc=cc,
        in_reply_to=message_id_hdr or None,
        references=references or None,
    )

    if dry_run:
        return {
            "ok": True,
            "dry_run": True,
            "verb": "gmail-reply",
            "account": account,
            "to": to_addr,
            "cc": cc or "",
            "subject": subject,
            "threadId": thread_id,
            "raw": raw,
        }

    send_body = {"raw": raw}
    if thread_id:
        send_body["threadId"] = str(thread_id)
    sent = run_gws_json(
        gws_bin, account, creds_file,
        ["gmail", "users", "messages", "send", "--params",
         json.dumps({"userId": "me"}), "--json", json.dumps(send_body)],
    )
    result = {"ok": True, "verb": "gmail-reply", "account": account, "to": to_addr,
              "subject": subject}
    if isinstance(sent, dict):
        result.update(sent)
    else:
        result["response"] = sent
    return result


def reply_to_message(message_id, body, cc=None, dry_run=False):
    """Alias of :func:`gmail_reply`."""
    return gmail_reply(message_id, body, cc=cc, dry_run=dry_run)


def decode_attachment_data(data_b64):
    text = (data_b64 or "").replace("\n", "").replace("\r", "").replace(" ", "")
    padded = text + "=" * (-len(text) % 4)
    try:
        return base64.urlsafe_b64decode(padded)
    except Exception as exc:
        fail("ATTACHMENT_DECODE_FAILED", "Attachment data is not valid base64url: %s" % exc)


def gmail_attachment_get(message_id, attachment_id, output=None):
    """Fetch a Gmail attachment; write to *output* or return metadata + bytes."""
    if (
        message_id is None
        or not str(message_id).strip()
        or attachment_id is None
        or not str(attachment_id).strip()
    ):
        fail("USAGE", "gmail-attachment-get requires <message-id> <attachment-id>")
    gws_bin, account, creds_file = require_preflight()
    params = {"userId": "me", "messageId": str(message_id), "id": str(attachment_id)}
    attachment = run_gws_json(
        gws_bin, account, creds_file,
        ["gmail", "users", "messages", "attachments", "get",
         "--params", json.dumps(params)],
    )
    if not isinstance(attachment, dict):
        fail("GWS_BAD_JSON", "gws attachments.get did not return an object.", exit_code=1)

    try:
        declared_size = int(attachment.get("size", 0))
    except (TypeError, ValueError):
        declared_size = 0
    if declared_size > MAX_ATTACHMENT_BYTES:
        fail(
            "ATTACHMENT_TOO_LARGE",
            "Attachment size (%d bytes) exceeds 25 MiB cap (%d bytes)."
            % (declared_size, MAX_ATTACHMENT_BYTES),
        )

    decoded = decode_attachment_data(attachment.get("data", ""))
    if len(decoded) > MAX_ATTACHMENT_BYTES:
        fail(
            "ATTACHMENT_TOO_LARGE",
            "Decoded attachment size (%d bytes) exceeds 25 MiB cap (%d bytes)."
            % (len(decoded), MAX_ATTACHMENT_BYTES),
        )

    result = {
        "ok": True,
        "verb": "gmail-attachment-get",
        "account": account,
        "messageId": str(message_id),
        "attachmentId": str(attachment_id),
        "size": len(decoded),
        "declaredSize": declared_size,
    }
    if output:
        expanded = os.path.expanduser(str(output))
        with open(expanded, "wb") as fh:
            fh.write(decoded)
        result["output"] = expanded
        result["bytesWritten"] = len(decoded)
    else:
        result["data"] = decoded
    return result


def get_attachment(message_id, attachment_id, output=None):
    """Alias of :func:`gmail_attachment_get`."""
    return gmail_attachment_get(message_id, attachment_id, output=output)


def parse_cli_args(raw_args):
    positionals = []
    shaped = {}
    dry_run = False
    i = 0
    while i < len(raw_args):
        arg = raw_args[i]
        if arg == "--dry-run":
            dry_run = True
            i += 1
            continue
        if "=" in arg and not arg.startswith("-"):
            key, val = arg.split("=", 1)
            shaped[key] = val
            i += 1
            continue
        positionals.append(arg)
        i += 1
    return positionals, shaped, dry_run


def emit_json(data):
    sys.stdout.write(json.dumps(data, indent=2) + "\n")


def emit_error_and_exit(error):
    payload = {"error": {"code": error.code, "message": error.message}}
    payload["error"].update(error.extra)
    sys.stderr.write(json.dumps(payload) + "\n")
    return error.exit_code


def handle_thread_get(positionals, shaped):
    thread_id = positionals[0] if positionals else shaped.pop("id", None)
    if not thread_id:
        fail("USAGE", "gmail-thread-get requires <thread-id>")
    fmt = shaped.pop("format", "full")
    return gmail_thread_get(thread_id, format=fmt)


def handle_reply(positionals, shaped, dry_run):
    message_id = positionals[0] if positionals else shaped.pop("id", None)
    body = shaped.pop("body", None)
    cc = shaped.pop("cc", None)
    if body is None:
        fail("USAGE", "gmail-reply requires body=TEXT")
    return gmail_reply(message_id, body, cc=cc, dry_run=dry_run)


def handle_attachment_get(positionals, shaped):
    message_id = positionals[0] if len(positionals) > 0 else shaped.pop("messageId", None)
    attachment_id = positionals[1] if len(positionals) > 1 else shaped.pop("id", None)
    output = shaped.pop("output", None)
    result = gmail_attachment_get(message_id, attachment_id, output=output)
    printable = dict(result)
    if isinstance(printable.get("data"), (bytes, bytearray)):
        printable["data"] = printable["data"].decode("utf-8", errors="replace")
    return printable


def main(argv=None):
    args = list(sys.argv[1:] if argv is None else argv)
    if not args or args[0] in ("--help", "-h", "help"):
        sys.stdout.write(
            "Usage:\n"
            "  gmail_threads.py gmail-thread-get <thread-id> [format=full]\n"
            "  gmail_threads.py gmail-reply <message-id> body=TEXT [cc=ADDR] [--dry-run]\n"
            "  gmail_threads.py gmail-attachment-get <message-id> <attachment-id> [output=<path>]\n"
        )
        return 0
    verb = args[0].replace("_", "-")
    rest = args[1:]
    try:
        refuse_secrets_in_argv(rest)
        positionals, shaped, dry_run = parse_cli_args(rest)
        if verb == "gmail-thread-get":
            emit_json(handle_thread_get(positionals, shaped))
        elif verb == "gmail-reply":
            emit_json(handle_reply(positionals, shaped, dry_run))
        elif verb == "gmail-attachment-get":
            emit_json(handle_attachment_get(positionals, shaped))
        else:
            fail("USAGE", "Unknown verb %r. See --help." % args[0])
        return 0
    except GmailThreadsError as exc:
        return emit_error_and_exit(exc)


if __name__ == "__main__":
    sys.exit(main())
