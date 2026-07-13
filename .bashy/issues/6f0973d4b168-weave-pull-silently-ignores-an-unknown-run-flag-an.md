---
id: 6f0973d4b168
kind: bug
title: weave pull silently ignores an unknown --run flag and no-ops ('nothing to merge')
status: open
reporter: qiangli
created: 2026-07-12T23:40:32.70633Z
---

The operator playbook documents merging a run as `bashy weave pull --run N`. But `weave pull` has NO --run flag — it takes a positional `[issue]`. Passing `--run 16` is silently accepted (cobra doesn't reject it here), the positional is empty, so pull tries to fast-forward already-merged branches, finds none, and prints nothing (exit 0). The operator believes the merge happened; it did not.

Two fixes: (1) reject unknown flags on weave pull (or make --run an alias of the positional, matching `weave start --run` and `weave status N`); (2) print an explicit 'no issue specified / nothing matched' rather than empty output.

Related interface drift found the same session: `weave kill` is documented in the operator notes as `--issue N` but actually takes a positional `<issue>` (`weave kill --issue 16` no-ops silently; `weave kill 16` works). The run/issue/--run vocabulary is inconsistent across start (--run), status (positional), kill (positional), pull (positional), salvage (positional).
