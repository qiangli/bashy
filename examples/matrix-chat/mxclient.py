"""Tiny stdlib Matrix/HTTP client shared by the Cloudbox shim gate and E2E."""
import json
import time
import urllib.error
import urllib.parse
import urllib.request


class Client:
    def __init__(self, base, token=None, headers=None):
        self.base = base.rstrip("/")
        self.token = token
        self.headers = dict(headers or {})

    def call(self, method, path, body=None, token=None, raw=None, content_type=None):
        """Return (status, decoded JSON or raw bytes). Never raises on HTTP status."""
        headers = dict(self.headers)
        token = token or self.token
        if token:
            headers["Authorization"] = "Bearer " + token
        payload = None
        if raw is not None:
            payload = raw
            headers["Content-Type"] = content_type or "application/octet-stream"
        elif body is not None:
            payload = json.dumps(body).encode()
            headers["Content-Type"] = "application/json"
        req = urllib.request.Request(self.base + path, payload, headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=60) as response:
                status, content, ctype = response.status, response.read(), response.headers.get("Content-Type", "")
        except urllib.error.HTTPError as exc:
            status, content, ctype = exc.code, exc.read(), exc.headers.get("Content-Type", "")
        if "json" in ctype:
            try:
                return status, json.loads(content or b"{}")
            except ValueError:
                pass
        return status, content


def txn():
    return "cb-" + str(time.time_ns())


def q(value):
    return urllib.parse.quote(value, safe="")


def login(client, localpart, password):
    return client.call("POST", "/_matrix/client/v3/login", {
        "type": "m.login.password",
        "identifier": {"type": "m.id.user", "user": localpart},
        "password": password})


def send_text(client, token, room, body):
    status, data = client.call("PUT", "/_matrix/client/v3/rooms/%s/send/m.room.message/%s" % (q(room), txn()),
                               {"msgtype": "m.text", "body": body}, token=token)
    return status, data


def send_image(client, token, room, image, name="cloudbox-e2e.png"):
    status, data = client.call("POST", "/_matrix/media/v3/upload?filename=" + q(name), token=token,
                               raw=image, content_type="image/png")
    if status != 200:
        return status, data, None
    mxc = data["content_uri"]
    status, data = client.call("PUT", "/_matrix/client/v3/rooms/%s/send/m.room.message/%s" % (q(room), txn()),
                               {"msgtype": "m.image", "body": name, "url": mxc,
                                "info": {"mimetype": "image/png", "size": len(image)}}, token=token)
    return status, data, mxc


def download(client, token, mxc):
    server, media = mxc[len("mxc://"):].split("/", 1)
    # Authenticated media (Matrix v1.11) first; legacy unauthenticated path as fallback.
    status, data = client.call("GET", "/_matrix/client/v1/media/download/%s/%s" % (q(server), q(media)),
                               token=token)
    if status == 200:
        return status, data
    return client.call("GET", "/_matrix/media/v3/download/%s/%s" % (q(server), q(media)), token=token)


def bodies(client, token, room):
    status, data = client.call("GET", "/_matrix/client/v3/rooms/%s/messages?dir=b&limit=50" % q(room),
                               token=token)
    if status != 200:
        return status, []
    return status, [e.get("content", {}).get("body") for e in data.get("chunk", [])
                    if e.get("type") == "m.room.message"]
