#!/usr/bin/env python3
"""Service exercise for the shared Defender gate; no firewall policy here.

Uses the same argv/evidence contract as Ollama's native-gate.py. The parent
Defender harness blocks this interpreter and the exact service executable.
"""
import json
import os
from pathlib import Path
import secrets
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request


def configuration(tool, binary, work, port, here):
    data = work / "data"
    data.mkdir()
    env = {k: v for k, v in os.environ.items()
           if k.upper() in {"SYSTEMROOT", "WINDIR", "PATH", "TEMP", "TMP", "COMSPEC"}}
    env["HOME"] = str(work)
    for name in ("no-tracking.env", "otel.env"):
        for line in (here / name).read_text().splitlines():
            if line and not line.startswith("#") and "=" in line:
                key, value = line.split("=", 1)
                env[key] = value
    if tool == "gitea":
        config = (here / "app.ini").read_text()
        replacements = {"DATA_DIR": data.as_posix(), "HTTP_ADDR": "127.0.0.1",
                        "HTTP_PORT": str(port), "ROOT_URL": f"http://127.0.0.1:{port}/",
                        "SECRET_KEY": secrets.token_hex(32)}
        for key, value in replacements.items():
            config = config.replace("{{" + key + "}}", value)
        ini = work / "app.ini"
        ini.write_text(config)
        env["GITEA_WORK_DIR"] = str(data)
        argv = [binary, "web", "--config", str(ini)]
        urls = [f"http://127.0.0.1:{port}{path}" for path in
                ("/api/healthz", "/api/v1/version", "/", "/explore/repos",
                 "/user/login", "/api/v1/settings/api")]
    elif tool == "zot":
        config = (here / "config.json").read_text()
        for key, value in {"DATA_DIR": data.as_posix(), "ADDR": "127.0.0.1",
                           "PORT": str(port)}.items():
            config = config.replace("{{" + key + "}}", value)
        config_path = work / "config.json"
        config_path.write_text(config)
        argv = [binary, "serve", str(config_path)]
        urls = [f"http://127.0.0.1:{port}{path}" for path in
                ("/v2/", "/v2/_catalog", "/v2/bashy-probe/tags/list",
                 "/v2/bashy-probe/manifests/latest")]
    elif tool == "seaweedfs":
        argv = [binary, "server", f"-dir={data}", "-ip=127.0.0.1", "-ip.bind=127.0.0.1",
                f"-master.port={port+1}", f"-volume.port={port+2}", "-filer",
                f"-filer.port={port+3}", "-s3", f"-s3.port={port}",
                "-volume.max=1", "-master.telemetry=false"]
        urls = [f"http://127.0.0.1:{port}/", f"http://127.0.0.1:{port}/bashy-probe/",
                f"http://127.0.0.1:{port+1}/cluster/status",
                f"http://127.0.0.1:{port+1}/dir/status", f"http://127.0.0.1:{port+3}/"]
    else:
        raise ValueError(f"unsupported service: {tool}")
    return argv, env, urls


def request(url):
    # No ambient proxy may route a loopback probe away from the gated service.
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    try:
        with opener.open(url, timeout=3) as response:
            return response.status
    except urllib.error.HTTPError as error:
        return error.code  # S3 correctly rejects unsigned requests with 403.


def exercise(tool, binary, work, port, here):
    argv, env, urls = configuration(tool, binary, work, port, here)
    log = work / "service.log"
    with log.open("w", encoding="utf-8") as output:
        proc = subprocess.Popen(argv, cwd=work, env=env, stdout=output, stderr=output)
        try:
            deadline = time.monotonic() + 180
            while True:
                if proc.poll() is not None:
                    raise RuntimeError(f"service exited before readiness: {proc.returncode}")
                try:
                    status = request(urls[0])
                    if tool == "seaweedfs" or status == 200:
                        break
                except (OSError, urllib.error.URLError):
                    pass
                if time.monotonic() >= deadline:
                    raise RuntimeError("service readiness deadline exceeded")
                time.sleep(1)
            # Exercise before and after the same 30s observation as Unix gates.
            for phase in range(2):
                for url in urls:
                    print(f"probe {url}: {request(url)}", flush=True)
                if phase == 0:
                    time.sleep(30)
            if proc.poll() is not None:
                raise RuntimeError("service exited during observation")
        finally:
            proc.terminate()
            try:
                proc.wait(timeout=15)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait(timeout=5)
    # The Defender harness also rejects denial signatures in this output.
    print(log.read_text(encoding="utf-8", errors="replace"), flush=True)


def main():
    binary, fixture, evidence, port = sys.argv[1:]
    evidence = Path(evidence)
    evidence.write_text(json.dumps({"status": "FAIL"}))
    tool = Path(fixture).read_text().strip()
    with tempfile.TemporaryDirectory(prefix="go-service-egress-") as temp:
        exercise(tool, binary, Path(temp), int(port), Path(__file__).resolve().parent)
    evidence.write_text(json.dumps({"status": "PROTOCOL_PASS", "tool": tool,
                                     "checks": ["ready", "http-probes", "idle", "shutdown"]}))


if __name__ == "__main__":
    main()
