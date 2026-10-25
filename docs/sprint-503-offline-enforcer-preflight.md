# S503 hosted offline enforcer preflight

GitHub Actions [run 40197732754](https://github.com/qiangli/bashy/actions/runs/40197732754) on workflow commit `e40b72eefb336133b6bd6c080a0fd99aafaa6781` passed all six native jobs. The [evidence summary](sprint-503/evidence/offline-enforcer-preflight-40197732754.json) records native OS/architecture, Python and enforcer SHA-256, both live denied TCP probes, baseline/restored connectivity, Windows rule state, and hashes of each uploaded raw JSON report.

| Runner | Method | Enforcer SHA-256 | Denied error (before/after) | Restored |
|---|---|---|---|---|
| `macos-15` | `macos-sandbox-deny-network` | `2ff75194b3e5689debee061c69f27f873ca21ff8fc54b7d49cbf5f2fb673f6ea` | 1/1 | yes |
| `macos-15-intel` | `macos-sandbox-deny-network` | `2ff75194b3e5689debee061c69f27f873ca21ff8fc54b7d49cbf5f2fb673f6ea` | 1/1 | yes |
| `ubuntu-24.04` | `linux-unshare-net` | `a23c8863860669003dc4660039fe642f5795c8c2195898ebc5d01afa1ac3d11c` | 101/101 | yes |
| `ubuntu-24.04-arm` | `linux-unshare-net` | `e0c145c3d4c06686544b1362ed64da819539166a5e1e6fd96093aea4d2539b6a` | 101/101 | yes |
| `windows-11-arm` | `windows-firewall-program-block` | `532aceda5485f1c2728baa0549a498d594f7056022a89cd71eb97df68e33672c` | 10013/10013 (WinError; raw errno 13) | yes |
| `windows-2025` | `windows-firewall-program-block` | `fa2c8bb1e35cdd62ad15cd06a1d8f2c5961e9725a9bfa1e7cfd2c322f49e9619` | 10013/10013 (WinError; raw errno 13) | yes |

Each job first connected to `1.1.1.1:443`, applied its OS policy, made two separate TCP attempts that were denied, exited or removed the policy, and then connected again. Linux used `sudo unshare --net`; macOS used `sandbox-exec` with `deny network*`; Windows installed an exact Python executable outbound block rule, verified it before and after both probes, removed it in `finally`, confirmed absence, and reconnected. The Windows NetSecurity module manifest SHA-256 was `04028a016fa8757a2c2c78d6045e27a2ebc968d5bd9e25cf81ff8d070fa716d5` on both images.

The first run `40197614189` was falsely red on Windows because Python reports the denied connection as `exc.errno == 13` while `exc.winerror == 10013`; its artifacts already showed an active rule, immediate denial, and clean restoration. The final preflight records both fields and classifies WinError 10013. The native E2E workflow must use `exc.winerror` or accept the raw errno 13 in its Windows offline canary. The E2E workflow also needs exact reviewed enforcer hashes from this run in its admission record.

This preflight tests OS containment of the Python TCP canary only. It does not exercise media, inference, or every executable that the full Windows pipeline may spawn; full E2E remains a separate gate. The test branch is `s503-offline-enforcer-preflight`.
