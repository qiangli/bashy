# S503.6 fifth native Linux probe: retained CLI mode failure

The [manual run](https://github.com/qiangli/bashy/actions/runs/40220215578)
at Bashy `96f408db5f9c44a16fd820a7c5ebc4d60d61311b` failed on both native
targets after the link-input and static ELF audits passed. Both reports have
`elf.static_elf=true` and the exact reviewed musl `_DYNAMIC` marker. They
remain `probe_passed=false` and `product_admitted=false`.

| Target | Artifact ID | Report SHA-256 | Retained ELF SHA-256 |
| --- | ---: | --- | --- |
| amd64 | `12609191864` | `c8e178e5226ab4938763f9db8bace9bab43cc08beca60643aa52fa9e5fa1ce5d` | `3f645450720f14df65f0e838321a88543ed70770d4bd38b975b22df2c9283bdb` |
| arm64 | `12609595870` | `eb7bb8f386d7843ec7d7e38f370fd60afea192ba86a5df50a0ac10874638b57b` | `800b28881e3c7e99cbafe9f7d6b219c2969473806f354ebc5244791cf419809b` |

The probe copied the linked `whisper-cli` with `shutil.copyfile`, which did
not preserve executable mode. Both retained artifacts have mode `0644`.
The attempted `--help` call raised `[Errno 13] Permission denied`; each
`cli-help.txt` contains only the recorded command, with no CLI output.
Attestation was skipped. The correction copies exact bytes, applies the
source executable mode, and checks both mode and SHA-256 before attempting
the smoke. A focused test checks the retained file is executable and that
source and unrelated output bytes remain unchanged. A native rerun is still
needed; this receipt is not a CLI or product acceptance claim.
