#!/usr/bin/env python3
"""Real two-host shaping promotion gate on a claimed OCI test host.

Uses two disposable containers with distinct homes, catalogs and source trees.
Only the candidate binary and tutorial are mounted read-only. Git transport
between hosts goes over a private network; no production remotes or credentials.
Requires an already cached image with bash, git, python3 and coreutils. Does not
build or download anything. Exit 2 is BLOCKED, never a passing gate.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shlex
import subprocess
import sys
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]


class Blocked(Exception):
    pass


class Gate:
    def __init__(self, args):
        self.args = args
        self.oci = shlex.split(args.oci)
        self.prefix = "shape-promotion-" + uuid.uuid4().hex[:12]
        self.network = self.prefix
        self.hosts = {role: self.prefix + "-" + role for role in ("a", "b")}
        self.started = []
        self.network_created = False
        self.steps = []
        self.records = []
        self.evidence = {"schema": "shaping-promotion-e2e-v1", "passed": False,
                         "checks": self.steps, "commands": self.records}

    def run(self, argv, expected=0, timeout=120):
        try:
            result = subprocess.run(argv, text=True, capture_output=True, timeout=timeout)
        except subprocess.TimeoutExpired as exc:
            def decode(value):
                return value.decode(errors="replace") if isinstance(value, bytes) else value or ""
            self.records.append({"argv": argv, "timeout_seconds": timeout,
                                 "stdout": decode(exc.stdout), "stderr": decode(exc.stderr)})
            raise
        self.records.append({"argv": argv, "exit_code": result.returncode,
                             "stdout": result.stdout, "stderr": result.stderr})
        if result.returncode != expected:
            raise AssertionError(f"command returned {result.returncode}, expected {expected}: "
                                 + shlex.join(argv) + "\n" + result.stdout + result.stderr)
        return result.stdout

    def host(self, role, *argv, expected=0):
        return self.run(self.oci + ["exec", self.hosts[role], *argv], expected)

    def shell(self, role, script):
        return self.host(role, "bash", "-euc", script)

    def shape(self, role, *argv, expected=0):
        try:
            return json.loads(self.host(role, "/candidate/bashy", "shape", *argv,
                                        "--json", expected=expected))
        except AssertionError:
            # Failed verification still emits its sealed evidence location.
            # Retain the check statuses/details before disposable hosts go away.
            try:
                result = json.loads(self.records[-1].get("stdout", ""))
                evidence = result.get("verdict", {}).get("evidence")
                if evidence:
                    raw = self.host(role, "cat", evidence)
                    self.evidence.setdefault("failed_verifications", []).append(json.loads(raw))
            except (ValueError, AssertionError, OSError):
                pass
            raise

    def check(self, condition, label):
        if not condition:
            raise AssertionError(label)
        self.steps.append(label)
        print(f"ok {len(self.steps)} - {label}", flush=True)

    def active(self, role):
        return self.shape(role, "status", "hello-world", "--user", "tester")["active"]

    def verify(self, role, candidate):
        result = self.shape(role, "verify", "hello-world", candidate, "--user", "tester")
        self.check(result["status"] == "verified", f"host {role} candidate verified")
        evidence = result["verdict"]["evidence"]
        raw = json.loads(self.host(role, "cat", evidence))
        # Inspect the actual sealed check record, not the agent's stdout.
        def smoke_pass(value):
            if isinstance(value, dict):
                if value.get("name") == "checks/smoke.bsh" and value.get("status") == "pass":
                    return True
                return any(smoke_pass(v) for v in value.values())
            return isinstance(value, list) and any(smoke_pass(v) for v in value)
        self.check(smoke_pass(raw), f"host {role} real smoke.bsh evidence passed")

    def execute(self):
        binary = Path(self.args.binary).resolve()
        if not binary.is_file() or not os.access(binary, os.X_OK):
            raise Blocked("--binary must name the built Linux candidate")
        if not self.args.image:
            raise Blocked("--image must name a cached Linux image with bash, git and python3")
        self.evidence["binary_sha256"] = hashlib.sha256(binary.read_bytes()).hexdigest()
        inputs = [Path(__file__).resolve(), *sorted((ROOT / "examples/shaping/hello-world").rglob("*"))]
        self.evidence["input_sha256"] = {
            str(path.relative_to(ROOT)): hashlib.sha256(path.read_bytes()).hexdigest()
            for path in inputs if path.is_file()
        }
        # Inspect before run; --pull=never makes missing prerequisites fail closed.
        try:
            self.evidence["image_id"] = self.run(self.oci + ["image", "inspect", self.args.image,
                                                           "--format", "{{.Id}}"]).strip()
        except (OSError, AssertionError) as exc:
            raise Blocked("cached OCI image/engine unavailable: " + str(exc)) from exc
        self.run(self.oci + ["network", "create", "--internal", self.network])
        self.network_created = True
        for role, name in self.hosts.items():
            # run can create a container and then fail to start it.
            self.started.append(name)
            self.run(self.oci + ["run", "-d", "--pull=never", "--name", name,
                                "--network", self.network, "--network-alias", "host-" + role,
                                "--hostname", "shape-host-" + role, "--cpus", "2", "--memory", "2g",
                                "-v", str(binary) + ":/candidate/bashy:ro",
                                "-v", str(ROOT / "examples/shaping/hello-world") + ":/example:ro",
                                "-e", "HOME=/work/user", "-e", "BASHY_HOME=/work/state",
                                "-e", "BASHY_FLEET_DIR=/work/fleet", "-e", "BASHY_HINTS=off",
                                "-e", "BASHY_BIN=/candidate/bashy", "-e", "GIT_TERMINAL_PROMPT=0",
                                "-e", "GIT_CONFIG_NOSYSTEM=1", "-w", "/tmp", self.args.image,
                                "bash", "-euc", "mkdir -p /work/user /work/state /work/fleet; cd /work; exec sleep infinity"])
            self.shell(role, "command -v git; command -v python3; /candidate/bashy --version")
        ids = [self.host(role, "cat", "/etc/hostname").strip() for role in self.hosts]
        self.check(len(set(ids)) == 2, "two distinct disposable hosts")
        # Internal networks need not run a DNS service (older Podman venues).
        address = self.run(self.oci + ["inspect", "--format",
                                       "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}",
                                       self.hosts["a"]]).strip()
        self.check(bool(address), "host a has a private network address")
        git_base = "git://" + address

        self.shell("a", "bash /example/register.sh /work/source\n"
                   "mkdir /work/remotes\n"
                   "git clone --bare /work/state/src/hello-world /work/remotes/org.git\n"
                   "git clone --bare /work/state/src/hello-world /work/remotes/upstream.git\n"
                   "git daemon --reuseaddr --export-all --enable=receive-pack "
                   "--base-path=/work/remotes --listen=0.0.0.0 --detach "
                   "--pid-file=/work/git-daemon.pid /work/remotes")
        for attempt in range(30):
            try:
                self.host("b", "git", "ls-remote", git_base + "/org.git", "HEAD")
                break
            except AssertionError:
                if attempt == 29:
                    raise
                time.sleep(0.2)
        # URL registration preserves the shared history. A local-directory
        # registration deliberately imports only files as a new base commit.
        self.host("b", "/candidate/bashy", "app", "add", "hello-world", "--type", "shaped",
                  "--source", git_base + "/org.git", "--label", "Hello World")
        for role in self.hosts:
            for tier in ("org", "upstream"):
                self.shape(role, "remote", "set", tier, f"{git_base}/{tier}.git", "--app", "hello-world")
        base = self.active("a")
        self.check(base == self.active("b"), "both hosts start on the same source revision")
        self.shell("a", "cp -R /work/state/src/hello-world /work/prepared\n"
                   "rm -rf /work/prepared/.git\n"
                   "sed -i 's/Hello, World!/Hello, Shaping!/' /work/prepared/public/index.html")
        candidate = self.shape("a", "wish", "hello-world", "Change greeting to Hello, Shaping!",
                               "--from", "/work/prepared", "--user", "tester")
        self.check(candidate["status"] == "ready", "host a imports greeting candidate")
        self.verify("a", candidate["id"])
        self.shape("a", "approve", "hello-world", candidate["id"], "--user", "tester")
        submission = self.shape("a", "submit", "hello-world", candidate["id"], "--user", "tester")
        self.check(submission["complete"], "host a submission has verified evidence")
        outgoing = self.shape("a", "outgoing", "--app", "hello-world", "--to", "org", expected=3)
        self.check(outgoing["pending"] == 1 and outgoing["changes"][0]["id"] == submission["id"],
                   "outgoing reports exactly the verified submission with exit 3")
        pushed = self.shape("a", "push", submission["id"], "--app", "hello-world", "--to", "org")
        branch = "refs/heads/" + pushed["pushed"][0]["branch"]
        remote_rev = self.host("b", "git", "ls-remote", git_base + "/org.git", branch).split()[0]
        self.check(remote_rev == submission["proposal_revision"], "org remote contains the exact proposal revision")
        if self.args.break_check == "org-ref":
            self.host("a", "git", "--git-dir=/work/remotes/org.git", "update-ref", "-d", branch)
        incoming = self.shape("b", "incoming", "--app", "hello-world", "--from", "org", expected=3)
        self.check(any(c.get("id") == submission["id"] and c["status"] == "incoming"
                       for c in incoming["incoming"]), "host b discovers the org submission")
        pulled = self.shape("b", "pull", branch, "--app", "hello-world", "--from", "org", "--user", "tester")
        self.check(pulled["pulled"]["kind"] == "change", "host b pulls into a side slot")
        for filename in ("checks/smoke.bsh", "contract.bsh"):
            original = self.host("b", "cat", "/work/state/src/hello-world/" + filename)
            received_file = self.host("b", "cat", pulled["pulled"]["path"] + "/" + filename)
            self.check(original == received_file, f"pull preserves trusted {filename}")
        self.check(self.active("b") == base, "pull does not activate unverified code")
        status = self.shape("b", "status", "hello-world", "--user", "tester")
        self.check(len(status["candidates"]) == 1, "pull creates exactly one host b candidate")
        received = status["candidates"][0]
        self.check(received["status"] == "ready", "received candidate awaits local verification")
        diff = self.host("b", "/candidate/bashy", "shape", "diff", "hello-world", received["id"], "--user", "tester")
        self.check("Hello, Shaping!" in diff and "Hello, World!" in diff, "host b received the greeting patch")
        self.verify("b", received["id"])
        self.shape("b", "approve", "hello-world", received["id"], "--user", "tester")
        self.check(self.active("b") == received["rev"], "host b activates only after local verification")
        self.shape("b", "undo", "hello-world", "--user", "tester")
        self.check(self.active("b") == base, "host b undo restores the original revision")
        self.check(self.active("a") == candidate["rev"], "host b undo leaves host a unchanged")
        upstream = self.shape("a", "push", submission["id"], "--app", "hello-world", "--to", "upstream")
        upstream_ref = "refs/heads/" + upstream["pushed"][0]["branch"]
        upstream_rev = self.host("b", "git", "ls-remote", git_base + "/upstream.git", upstream_ref).split()[0]
        self.check(upstream_rev == submission["proposal_revision"], "upstream records the exact submitted proposal")
        self.evidence.update(base_revision=base, submission=submission["id"],
                             proposal_revision=remote_rev, upstream_ref=upstream_ref,
                             upstream_revision=upstream_rev, passed=True)

    def cleanup(self):
        errors = []
        for name in reversed(self.started):
            try:
                self.run(self.oci + ["rm", "-f", name])
            except Exception as exc:
                errors.append(str(exc))
        if self.network_created:
            try:
                self.run(self.oci + ["network", "rm", self.network])
            except Exception as exc:
                errors.append(str(exc))
        if errors:
            raise RuntimeError("; ".join(errors))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--image", default=os.environ.get("SHAPING_PROMOTION_IMAGE"))
    parser.add_argument("--oci", default=os.environ.get("SHAPING_PROMOTION_OCI", "bashy oci"))
    parser.add_argument("--evidence", required=True, help="new evidence JSON path (never overwritten)")
    parser.add_argument("--break-check", choices=["org-ref"], help="deliberately remove the pushed org ref; gate must fail")
    args = parser.parse_args()
    evidence_path = Path(args.evidence)
    evidence_path.parent.mkdir(parents=True, exist_ok=True)
    # Reserve the output first; reruns cannot replace earlier failures.
    with evidence_path.open("x") as output:
        gate = Gate(args)
        code = 0
        try:
            gate.execute()
        except Blocked as exc:
            gate.evidence["error"] = str(exc)
            code = 2
        except Exception as exc:
            gate.evidence["error"] = str(exc)
            code = 1
        finally:
            try:
                gate.cleanup()
            except Exception as exc:
                gate.evidence["cleanup_error"] = str(exc)
                gate.evidence["passed"] = False
                code = 1
            json.dump(gate.evidence, output, indent=2)
            output.write("\n")
        print(("PASS" if code == 0 else "BLOCKED" if code == 2 else "FAIL")
              + ": shaping promotion: " + str(len(gate.steps)) + " checks; evidence=" + str(evidence_path))
        if code:
            print(gate.evidence.get("error", gate.evidence.get("cleanup_error")), file=sys.stderr)
        return code


if __name__ == "__main__":
    sys.exit(main())
