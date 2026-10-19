#!/usr/bin/env python3
"""Inventory the third-party Python packages the dhnt repos use, with CPython 3.15 readiness.

Stdlib only; run with `bashy python scripts/python-package-inventory.py`.
Writes docs/python-package-inventory.tsv and docs/python-package-inventory.md.

Scans bashy, yoke, sh, bashsharp, bashsharp-tests, coreutils, outpost and ycode
(siblings under --root, default $DHNT_ROOT, else the parent of this repo) for
requirements files, pyproject.toml, PEP 723 blocks, pip/uv install lines,
`--with` flags and non-stdlib imports in .py files and Python fences. Readiness
comes from the PyPI JSON API (sequential, polite). PyPI results change over
time, so a re-run is byte-identical only while PyPI is unchanged.
"""
import argparse, ast, json, os, re, subprocess, sys, time, tomllib, warnings
import urllib.error, urllib.request

REPOS = "bashy yoke sh bashsharp bashsharp-tests coreutils outpost ycode".split()
SKIP_DIRS = {".git", ".cache", "node_modules", "external", "vendor", "third_party", "tmp",
             "priorart", ".yash-tests", ".zsh-tests", "reference", ".venv", "__pycache__",
             "dist", "results"}
SKIP_PREFIXES = ("ycode/examples/agent-mini/src/",)
TEXT_EXT = {".sh", ".bash", ".bsh", ".bpp", ".md", ".yaml", ".yml", ".go", ".txt", ".py",
            ".toml", ".dhnt", ".mk", ".json"}
TEXT_NAMES = {"Makefile", "Containerfile", "Dockerfile"}
STDLIB = set(sys.stdlib_module_names)
warnings.simplefilter("ignore")

# Modules imported by first-party code that are not PyPI packages.
LOCAL_IMPORTS = {
    "minisweagent_bounded": "first-party harness package",
    "stats": "fixture-local module",
    "sdk": "test-local module",
    "arm_home": "hardware-vendor SDK named in validation evidence, not on PyPI",
    "openenv": "package under test, supplied by the checkout the dag drives",
    "echo_env": "package under test, supplied by the checkout the dag drives",
    "minisweagent": "package under test, supplied by the checkout the dag drives",
    "nanochat": "package under test, supplied by the checkout the dag drives",
    "harbor": "package under test, supplied by the checkout the dag drives",
}
IMPORT_TO_DIST = {"yaml": "pyyaml"}

# Optional checkouts the python-package gate imports from; their direct deps count.
EXTERNAL = {
    "nanochat": ("nanochat", "dc54a1a"),
    "mini-swe-agent": ("mini-swe-agent", "04d809c"),
}

# Call-site purposes, longest matching prefix wins.
PURPOSES = [
    ("bashy/builds/openclaw/", "openclaw image build: rootfs patching, tree comparison, mock OTLP collector, egress probe"),
    ("bashy/builds/prima.cpp/", "prima.cpp build audit: input collection, tiny model fixture, egress exercise, license review"),
    ("bashy/builds/ollama/", "ollama build audit: compiler identity/evidence, fixture fetch, native gate"),
    ("bashy/builds/_shared/", "shared build helpers: realpath of the interpreter for egress firewall, Windows compiler discovery"),
    ("bashy/builds/", "build recipe helper and egress/exercise checks"),
    ("bashy/scripts/", "bashy repo script (see first call)"),
    ("bashy/.github/workflows/", "CI workflow step"),
    ("bashy/eval/", "agent-shell eval runner"),
    ("bashy/internal/", "Go test that needs python3 on PATH"),
    ("yoke/", "yoke Go test or script using python3 (one-liner or availability probe)"),
    ("sh/", "interpreter/Bash# test or grammar fixture using a Python fence or import"),
    ("coreutils/", "coreutils script or CI step"),
    ("ycode/evals/", "ycode eval spec setup command"),
    ("bashsharp-tests/tests/", "Bash# conformance fixture check"),
    ("bashy/examples/", "example pipelines and fixtures"),
    ("bashy/docs/", "documentation snippets"),
    ("bashsharp-tests/tools/", "Bash# polyglot and Python-package gates"),
    ("ycode/internal/eval/harness/", "eval harness runners and setup"),
    ("ycode/examples/", "ycode example agents and benchmarks"),
]

