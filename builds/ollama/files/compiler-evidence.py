"""Compiler-time content store and narrow native input provenance."""
import hashlib
import os
from pathlib import Path, PureWindowsPath
import re


def dependencies(depfile):
    raw = depfile.read_text().replace('\\\n', ' ').replace('\\\r\n', ' ')
    match = re.search(r'(?<!\\):\s', raw)
    if not match:
        raise ValueError('malformed compiler dependency file')
    names, current = [], []
    body = raw[match.end():]
    i = 0
    while i < len(body):
        char = body[i]
        if char.isspace():
            if current:
                names.append(''.join(current))
                current = []
        elif char == '\\':
            if i + 1 >= len(body):
                raise ValueError('malformed compiler dependency escape')
            following = body[i + 1]
            if following.isspace():
                current.append(following)
                i += 1
            elif re.match(r'^[A-Za-z]:', ''.join(current)):
                # MinGW emits literal backslashes in absolute source paths.
                current.append('/')
            else:
                current.append(following)
                i += 1
        else:
            current.append(char)
        i += 1
    if current:
        names.append(''.join(current))
    return names


def portable_path(value):
    """Audit Windows records on the Linux aggregation job without host resolution."""
    value = str(value)
    return PureWindowsPath(value) if re.match(r"^[A-Za-z]:[/\\]", value) else Path(value)


def origin(path, environment=None):
    path = portable_path(path)
    if environment is None:
        environment = {key: str(Path(os.environ[key]).resolve()) for key in
                       ("BASHY_COMPILER_SOURCE", "BASHY_COMPILER_GOROOT", "BASHY_COMPILER_TMP", "BASHY_MINGW_ROOT")
                       if key in os.environ}
    for env, label in [('BASHY_COMPILER_SOURCE', 'source'), ('BASHY_COMPILER_GOROOT', 'go-toolchain'),
                       ('BASHY_COMPILER_TMP', 'go-generated')]:
        root = environment.get(env)
        if not root:
            continue
        try:
            rel = path.relative_to(portable_path(root)).as_posix()
        except ValueError:
            continue
        if '..' in portable_path(rel).parts:
            raise ValueError('unreviewed external native compiler dependency: traversal')
        if label == 'go-toolchain' and not rel.startswith('src/runtime/cgo/'):
            break
        if label == 'go-generated' and not re.fullmatch(
                r'(cgo-gcc-input-[0-9]+\.c|go-link-[0-9]+/trivial\.c|go-build[0-9]+/b[0-9]+/(generated\.c|_cgo_(export\.[ch]|main\.c)|[^/]+\.cgo[12]\.[ch]))', rel):
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
    # CLANG64 layout (shared windows-compilers.py probe): clang resource
    # headers live under lib/clang/<version>/include beside the GCC-runtime
    # and sysroot prefixes the older ucrt64 layout used.
    root = environment.get('BASHY_MINGW_ROOT')
    if root:
        try:
            rel = path.relative_to(portable_path(root)).as_posix()
            if rel.startswith(('include/', 'lib/clang/', 'lib/gcc/', 'x86_64-w64-mingw32/include/')):
                return 'system', str(path)
        except ValueError:
            pass
    raise ValueError(f'unreviewed external native compiler dependency: {path}')


def _persist(store, digest, content):
    """Copy content bytes into the content-addressed store.

    Parallel cgo compilations snapshot identical headers at once, so
    concurrent writers race on the same blob name (same digest means same
    bytes). Windows file locking refuses an atomic replace onto a blob that
    another writer still holds open ([WinError 5] Access is denied), so never
    a bare os.replace onto an existing or locked path: fast-path a blob that
    is already complete, otherwise copy to a private temp file and retry the
    atomic replace with backoff until the lock clears.
    """
    import tempfile
    import time
    blob = store / digest
    try:
        if blob.is_file() and blob.stat().st_size == len(content):
            return
    except OSError:
        pass
    fd, pending = tempfile.mkstemp(dir=store)
    try:
        with os.fdopen(fd, 'wb') as f:
            f.write(content)
        delay = 0.05
        for _ in range(12):
            try:
                if blob.is_file() and blob.stat().st_size == len(content):
                    return  # a racing writer stored these exact bytes first
            except OSError:
                pass
            try:
                os.replace(pending, blob)
                return
            except PermissionError:
                time.sleep(delay)
                delay = min(delay * 2, 2.0)
        # Bounded wait over: one last replace surfaces the real error.
        os.replace(pending, blob)
    finally:
        try:
            os.unlink(pending)
        except OSError:
            pass


def snapshot(depfile, store):
    rows = []
    for name in dependencies(depfile):
        path = Path(name).resolve()  # compiler's cwd, never the repository root
        content = path.read_bytes()
        digest = hashlib.sha256(content).hexdigest()
        _persist(store, digest, content)
        category, relative = origin(path)
        rows.append(dict(path=str(path), name=name, origin=category, relative=relative, sha256=digest))
    if not rows:
        raise ValueError('empty compiler dependencies')
    return rows
