#!/usr/bin/env python3
"""Local release build/package entry point. GitHub publication is external."""
import argparse
from concurrent.futures import ThreadPoolExecutor, as_completed
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import threading
import time

sys.dont_write_bytecode = True
from dist_package import package, digest, PACKAGE_WORKERS

ROOT = Path(__file__).resolve().parents[1]
NFPM_VERSION = "v2.47.0"  # qiangli/goreleaser 7af3788/go.mod, same packaging library.
SOURCES = {"bashy": "github.com/qiangli/bashy", "bash": "github.com/qiangli/bash",
           "outpost": "github.com/qiangli/outpost", "yoke": "github.com/qiangli/yoke"}
SIGNED_RUNNERS = {"linux/amd64", "linux/arm64", "darwin/arm64", "windows/amd64"}


def read_lock(path):
    return dict(line.split("=", 1) for line in path.read_text().splitlines()
                if line and not line.startswith("#") and "=" in line)


def load_inputs(path):
    inputs = json.loads(path.read_text())
    if not re.fullmatch(r"v\d+\.\d+\.\d+(?:-rc\.\d+)?-dev", inputs.get("tag", "")):
        raise ValueError("an exact -dev candidate tag is required")
    roots = {}
    for role in SOURCES:
        source = inputs["sources"][role]
        if not re.fullmatch(r"[0-9a-f]{40}", source["commit"]):
            raise ValueError(f"full source commit required: {role}")
        relative = Path(source["path"])
        if relative.is_absolute() or ".." in relative.parts:
            raise ValueError("source paths must stay within the prepared source root")
        roots[role] = (path.parent / relative).resolve()
    if len(set(roots.values())) != len(SOURCES):
        raise ValueError("independent source roles must have independent checkouts")
    return inputs, roots


def clean_source(path, expected):
    sha = subprocess.check_output(["git", "-C", str(path), "rev-parse", "HEAD"], text=True).strip()
    dirty = subprocess.check_output(["git", "-C", str(path), "status", "--porcelain"], text=True).strip()
    if sha != expected or dirty:
        raise ValueError(f"source identity/cleanliness differs: {path.name}")


def isolated_modcache(shared, destination, mutable):
    """Share immutable downloaded modules; copy only the runner's writable module."""
    destination.mkdir(parents=True)
    (destination / "cache").symlink_to(shared / "cache", target_is_directory=True)
    for current, directories, _ in os.walk(shared):
        directories[:] = [d for d in directories if d != "cache"]
        for name in directories[:]:
            if "@" not in name:
                continue
            source = Path(current) / name
            relative = source.relative_to(shared)
            target = destination / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            if source == mutable:
                shutil.copytree(source, target, symlinks=True)
                for blob in ("ycode-runner.gz", "ycode-runner.sigstore.json"):
                    (target / "external/ollama/runner_embed" / blob).unlink(missing_ok=True)
            else:
                target.symlink_to(source, target_is_directory=True)
            directories.remove(name)


