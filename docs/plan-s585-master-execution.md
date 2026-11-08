# Sprint 585 — Klatch v1.2.7 release and end-to-end tests

Sprint UUID: `73032105-b38f-5cd7-8f47-a732dfee103f`. Release-gate sprint: no
tag or release without the operator's explicit approval (R1 carries the gate).

| Story | Work | State (2026-11-08) |
| --- | --- | --- |
| R1 `295f6aecae48` | Klatch v1.2.7 build and pin; next rc from bashy main; gates, QA, promotion; native Klatch on the Windows test host | Build and pin done (yoke 1b05a5ad in bashy's yoke pin). Waits for the rc and operator approval |
| O3 `030a968b059b` | Klatch through two satellites at once on macOS, Linux, native Windows | e2e run on all three; Windows reboot check moved to todo 92fbe9c4 |
| O5 `1283eb5b8a06` | outpost-profile up/down/status reaches installs through the release source pin | On bashy main (5ad1d9ce); verify on the installed rc |
| E1 `a8c4da9cf37d` | tests/klatch-e2e four-topology runs | Harness delivered; recorded runs wait for the release |
| V1 `e59fc7ed5535` | Access topologies (a) and (c) | Runs on the release |
| C4 `00480e8f2c3c` | Final Klatch invite/chat UAT | Runs on the release |

Order: R1 rc and gates → operator approval → tag/promote → O5, E1, V1, C4 on
the released build → Windows host switch to native Klatch. Reuse existing
harnesses (tests/klatch-e2e, the self-hosted multi-satellite sandbox e2e);
hosts by role, credentials from the environment, raw evidence kept off the
public repo.
