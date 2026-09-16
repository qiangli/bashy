#!/usr/bin/env python3
"""Bounded native-decorator product checks against one explicit Bashy binary."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

binary = str(Path(sys.argv[1]).resolve())
checks = 0
with tempfile.TemporaryDirectory(prefix="bashy-decorators-") as scratch:
    root = Path(scratch)
    env = {"PATH": os.environ.get("PATH", "/usr/bin:/bin"), "HOME": scratch,
           "BASHY_HOME": str(root / "home"), "LC_ALL": "C", "BASHY_HINTS": "off",
           "OTEL_TRACES_EXPORTER": "file", "BASHY_OTEL_SPOOL": str(root / "spans.jsonl")}

    def run(source, extra=None):
        result = subprocess.run([binary, "--bashpp", "-c", source], env=env | (extra or {}),
                                cwd=scratch, capture_output=True, text=True, timeout=20)
        return result

    def check(condition, message):
        global checks
        if not condition:
            raise SystemExit("FAIL: " + message)
        checks += 1
        print("PASS: " + message)

    result = run('@trace()\nfunc work() { echo traced }\nwork()\n')
    spans = [json.loads(line) for line in (root / "spans.jsonl").read_text().splitlines()]
    check(result.returncode == 0 and result.stdout == "traced\n" and
          any(row.get("_msg") == "call work" or row.get("name") == "call work" for row in spans),
          "native trace produces a call span")

    result = run('@guard(effects: "read")\nfunc work() { touch denied }\nwork()\n',
                 {"BASHY_AUDIT": str(root / "audit.jsonl")})
    records = [json.loads(line) for line in (root / "audit.jsonl").read_text().splitlines()]
    check(result.returncode != 0 and not (root / "denied").exists() and
          any(row.get("decision") == "deny" and row.get("binary") == "touch" for row in records),
          "guard denies a command and records the denial")

    result = run('n=0\n@retry(n: 3, backoff: "0s")\nfunction work() { n=$((n+1)); [ "$n" -eq 3 ]; }\nwork\necho "$n:$?"\n')
    check(result.returncode == 0 and result.stdout == "3:0\n", "retry runs exactly three attempts")

    policy = root / "advice.json"
    policy.write_text(json.dumps({"schema": "bashy-advice-v1", "rules": [
        {"id": "deny-write", "name": "work", "decorator": "guard", "args": {"effects": "read"}}]}))
    source = 'func guard(c *Call) { echo bypass; c.Next(); }\nfunc work() { touch advised }\nwork()\n'
    result = run(source, {"BASHY_ADVICE": str(policy)})
    check(result.returncode != 0 and "bypass" not in result.stdout and not (root / "advised").exists(),
          "source cannot shadow a policy guard")
    result = run(source, {"BASHY_ADVICE": str(policy), "VSC_PROFILE": "cert"})
    check(result.returncode == 0 and (root / "advised").exists(), "certification loads zero advice rules")

    for filename, contents in [("broken.json", "{broken"), ("missing.json", None)]:
        path = root / filename
        if contents is not None:
            path.write_text(contents)
        result = run('func work() { echo escaped }\nwork()\n', {"BASHY_ADVICE": str(path)})
        check(result.returncode != 0 and "escaped" not in result.stdout and
              "advised calls refused" in result.stderr, filename + " fails closed")

    result = run('@retry(2, n: 3)\nfunc work() { echo escaped }\nwork()\n')
    check(result.returncode != 0 and "escaped" not in result.stdout, "duplicate retry arguments are rejected")
print(f"native decorators: {checks}/{checks} PASS ({binary})")
