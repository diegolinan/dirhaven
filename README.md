# Dirhaven

**Your files. Your boundaries.**

Give AI agents access to the files they need, without automatically granting access to every file on the machine.

Dirhaven is a **local-first filesystem MCP server written in Go**. It exposes local file and directory operations to compatible Model Context Protocol (MCP) clients, constrained to roots that the operator explicitly allows.

> **Status: early development, not a tagged release.** The current implementation was extracted into a new standalone source module. It must not be treated as audited, fully portable or feature-complete. Distribution licensing and contribution terms are not yet finalized.

## What Dirhaven does

- Operates locally over **stdio MCP**; it does not start a network server or require a cloud account.
- Limits operations to explicitly allowed directory roots using Go filesystem confinement mechanisms.
- Offers **14 compatibility-oriented filesystem tools** plus **3 bounded source-selection tools**.
- Supports reading, searching, listing, creating, editing, moving and inspecting files inside the authorized roots.
- Offers opt-in, metadata-only local call metrics.
- Supports explicit roots on Windows, macOS and Linux. The broader Windows drive-discovery mode is optional, **not a safe default**.

**Dirhaven does not run AI models.** Your MCP client supplies the agent and decides which available tools to call.

## Quick start

### Requirements

- Go **1.27** or later to build from source.
- A compatible MCP client capable of launching a local stdio server.

### Build

```sh
git clone https://github.com/diegolinan/dirhaven.git
cd dirhaven
go build ./cmd/dirhaven
```

The executable will be `dirhaven.exe` on Windows or `dirhaven` on macOS/Linux.

### Run

Point the server at one existing directory you deliberately want your agent to access:

```sh
./dirhaven serve --allow /absolute/path/to/workspace
```

Windows PowerShell:

```powershell
.\dirhaven.exe serve --allow 'C:\path\to\workspace'
```

To allow more than one location, repeat `--allow PATH`. The directories must exist.

Dirhaven speaks MCP over standard input/output. **It is not an interactive shell**: configure your MCP client to launch the binary and supply the arguments above.

Example MCP client configuration (replace the binary path and root with paths on your machine):

```json
{
  "mcpServers": {
    "dirhaven": {
      "command": "/absolute/path/to/dirhaven",
      "args": ["serve", "--allow", "/absolute/path/to/workspace"]
    }
  }
}
```

Different MCP hosts may use different configuration formats. This snippet illustrates the command and argument boundary, not a universal configuration schema.

### Connecting Dirhaven to ChatGPT

**Additional setup is required.** Dirhaven provides the local MCP server, but **does not provide or manage OpenAI Secure MCP Tunnels, ChatGPT plugins, accounts, credentials, or workspace permissions**.

Unlike MCP clients that can launch a local `stdio` server directly, ChatGPT cannot directly launch Dirhaven on your computer. For the current local `stdio` version, you must independently set up an OpenAI-supported connection to your local server (such as **Secure MCP Tunnel**) and configure the corresponding **custom MCP plugin** in ChatGPT, subject to your account and workspace permissions.

**Dirhaven does not configure or install these OpenAI components.** Their availability, setup and naming may change independently of this project. Always follow OpenAI's current official documentation rather than relying on instructions copied into this repository:

- [Connecting custom MCP servers in ChatGPT](https://help.openai.com/en/articles/12584461-connecting-custom-mcp-servers-in-chatgpt)
- [Secure MCP Tunnel](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels)

This setup is **specific to ChatGPT**, not a universal requirement for using Dirhaven with other MCP clients. Only grant access to folders you intend to share with the connected AI client.

### Optional local metrics

```sh
./dirhaven serve --allow /absolute/path/to/workspace --metrics-file /private/existing-directory/calls.jsonl
```

The metrics file is opt-in. Its parent directory must already exist. Records contain allowlisted tool names, byte counts, timing and error flags, not file contents, requested paths or credentials. Keep the metrics directory private.

## MCP tool surface

The 14 compatibility-oriented filesystem tool names are:

| Category | Tools |
| --- | --- |
| Read | `read_file`, `read_text_file`, `read_media_file`, `read_multiple_files` |
| Write | `write_file`, `edit_file`, `create_directory`, `move_file` |
| Discover and inspect | `list_directory`, `list_directory_with_sizes`, `directory_tree`, `search_files`, `get_file_info`, `list_allowed_directories` |

Three additional tools support bounded, source-preserving retrieval:

- `read_file_lines`: retrieve exact numbered text lines with SHA-256 change detection and pagination.
- `search_text_file`: find literal text in one file, returning original line numbers with bounded output.
- `search_text_files`: search multiple explicitly authorized files with bounded output, cursors and snapshot checks.

These three tools do **not** summarize or semantically interpret source material. They preserve explicit source evidence.

## Access boundaries and limitations

**A filesystem MCP server can modify data.** `--allow` is an access scope, not a read-only permission or a substitute for OS filesystem security.

- Use the smallest practical set of allowed roots, ideally a disposable workspace for first use.
- Do not assume that an AI agent, MCP client or model will make safe decisions just because the server validates paths.
- Dirhaven relies on the operating system's permissions and Go root-confinement APIs; it does not provide process isolation, user impersonation or an OS-level sandbox.
- Do not expose its stdio transport to untrusted remote clients without an independently reviewed security boundary.
- Windows-only `--allow-all-drives` (equivalent to `--allow '*'`) allows **all currently accessible drive letters**, including newly mounted drives. This is deliberately broad and should not be used for a least-privilege deployment.
- Unmapped UNC shares and arbitrary non-drive mount points are not automatically covered by the Windows wildcard.
- Dynamic drive discovery is unavailable on macOS/Linux. Explicit roots remain supported.

See [Security and compatibility notes](docs/SAFETY_AND_COMPATIBILITY.md) for the precise development claims.

## Development

```sh
go test ./...
go test -race ./...     # requires a race-enabled Go toolchain
go vet ./...
go build ./cmd/dirhaven
```

GitHub Actions runs native jobs on Windows, Ubuntu Linux and macOS. A Windows SUBST drive-mount acceptance test exists but is **opt-in** and is not run by default because it creates a temporary OS-level drive mapping. An optional Node-reference parity test runs only if `FS_NODE_REFERENCE` points to a separately installed compatible reference executable.

Passing these tests does not imply that every OS filesystem feature, edge case or real MCP host integration has been independently validated.

## Scope of this repository

```text
cmd/dirhaven/           Standalone stdio CLI
internal/filesystem/    MCP tool handlers, root confinement and tests
internal/mcpmetrics/    Optional content-free local metrics
docs/                   Security and compatibility scope
.github/workflows/      Native CI on Windows, Linux and macOS
```

This repository is a **source-only extraction**. It does not include the private integration repository's Git history, tunnel profiles, vendor executables, accounts, deployment scripts, credentials or personal benchmark transcripts.

## Roadmap

1. Validate the independent package and each exposed MCP tool on native runners.
2. Add clean install examples and compatible-client smoke tests.
3. Publish a pinned, reviewed dependency inventory and choose an explicit open-source license after rights review.
4. Document support/versioning guarantees and prepare the first tagged pre-release.
5. Continue cross-platform confinement and compatibility testing.

**License status:** no open-source license has been selected or granted yet. Public source visibility alone is not a license to reuse or redistribute the code. An explicit license is required before describing a tagged release as open source.
