# Dirhaven v0.1.0 - first pre-release

**Your files. Your boundaries.**

Dirhaven is a standalone local filesystem MCP server written in Go. Its primary goal is to let people work with files on their own computer from **ordinary ChatGPT Web conversations** through a separately configured private MCP connection. It can also be used with compatible local MCP clients.

This is an **early pre-release**, not a security-audited production service.

## Included

- **17 MCP tools:** 14 filesystem-compatible operations for finding, reading, creating, editing, moving and inspecting files, plus 3 bounded, source-preserving text retrieval tools.
- **Windows, Linux and macOS** native builds, with SHA-256 checksums and bundled third-party license notices.
- `dirhaven serve --allow PATH` for repeatable, user-chosen directories on every supported operating system. Dirhaven does not assume a user's drive, mount point or home-directory layout.
- Optional `dirhaven serve --allow-all` to grant broad access within the permissions of the user running Dirhaven. On Windows this discovers available drive letters dynamically; on Linux/macOS it authorizes paths under `/`, including mounted directories. The older Windows-only `--allow-all-drives` remains for compatibility.
- Local MCP `stdio` transport. Dirhaven is not a hosted service or a ChatGPT plugin.

## Getting started

1. Download the package for your operating system below and extract it.
2. Read `README.md`, `SECURITY.md`, and `LICENSE` in the package.
3. Configure a supported local MCP client or OpenAI Secure MCP Tunnel to run:

   ```text
   dirhaven serve --allow /absolute/path/you/choose
   ```

   On Windows, use an absolute Windows directory path and `dirhaven.exe`.

OpenAI tunnel setup, custom plugin creation, accounts and credentials are **external** to Dirhaven. Consult current OpenAI documentation for those steps. Nothing in the download installs a tunnel or grants access automatically.

## Verification and limits

- Native Go tests, `go vet` and executable builds were run on Windows, Ubuntu Linux and macOS in GitHub Actions; race tests ran on Linux and macOS.
- Release packages include SHA-256 checksum files, Apache-2.0 license, project notice, third-party license copies and security/contribution documentation.
- A private ChatGPT Web integration successfully read, created and edited a real local file through an existing Secure MCP Tunnel, with independent disk verification.
- **Not a formal security audit.** That private live integration uses broad Windows drive access, not a proof of denial outside a restricted root. Local confinement tests exist, but use with appropriately trusted MCP clients and OS user permissions.
- Availability of ChatGPT write operations can depend on account or workspace permissions.

## Licensing

Dirhaven's code is licensed under **Apache License 2.0**. Third-party dependencies retain their own license terms, included in the packages. The Dirhaven banner and brand artwork are **not** included in the Apache grant.

Found an issue? Use [GitHub Issues](https://github.com/diegolinan/dirhaven/issues) for non-sensitive bugs. Report security vulnerabilities privately via GitHub's private vulnerability reporting.
