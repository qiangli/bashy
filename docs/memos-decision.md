# Memos Integration Assessment and Adapter Architecture

**Status:** PARKED by operator decision 2026-11-10. The recommendation below is recorded for reference only; no follow-up is filed (Sprint 614, Story #105 / dfb2cc56bd4f).  
**Context:** Collaboration MVP: assess `usememos/memos` integration, compare against Notepad and KB, and specify the smallest software-shaped adapter with CLI parity under `examples/`.

---

## 1. Codebase Inventory: Exists vs. Missing Matrix

An audit of the bashy repository (commands, examples, builds, and CI pipelines) verifies that Memos is **not** packaged, deployed, or integrated anywhere in the project.

| Area | Component / Path | Status | Evidence / Notes |
|---|---|---|---|
| **Commands** | Registered CLI verbs | **Missing** | No `memos` or `memo` verb exists in `commands add`, built-ins, or default aliases. |
| **Examples** | `examples/desk/kit/.../fixture` | **No Memos** | Internal test fixture named `memo` (`fixture/main.go`) is a generic Desk item stub; no connection to `usememos/memos`. |
| **Examples** | `examples/desk/notepad` | **Different** | Existing Desk tool for viewing/editing single Markdown/text documents; no micro-note stream. |
| **Builds & CI** | `Makefile`, `.github/`, scripts | **Missing** | No build targets, container recipes, or vendor dependencies for Memos. |
| **Core Engine** | `internal/agentos/decorators*` | **Unrelated** | `@memo` decorator provides in-memory function call memoization; no note store. |

**Conclusion:** Zero prior integration. Greenfield evaluation.

---

## 2. Comparative Analysis: Memos vs. Existing Notepad + KB

| Feature / Dimension | Notepad (`examples/desk/notepad`) | Knowledge Base (`bashy kb`) | Memos (`usememos/memos`) |
|---|---|---|---|
| **Primary Pattern** | Single-document viewer and editor | Durable wiki & agent strategy memory | Micro-blogging timeline & fast note stream |
| **Target Audience** | Human editing + agent document export | Agents (session memory, rules, runbooks) | Humans (daily logs) + agents (stream events) |
| **Data Structure** | Isolated files (`<id>.json`, md/txt/pdf) | Markdown pages with YAML frontmatter | Micro-notes with tags (`#tag`), pins, attachments |
| **Storage Engine** | Filesystem (`$BASHY_HOME/desk/notepad/`) | Git repo (`docs/kb/`) or host (`$BASHY_HOME/kb`) | Embedded SQLite database (`memos_prod.db`) |
| **Editing Model** | Author-owned whole document editing | Curated lifecycle (add, retro, supersede) | **Author-owned micro-edits** (no co-editing) |
| **Visibility Model** | Local loopback (shared via capability code) | Scoped rings (`repo`, `host`, `agent`) | Three tiers: `PRIVATE`, `PROTECTED`, `PUBLIC` |
| **API & Protocol** | Desk REST API (`/api/action`, `/api/state`) | CLI verbs + MCP tools (`kb_recall`, etc.) | REST API (`/api/v1`), gRPC, native MCP (`/mcp`) |
| **CLI Support** | Full Desk verbs (`add`, `set`, `open`, etc.) | Full CLI verbs (`add`, `show`, `search`, etc.) | Requires Bash# wrapper for Bashy CLI parity |

**Key Takeaway:** Memos does not displace Notepad (which excels at multi-format document conversion and Desk integration) or KB (which is the authoritative, git-committed repository memory). Instead, Memos fills the fast timeline/micro-note stream gap with native tagging, resource attachments, and multi-tier visibility.

---

## 3. Upstream Evaluation: `usememos/memos`

* **Pinned Release:** `26.11.0` (CalVer release, November 2026; backwards-compatible with `v0.24.0` REST API baseline).
* **License:** **MIT License** (fully permissive, 100% compliant with Bashy's foundation licensing requirements).
* **API & Protocols:**
  * **REST API:** Standard HTTP/JSON endpoints under `/api/v1/` (`/api/v1/memos`, `/api/v1/users`, etc.) generated via gRPC-gateway.
  * **Model Context Protocol (MCP):** Native built-in MCP server endpoint (`/mcp`) supporting standard MCP tools (search, create, update, delete memos).
* **Authentication & Token Handling:**
  * Personal Access Tokens (PAT) with format `memos_pat_<32-chars>`.
  * Passed via standard HTTP header: `Authorization: Bearer memos_pat_...`.
  * Displayed once upon generation; stored as a salted SHA-256 hash in SQLite.
  * Credentials stored securely on host under `$BASHY_HOME/credentials/memos.token` (permissions `0600`) or via `MEMOS_TOKEN`.
* **Platform, Build & Operational Footprint:**
  * **Pure Go Backend:** Uses `modernc.org/sqlite` (pure Go port; **zero CGO**, compiles with `CGO_ENABLED=0`).
  * **Self-Contained Frontend:** Pre-built React/Tailwind web assets embedded directly into the Go binary via `embed.FS`.
  * **Cross-Platform Binaries:** Upstream publishes signed single-binary archives for Linux (amd64, arm64), macOS/Darwin (amd64, arm64), and Windows (amd64) with `checksums.txt`.
  * **Operational Cost:** Zero external runtime services; requires only loopback port (`--port 5230`) and data directory (`--data <dir>`). Idle memory < 50MB.

---

## 4. Architecture, Visibility, and Contract Mapping

1. **Visibility Mapping:**
   * Bashy Private / Owner-only $\rightarrow$ Memos `PRIVATE` (visible strictly to the memo author).
   * Bashy Circle / Team Members $\rightarrow$ Memos `PROTECTED` (visible to authenticated workspace members).
   * Bashy Public / Web $\rightarrow$ Memos `PUBLIC` (visible to anonymous/external viewers if public access is enabled).
2. **Identity & Source Preservation:**
   * The active caller (`BASHY_PRINCIPAL` or agent name, e.g., `agent:name`) is injected into note content/metadata via structured tags (`#agent/<name>`, `#source/bashy`) and payload properties.
3. **Native Storage & Ref Adapter:**
   * Notes remain native inside Memos' SQLite database. **No duplicate note catalog** or secondary database.
   * Entity reference format: `memo:<id>` or `memo:<uid>` resolved via `bashy define memo:<id>` using the Bash# adapter.
4. **Explicit KB Reference Registration (No Two-Way Sync):**
   * Durable persistence avoids complex two-way sync. When an agent promotes a memo to project knowledge, it executes an explicit one-way command:
     `bashy memos register-kb <id> [--ring repo|host]`
     which calls `bashy kb note add` recording a distilled summary and citation pointer (`Ref: memo:<id>`).
5. **Optional Git Snapshot Export:**
   * `bashy memos snapshot [--dir <path>] [--format md|json]` dumps all/filtered memos to flat Markdown files with frontmatter in a Git directory, committing via standard local Git tooling.

---

## 5. Software Shaping & "Rod, Not Fish" Design

Following `[[kb:bashy/extend-bashy-small-verbs-bash-skill-scripts-shaping-hooks-and-th]]`:
* **Reusable Bash# Wrapper:** `examples/memos/memos.bsh` provides the unified command and embedded skill (`~~~skill as guide`).
* **Independent Sub-Operations:** The wrapper composes existing CLI tools (`curl`, `tar`, upstream binary execution) rather than creating monolithic binaries.
* **Shaping Hook:** Declared in `examples/memos/config.json` and `examples/memos/hooks/format.bsh`. Users and shaping agents can customize export templates, tag categorization, and default visibility rules without modifying Go code.
* **Zero Go in Core:** No new Go code in `internal/` or core binaries.

---

## 6. Agent-First CLI Parity Plan

Every feature in the Memos web UI is mapped 1:1 to non-interactive, scriptable CLI verbs with `--json` support:

| Web UI Action | Bashy CLI Verb | Non-Interactive Flags & Options |
|---|---|---|
| Browse timeline & filter | `bashy memos list` | `[--visibility LEVEL] [--tag TAG] [--search QUERY] [--limit N] --json` |
| View memo details | `bashy memos show <id>` | `--json` |
| Create new memo | `bashy memos add` | `[--body TEXT] [-f FILE\|-] [--visibility LEVEL] [--tags TAGS] [--dry-run] --json` |
| Edit existing memo | `bashy memos set <id>` | `[--body TEXT] [-f FILE\|-] [--visibility LEVEL] [--dry-run] --json` |
| Delete / archive memo | `bashy memos rm <id>` | `[--archive] [--dry-run] --json` |
| Pin / unpin memo | `bashy memos pin <id>` | `[--unpin] [--dry-run] --json` |
| Manage tags | `bashy memos tags` | `list \| rename OLD NEW \| rm TAG [--json]` |
| Manage resources / files | `bashy memos resource` | `add -f FILE \| list \| rm ID [--json]` |
| Manage API tokens | `bashy memos token` | `mint [--name NAME] \| list \| revoke ID [--json]` |
| Admin / system status | `bashy memos admin` | `status \| users \| set-config KEY=VAL [--json]` |
| Service daemon lifecycle | `bashy memos daemon` | `start [--port PORT] [--data DIR] \| stop \| status [--json]` |
| Register ref to KB | `bashy memos register-kb <id>` | `[--ring repo\|host] [--tags TAGS] --json` |
| Export snapshot to Git | `bashy memos snapshot` | `[--dir PATH] [--since DATE] [--format md\|json] --json` |

---

## 7. Placement and Graduation Path

* **Initial Placement:** `examples/memos/` (Tier: Experimental / Alpha). Independently usable example; does not contaminate core bashy binaries.
* **Graduation Path:**
  1. *Example (`examples/memos/`):* Standalone Bash# wrapper + pre-built binary runner.
  2. *Optional Add-on (Preview / Beta):* Tested across macOS, Linux, and Windows; integrated with `bashy binmgr` or package manager.
  3. *Core (Supported):* Not recommended for Memos. External multi-tier web applications remain optional add-ons or examples to protect the pure-Go, zero-dependency core release.

---

## 8. Recommendation

**Adopt `usememos/memos` pinned at release `26.11.0` as an example-level software-shaped integration under `examples/memos/`.**  
Drive the integration entirely through a reusable Bash# wrapper (`examples/memos/memos.bsh`) interacting with Memos' native REST API and MCP server. Maintain author-owned editing, enforce token storage under `$BASHY_HOME/credentials/`, support explicit one-way KB reference registration (`register-kb`), and provide Git snapshot exports (`snapshot`).  
**Explicitly Rejected:** Do NOT fork upstream Memos, do NOT bundle Memos into the core Go binary, do NOT deploy centralized servers in this spike, do NOT build a two-way sync engine, and do NOT create a parallel note catalog.

---

## 9. Draft Follow-Up Story (Do Not File)

```markdown
Title: Implement Memos example adapter and agent CLI wrapper under examples/memos
Scope:
  - Add examples/memos/memos.bsh with full CLI parity for user and admin actions.
  - Implement automated fetch/verification of pinned usememos/memos 26.11.0 binary per OS.
  - Add token credential storage under $BASHY_HOME/credentials/memos.token (0600).
  - Implement bashy memos register-kb <id> for one-way KB reference registration.
  - Implement bashy memos snapshot for Git Markdown/JSON exports.
  - Add examples/memos/register.sh and unit/contract loopback tests (checks/smoke.bsh).
Done-When:
  - examples/memos/memos.bsh executes list, add, show, set, rm, pin, and token commands against a loopback instance with valid --json output on macOS, Linux, and Windows.
  - bashy memos register-kb produces a valid kb: reference entry in docs/kb/ without duplicating full document storage.
  - No Go code added to internal/ or core bashy binary; clean GOWORK=off build.
```
