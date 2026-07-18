# POSIX BRE/ERE Conformance Worklist

Next 4 conformance targets for the regex engine, in priority order.

- **Anchoring parity** — `^`/`$` anchoring inside groups/alternation (codex is on it now).
- **Back-references `\1`..`\9`** — edge cases around repeated, nested, and out-of-order back-refs.
- **Collation / equivalence classes** — bracket forms `[.x.]` (collating) and `[=x=]` (equivalence).
- **ERE-vs-BRE operator differences** — unescaped `(`, `|`, `+`, `?`, `{}` are literals in BRE but operators in ERE.