NOTES = {
    "scancode-toolkit": "its wheels are version-tagged cp3NN-none-any up to cp314, so the sdist is the only 3.15 route",
    "pyyaml": "an sdist install needs no compiler: it falls back to the pure-Python loader (no libyaml speedups)",
    "litellm": "requires-python is capped below 3.15, so pip and uv refuse to resolve it on 3.15",
    "contree-sdk": "requires-python is capped below 3.15, so pip and uv refuse to resolve it on 3.15",
    "aider-chat": "requires-python is capped below 3.13, so it already cannot run on the current 3.13 default; it needs 3.12",
    "torch": "no 3.15 wheels on any platform; nanochat cannot run under 3.15 until torch ships them",
}
PIP_RE = re.compile(r"(?:\bpip3?|\bpython3?\s+-m\s+pip|\buv\s+pip|\buv\s+tool|\buv\s+add|\bpipx)\s+(?:install|add)\s+(.*)")
UVX_RE = re.compile(r"(?:\buvx|\bpipx\s+run|\buv\s+tool\s+run)\s+(\S+)")
WITH_RE = re.compile(r"--with[ =][\"']?([^\s\"']+)")
PY_CALL_RE = re.compile(r"(?:^|[\s;&|(`$\"'=])(?:bashy\s+)?python3?(?:\s+(?:-[A-Za-z]\b|[\w./\"$-]+\.py\b|-))")
GO_EXEC_RE = re.compile(r'(?:exec\.Command(?:Context)?\([^)]*|LookPath\()"python3?"')
FENCE_RE = re.compile(r"^~~~\s*(?:python|py)\b[^\n]*\n(.*?)^~~~\s*$", re.S | re.M)
VALUE_FLAGS = {"-r", "-c", "-i", "-f", "-t", "--python", "--target", "--index-url", "--extra-index-url",
               "--find-links", "--constraint", "--requirement", "--prefix", "--root", "-p"}
WORD_DENY = {"it", "them", "this", "that", "here", "from", "the", "a", "pip", "uv", "install", "editable", "etc", "for", "all"}


def norm(name):
    return re.sub(r"[-_.]+", "-", name).lower()


def split_req(spec):
    """Return (normalized name, version spec) or None for a requirement string."""
    spec = re.sub(r"\s*([<>=!~]=?)\s*", r"\1", spec.strip().strip("\"'`,"))
    spec = re.sub(r"\$\{[A-Za-z_]+:-([^}]*)\}", r"\1", spec)
    m = re.match(r"^([A-Za-z0-9][A-Za-z0-9._-]*)(\[[^\]]*\])?\s*((?:[<>=!~]=?|===)[^;#\s]*(?:\s*,\s*(?:[<>=!~]=?)[^;#\s,]*)*)?", spec)
    if not m:
        return None
    return norm(m.group(1)), (m.group(3) or "").replace(" ", "")


def repo_files(root, repo):
    base = os.path.join(root, repo)
    try:
        out = subprocess.run(["git", "-C", base, "ls-files"], capture_output=True, text=True, check=True).stdout
        files = [f for f in out.splitlines() if os.path.isfile(os.path.join(base, f))]
    except (OSError, subprocess.CalledProcessError):
        files = []
        for d, ds, fs in os.walk(base):
            ds[:] = sorted(x for x in ds if x not in SKIP_DIRS)
            files += [os.path.relpath(os.path.join(d, f), base) for f in fs]
    keep = []
    for f in sorted(files):
        if set(f.split("/")[:-1]) & SKIP_DIRS or (repo + "/" + f).startswith(SKIP_PREFIXES):
            continue
        if os.path.splitext(f)[1] in TEXT_EXT or os.path.basename(f) in TEXT_NAMES:
            keep.append(f)
    return keep


def read(path):
    try:
        if os.path.getsize(path) > 2_000_000:
            return ""
        with open(path, encoding="utf-8", errors="replace") as fh:
            return fh.read()
    except OSError:
        return ""


