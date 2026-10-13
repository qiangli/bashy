"""Compiler-time content store and narrow native input provenance."""
import hashlib
import os
from pathlib import Path
import re


def dependencies(depfile):
    raw = depfile.read_text().replace('\\\n', ' ').replace('\\\r\n', ' ')
    match = re.search(r'(?<!\\):\s', raw)
    if not match:
        raise ValueError('malformed compiler dependency file')
    return [re.sub(r'\\(.)', r'\1', s) for s in re.findall(r'(?:\\.|[^\s\\])+', raw[match.end():])]


def origin(path, environment=None):
    environment = os.environ if environment is None else environment
    for env, label in [('BASHY_COMPILER_SOURCE', 'source'), ('BASHY_COMPILER_GOROOT', 'go-toolchain'),
                       ('BASHY_COMPILER_TMP', 'go-generated')]:
        root = environment.get(env)
        if not root:
            continue
        try:
            rel = path.relative_to(Path(root).resolve()).as_posix()
        except ValueError:
            continue
        if label == 'go-toolchain' and not rel.startswith('src/'):
            break
        if label == 'go-generated' and not re.fullmatch(
                r'(cgo-gcc-input-[^/]+\.c|go-build[^/]+/b[0-9]+/(generated\.c|_cgo_(export\.[ch]|main\.c)|[^/]+\.cgo[12]\.[ch]))', rel):
            break
        return label, rel
    # Only reviewed system SDK/include roots; never a blanket /tmp or module cache exemption.
    roots = ('/usr/include/', '/usr/lib/gcc/', '/usr/lib/clang/', '/usr/local/lib/clang/',
             '/System/Library/Frameworks/', '/Library/Developer/CommandLineTools/SDKs/',
             '/Library/Developer/CommandLineTools/usr/lib/clang/',
             '/Applications/Xcode.app/Contents/Developer/Platforms/',
             '/Applications/Xcode.app/Contents/Developer/Toolchains/')
    if str(path).startswith(roots):
        return 'system', str(path)
    # MinGW headers must belong to the compiler installation verified at setup.
    root = environment.get('BASHY_MINGW_ROOT')
    if root:
        try:
            rel = path.relative_to(Path(root).resolve()).as_posix()
            if rel.startswith(('include/', 'lib/gcc/', 'x86_64-w64-mingw32/include/')):
                return 'system', str(path)
        except ValueError:
            pass
    raise ValueError(f'unreviewed external native compiler dependency: {path}')


def snapshot(depfile, store):
    rows = []
    for name in dependencies(depfile):
        path = Path(name).resolve()  # compiler's cwd, never the repository root
        category, relative = origin(path)
        content = path.read_bytes()
        digest = hashlib.sha256(content).hexdigest()
        blob = store / digest
        blob.write_bytes(content)
        rows.append(dict(path=str(path), name=name, origin=category, relative=relative, sha256=digest))
    if not rows:
        raise ValueError('empty compiler dependencies')
    return rows
