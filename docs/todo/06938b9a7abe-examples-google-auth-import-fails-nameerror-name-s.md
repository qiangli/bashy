---
id: 06938b9a7abe
kind: bug
title: 'examples/google auth-import fails NameError: name ''slot'' is not defined (2 wrapper tests red on main)'
seq: 3307
status: todo
priority: p2
created: 2026-11-24T14:04:32.783115Z
---

Pre-existing on bashy main 532a316a (verified 2026-11-24 by the Sprint 691 conductor, not introduced by H2): python3 -m unittest test_cli_wrapper in examples/google -> 2 FAIL, incl. test_auth_import_client_secret_and_per_account_resolution; traceback handle_auth_import -> NameError: name 'slot' is not defined. Suspected origin e8ece26f (YouTube credential slot, 2026-11-18); bisect not yet run. Classification: product defect. Tracked separately per the no-regression policy; not repaired in Sprint 691.
