# Dirhaven v0.1.0 - release readiness (draft)

**Status: NOT READY TO TAG OR PUBLISH AS OPEN SOURCE.**

This document records observed behavior and release gates. It does not grant any copyright or redistribution license.

## Verified

- Standalone Go MCP server with 17 tools: 14 filesystem compatibility entrypoints and 3 bounded source-retrieval entrypoints.
- Native Go CI covers Windows, Linux and macOS (the exact release commit must still pass CI).
- A private ChatGPT Web conversation used the **standalone Dirhaven binary** through an **existing** OpenAI Secure MCP Tunnel and custom plugin. A temporary file was created, edited, read back through MCP and independently verified on local disk, then deleted. The repository remained clean afterward.
- The existing integration was updated in place; no new tunnel or distributable plugin was created.

## Security and integration limitations

- The private live deployment explicitly uses `--allow-all-drives`, exposing available Windows drive-letter roots (observed C: and D:). This does **not** demonstrate access denial outside a narrowly allowed root.
- Confinement has local test coverage. Live ChatGPT Web out-of-root denial, adversarial escape attempts and independent security review remain **unverified**.
- `--allow PATH` is the recommended default for published examples. `--allow-all-drives` is an opt-in broad-access mode, **not** the default.
- OpenAI tunnels, custom plugin setup, credentials, workspace permissions and approvals are external dependencies and are not part of Dirhaven distribution.
- Private successful integration is not a guarantee that every account, operating system or MCP host allows writes or supports all 17 tools.

## Remaining release gates

- [ ] Owner approves source-code provenance, copyright, third-party reuse rights and an explicit root `LICENSE`.
- [ ] Audit direct **and transitive** Go dependency licenses and distribute required notices.
- [ ] Run and publish a reproducible ChatGPT Web acceptance report with an explicitly authorized directory and an out-of-root denial check, without changing the owner's broader live configuration unexpectedly.
- [ ] Confirm the public tool/schema contract for all 17 tools and document compatibility expectations.
- [ ] Produce release builds for Windows, Linux and macOS, with SHA-256 checksums and clean-install checks.
- [ ] Add security contact, vulnerability reporting policy, contribution terms and support limitations.
- [ ] Audit the **exact release tree and artifacts** for local credentials, filesystem paths, private identifiers, unpublished material and third-party branding.
- [ ] Confirm native CI is green on the **exact commit** that will be tagged.
- [ ] Only after owner approval, create an explicitly labeled `v0.1.0` pre-release tag and release entry.

## Proposed first pre-release scope

`dirhaven serve --allow /absolute/path` starts a local `stdio` MCP server. It is a standalone source/build distribution; it **does not** provide a hosted service, install OpenAI plugins, create tunnels or grant unrestricted filesystem access by default.

The public release notes can claim private ChatGPT Web read/write integration was exercised, but **must not** claim formally audited confinement, universally available ChatGPT write actions, production-grade lifecycle management or general release readiness.

Related GitHub gate: [issue #2](https://github.com/diegolinan/dirhaven/issues/2).