def clean_tokens(rest):
    rest = re.split(r"&&|\|\||[;|>`)]|\s#", rest)[0]
    toks, skip = [], False
    for t in rest.split():
        if skip:
            skip = False
            continue
        if t in VALUE_FLAGS:
            skip = True
            continue
        if t.startswith("-"):
            continue
        t = t.strip("\"',.)")
        if not t or t in WORD_DENY or not re.match(r"^[A-Za-z][A-Za-z0-9._-]*(\[[\w,-]+\])?([<>=!~]=?[\w.*${}:,<>=!~-]+)?$", t) or t.endswith((".txt", ".whl", ".toml", "-")):
            continue
        toks.append(t)
    return toks


class Inv:
    def __init__(self):
        self.pkgs = {}       # name -> {"pins": set, "sites": set, "kinds": set}
        self.callsites = {}  # path -> {"n": int, "third": set}
        self.unresolved = {}

    def add(self, name, pin, site, kind):
        d = self.pkgs.setdefault(name, {"pins": set(), "sites": set(), "kinds": set()})
        if pin:
            d["pins"].add(pin)
        d["sites"].add(site)
        d["kinds"].add(kind)

    def add_req(self, spec, site, kind):
        r = split_req(spec)
        if r:
            self.add(r[0], r[1], site, kind)


def scan_pyproject(inv, text, site, kind):
    try:
        data = tomllib.loads(text)
    except tomllib.TOMLDecodeError:
        return
    proj = data.get("project", {})
    for dep in proj.get("dependencies", []):
        inv.add_req(dep, site, kind)
    for deps in proj.get("optional-dependencies", {}).values():
        for dep in deps:
            if not dep.startswith("mini-swe-agent["):
                inv.add_req(dep, site, kind)
    for deps in data.get("dependency-groups", {}).values():
        for dep in deps:
            if isinstance(dep, str):
                inv.add_req(dep, site, kind)


def scan_imports(inv, tree, local, site):
    for n in ast.walk(tree):
        mods = []
        if isinstance(n, ast.Import):
            mods = [a.name for a in n.names]
        elif isinstance(n, ast.ImportFrom) and n.level == 0 and n.module:
            mods = [n.module]
        for m in mods:
            top = m.split(".")[0]
            if top in STDLIB or top in local or top == "__future__":
                continue
            if top in LOCAL_IMPORTS:
                inv.unresolved.setdefault(top, set()).add(site)
                continue
            inv.add(IMPORT_TO_DIST.get(top, norm(top)), "", site, "import")


def scan_repo(inv, root, repo):
    base = os.path.join(root, repo)
    files = repo_files(root, repo)
    local_by_dir = {}
    for f in files:
        if f.endswith(".py"):
            local_by_dir.setdefault(os.path.dirname(f), set()).add(os.path.basename(f)[:-3])
    for f in files:
        site = repo + ":" + f
        text = read(os.path.join(base, f))
        name = os.path.basename(f)
        in_docs = f.startswith("docs/") or (f.endswith(".md") and name != "dag.md")
        kind = "doc" if in_docs else "cmd"
        if re.match(r"requirements.*\.txt$", name):
            for line in text.splitlines():
                line = line.split("#")[0].strip()
                if line and not line.startswith("-"):
                    inv.add_req(line, site, "requirements")
            continue
        if name == "pyproject.toml":
            scan_pyproject(inv, text, site, "pyproject")
        for m in re.finditer(r"^# /// script\n((?:#.*\n)+?)# ///", text, re.M):
            body = "\n".join(l[2:] for l in m.group(1).splitlines())
            for dep in re.findall(r'"([^"]+)"', body):
                inv.add_req(dep, site, "pep723")
        if name == "build.bsh" or f.endswith(".bsh"):
            for m in re.finditer(r"^~~~pyproject\n(.*?)^~~~", text, re.M | re.S):
                scan_pyproject(inv, m.group(1), site, "pyproject")
        if f.endswith(".py"):
            try:
                scan_imports(inv, ast.parse(text), local_by_dir.get(os.path.dirname(f), set()), site)
            except (SyntaxError, ValueError):
                pass
        if f.endswith((".md", ".bsh", ".bpp", ".sh", ".go")):
            for m in FENCE_RE.finditer(text):
                try:
                    scan_imports(inv, ast.parse(m.group(1)), set(), site)
                except (SyntaxError, ValueError):
                    pass
        for line in text.splitlines():
            if f.endswith(".go") and "pip" not in line and "uvx" not in line:
                continue
            m = PIP_RE.search(line)
            if m:
                for t in clean_tokens(m.group(1)):
                    inv.add_req(t, site, kind)
            m = UVX_RE.search(line)
            if m:
                for t in clean_tokens(m.group(1)):
                    inv.add_req(t, site, kind)
            for w in WITH_RE.findall(line):
                if "uv" in line:
                    inv.add_req(w, site, kind)
        shellish = f.endswith((".sh", ".bash", ".bsh", ".bpp", ".yml", ".yaml")) or name in ("Makefile", "dag.md")
        if shellish:
            n, third, first = 0, set(), ""
            for line in text.splitlines():
                if PY_CALL_RE.search(line) and not line.lstrip().startswith(("#", "//")) and "uv run" not in line and ".venv/" not in line:
                    n += 1
                    first = first or line.strip()
                m = re.match(r"^\s*(?:import|from)\s+([A-Za-z_]\w*)", line)
                if m and n and m.group(1) not in STDLIB:
                    third.add(m.group(1))
            if n:
                inv.callsites[site] = {"n": n, "third": third, "first": first}
        elif f.endswith(".go") and GO_EXEC_RE.search(text):
            inv.callsites[site] = {"n": len(GO_EXEC_RE.findall(text)), "third": set(), "first": GO_EXEC_RE.search(text).group(0)}


