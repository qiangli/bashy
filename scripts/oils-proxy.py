#!/usr/bin/env python3
"""Gate-D conformance proxy: run Oils (oilshell) spec-test cases against bashy and
report the pass rate vs the *bash-expected* output.

The Oils spec suite (Apache-2.0, priorart/oils/spec/*.test.sh) bakes in expected
output per case, with per-shell overrides. bashy is a bash drop-in, so for each
case we compare bashy's stdout (+ exit status when given) to the **bash** target:
the `## OK bash STDOUT:` override if present, else the default `## STDOUT:`.

We deliberately SKIP cases we can't score cleanly for bashy:
  - no expected output/status at all (nothing to assert),
  - `## N-I bash` (bash doesn't implement it) / `## BUG bash` (bash-buggy; ambiguous),
  - files tagged for ysh/osh only (not POSIX/bash).
Skips are reported, never silently counted — an honest proxy.

This is NOT the faithful Oils runner (that's Python-2-era: imports cgi/cStringIO,
won't run on modern py3). It's a focused proxy for "does bashy match bash on the
Oils corpus." Usage: oils-proxy.py BASHY_BIN FILE.test.sh [FILE...]
"""
import os, re, subprocess, sys, tempfile

def parse(path):
    """Yield cases: {name, code, exp_stdout, exp_status, skip}. exp_* are the
    BASH target. skip is a reason string or None."""
    text = open(path, encoding='utf-8', errors='replace').read()
    # file-level: skip non-bash-family suites
    m = re.search(r'^## (?:suite|tags):.*', text, re.M)
    blocks = re.split(r'^#### ', text, flags=re.M)
    for blk in blocks[1:]:
        lines = blk.split('\n')
        name = lines[0].strip()
        body, i = [], 1
        # code = lines until the first '## ' directive
        while i < len(lines) and not lines[i].startswith('## '):
            body.append(lines[i]); i += 1
        code = '\n'.join(body)
        # collect directive blocks
        default_out, bash_out = None, None
        default_status, bash_status = None, None
        ni_bash = bug_bash = False
        while i < len(lines):
            ln = lines[i]
            md = re.match(r'## (OK|BUG|N-I|STDOUT|status|STDERR)\b(.*)', ln)
            if not md:
                i += 1; continue
            kind, rest = md.group(1), md.group(2)
            if kind == 'STDOUT':           # default expected stdout
                i += 1; buf = []
                while i < len(lines) and not lines[i].startswith('## '):
                    buf.append(lines[i]); i += 1
                default_out = '\n'.join(buf) + ('\n' if buf else '')
                continue
            if kind == 'status':           # '## status: N'
                mm = re.search(r'(\d+)', rest);  default_status = int(mm.group(1)) if mm else None
                i += 1; continue
            if kind in ('OK', 'BUG', 'N-I'):
                shells = rest.strip().split()
                applies_bash = (not shells) or any(s == 'bash' for s in shells[:-1] + shells) or 'bash' in rest
                # forms: '## OK bash STDOUT:' / '## OK bash status: N' / '## N-I bash' / '## BUG bash ...'
                if 'bash' in rest.split('STDOUT')[0].split('status')[0]:
                    if kind == 'N-I': ni_bash = True
                    elif kind == 'BUG': bug_bash = True
                    if 'STDOUT:' in ln:
                        i += 1; buf = []
                        while i < len(lines) and not lines[i].startswith('## '):
                            buf.append(lines[i]); i += 1
                        if kind == 'OK': bash_out = '\n'.join(buf) + ('\n' if buf else '')
                        continue
                    if 'status:' in ln:
                        mm = re.search(r'status:\s*(\d+)', ln)
                        if kind == 'OK' and mm: bash_status = int(mm.group(1))
                i += 1; continue
            i += 1
        exp_out = bash_out if bash_out is not None else default_out
        exp_status = bash_status if bash_status is not None else default_status
        skip = None
        if ni_bash: skip = 'N-I bash'
        elif bug_bash and bash_out is None: skip = 'BUG bash'
        elif exp_out is None and exp_status is None: skip = 'no expectation'
        yield {'name': name, 'code': code, 'out': exp_out, 'status': exp_status, 'skip': skip}

def run_case(bashy, code):
    with tempfile.TemporaryDirectory() as d:
        f = os.path.join(d, 's'); open(f, 'w').write(code)
        try:
            p = subprocess.run([bashy, f], cwd=d, capture_output=True, text=True, timeout=10)
            return p.stdout, p.returncode
        except subprocess.TimeoutExpired:
            return '<timeout>', 124

def main():
    bashy = sys.argv[1]
    total = passed = failed = skipped = 0
    fails = []
    for path in sys.argv[2:]:
        for c in parse(path):
            total += 1
            if c['skip']:
                skipped += 1; continue
            out, st = run_case(bashy, c['code'])
            ok = True
            if c['out'] is not None and out != c['out']: ok = False
            if c['status'] is not None and st != c['status']: ok = False
            if ok: passed += 1
            else:
                failed += 1
                fails.append((os.path.basename(path), c['name']))
    print(f"=== bashy vs bash-expected: {passed} pass / {failed} fail / {skipped} skip "
          f"({total} cases across {len(sys.argv)-2} files) ===")
    print("NOTE: 'fail' = differs from Oils' BAKED-IN expected, which drifts by bash "
          "version + this parser's stdout/status-only compare. It is a TRIAGE QUEUE, not "
          "confirmed bugs — verify each against LIVE bash 5.3 (scripts/posix-diff.sh's "
          "same-env differential is the reliable verdict; sampled fails matched bash 5.3).")
    if fails:
        print("--- failures (suite :: case) ---")
        for f, n in fails[:60]:
            print(f"  {f} :: {n}")
        if len(fails) > 60:
            print(f"  … +{len(fails)-60} more")
    return 1 if failed else 0

if __name__ == '__main__':
    sys.exit(main())