class Dist:
    def __init__(self, inputs, sources, destination, jobs, budget):
        self.inputs, self.sources, self.destination = inputs, sources, destination
        self.jobs, self.budget = jobs, budget
        self.started_wall = time.time()
        self.started = time.monotonic()
        self.deadline = self.started + budget
        self.timings, self.guard = [], threading.Lock()
        self.platforms = json.loads((ROOT / "release/platforms.json").read_text())
        self.lock = read_lock(ROOT / "tools/bashy-build/image.lock")
        if os.environ.get("BASHY_BUILD_RUN") != "1" or digest(Path("/bashy")) != self.lock["bashy_sha256"]:
            raise ValueError("the locked previous-release builder is required")
        self.env = os.environ.copy()
        self.env.update(GOWORK="off", GOTOOLCHAIN="local", CGO_ENABLED="0", GOFLAGS="-p=1",
                        GOMAXPROCS=str(max(1, 8 // jobs)), GOMEMLIMIT="2GiB", BASHY="/bashy")
        self.tools = ROOT.parent / "build-tools"
        if (self.tools / "bin").is_dir():
            self.env["PATH"] = str(self.tools / "bin") + os.pathsep + self.env["PATH"]
        self.tool_receipt = json.loads((self.tools / "tool-receipt.json").read_text()) if (self.tools / "tool-receipt.json").is_file() else None
        if self.tool_receipt:
            for name, expected in self.tool_receipt["hashes"].items():
                if digest(self.tools / name) != expected:
                    raise ValueError("qualified build-tool hash differs: " + name)
        self.cache = Path(os.environ["GOMODCACHE"])
        self.work = Path(tempfile.mkdtemp(prefix="dist-", dir=os.environ["GOTMPDIR"]))
        self.logs = destination / "logs"
        self.logs.mkdir(parents=True, exist_ok=True)

    def run(self, name, command, cwd=None, env=None):
        start = time.monotonic()
        remaining = self.deadline - start
        if remaining <= 0:
            raise TimeoutError("dist budget exhausted before " + name)
        log = self.logs / (re.sub(r"[^a-zA-Z0-9_.-]", "_", name) + ".log")
        status = -1
        try:
            with log.open("wb") as stream:
                child = subprocess.Popen(command, cwd=cwd, env=env or self.env,
                                         stdout=stream, stderr=subprocess.STDOUT, start_new_session=True)
                try:
                    status = child.wait(timeout=remaining)
                except subprocess.TimeoutExpired:
                    os.killpg(child.pid, signal.SIGTERM)
                    try:
                        child.wait(timeout=3)
                    except subprocess.TimeoutExpired:
                        os.killpg(child.pid, signal.SIGKILL)
                        child.wait()
                    raise TimeoutError("dist budget exhausted during " + name)
            if status:
                raise RuntimeError(f"{name} failed ({status}); see {log.name}")
        finally:
            elapsed = round(time.monotonic() - start, 3)
            with self.guard:
                self.timings.append({"stage": name, "elapsed_seconds": elapsed, "exit_code": status})
            print(f"dist: {name}: {elapsed}s", flush=True)

    def prepare(self):
        if os.environ.get("BASHY_BUILD_RUN") != "1" or digest(Path("/bashy")) != self.lock["bashy_sha256"]:
            raise ValueError("the locked previous-release builder is required")
        for name in ("rpm", "dpkg-deb", "tar"):
            if not shutil.which(name, path=self.env["PATH"]):
                raise ValueError("missing package reader " + name + "; stage scripts/prepare-dist-rpm.sh outside the builder image")
        self.run("rpm-reader-version", ["rpm", "--version"])
        for role, source in self.sources.items():
            clean_source(source, self.inputs["sources"][role]["commit"])
            module = source / "external/otel" if role == "yoke" else source
            self.run("download-" + role, ["go", "mod", "download"], cwd=module)
        self.mutable_yoke = Path(subprocess.check_output(
            ["go", "list", "-mod=readonly", "-m", "-f", "{{.Dir}}", "github.com/qiangli/yoke"],
            cwd=self.sources["bashy"], env=self.env, text=True).strip())
        self.nfpm = Path(os.environ["GOCACHE"]).parent / "dist-tools" / "nfpm"
        if not self.nfpm.exists():
            self.nfpm.parent.mkdir(parents=True, exist_ok=True)
            environment = dict(self.env, GOBIN=str(self.nfpm.parent))
            self.run("nfpm-bootstrap", ["go", "install", "github.com/goreleaser/nfpm/v2/cmd/nfpm@" + NFPM_VERSION], env=environment)
        info = subprocess.check_output(["go", "version", "-m", str(self.nfpm)], text=True)
        if "github.com/goreleaser/nfpm/v2\t" + NFPM_VERSION not in info:
            raise ValueError("cached nFPM does not match its pinned version")
        # Initialize the shared verifier/cache once; target builds have private slots.
        self.run("signed-runner-cache", ["/usr/bin/dash", "scripts/fetch-signed-runner.sh", "linux", "amd64"],
                 self.sources["bashy"], dict(self.env, SIGNED_RUNNER_CACHE=str(self.work.parent / "signed-runner")))

    def target(self, target):
        goos, arch = target.split("/")
        workspace = self.work / goos / arch
        workspace.mkdir(parents=True)
        sources = {}
        for role, original in self.sources.items():
            copy = workspace / role
            self.run(target + "-source-" + role, ["git", "clone", "--quiet", "--shared", str(original), str(copy)])
            self.run(target + "-pin-" + role, ["git", "checkout", "--quiet", "--detach", self.inputs["sources"][role]["commit"]], copy)
            # Command adapters judge their cached binary stale by file mtime; a fresh
            # clone would make every warm adapter rebuild inside a timed smoke. Pin
            # the copy's mtimes to its commit time so the shared adapter cache stays warm.
            commit_time = int(subprocess.check_output(["git", "-C", str(copy), "show", "-s", "--format=%ct", "HEAD"], text=True).strip())
            for name in subprocess.check_output(["git", "-C", str(copy), "ls-files", "-z"], text=True).split("\0"):
                if name:
                    os.utime(copy / name, (commit_time, commit_time), follow_symlinks=False)
            sources[role] = copy
        module_cache = workspace / "modules"
        isolated_modcache(self.cache, module_cache, self.mutable_yoke)
        env = dict(self.env, GOOS=goos, GOARCH=arch, GOMODCACHE=str(module_cache),
                   SIGNED_RUNNER_CACHE=str(self.work.parent / "signed-runner"))
        stage = self.destination / "products" / goos / arch
        stage.mkdir(parents=True)
        suffix = ".exe" if goos == "windows" else ""
        base = self.inputs["tag"].removesuffix("-dev")
        source_sha = self.inputs["sources"]["bashy"]["commit"]
        runtime_pin = subprocess.check_output(["go", "list", "-m", "-f", "{{.Version}}", "github.com/qiangli/bash"],
                                               cwd=sources["bashy"], env=env, text=True).strip()
        runtime_sha = subprocess.check_output(["git", "rev-parse", runtime_pin.rsplit("-", 1)[-1]], cwd=sources["bash"], text=True).strip()
        runtime_time = subprocess.check_output(["git", "show", "-s", "--format=%cI", runtime_sha], cwd=sources["bash"], text=True).strip()
        flags = (f"-w -X github.com/bashsharp/bashsharp/cli.bashVersion=5.3.0(1)-bashy-{base} "
                 f"-X github.com/bashsharp/bashsharp/cli.buildID={source_sha} "
                 f"-X github.com/bashsharp/bashsharp/transpile.ShellRuntimeCommit={runtime_sha} "
                 f"-X github.com/bashsharp/bashsharp/transpile.ShellRuntimeCommitTime={runtime_time}")
        tags = "embed_runner" if target in SIGNED_RUNNERS else ""
        if tags:
            self.run(target + "-runner", ["/usr/bin/dash", "scripts/fetch-signed-runner.sh", goos, arch], sources["bashy"], env)
        elif target != "windows/arm64":
            raise ValueError("unsupported runner profile: " + target)
        self.run(target + "-bashy", ["/usr/bin/dash", "scripts/build-bashy-artifact.sh", str(stage / ("bashy" + suffix)), flags, tags], sources["bashy"], dict(env, BASHY_EXE="/bashy"))
        host_env = {key: value for key, value in env.items() if key not in ("GOOS", "GOARCH")}
        if [goos, arch] == subprocess.check_output(["go", "env", "GOHOSTOS", "GOHOSTARCH"], env=env, text=True).split():
            # The tracked SPA must be fresh against its source. Probe pnpm through the
            # CANDIDATE just built: the builder image's own bashy predates the toolchain
            # adapters and cannot provision pnpm (Sprint 692 release story).
            self.run(target + "-spa-fresh", ["/usr/bin/dash", "scripts/build-meet-spa.sh", "check"], sources["bashy"],
                     dict(host_env, BASHY_BIN=str(stage / ("bashy" + suffix))))
        self.run(target + "-spa", ["/usr/bin/dash", "scripts/verify-meet-spa-release.sh", str(stage / ("bashy" + suffix)), goos + "_" + arch], sources["bashy"], host_env)
        shell_sha = self.inputs["sources"]["bash"]["commit"]
        shell_time = subprocess.check_output(["git", "show", "-s", "--format=%cI", shell_sha], cwd=sources["bash"], text=True).strip()
        shell_flags = "-s " + flags.replace(source_sha, shell_sha).replace(runtime_sha, shell_sha).replace(runtime_time, shell_time)
        host = subprocess.check_output(["go", "env", "GOHOSTOS", "GOHOSTARCH"], env=env, text=True).split()
        native = [goos, arch] == host and goos in ("linux", "darwin") and shutil.which("cc")
        for shell in ("bash", "sh"):
            destination = stage / (shell + (".real" if native else suffix))
            self.run(target + "-" + shell, ["/usr/bin/dash", "../scripts/go-product.sh", "build", "-mod=readonly", "-trimpath", "-ldflags", shell_flags, "-o", str(destination), "./" + shell], sources["bash"] / "cmd", dict(env, BASHY_EXE=""))
            if native:
                self.run(target + "-" + shell + "-launcher", ["cc", "-x", "c", "-std=c11", "-O2", "-Wall", "-Wextra", "-Werror", "-o", str(stage / shell), str(sources["bash"] / "native/siglaunch.c.in")], env=env)
        outpost_sha = self.inputs["sources"]["outpost"]["commit"]
        outpost_flags = f"-s -w -X github.com/qiangli/outpost/internal/agent.releaseTag={base} -X github.com/qiangli/outpost/internal/agent.ldCommit={outpost_sha} -X github.com/qiangli/outpost/internal/agent.ldDirty=false"
        self.run(target + "-outpost", ["go", "build", "-mod=readonly", "-trimpath", "-tags", "fb_archives", "-ldflags", outpost_flags, "-o", str(stage / ("outpost" + suffix)), "./cmd/outpost"], sources["outpost"], env)
        if goos == "linux":
            self.run(target + "-scratch", ["/usr/bin/dash", "scripts/build-bashy-artifact.sh", str(stage / "bashy-scratch"), flags, "bashy_scratch"], sources["bashy"], dict(env, BASHY_EXE="/bashy"))
        if target in self.platforms["otel_platforms"]:
            self.run(target + "-otel", ["go", "build", "-trimpath", "-ldflags=-s -w", "-o", str(stage / ("otel" + suffix)), "./cmd/otel"], sources["yoke"] / "external/otel", env)
        sbom = self.destination / "metadata/sbom"
        sbom.mkdir(parents=True, exist_ok=True)
        self.run(target + "-sbom", ["/bashy", "release", "sbom", "--binary", str(stage / ("bashy" + suffix)), "--gate", "--os", goos, "--arch", arch, "-o", str(sbom / f"bashy-{goos}-{arch}.spdx.json")], sources["bashy"], env)

    def build(self):
        self.prepare()
        with ThreadPoolExecutor(max_workers=self.jobs) as pool:
            futures = [pool.submit(self.target, target) for target in self.platforms["platforms"]]
            for future in as_completed(futures):
                future.result()

    def checkpoint(self):
        (self.destination / "build-state.json").write_text(json.dumps({
            "inputs": self.inputs, "started_unix": self.started_wall,
            "timings": self.timings, "jobs": self.jobs, "budget": self.budget,
            "nfpm": str(self.nfpm), "complete": False}, indent=2) + "\n")

    def resume(self):
        state = json.loads((self.destination / "build-state.json").read_text())
        if state["inputs"] != self.inputs or state["jobs"] != self.jobs or state["budget"] != self.budget:
            raise ValueError("packaging inputs or limits differ from built candidate")
        self.started_wall = state["started_unix"]
        self.started = time.monotonic() - (time.time() - self.started_wall)
        self.deadline = self.started + self.budget
        self.timings = state["timings"]
        self.nfpm = Path(state["nfpm"])
        for role, source in self.sources.items():
            clean_source(source, self.inputs["sources"][role]["commit"])

    def seal(self):
        signature = self.destination / "metadata/signature-status/darwin-arm64.json"
        if not signature.is_file():
            raise ValueError("native Darwin signing finish/status required before packaging")
        signing = json.loads(signature.read_text())
        if not isinstance(signing.get("signed"), bool):
            raise ValueError("invalid native Darwin signing status")
        start = time.monotonic()
        layouts, assets = package(self.sources["bashy"], self.sources["outpost"], self.destination / "products",
                                  self.destination, self.inputs["tag"], self.platforms["platforms"], self.platforms["otel_platforms"], self.nfpm, run=self.run,
                                  expected={role: self.inputs["sources"]["bash" if role in ("bash", "sh") else role]["commit"] for role in ("bashy", "outpost", "bash", "sh")})
        self.run("linux-package-contents", ["/usr/bin/dash", "scripts/verify-linux-packages.sh", str(self.destination / "assets")], self.sources["bashy"])
        self.timings.append({"stage": "package-checksum", "elapsed_seconds": round(time.monotonic() - start, 3), "exit_code": 0})
        elapsed = round(time.monotonic() - self.started, 3)
        report = {"schema": "bashy-dist-v1", "source_sha": self.inputs["sources"]["bashy"]["commit"],
                  "tag": self.inputs["tag"], "sources": self.inputs["sources"],
                  "builder": {"version": self.lock["bashy_version"], "image": self.lock["image"], "digest": self.lock["image_digest"]},
                  "auxiliary_tool_receipt": self.tool_receipt,
                  "jobs": self.jobs, "package_workers": PACKAGE_WORKERS, "cpu_limit": 8, "cache": "persistent Go build/download caches; isolated mutable runner modules",
                  "elapsed_seconds": elapsed, "budget_seconds": self.budget, "budget_met": elapsed <= self.budget,
                  "signature_status": {"darwin/arm64": signing},
                  "runner_profiles": {target: "signed-embedded" if target in SIGNED_RUNNERS else "unavailable-no-embed" for target in self.platforms["platforms"]},
                  "complete": True, "assets": assets, "layouts": layouts, "timings": self.timings}
        (self.destination / "release-assets.json").write_text(json.dumps(report, indent=2) + "\n")
        if not report["budget_met"]:
            raise TimeoutError("complete dist exceeded the measured budget; publication is blocked")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--inputs", type=Path, default=ROOT.parent / "dist-inputs.json")
    parser.add_argument("--dist", type=Path, default=ROOT.parent / "dist")
    parser.add_argument("--jobs", type=int, default=2)
    parser.add_argument("--budget-seconds", type=int, default=300)
    parser.add_argument("--plan", action="store_true")
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--build-only", action="store_true")
    mode.add_argument("--package-only", action="store_true")
    args = parser.parse_args()
    # 300 s is the release budget to aim for (recorded in the manifest); a longer explicit
    # cap is allowed for a cold-cache run and is visible in elapsed_seconds.
    if not 1 <= args.jobs <= 4 or not 1 <= args.budget_seconds <= 1800:
        parser.error("jobs must be 1..4; the release budget must be 1..1800 seconds (300 is the target)")
    inputs, sources = load_inputs(args.inputs.resolve())
    if args.plan:
        print(json.dumps({"tag": inputs["tag"], "sources": inputs["sources"], "jobs": args.jobs,
                          "budget_seconds": args.budget_seconds,
                          "platforms": json.loads((ROOT / "release/platforms.json").read_text()),
                          "publication": "external; no GitHub API calls"}, indent=2))
        return
    if args.dist.exists() and not args.package_only:
        raise ValueError("fresh dist destination required")
    dist = Dist(inputs, sources, args.dist.resolve(), args.jobs, args.budget_seconds)
    try:
        if args.package_only:
            dist.resume()
        else:
            dist.build()
            dist.checkpoint()
        if not args.build_only:
            signature = dist.destination / "metadata/signature-status/darwin-arm64.json"
            if not signature.exists() and not args.package_only:
                signature.parent.mkdir(parents=True, exist_ok=True)
                dist.run("darwin-signing-finish", ["/bin/bash", "scripts/sign-darwin-release.sh",
                         str(dist.destination / "products/darwin/arm64"), str(signature)], dist.sources["bashy"])
            dist.seal()
    except Exception as exc:
        (dist.destination / "failure.json").write_text(json.dumps({"complete": False, "error": str(exc), "timings": dist.timings}, indent=2))
        raise


if __name__ == "__main__":
    main()