def scan_external(inv, root):
    for name, (label, sha) in EXTERNAL.items():
        path = os.path.join(root, "..", name, "pyproject.toml")
        if os.path.isfile(path):
            scan_pyproject(inv, read(path), "external:%s@%s/pyproject.toml" % (label, sha), "external")


def fetch(url, retries=2):
    for i in range(retries + 1):
        try:
            with urllib.request.urlopen(urllib.request.Request(url, headers={"User-Agent": "dhnt-python-inventory"}), timeout=20) as r:
                return json.load(r)
        except urllib.error.HTTPError as e:
            if e.code == 404:
                return None
        except (urllib.error.URLError, TimeoutError, OSError, ValueError):
            pass
        time.sleep(1 + i)
    return "error"


PLATFORMS = {
    "macos_arm64": lambda p: re.search(r"macosx_\d+_\d+_(arm64|universal2)$", p) is not None,
    "manylinux_x86_64": lambda p: re.search(r"manylinux[\w]*_x86_64", p) is not None,
    "musllinux_x86_64": lambda p: re.search(r"musllinux_\d+_\d+_x86_64", p) is not None,
    "win_amd64": lambda p: p == "win_amd64" or "win_amd64" in p.split("."),
}


def wheel_compatible(py_tag, abi_tag):
    """True when a wheel's python/abi tags load on CPython 3.15 (not free-threaded)."""
    for py in py_tag.split("."):
        if py.startswith("py3") or py == "py2.py3":
            return True
        m = re.match(r"cp(\d)(\d+)$", py)
        if m and m.group(1) == "3":
            minor = int(m.group(2))
            if minor == 15 and abi_tag.split(".")[0] in ("cp315", "abi3", "none"):
                return True
            if minor <= 15 and "abi3" in abi_tag.split("."):
                return True
    return False


