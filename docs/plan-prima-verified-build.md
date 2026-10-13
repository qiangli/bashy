# Prima verified build: Sprint 436, Story #2029

Spec: umbrella `docs/sphere-dks-regression-findings-2026-10-12.md` and
`docs/bashy-service-supply-chain-policy.md`, both read in full.

1. Inspect the existing builds pipeline and outpost archive contract; resolve
   the source/tag/license lineage before executing any build.
2. Pin a permissive official stable release and all linked dependencies;
   implement native hosted build, vulnerability audit, SBOM and meaningful
   no-egress gates using `builds/`.
3. Coordinate the archive/signature contract with the Sprint 436 manager for
   separate outpost binmgr changes. Review before release publication.
4. Run targeted verification, commit on the current worker branch with sprint
   trailers, and report exact pins, hosted CI evidence and signatures.

Step 1 uncovered a blocking mismatch: `shard-binaries` points to the old
ZeroMQ source, whereas the MIT nng candidate is untagged. Neither discovered
official upstream has release tags. Step 2 cannot proceed under Rule 1.
The partial delivery adds a blocked candidate manifest, read-only hosted
source inspection and negative tests. No fictional stable tag, dependency
audit, SBOM, signature or build success is supplied.

Detailed source evidence, remaining work and proposed consumer contract:
[`builds/prima.cpp/README.md`](../builds/prima.cpp/README.md).
