# Go example corpus for Bash++

Status: testing policy and implementation brief. The tracked implementation
work is `sh` todo `2daf9ef04ad4` / weave run `sh#31`.

## Goal

Use small, executable Go examples as an additional language-coverage corpus
for Bash++ syntax and runtime behavior. For every supported example, run the
original Go program as an oracle, run the independently authored Bash++ form,
and compare observable behavior.

This corpus supplements—not replaces—the Bash/POSIX conformance suites and the
Bash++ superset gate. Go behavior is an oracle only for Bash++ constructs that
deliberately claim Go semantics.

## Sources and licensing

### A Tour of Go

The official Tour source is distributed under the Go project's BSD-style
license unless otherwise noted. Adapted fixtures must:

- pin the exact `golang.org/x/website` revision;
- retain the required Go Authors copyright and license notice;
- record the original Tour path/example identifier;
- identify material changes made for Bash++;
- include any required license text in the repository's third-party notices.

Source: <https://go.dev/tour/> and
<https://pkg.go.dev/golang.org/x/website/tour>.

### Go by Example

The repository README licenses the work, copyright Mark McGranaghan, under the
Creative Commons Attribution 3.0 Unported license (CC BY 3.0). Its examples may
therefore be adapted into Bash++ fixtures when the attribution requirements
are followed. Each adaptation must:

- pin the exact `github.com/mmcgrana/gobyexample` commit;
- record the original example title, repository path, and source URL;
- attribute Mark McGranaghan and link or include the CC BY 3.0 license;
- clearly identify the Bash++ fixture as an adaptation and summarize material
  changes; and
- add the attribution and license information to the repository's third-party
  notices before adapted material is committed.

The Go Gopher artwork is separately credited to Renée French under CC BY 3.0.
Do not copy it into the corpus unless that separate attribution is retained.
Independently authored tests inspired only by a topic name should still record
their topic source, but must not be mislabeled as adaptations.

Sources: <https://github.com/mmcgrana/gobyexample#license>,
<https://creativecommons.org/licenses/by/3.0/>, and
<https://gobyexample.com/>.

## Corpus manifest

Every case must have a machine-readable record containing at least:

```text
id
title
origin kind and pinned revision
origin path or independently-authored topic reference
license/provenance classification
Bash++ phase and feature
status: supported | planned | not-applicable
Go oracle source
Bash++ source
required runtime capabilities
expected stdout bytes
expected stderr bytes
expected exit status
normalization rule, if any
platform constraints
timeout
```

The runner must reject an empty corpus, missing fixtures, unknown statuses,
duplicate IDs, and a misleading `0/0` success.

## Differential method

For each `supported` case:

1. Execute the pinned Go oracle with a pinned Go toolchain.
2. Execute the Bash++ fixture with Bash++ explicitly enabled.
3. Capture stdout, stderr, exit status, timeout, and declared side effects.
4. Apply only the case's checked-in normalization rule.
5. Compare the normalized observations byte-for-byte.
6. Report a per-case result and an aggregate denominator.

Normalization must be narrow and reviewable. It may remove declared
nondeterminism such as temporary paths or scheduler ordering; it must never
erase semantic differences, errors, panics, missing output, or exit-status
changes.

The corpus must be hermetic in CI: no network fetches, moving branches, host
tool discovery, wall-clock assumptions, or unbounded processes. Source
refresh is a separate, explicit operation that verifies provenance and updates
the pinned revision.

## Coverage phases

The first tranche should contain deterministic, implemented features only:

- values, variables, and constants;
- loops and conditionals;
- functions;
- multiple return values and the Bash++ error bridge;
- arrays, slices, and maps only where runtime support exists.

Later tranches may add:

- structured values and records (Bash++ L0/L1);
- goroutines and channels (L2), with ordering-safe assertions;
- approved standard-library bridge calls (L3), behind effect capabilities;
- filesystem behavior using temporary fixtures;
- controlled time, randomness, and network fakes.

Examples for unsupported roadmap syntax remain `planned`; they must not be
implemented as expected failures that make a red language feature look green.
`not-applicable` is reserved for Go semantics Bash++ explicitly does not claim,
with a documented reason.

## Required gates

- The Go oracle and Bash++ fixture are both executed; static fixture checks do
  not count.
- Each supported case passes repeatedly and under the race detector where the
  harness permits.
- Bash++ mode-off and mode-on retain byte-identical results across the existing
  Bash conformance corpus.
- Focused `syntax`, `expand`, and `interp` tests pass.
- Full relevant Go tests pass.
- Provenance and third-party notices pass review.
- Generated or adapted fixtures are committed; network access is not needed
  to run the gate.

## Ownership

The `sh` submodule owns parser, expansion, interpreter, fixtures, and the
differential runner. Bashy owns product activation (`--bashpp`, `.bpp`, and
binary defaults), integration coverage, and this policy. Any accepted `sh`
change must be pushed first, followed by Bashy's sibling pin and the umbrella
submodule pins.
