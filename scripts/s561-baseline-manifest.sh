#!/bin/sh
# Recreate the Sprint 561 A1 build inventory. Run after GOWORK=off make build.
set -eu
cd "$(dirname "$0")/.."
source_commit=${1:-$(git rev-parse HEAD)}
GOWORK=off go mod edit -json | python3 -c '
import hashlib, json, os, pathlib, platform, re, subprocess, sys

mod = json.load(sys.stdin)
source = os.environ.get("S561_SOURCE_COMMIT", "") or sys.argv[1]
def digest(path):
    p = pathlib.Path(path)
    return hashlib.sha256(p.read_bytes()).hexdigest() if p.is_file() else None
def run(*args):
    return subprocess.check_output(args, text=True).strip()
def optional_harness_commit():
    p = os.environ.get("VSC_KIT", "/Users/qiangli/projects/poc/vsc-pcts-harness-kit")
    if not pathlib.Path(p, ".git").exists():
        return None
    return run("git", "-C", p, "rev-parse", "HEAD")
fixture = pathlib.Path("internal/agentos/verify.go").read_text()
tar = re.search(r"const bash53TarballSHA256 = \"([0-9a-f]{64})\"", fixture)
if not tar:
    raise SystemExit("pinned Bash tarball SHA-256 not found")
require = sorted(({"path": x["Path"], "version": x["Version"], "indirect": x.get("Indirect", False)} for x in mod["Require"]), key=lambda x: x["path"])
replace = sorted(({"old": x["Old"], "new": x["New"]} for x in mod["Replace"]), key=lambda x: (x["old"]["Path"], x["old"].get("Version", "")))
shell_runtime = next(x["new"]["Version"] for x in replace if x["old"]["Path"] == "mvdan.cc/sh/v3")
stamp = ["-X github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-dev",
         "-X github.com/qiangli/bashy/internal/cli.buildID=" + source[:7],
         "-X github.com/qiangli/bashsharp/transpile.ShellRuntimeCommit=" + shell_runtime,
         "-X github.com/qiangli/bashsharp/transpile.ShellRuntimeCommitTime="]
out = {
  "schema": "bashy.s561.baseline.v1",
  "source": {"repository": "github.com/qiangli/bashy", "commit": source,
             "go_mod_sha256": digest("go.mod"), "go_sum_sha256": digest("go.sum"),
             "bash53suite_tree": run("git", "rev-parse", source + ":tools/bash53suite"),
             "bash53suite_last_change_commit": run("git", "log", "-1", "--format=%H", source, "--", "tools/bash53suite")},
  "go_module": {"go": mod["Go"], "toolchain": mod.get("Toolchain"), "require": require, "replace": replace},
  "toolchain": {"version": run("go", "version"), "product_runtime_source_sha256": "a9dd82ee8b8089ec69dc90cc8a2d3553839db41b3f01807311507c7fc117a3e0"},
  "build": {"command": "GOWORK=off make build", "gowork": "off", "cgo_enabled": run("go", "env", "CGO_ENABLED"),
            "build_tags": ["bashy_runtime_snapshot"], "make_bashy_tags": [],
            "build_id": source[:7], "product_wrapper_sha256": digest("scripts/go-product.sh"),
            "flags": ["-trimpath", "-overlay (scripts/go-product.sh via tools/productgo)"],
            "bash_ldflags": ["-s", "-w"] + stamp, "bashy_ldflags": ["-w"] + stamp,
            "artifacts_sha256": {p: digest(p) for p in ("bin/bash", "bin/bash.real", "bin/bashy", "bin/bashy.real")},
            "artifact_note": "A null hash means this Makefile build did not emit that path; do not invent a .real payload."},
  "fixtures": {"gnu_bash_5_3_tarball_sha256": tar.group(1), "tarball_url": "https://ftp.gnu.org/gnu/bash/bash-5.3.tar.gz"},
  "certification": {"profile_d_external_utility_providers": [], "profile_b_system_provider_versions": None,
                    "profile_b_note": "Baseline not rerun by operator decision; record exact provider versions at A4 final-candidate run."},
  "container": {"test_bash_container_image_digest": None,
                "base_image": "docker.io/library/ubuntu@sha256:33ceb71981b602c1a7443a53469e4dba065f7503eab3078a2d7a57a2ab987517",
                "note": "No test image built for A1; fill from inspected image after an authorized run."},
  "host": {"os": platform.system(), "arch": platform.machine(),
           "inspected_licensed_harness_commit": optional_harness_commit()},
}
out["cert_closure"] = json.loads(run("python3", "scripts/cert-closure.py", "--rev", source))
print(json.dumps(out, indent=2, sort_keys=True))
' "$source_commit"
