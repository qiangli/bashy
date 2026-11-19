---
id: 01a1f354-acf3-7bac-8aec-c8fb98f6ba80
seq: 29
form: page
type: runbook
title: 'Repeat full POSIX Profile D tests: kit application, baseline reuse and annual renewal'
description: Public runbook for Bashy users obtaining The Open Group VSC-PCTS kit, repeating full Profile D Shell and Utilities tests, or preparing annual recertification; distinguishes shell-only, full engineering and formal evidence.
tags:
    - posix
    - pcts
    - vsc
    - profile-d
    - runbook
    - certification
    - recertification
    - license
status: candidate
evidence: Official catalogue and OSS v1.4 application checked 2026-11-19; Bashy docs/vsc-pcts-run-status.md, docs/s561/a4-licensed-results.md and existing runbooks reviewed. Full standalone formal recipe is not validated.
source:
    tool: s575-codex-manager
    episode: ep-59d096ca4a0010fe
created: "2026-11-19T10:22:55Z"
updated: "2026-11-19T10:23:23Z"
---

Use this runbook when repeating Bashy's full POSIX Shell and Utilities Profile D engineering tests, preparing a certification run, or planning the next annual renewal. Profile D is a Bashy harness label; select the applicable Open Group Product Standard separately.

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
- [S561 complete licensed-run baseline and handoff](https://github.com/qiangli/bashy/blob/main/docs/s561/a4-licensed-results.md).
- [Evidence identity and reuse](https://github.com/qiangli/bashy/blob/main/docs/s561/evidence-identity.md).
- [Required command/provider coverage](https://github.com/qiangli/bashy/blob/main/docs/posix-command-coverage.md).
- [Historical certification handoff](https://github.com/qiangli/bashy/blob/main/docs/posix-cert-handoff-runbook.md). Its container and shell-subset instructions are historical, not the current full Profile D launch recipe.
- [Public claims and limits](https://github.com/qiangli/bashy/blob/main/docs/public-claims-evidence.md).
- [Public test DAG](https://github.com/qiangli/bashy/blob/main/dag.md). Its test-posix-shell target is explicitly shell-only; it does not launch the full Profile D run.

Authorized maintainers also need the matching licensed harness kit and its END-TO-END.md, RUNBOOK.md, CERTIFICATION.md and scope contract. Obtain those through the maintainer's authorized access process. This public KB does not distribute that kit or claim a public turnkey full-run command exists.

## Choose the exact procedure being repeated

The documented S561 full Profile D engineering measurement on 2026-11-04 used 117 journals for the full configured scenario. Its accepted engineering result was no regression with documented residual non-passes; it was not an official certification award. Use the linked baseline for exact candidate, harness, approval, arm, counts and disposition references.

The separate formal driver invokes one uncapped default-scenario tcc -ep and expects one journal. That is a different execution procedure. Passing shell-only tests or synthetic driver tests does not establish equivalence with the S561 measurement. Do not silently replace one procedure with the other.

For repeatability, the chosen DAG/entrypoint must resolve to an exact existing recipe and matching evidence. If that mapping is unavailable, record the missing reference before provisioning or launching. Do not reconstruct a new runner from historical snippets.

## Repeat the full engineering measurement

1. Select the exact baseline and matching harness revision from the S561 handoff. Read that revision's lifecycle and approval instructions; use their existing commands. Record whether the declared SUT is the original combined payload or independently built Bash and Coreutils artifacts. They have different provenance.
2. Record clean source revisions, dependency identities, exact build recipe/flags, builder image digest, Go version and every launcher/payload hash. Keep build environment identity separate from the native execution host identity.
3. Prepare a fresh dedicated native Ubuntu x86_64 venue using the supported harness lifecycle. Its recorded configuration includes non-root tester identity, filesystem fixtures, locales, privilege helpers, writable results/workspaces, login session and controlling terminal. Reuse the baseline's host recipe.
4. Verify the actual launch environment, not just an isolated probe: shell route, required Go utility routes, mailbox variables/spool, scheduler executable/shared state/lifecycle and tester-owned PTY. Pure Go describes tested providers; host build/harness tools have separately declared roles. A host utility or shell fallback in tested execution does not satisfy a zero-external-provider claim.
5. Run the same approved full scenario with unchanged test purposes, applicability, oracles and limits. Preserve the configured complete inventory and separately declared optional scope. A shell-only gate is not a substitute.
6. Preserve original journals, official suite reports, input/configuration inventories, provider/build receipts and outcome identities. Compare the same test identities with the baseline; counts alone do not prove no regression. Keep authority rulings and manual inspection decisions separate from raw results.
7. Retrieve and verify evidence before retiring the owned venue through the supported lifecycle. Record teardown and the private evidence locator. Never manufacture completion records for a failed run.

Current limitation: the separated-artifact formal path remains under investigation for setup equivalence. The original formal path also lacks orchestration present in the established diagnostic/full-measurement path. This page is a candidate runbook, not approval to launch either path or a claim those gaps are repaired. No product-code change is prescribed here.

## Next year's repeat or recertification

Maintain a private renewal record with the product/platform and declared scope, previous certification or engineering status, license start/expiry, current authorized suite version, candidate and harness pins, builder/venue identities, provider roles, complete command/configuration, original reports and authority/manual dispositions.

Track test-kit licensing and product-certification renewal separately. Before the next cycle, recheck the current catalogue, agreement, Product Standard and applicable renewal rules; confirm access to the retained evidence and operational recipe. A new source build or platform must not inherit an earlier verdict without the documented identity/retest basis.

For an official application, use the [Certification Guide](https://posix.opengroup.org/certification_guide.html), [Product Standards and questionnaires](https://posix.opengroup.org/prodstandards.html) and [Certification Policy](https://posix.opengroup.org/docs/POSIX_Certification_Policy_v1.1.pdf). Follow the authority's current requirements for registration, testing, submission and renewal. Running the kit alone does not grant certification or trademark rights.

## Public/private boundary

This page contains public application links and scrubbed operational guidance. Keep signed applications, personal contact details, credentials, infrastructure identifiers, licensed source, raw journals and restricted correspondence in the authorized private store. Publish only material your agreement and applicable written consent permit.
