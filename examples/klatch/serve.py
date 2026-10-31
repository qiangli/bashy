#!/usr/bin/env python3
"""`bashy klatch serve|stop|url` — run Klatch on the owner's own machine.

Nothing here names a host, a count or a handle. The owner chooses the data
directory and the port. Reach is one registration: when this machine runs an
outpost daemon, the app is registered with it under its name (`klatch`), and
outpost's own routing makes it reachable (satellite /matrix/h/<host>/app/klatch/).
Without outpost, `--lan` binds the LAN address as the offline fallback;
otherwise it stays on loopback. Nothing else is composed here.

Supply chain: the server binary comes from `bashy tools install klatch` (the
bashy-built, sigstore-verified pin). The transform helpers and the web bundle
come from the same release, verified against the sha256 pins in that reviewed
tool definition; a mismatch aborts. No unsigned or third-party build is used.
"""
import argparse
import hashlib
import io
import json
import os
import platform
import re
import shutil
import socket
import subprocess
import sys
import tarfile
import time
import urllib.request
from pathlib import Path

UNIT = """name: klatch
kind: service
description: Klatch chat server (bashy klatch serve)
tool: klatch
exec:
  - {binary}
  - -p
  - "{bind}:{port}"
  - -s
  - {data}
  - -w
  - {web}
  - -t
  - {transform}
ports:
  - name: http
    port: {port}
    bind: {bind}
health:
  http: /admin/status
  interval: 10s
  timeout: 3s
  failures: 3
  start_period: 30s
restart:
  policy: on-failure
  backoff: {{min: 1s, max: 30s, healthy_after: 30s}}
  start_limit: {{burst: 5, interval: 5m}}
stop:
  signal: TERM
  timeout: 20s
upgrade: restart
enabled: true
"""
WEB_PIN = re.compile(r"(https://\S+/klatch-web-[^\s/]+\.tar\.gz)\S*\s*\(sha256 ([0-9a-f]{64})\)")


def fail(message):
    print("klatch: " + message, file=sys.stderr)
    raise SystemExit(1)


