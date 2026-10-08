# Security policy

## Reporting a vulnerability

Please report suspected vulnerabilities privately. Do not open a public issue,
pull request or discussion that describes one.

1. Use GitHub's private vulnerability reporting for this repository: the
   **Security** tab, then **Report a vulnerability**
   (<https://github.com/qiangli/bashy/security/advisories/new>).
2. If that option is not available to you, open a public issue that says only
   that you need a private security contact. Include no technical details, no
   proof of concept and no affected-system information. A maintainer will reply
   with a private channel.

Include, as far as you can:

- the affected version (`bashy --version`) and operating system;
- what an attacker can do and what they need first (local user, network
  position, a crafted script, a crafted archive);
- the smallest reproduction you have;
- whether the issue is already public or has a deadline.

## What to expect

- **Acknowledgement** within 5 working days.
- **Assessment** — whether we treat it as a vulnerability, its severity and the
  versions affected — within 14 days of acknowledgement.
- **Fix and advisory.** A confirmed vulnerability is fixed in a patch release
  and published as a GitHub security advisory, with a CVE when one applies. We
  coordinate the disclosure date with you; the default is 90 days from your
  report, sooner once a fix is available.
- **Credit** in the advisory unless you ask us not to.

These are targets for a small project, not a contractual service level.

## Supported versions

bashy has not yet reached 1.0.0. Until it does:

| Version | Security fixes |
|---|---|
| Latest `v0.x` release | Yes |
| Any earlier `v0.x` release | No — upgrade to the latest |
| `main` (unreleased) | Fixed in place; reported against the next release |

From 1.0.0 the policy becomes: the latest minor release of the current major
receives security fixes, and the end-of-support date of any earlier minor is
stated in its release notes before it ends. Versioning rules are in
[docs/release-roadmap-and-versioning.md](docs/release-roadmap-and-versioning.md).

## Scope

In scope — this repository and the artifacts built from it:

- the `bashy`, `bash` and `sh` executables and the in-process utilities they
  carry, including the shell language modes (Classic, Bash#), fenced-block
  runners, and the agentic layer (Yoke) with its MCP server and local services
  (`bashy app`, `bashy llm`, `loom`, `sshd`, `proxy`);
- the release pipeline: archives, checksums, install scripts and package
  manifests (Homebrew, Scoop, Winget);
- tool downloads performed by bashy's tool registry, including missing or
  weak integrity pinning;
- sandbox and containment escapes (`@contain`, `bashy oci`), policy bypass of
  declared command effects, and secret leakage through the audit log, output
  firewall or redaction path.

Out of scope:

- behavior that is the documented purpose of a shell: a user running a script
  they chose to run can run arbitrary commands;
- vulnerabilities in third-party software that bashy downloads and runs as a
  separate program (for example `ollama`, `podman`, `gh`); report those
  upstream — but tell us if bashy's pinning or invocation makes them worse;
- vulnerabilities in a dependency that bashy does not reach; we still welcome
  the report, and fix the pin if it is reachable;
- denial of service that requires local, already-privileged access;
- missing hardening that has no demonstrable impact;
- social engineering, physical access, and findings from automated scanners
  without a demonstrated impact.

Known limits are documented rather than hidden. In particular the secret
output firewall guarantee ends at `execve`, and the unix binaries rely on a
launcher that preserves inherited signal dispositions. A report that these
limits exist is not a vulnerability; a way around a guarantee that is
documented to hold is.

## Safe harbor

We will not pursue or support legal action against good-faith research that
stays within this policy: you test only against systems you own or have
permission to test, avoid privacy violations and data destruction, do not
degrade service for others, and give us reasonable time to fix the issue
before disclosure.

## Dependencies and supply chain

bashy's dependency closure is permissive-licensed and pure Go for the core;
see [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md) and
[docs/licensing-supply-chain-policy.md](docs/licensing-supply-chain-policy.md).
Release archives are published with a `checksums.txt`; a checksum fetched from
the release it verifies proves transit integrity, not provenance. Signing,
notarization and a release SBOM are tracked for 1.0.0 and are listed in the
[CHANGELOG](CHANGELOG.md) as they land.