def readiness(files):
    pure = False
    plat = {k: "none" for k in PLATFORMS}
    sdist = False
    newest_cp = 0
    for f in files:
        fn = f["filename"]
        if f.get("yanked"):
            continue
        if fn.endswith(".whl"):
            parts = fn[:-4].split("-")
            py_tag, abi_tag, plat_tag = parts[-3], parts[-2], parts[-1]
            for py in py_tag.split("."):
                m = re.match(r"cp3(\d+)$", py)
                if m:
                    newest_cp = max(newest_cp, int(m.group(1)))
            if not wheel_compatible(py_tag, abi_tag):
                continue
            if plat_tag == "any":
                pure = True
                continue
            for k, test in PLATFORMS.items():
                if any(test(p) for p in plat_tag.split(".")):
                    plat[k] = "wheel"
        elif fn.endswith((".tar.gz", ".zip")):
            sdist = True
    if pure:
        return "pure", {k: "pure" for k in PLATFORMS}, ""
    for k in plat:
        if plat[k] == "none" and sdist:
            plat[k] = "sdist-only"
    have = [k for k, v in plat.items() if v == "wheel"]
    if len(have) == len(PLATFORMS):
        return "yes", plat, ""
    missing = ", ".join("%s (%s)" % (k, v) for k, v in plat.items() if v != "wheel")
    newest = ("; newest wheels are cp3%d" % newest_cp) if newest_cp else ""
    state = "partial" if have else "no"
    return state, plat, "no cp315-compatible wheel for " + missing + newest


def license_of(info):
    expr = (info.get("license_expression") or "").strip()
    if expr:
        return expr
    lic = (info.get("license") or "").strip()
    if lic and "\n" not in lic and len(lic) <= 60:
        return lic
    cls = [c.split(" :: ")[-1] for c in info.get("classifiers", []) if c.startswith("License :: ")]
    cls = [c for c in cls if c != "OSI Approved"]
    return " / ".join(sorted(set(cls))) if cls else "see PyPI"


def requires_gap(rp):
    rp = rp or ""
    for m in re.finditer(r"(<=?)\s*3\.(\d+)", rp):
        limit = int(m.group(2)) - (1 if m.group(1) == "<" else 0)
        if limit < 15:
            return "requires-python %s excludes 3.15" % rp
    return ""


def evaluate(name, pins):
    data = fetch("https://pypi.org/pypi/%s/json" % name)
    time.sleep(0.25)
    row = {"name": name, "found": isinstance(data, dict)}
    if not row["found"]:
        row.update(status="error" if data == "error" else "not-on-pypi", latest="", license="", requires_python="", plat={k: "?" for k in PLATFORMS}, gap="PyPI lookup %s" % ("failed" if data == "error" else "returned 404"), pin_status="", pin_gap="")
        return row
    info = data["info"]
    state, plat, gap = readiness(data.get("urls", []))
    rg = requires_gap(info.get("requires_python"))
    if rg:
        state, gap = "no", (rg + "; " + gap).strip("; ")
    if NOTES.get(name) and state not in ("pure", "yes"):
        gap = (gap + "; " if gap else "") + "note: " + NOTES[name]
    row.update(status=state, latest=info["version"], license=license_of(info), requires_python=info.get("requires_python") or "", plat=plat, gap=gap, pin_status="", pin_gap="")
    exact = [p[2:] for p in sorted(pins) if p.startswith("==")]
    if exact and exact[0] != info["version"]:
        pdata = fetch("https://pypi.org/pypi/%s/%s/json" % (name, exact[0]))
        time.sleep(0.25)
        if isinstance(pdata, dict):
            ps, _, pg = readiness(pdata.get("urls", []))
            same = ps == state and pg == readiness(data.get("urls", []))[2]
            row.update(pin_status=ps, pin_gap=("pinned %s: " % exact[0] + pg) if pg and not same else "")
        else:
            row.update(pin_status="error", pin_gap="pinned %s: PyPI lookup failed" % exact[0])
    elif exact:
        row["pin_status"] = state
    return row


def pinned(d):
    exact = sorted(p for p in d["pins"] if p.startswith("=="))
    return ",".join(exact) or "unpinned"


def purpose(path):
    rel = path.split(":", 1)[1]
    repo = path.split(":", 1)[0]
    full = repo + "/" + rel
    best = ""
    for pre, text in PURPOSES:
        if full.startswith(pre) and len(pre) > len(best):
            best, why = pre, text
    return why if best else "other"


