---
id: 01a1f354-acf3-7bac-8aec-c8fb98f6ba80
seq: 29
form: page
type: runbook
title: 'Repeat full POSIX Profile D tests: kit application, baseline reuse and annual renewal'
description: 'Public S575 full Profile D engineering playbook: licensed kit application, exact separated Bash/Coreutils baseline, isolated native preparation, Go-only providers, original 117-target / 9,337-TP launch, verified retention and owned venue cleanup; repeat tests and annual certification preparation.'
tags:
    - posix
    - pcts
    - vsc
    - profile-d
    - runbook
    - certification
    - recertification
    - license
status: validated
evidence: Validated against the completed S575 117-target / 9,337-TP engineering run at the tagged source and harness 6966bc6458e862d7ae03a4a01e7044ab842399ba. Original full-mode retention, independent parser/archive integrity and owned retirement passed. The preserved result is 4 FAIL, 54 UNRESOLVED and 9 INSPECT; this is not certification acceptance. Command signatures checked against that harness. Kit application links were checked on 2026-11-19; future applicants must recheck current official terms. No new test execution performed for this update.
source:
    tool: s575-codex-manager
    episode: ep-59d096ca4a0010fe
created: "2026-11-19T10:22:55Z"
updated: "2026-11-20T10:43:41Z"
---

Use this runbook when repeating Bashy's full POSIX Shell and Utilities Profile D engineering tests, preparing a certification run, or planning the next annual renewal. Profile D is a Bashy harness label; select the applicable Open Group Product Standard separately.