def run(*args, stdin=None, check=True):
    env = dict(os.environ, BASHY_HINTS="off")
    proc = subprocess.run([os.environ.get("BASHY_BIN", "bashy"), *args], input=stdin, text=True,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
    if check and proc.returncode != 0:
        fail("bashy %s: %s" % (" ".join(args[:3]), proc.stderr.strip() or proc.returncode))
    return proc.stdout


def platform_key():
    osname = {"Darwin": "darwin", "Linux": "linux"}.get(platform.system())
    arch = {"x86_64": "amd64", "AMD64": "amd64", "arm64": "arm64", "aarch64": "arm64"}.get(platform.machine())
    if not osname or not arch:
        fail("no Klatch build for %s/%s" % (platform.system(), platform.machine()))
    return "%s/%s" % (osname, arch)


def tool_pins(runner=run, key=None):
    """Server archive and web bundle pins from the reviewed klatch tool definition."""
    doc = json.loads(runner("tools", "show", "klatch", "--json"))
    doc = doc.get("result", doc)
    managed = (doc.get("cli") or {}).get("managed") or {}
    asset = (managed.get("assets") or {}).get(key or platform_key())
    if not asset or not asset.get("url") or not asset.get("sha256"):
        fail("the klatch tool definition has no pinned build for this platform")
    notes = " ".join(str(doc.get(k) or "") for k in ("notes",)) + " " + str(managed.get("notes") or "")
    web = WEB_PIN.search(notes)
    if not web:
        fail("the klatch tool definition has no pinned web bundle")
    return {"server": (asset["url"], asset["sha256"]), "web": (web.group(1), web.group(2)),
            "version": managed.get("version", "")}


def fetch(url, sha256, opener=urllib.request.urlopen):
    with opener(url, timeout=120) as res:
        data = res.read()
    if hashlib.sha256(data).hexdigest() != sha256:
        fail("sha256 mismatch for %s" % url.rsplit("/", 1)[-1])
    return data


def extract(data, dest, prefix=""):
    """Extract regular files (under PREFIX, prefix stripped) into DEST; refuse links and escapes."""
    dest = Path(dest)
    if dest.exists():
        shutil.rmtree(dest)
    dest.mkdir(parents=True)
    count = 0
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as tar:
        for m in tar.getmembers():
            name = m.name[2:] if m.name.startswith("./") else m.name
            if prefix:
                if not name.startswith(prefix + "/"):
                    continue
                name = name[len(prefix) + 1:]
            if not name or m.isdir():
                continue
            if not m.isfile():
                fail("refusing non-regular archive member %s" % m.name)
            target = (dest / name).resolve()
            if dest.resolve() not in target.parents:
                fail("refusing archive member outside the target: %s" % m.name)
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(tar.extractfile(m).read())
            target.chmod(0o755 if m.mode & 0o111 else 0o644)
            count += 1
    if not count:
        fail("archive held no files%s" % (" under %s/" % prefix if prefix else ""))
    return count


APP = "klatch"


def outpost_present(runner):
    """True when an outpost daemon on this machine answers (apps list succeeds)."""
    try:
        runner("outpost", "apps", "list", check=True)
        return True
    except SystemExit:
        return False


def lan_ip():
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        s.connect(("192.0.2.1", 9))  # TEST-NET-1: no packet is sent for UDP connect
        return s.getsockname()[0]
    finally:
        s.close()


def check_space(path, min_free_gb):
    path = Path(path)
    probe = path if path.exists() else next(p for p in path.parents if p.exists())
    free = shutil.disk_usage(probe).free / 1e9
    if free < min_free_gb:
        fail("only %.1f GB free at %s (need %.1f GB); free space or pass --min-free" % (free, probe, min_free_gb))
    return free


def ensure_secret(ref, runner=run):
    if runner("secret", "show", ref, check=False).strip():
        return
    value = runner("secret", "gen")
    runner("secret", "set", ref, stdin=value)


def wait_healthy(base, timeout=60, opener=urllib.request.urlopen):
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            with opener(base + "/admin/status", timeout=3):
                return True
        except Exception:
            time.sleep(1)
    fail("Klatch did not become healthy at %s" % base)


def serve(args, runner=run, opener=urllib.request.urlopen, healthy=wait_healthy, setup=None):
    data = Path(args.data).expanduser()
    free = check_space(data, args.min_free)
    pins = tool_pins(runner)
    runner("tools", "install", "klatch")  # signed pin: digest + keyless sigstore, refuses substitutes
    doc = json.loads(runner("tools", "show", "klatch", "--json"))
    binary = ((doc.get("result", doc).get("cli") or {}).get("binary")) or ""
    if not os.path.isabs(binary):
        fail("klatch install did not record a pinned binary path")
    data.mkdir(parents=True, exist_ok=True)
    extract(fetch(*pins["server"], opener=opener), data / "transform", prefix="transform")
    extract(fetch(*pins["web"], opener=opener), data / "web")
    bind = "0.0.0.0" if args.lan else "127.0.0.1"
    unit = UNIT.format(binary=binary, bind=bind, port=args.port, data=data / "store",
                       web=data / "web", transform=data / "transform")
    (data / "store").mkdir(exist_ok=True)
    runner("service", "add", "-f", "-", "--force", stdin=unit)
    runner("service", "start", "klatch")
    local = "http://127.0.0.1:%d" % args.port
    healthy(local)
    reach = {"local": local}
    if outpost_present(runner):
        # One registration; outpost's routing does the rest. The public address
        # is listed by `outpost apps list` — record the app name, not a guess.
        runner("outpost", "apps", "add", APP, "--url", local)
        reach["outpost"] = "outpost-app:" + APP
    if args.lan:
        reach["lan"] = "http://%s:%d" % (lan_ip(), args.port)
    url = reach.get("outpost") or reach.get("lan") or local
    ensure_secret(args.admin_ref, runner)
    domain = args.domain or (url.split("://", 1)[1].rstrip("/") if "://" in url else "%s:%d" % (socket.gethostname(), args.port))
    (setup or admin_setup)(local, args.admin_ref, domain)
    runner("secret", "set", args.url_ref, stdin=url + "\n")
    return {"url": url, "local": local, "reach": sorted(k for k in reach if k != "local") or ["loopback"],
            "reaches": reach,
            "data": str(data), "version": pins["version"], "free_gb": round(free, 1),
            "admin_ref": args.admin_ref, "url_ref": args.url_ref}


def admin_setup(base, admin_ref, domain):
    """Bootstrap the admin password and closed access through provision.py's client."""
    sys.path.insert(0, str(Path(__file__).resolve().parent))
    import provision
    client = provision.Client(base)
    password = provision.vault(admin_ref)
    if client.bootstrap_needed():
        client.setup_admin(password)
    session = client.admin_session(password)
    if not session:
        fail("admin login failed for %s" % admin_ref)
    client.set_config(session, domain)


def main(argv):
    p = argparse.ArgumentParser(prog="klatch")
    sub = p.add_subparsers(dest="verb", required=True)
    s = sub.add_parser("serve", help="install (signed) and run Klatch on this machine")
    s.add_argument("--data", default=os.path.join(os.environ.get("BASHY_HOME", os.path.expanduser("~/.bashy")), "klatch"))
    s.add_argument("--port", type=int, default=7100)
    s.add_argument("--lan", action="store_true", help="also bind the LAN address (fallback without outpost)")
    s.add_argument("--domain", help="federation domain (default: the reach address)")
    s.add_argument("--min-free", type=float, default=5.0, help="GB that must stay free (default 5)")
    s.add_argument("--admin-ref", default="klatch/admin")
    s.add_argument("--url-ref", default="klatch/url")
    sub.add_parser("stop", help="stop the Klatch service")
    sub.add_parser("url", help="print the stored Klatch URL")
    args = p.parse_args(argv)
    if args.verb == "serve":
        print(json.dumps(serve(args), indent=2, sort_keys=True))
    elif args.verb == "stop":
        run("service", "stop", "klatch")
        print(json.dumps({"stopped": "klatch"}))
    else:
        print(run("secret", "show", "klatch/url").strip())


if __name__ == "__main__":
    main(sys.argv[1:])