def write_outputs(repo_root, inv, rows):
    cols = ["package", "pinned", "constraints", "kinds", "usage_sites", "license", "latest", "requires_python",
            "macos_arm64", "manylinux_x86_64", "musllinux_x86_64", "win_amd64", "ready_315", "pinned_ready_315", "gap"]
    lines = ["\t".join(cols)]
    for r in rows:
        d = inv.pkgs[r["name"]]
        gap = "; ".join(x for x in (r["gap"], r["pin_gap"]) if x)
        lines.append("\t".join([
            r["name"], pinned(d), ",".join(sorted(d["pins"])) or "none", ",".join(sorted(d["kinds"])),
            ";".join(sorted(d["sites"])), r["license"], r["latest"], r["requires_python"],
            *[r["plat"][k] for k in PLATFORMS], r["status"], r["pin_status"], gap]).replace("\n", " "))
    with open(os.path.join(repo_root, "docs", "python-package-inventory.tsv"), "w", encoding="utf-8", newline="\n") as fh:
        fh.write("\n".join(lines) + "\n")

    counts = {}
    for r in rows:
        counts[r["status"]] = counts.get(r["status"], 0) + 1
    runtime = [r for r in rows if "doc" not in inv.pkgs[r["name"]]["kinds"] or len(inv.pkgs[r["name"]]["kinds"]) > 1]
    notready = [r for r in rows if r["status"] in ("no", "partial", "error", "not-on-pypi") or r["pin_status"] in ("no", "partial", "error")]
    md = ["# Python package inventory", "",
          "Generated by `scripts/python-package-inventory.py` from the sibling repos; do not edit by hand.",
          "The full per-package data (usage sites, versions, per-platform wheel status) is in",
          "[`python-package-inventory.tsv`](python-package-inventory.tsv).", "",
          "No repo carries a `requirements.txt`, `pyproject.toml` or lock file for bashy itself: packages are declared",
          "inline (pip/uv commands, `--with`, Python fences, imports). The inventory scans bashy, yoke, sh, bashsharp,",
          "bashsharp-tests, coreutils, outpost and ycode, plus the direct dependencies of the two optional external",
          "checkouts the Python-package gate imports (nanochat, mini-SWE-agent).", "",
          "## Summary", "",
          "- Third-party packages: **%d**" % len(rows)]
    labels = [("pure", "pure-Python wheel (works on 3.15 on every platform)"),
              ("yes", "binary wheels for all four platforms"),
              ("partial", "wheels for some platforms only"),
              ("no", "no cp315-compatible wheel"),
              ("not-on-pypi", "name not found on PyPI (likely scan noise or private)"),
              ("error", "PyPI lookup failed")]
    for k, label in labels:
        if counts.get(k):
            md.append("- `%s`: %d, %s" % (k, counts[k], label))
    md += ["", "Readiness is judged on PyPI's latest release (and the pinned release when pinned with `==`), for macOS arm64,",
           "manylinux x86_64, musllinux x86_64 and win_amd64. A wheel counts when it is `py3-none-any`, `cp315`, or",
           "`abi3` built for CPython 3.15 or older. `sdist-only` means a source build (compiler needed) is the only route.", "",
           "| Package | Pinned (==) | Licence | Latest | 3.15 | macOS arm64 | manylinux x86_64 | musllinux x86_64 | win_amd64 | Where |",
           "|---|---|---|---|---|---|---|---|---|---|"]
    for r in rows:
        d = inv.pkgs[r["name"]]
        sites = sorted(d["sites"])
        where = "<br>".join("`%s`" % s for s in sites[:3]) + (" (+%d more)" % (len(sites) - 3) if len(sites) > 3 else "")
        md.append("| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |" % (
            r["name"], pinned(d), r["license"].replace("|", "/"), r["latest"] or "-", r["status"],
            *[r["plat"][k] for k in PLATFORMS], where))
    md += ["", "## Not ready for 3.15", ""]
    if not notready:
        md.append("None: every inventoried package is pure-Python or ships wheels for all four platforms.")
    else:
        md += ["Each package below is recorded with its exact gap; none is dropped. The workaround is to pin the line for",
               "the project that needs it with a `.python-version` file (`echo 3.14 > .python-version`; `.python-version`",
               "wins over the managed default, as does `BASHY_PYTHON_VERSION=3.14`). Nothing is silently downgraded.", ""]
        for r in notready:
            d = inv.pkgs[r["name"]]
            md.append("### %s" % r["name"])
            md.append("")
            md.append("- Status: `%s` (latest %s)%s" % (r["status"], r["latest"] or "n/a", (", pinned release: `%s`" % r["pin_status"]) if r["pin_status"] else ""))
            md.append("- Exact gap: %s" % ("; ".join(x for x in (r["gap"], r["pin_gap"]) if x) or "none on latest"))
            md.append("- Used at: " + ", ".join("`%s`" % s for s in sorted(d["sites"])))
            md.append("- Workaround: pin 3.14 through `.python-version` in the project directory that runs it.")
            md.append("")
    md += ["## Stdlib-only Python call sites", "",
           "Files that invoke `python`/`python3` (or `bashy python`) directly. Third-party imports found in the Python",
           "they run are listed in the package table above; a file here with no package attributed to it uses only the",
           "standard library. Inline snippets in the shell files are scanned for non-stdlib `import` lines too.", "",
           "| Path | Calls | Purpose | First call | Inline third-party imports |", "|---|---|---|---|---|"]
    for path in sorted(inv.callsites):
        c = inv.callsites[path]
        first = c["first"].replace("qiangli", "<org>").replace("|", "\\|").replace("`", "'")[:90]
        md.append("| `%s` | %d | %s | `%s` | %s |" % (path, c["n"], purpose(path), first, ", ".join(sorted(c["third"])) or "none"))
    py_tp = {}
    for name, d in inv.pkgs.items():
        for s in d["sites"]:
            if "import" in d["kinds"] and s.endswith((".py", ".md", ".bsh", ".bpp", ".sh")):
                py_tp.setdefault(s, set()).add(name)
    md += ["", "Python files with third-party imports (everything else scanned imports only the standard library):", ""]
    for s in sorted(py_tp):
        md.append("- `%s`: %s" % (s, ", ".join(sorted(py_tp[s]))))
    if inv.unresolved:
        md += ["", "Imports ignored as non-PyPI (first-party or evidence-only):", ""]
        for k in sorted(inv.unresolved):
            md.append("- `%s`: %s (%s)" % (k, LOCAL_IMPORTS[k], ", ".join("`%s`" % s for s in sorted(inv.unresolved[k]))))
    md += ["", "## Out of scope", "",
           "Transitive dependencies of the external checkouts the dag examples drive (`uv sync --frozen` in harbor, openenv,",
           "hermes-agent, nanochat, mini-SWE-agent) come from those projects' own lock files and are not inventoried; only",
           "the two gate checkouts' direct dependencies are. Vendored upstream trees (`external/`, `priorart/`, `.cache/`,",
           "`ycode/examples/agent-mini/src/`) are skipped. The managed interpreter itself (python-build-standalone via uv)",
           "is provisioned by yoke and is not a package.", "",
           "## Refresh", "",
           "```sh",
           "DHNT_ROOT=/path/to/umbrella bashy python scripts/python-package-inventory.py",
           "```", "",
           "`--root DIR` is the same as `DHNT_ROOT`; the default is the parent of this repo. The script needs network",
           "access to pypi.org, queries it sequentially, and rewrites this file and the TSV. A second run reproduces both",
           "byte-identically unless PyPI changed in between (new releases, new wheels). Add a package to the inventory by",
           "declaring it where the repos already do (pip/uv command, Python fence, import) and re-running.", ""]
    with open(os.path.join(repo_root, "docs", "python-package-inventory.md"), "w", encoding="utf-8", newline="\n") as fh:
        fh.write("\n".join(md))
    return counts


def main():
    here = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    ap = argparse.ArgumentParser()
    ap.add_argument("--root", default=os.environ.get("DHNT_ROOT") or os.path.dirname(here))
    args = ap.parse_args()
    inv = Inv()
    for repo in REPOS:
        if os.path.isdir(os.path.join(args.root, repo)):
            scan_repo(inv, args.root, repo)
        else:
            print("skip: repo not found: " + repo, file=sys.stderr)
    scan_external(inv, args.root)
    rows = [evaluate(n, inv.pkgs[n]["pins"]) for n in sorted(inv.pkgs)]
    counts = write_outputs(here, inv, rows)
    print("packages=%d %s callsites=%d" % (len(rows), json.dumps(counts, sort_keys=True), len(inv.callsites)))


if __name__ == "__main__":
    main()