Canonical public location: [Bashy repository KB](https://github.com/qiangli/bashy/blob/main/docs/kb/pages/full-profile-d-posix-runbook.md). No internal account is needed to read it. In a Bashy source checkout, use `bashy kb show full-profile-d-posix-runbook`. Kit access and private operational configuration remain separate prerequisites for execution.

## Proven separated-source baseline

The original full engineering procedure completed on 2026-11-19 using separate Bash and pure-Go Coreutils artifacts: 117 targets/journals and 9,337 execution TPs, zero capped targets and zero UNINITIATED/UNREPORTED results. The retained outcome is **9,270 PASS group, 4 FAIL, 54 UNRESOLVED and 9 INSPECT**. This is a repeatable engineering measurement, not an official certification pass.

Use [the S575 baseline record](https://github.com/qiangli/bashy/blob/main/docs/s575-posix-engineering-baseline.md) for all source, build, artifact and evidence hashes. Marker **posix-baseline/s575-2026-11-19** is published in the public umbrella, Bash and Coreutils repositories at their respective exact tested commits:

- Umbrella source snapshot: `295cd32d032fb94dda2c032c26109032bd5af4b8`.
- Bash: `7855c2f038f2c8ccba1baf8478b0b8fb3dcd8cd2`.
- Coreutils: `c656935287fa4bff4920b08e41156d13d7c5702f`.
- Shared CLI dependency: `4141cc07cfdc6c0a838825485ef41e54a55fa9a0`.
- Matching authorized private harness: `6966bc6458e862d7ae03a4a01e7044ab842399ba`.
- The umbrella's Bashy pin is a separately recorded syntax-only harness tool, outside the tested provider PATH; it is not the released full Bashy product.

The existing shared engine/CLI stays pinned. A later source, dependency, builder, platform or artifact must establish its own evidence/reuse basis. An aggregate count or source-closure comparison alone does not prove artifact equality.


## Obtain the licensed kit

The kit is The Open Group's VSC-PCTS2016 Shell and Utilities suite. Start at the [official suite catalogue](https://posix.opengroup.org/testsuites.html), which identifies currently authorized versions and links the current agreements. Recheck this page when applying or renewing; a remembered version is not a download entitlement.

For an open-source project:

1. Follow the catalogue's open-source VSC-PCTS2016 agreement link. The form checked on 2026-11-19 is [OSS agreement v1.4](https://posix.opengroup.org/docs/VSC-PCTS2016-TestSuite-TimeLimited-License-Agreement-OSS-v1.4.pdf).
2. Enter the organization or individual name, open-source product description and project URL; complete the specified site and contact fields in Schedule 1.
3. Have the authorized signatory sign and date it. Email the entire completed agreement using the submission address printed on its first page.
4. Wait for The Open Group's countersignature and suite-access instructions. Obtain the kit through that authorized channel and record its version, receipt date and license expiry.
5. Confirm that your people, product and test site are covered before sharing access.

The current OSS form provides a no-charge, 12-month license beginning with the suite-access notification. For another licensing route, use the catalogue's standard agreement and [official FAQ](https://posix.opengroup.org/posixfaq.html). Applying for kit access and registering a product for certification are separate steps.

Keep the kit and restricted materials outside public Git. Follow your actual agreement for use, redistribution and publication; another project's publication permission does not transfer to you.

## Read Bashy's existing documentation first

- [Current conformance campaign and scope](https://github.com/qiangli/bashy/blob/main/docs/vsc-pcts-run-status.md).
- [S575 separated-source engineering baseline](https://github.com/qiangli/bashy/blob/main/docs/s575-posix-engineering-baseline.md).
- [S561 historical complete licensed-run baseline and handoff](https://github.com/qiangli/bashy/blob/main/docs/s561/a4-licensed-results.md).
- [Evidence identity and reuse](https://github.com/qiangli/bashy/blob/main/docs/s561/evidence-identity.md).
- [Required command/provider coverage](https://github.com/qiangli/bashy/blob/main/docs/posix-command-coverage.md).
- [Historical certification handoff](https://github.com/qiangli/bashy/blob/main/docs/posix-cert-handoff-runbook.md). Its container and shell-subset instructions are historical, not the current full Profile D launch recipe.
- [Public claims and limits](https://github.com/qiangli/bashy/blob/main/docs/public-claims-evidence.md).
- [Public test DAG](https://github.com/qiangli/bashy/blob/main/dag.md). Its test-posix-shell target is explicitly shell-only; it does not launch the full Profile D run.

Authorized maintainers also need the matching licensed harness kit and its END-TO-END.md, RUNBOOK.md, CERTIFICATION.md and scope contract. Obtain those through the maintainer's authorized access process. This public KB does not distribute that kit or claim a public turnkey full-run command exists.

## Choose the exact procedure being repeated

The proven S575 run reused the established full engineering chain: `standalone/profile-d-do.sh launch` → `scripts/launch-do-arm.sh` → `scripts/run-do-full-arm.sh` → `scripts/test-arm-native.sh profile-d` → the original per-set runner. Its contract is **scope=full, selection_count=117**, with 117 journals and 9,337 execution TPs. The first selection is the shell scenario; that does not make it a separate shell-only run.

The separate formal driver invokes one default-scenario `tcc -ep` and expects one journal. That different procedure was not qualified by this 117-journal run. Do not substitute it, or a shell-only/public DAG target, for the proven full engineering procedure. Synthetic driver tests do not establish full native execution.

## Repeat the full engineering measurement

### Freeze and qualify the inputs

1. Obtain the licensed kit and the exact matching harness through authorized access. Read its `END-TO-END.md`, `RUNBOOK.md`, `CERTIFICATION.md` and scope contract at the pinned revision. Public users can read this playbook without internal access; executing the licensed suite still requires the kit and its operational configuration.
2. Select the exact source and artifact candidate. Use the existing producer DAG/container recipes, published fixed dependencies and `GOWORK=off`; no umbrella checkout or sibling source is a build prerequisite. Record clean source revisions, dependency graph, actual recipe/flags, builder image digest, Go version and each launcher/payload hash. Keep builder identity separate from native execution-host identity.
3. For the S575 strict-shell layout, `/vsc/sut/sh` resolves to the independent Bash launcher and its measured payload; Coreutils is a distinct measured artifact. Preserve the original ELF, source-clean, manual-page and syntax-tool checks. Do not require all roles to be one combined binary or treat the syntax checker as a SUT utility provider.
4. Preserve the executed Coreutils bindings to the Go shell and Go-only cron namespace: `-X github.com/qiangli/coreutils/pkg/schedule.shellPath=/vsc/sut/sh` and `-X github.com/qiangli/coreutils/cmds/crontab.defaultCronPATH=/opt/bashy/bin`. These belong in the original static certification recipe and hashed flag receipts. Trimpath Go build info omits linker arguments; do not claim it proves them. Do not append a host-utility PATH tail to cron jobs. S575 separately proved an actual normally scheduled job resolved and executed Go utilities.
5. Publish the exact source/harness/manifest pins and obtain genuine candidate approval using the existing transfer/phase gates. A previous approval cannot be copied onto different bytes. The S575 artifact-manifest SHA256 was `6dde18df80537f6c4be1596222683d69c8a98c660a117474844d1530f1891138`; a new candidate needs its own measured value.

### Prepare an isolated native venue

Use a fresh private run-state directory and the harness's reviewed environment file. In the command templates below, `RUN_ENV` is that environment file; `NAME` is the new leased test-host name; `ARM` is a unique run name. The environment binds the exact candidate, genuine approval, artifact bundle, dedicated state file, execution image, approved account/budget and SSH configuration. Keep it private and access-restricted.

Run the existing commands under the existing secret projection for the **isolated test account**, with `DIGITALOCEAN_CONTEXT=default` and its access token supplied without logging it. The production kit-download account and unrelated resources must remain untouched. Do not paste credentials into this page or shell history.

From the authorized harness root:

```sh
. "$RUN_ENV"
./standalone/do-provision.sh --create-ephemeral --role bashy "$NAME"
./standalone/profile-d-do.sh prepare --adopt "$NAME"
```

This explicit create-then-adopt route was used successfully. The generic prepare/create default can select a different account path; do not improvise around its protection guards.

The native execution venue is Ubuntu x86_64. Preserve the reviewed numeric image pin and expected OS identity; verify availability before a paid create. A floating Ubuntu slug is not an equivalent identity. If a vendor retires an image, stop and qualify the replacement reference through the original review/approval process; do not quietly change the expected OS hash. S575's actual `/etc/os-release` SHA256 was `87458b3b1f1ca0e5c693ade771de7007c3783bdc692fa6b39eb6414bf2af2763`.

Let the existing preparation finish: cloud initialization/package freeze, original licensed-archive checksum, non-root tester and privilege helpers, five locales, filesystem fixtures, TET/native helpers, and original compile-only testcase build. Readiness required 178 readable testcase executables and empty tester-writable results. Compilation is preparation, not licensed TP execution.

### Verify the real runtime environment

Use the existing mandatory readiness checks, not a new runner or parallel gate framework:

- Declared base inventory: **102 routes = 92 Go utilities + 10 shell commands; zero external utility providers**. Preserve the original 14 optional exclusions and all applicability rules.
- TCC/tester and scheduler use `/vsc/sut/sh`; argv/entrypoint must retain strict `sh` semantics. A scheduler wrapper reaching host `/bin/sh` invalidates the pure-Go execution claim.
- Mailbox/spool configuration, fixture ownership, tester login registration and controlling PTY are actually present. The original launcher performs its guarded PTY setup and sets the selected shell after switching user.
- The selected Coreutils scheduler daemon is running with the arm's shared state. `at`/`batch` job environments and cron's independently built job environment must retain their declared Go routes; exporting a PATH only on the daemon does not set cron's default job PATH.
- Original privileges, writable workspace/results, native fixtures and final source/artifact/phase gates pass.

Pure Go describes the tested providers. Native build tools and licensed harness helpers have separately recorded infrastructure roles. Do not expand the test inventory, install external replacements for SUT utilities, change limits, rewrite assertions or convert non-passes to pass to make preparation appear complete.

### Launch and observe the original full scenario

After the existing preparation/readiness command passes, in the same isolated configuration:

```sh
./standalone/profile-d-do.sh launch "$ARM"
```

The existing durable systemd unit owns the run. Record `scope=full`, `selection_count=117`, unit policy, immutable input inventory and the **first actual licensed TP journal/time**. A started unit or completed preflight alone is not proof that TPs have begun.

Monitor using the existing status/journal/ledger paths. Compare named cases with measured history before declaring a stall; long quiet intervals are normal for several suites. S575's licensed execution took about 6 h 35 min. Its `ed` first-case quiet interval was about 20 minutes and mailx's initial interval about 10 minutes. Retain the original per-target limits and durable-unit policy; do not raise a timeout or restart because a journal is temporarily quiet.

Use the original phase-aware parser. Journal result lines include build/setup records, so counting every raw result line is not the execution denominator. Preserve each original result and separately record scope, authority rulings and manual decisions.

### Retrieve, verify and retire

Wait for terminal unit state, the original completion marker, final ledger/reports and end inventory. All 117 targets and exactly 9,337 execution results must be accounted for. A zero process exit is not an all-green test result.

Use the canonical ordered target file generated from the frozen registry (shell plus 116 utility targets), not a handwritten 117-name substitute. In this template, `EXPECTED_TARGETS` names that original inventory; `PRIVATE_EVIDENCE` is the licensed retention directory outside all Git worktrees; `PUBLIC_SUMMARY` is the separate allowlisted-summary directory:

```sh
./scripts/retrieve-campaign-evidence.sh \
  --role bashy --arm "$ARM" \
  --mode full-profile-d \
  --expected-sets 117 --expected-tps 9337 \
  --expected-targets "$EXPECTED_TARGETS" \
  --private-dir "$PRIVATE_EVIDENCE" \
  --public-dir "$PUBLIC_SUMMARY"
```

This original full-mode retention passed **unchanged** for S575. No legacy-harness exception, targeted-validator override, frozen-arm edit or fabricated state flag was needed. In particular, combined-binary checks inside a targeted-only branch are not evidence that the full branch will fail; inspect the actual enclosing condition and command result before proposing a correction.

Verify the original before/after/final manifests, input equality, all journal/public/stage checksums and an independent recomputation with the original parser. Seal the private archive, record its SHA256 and read back its files/checksum manifest. Preserve outcome accounting: the S575 archive retained **4 FAIL, 54 UNRESOLVED, 9 INSPECT**. Retrieval's runner `failures=0` does not erase those four test FAILs. Keep manual decisions and authority dispositions separate from raw codes.

After evidence and exact-owned-resource review, use the existing guarded retirement command in the same isolated account/state:

```sh
./scripts/do-retire-ephemeral.sh "$NAME" "$PRIVATE_EVIDENCE"
```

The helper requires genuine completed retrieval, checks retained journal/public/stage hashes again and binds deletion to the leased resource identity. Verify provider API absence afterward and confirm unrelated resources are unchanged. Retain the retirement receipt privately; never delete an unrelated host or a protected kit source.

S575 completed these retention and cleanup steps. Its sealed archive SHA256 is `63cd77b944361e999652210f30c39d5f74e5785921b6df2384b6d7c721405fe1`; independent review verified 117 journals, 23 public files, 12 stage files and all 196 archived regular files. The source marker preserves this measurement without claiming official certification or triggering an RC38 product release.

The [planned Sprint 675 follow-up](https://github.com/dhnt/umbrella/blob/main/docs/plan-posix-certification-after-separation.md) owns investigation and preparation for the retained 67 non-pass/manual outcomes. Do not silently waive them or treat a historical manual disposition as current approval.

## Next year's repeat or recertification

Maintain a private renewal record with the product/platform and declared scope, previous certification or engineering status, license start/expiry, current authorized suite version, candidate and harness pins, builder/venue identities, provider roles, complete command/configuration, original reports and authority/manual dispositions.

Track test-kit licensing and product-certification renewal separately. Before the next cycle, recheck the current catalogue, agreement, Product Standard and applicable renewal rules; confirm access to the retained evidence and operational recipe. A new source build or platform must not inherit an earlier verdict without the documented identity/retest basis.

For an official application, use the [Certification Guide](https://posix.opengroup.org/certification_guide.html), [Product Standards and questionnaires](https://posix.opengroup.org/prodstandards.html) and [Certification Policy](https://posix.opengroup.org/docs/POSIX_Certification_Policy_v1.1.pdf). Follow the authority's current requirements for registration, testing, submission and renewal. Running the kit alone does not grant certification or trademark rights.

## Public/private boundary

This page contains public application links and scrubbed operational guidance. Keep signed applications, personal contact details, credentials, infrastructure identifiers, licensed source, raw journals and restricted correspondence in the authorized private store. Publish only material your agreement and applicable written consent permit.
