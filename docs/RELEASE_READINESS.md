# Dirhaven v0.1.0 - release readiness (draft)

**Status: v0.1.0 pre-release approved by project owner; publish only after CI succeeds on the exact release commit.**

This document records observed behavior and release gates. The software license is the repository's root LICENSE (Apache-2.0); this document does not modify it.

## Verified

- Standalone Go MCP server with 17 tools: 14 filesystem compatibility entrypoints and 3 bounded source-retrieval entrypoints.
- Native Go CI previously passed Windows, Ubuntu Linux and macOS, including builds and tests (the exact release commit must still pass CI).
- A private ChatGPT Web conversation used the **standalone Dirhaven binary** through an **existing** OpenAI Secure MCP Tunnel and custom plugin. A temporary file was created, edited, read back through MCP and independently verified on local disk, then deleted. The repository remained clean afterward.
- The existing integration was updated in place; no new tunnel or distributable plugin was created.

## Security and integration limitations

- The private live deployment explicitly uses `--allow-all-drives`, exposing available Windows drive-letter roots (observed C: and D:). This does **not** demonstrate access denial outside a narrowly allowed root.
- Confinement has local test coverage. Live ChatGPT Web out-of-root denial, adversarial escape attempts and independent security review remain **unverified**.
- `--allow PATH` is the recommended default for published examples. `--allow-all-drives` is an opt-in broad-access mode, **not** the default.
- OpenAI tunnels, custom plugin setup, credentials, workspace permissions and approvals are external dependencies and are not part of Dirhaven distribution.
- Private successful integration is not a guarantee that every account, operating system or MCP host allows writes or supports all 17 tools.

## Remaining release gates

- [x] Add the exact Apache-2.0 root `LICENSE`, project `NOTICE`, and a separate brand-artwork rights statement. **This grant cannot be treated as reversible simply by withholding a release tag.**
- [x] Owner states Dirhaven is a personal project made at home with AI assistants, with no employees or client commission. Git history shows extraction from the owner's earlier private agent-runtime repository. This is the owner's rights representation, not an independent legal opinion.
- [x] Inventory the full 15-module Go graph with versions and preserve original module-level LICENSE and PATENTS texts in `third_party/licenses/`.
- [x] Validated download packages for all three operating systems at commit `4e36db4`, including binary SHA-256 hashes and accompanying license/notice files. The release packaging workflow independently checks those contents again before publishing the tagged version.
- [ ] Future hardening: publish a reproducible narrowly scoped ChatGPT Web acceptance report for out-of-root denial. The real ChatGPT Web read/write workflow passed through the existing broad-access configuration, and local confinement tests exist. Do not claim the former proves narrow-root isolation.
- [ ] Future hardening: publish a versioned public schema/compatibility contract for the 17 tools; current README documents their names and scope.
- [x] Prepare Windows, Linux and macOS packaging with binaries, SHA-256 checksums and license notices; the workflow runs manually or on a version tag, and publishes a GitHub pre-release only for version tags.
- [x] Previous packaging workflow passed on all three OSes at commit `4e36db4`; all three downloaded ZIPs had the required notices and valid checksums. A Linux binary was independently launched. Tagged release assets will be revalidated by the publishing workflow.
- [x] Add `SECURITY.md` and `CONTRIBUTING.md` with vulnerability-handling guidance, contribution terms and current support limitations.
- [x] Confirmed GitHub Private Vulnerability Reporting is enabled (`enabled: true`).
- [x] Bounded credential/path-pattern scan of the source tree and the three previous artifact ZIPs found no matches. This is not a comprehensive third-party audit or a proof of absence of private data.
- [ ] Confirm native CI is green on the **exact commit** that will be tagged.
- [x] Owner approved proceeding with `v0.1.0` as a pre-release.
- [ ] After exact-release-commit CI passes, tag `v0.1.0`; the release workflow uploads tested packages and creates a GitHub pre-release.

## Proposed first pre-release scope

`dirhaven serve --allow /absolute/path` starts a local `stdio` MCP server. `--allow-all` is explicitly available on Windows, Linux and macOS when a user chooses broad filesystem access. It is a standalone source/build distribution; it **does not** provide a hosted service, install OpenAI plugins, create tunnels or grant unrestricted filesystem access by default.

The public release notes can claim private ChatGPT Web read/write integration was exercised, but **must not** claim formally audited confinement, universally available ChatGPT write actions, production-grade lifecycle management or general release readiness.

Related GitHub gate: [issue #2](https://github.com/diegolinan/dirhaven/issues/2).
