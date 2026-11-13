# Dirhaven

**Your files. Your boundaries.**

**Use ChatGPT on the web to work with files on your own computer, inside folders you explicitly authorize.**

Dirhaven is a **local-first filesystem MCP server written in Go**. Its primary use case is connecting ChatGPT Web to your local filesystem through an OpenAI-supported MCP connection, while enforcing explicitly configured directory boundaries. Other MCP clients that support local `stdio` servers can also use Dirhaven directly.

Dirhaven itself runs locally. ChatGPT does **not** connect straight to your computer: this use case requires a separate OpenAI Secure MCP Tunnel and custom MCP plugin configuration. Dirhaven does not ship those components.

> **Status: early development, not a tagged release.** The current implementation was extracted into a new standalone source module. It must not be treated as audited, fully portable or feature-complete. Distribution licensing and contribution terms are not yet finalized.

## Why Dirhaven exists

ChatGPT Web is a powerful place to work through ideas, code, documents, and decisions. But working with a file that already lives on your own computer can mean uploading it, copying its contents into a conversation, or switching to a separate coding or computer-use environment.

**Dirhaven exists to close that gap.** Its goal is to make it possible to stay in a regular ChatGPT Web conversation and ask ChatGPT to find, read, create, edit, or organize files on your computer, **without manually transferring those files into the chat**, once the required connection is configured.

The value is not simply another implementation of a filesystem MCP server. It is the connection between **a web conversation and the user's actual local filesystem**, with access limited to directories the user explicitly authorizes.

Dirhaven is **not** a replacement for Codex CLI, ChatGPT Work, a terminal, or a general-purpose computer-control agent. It provides a focused, locally running filesystem capability. OpenAI's tunnel and plugin infrastructure provide the separate connection into ChatGPT Web.

### The acceptance test that matters

Dirhaven fulfills its primary purpose when a user can open ChatGPT Web, request a change to a file on their own computer, have that change performed through the **standalone Dirhaven server** without manually uploading the file, and confirm that attempts to access files **outside the authorized roots are denied**.

A successful build, green tests on three operating systems, and a working local MCP connection are necessary engineering evidence. **None of them alone proves the ChatGPT Web workflow works.** Until an end-to-end test demonstrates that exact path using disposable files and a separately configured OpenAI connection, the product's primary use case remains unvalidated.

## What Dirhaven does

- Enables the **ChatGPT Web -> secure connection -> local files** workflow when the required OpenAI integration is configured.
- Runs locally over **stdio MCP**; it does not start a network server or itself require a cloud account.
- Limits operations to explicitly allowed directory roots using Go filesystem confinement mechanisms.
- Offers **14 compatibility-oriented filesystem tools** plus **3 bounded source-selection tools**.
- Supports reading, searching, listing, creating, editing, moving and inspecting files inside the authorized roots.
- Offers opt-in, metadata-only local call metrics.
- Supports explicit roots on Windows, macOS and Linux. The broader Windows drive-discovery mode is optional, **not a safe default**.

**Dirhaven does not run AI models.** Your MCP client supplies the agent and decides which available tools to call.

## Primary use case: ChatGPT Web accessing local files

Dirhaven provides the local filesystem endpoint. To use it **from ChatGPT in a browser**, an operator must separately connect ChatGPT to that endpoint using OpenAI's **Secure MCP Tunnel** and a **custom MCP plugin**. ChatGPT cannot launch a local `stdio` executable directly from the web interface.

Dirhaven **does not bundle, create, configure, or maintain** OpenAI tunnels, tunnel clients, plugins, account credentials or workspace permissions. Those are external setup requirements, may change independently and may be restricted by account or workspace policies. Follow the official instructions instead of a copied setup tutorial:

- [Connecting custom MCP servers in ChatGPT](https://help.openai.com/en/articles/12584461-connecting-custom-mcp-servers-in-chatgpt)
- [Secure MCP Tunnel](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels)

Once the connection is configured and running, ChatGPT can discover and request Dirhaven tools within the authorized directories, subject to client-side approvals and permissions. **Dirhaven has not yet completed a published end-to-end ChatGPT Web acceptance test**, so the entire workflow remains a release validation gate.

The current Secure MCP Tunnel mechanism supports private connections, **not public plugin submission**. Dirhaven is source code that operators can deploy for their own use; it is not a preinstalled ChatGPT plugin. Other compatible MCP clients may connect directly over `stdio` without an OpenAI tunnel.

## Build and run Dirhaven locally

### Requirements

- Go **1.27** or later to build from source.
- A compatible MCP client, or an OpenAI Secure MCP Tunnel configured to launch the local `stdio` server.

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

Dirhaven speaks MCP over standard input/output. **It is not an interactive shell**. Configure the OpenAI tunnel client or a compatible local MCP client to launch the binary with the arguments above.

Example for a **local MCP client** (not ChatGPT Web or a Secure MCP Tunnel configuration; replace the binary path and root with paths on your machine):

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

1. **Validate end-to-end ChatGPT Web access to a disposable local directory** through a separately configured OpenAI Secure MCP Tunnel and custom MCP plugin, including tool discovery, safe read/write operations and denial outside authorized roots.
2. Keep the 17 exposed MCP tools compatible and maintain native Windows, Linux and macOS tests; add smoke tests for other MCP clients.
3. Publish a pinned, reviewed dependency inventory and choose an explicit open-source license after rights review.
4. Document support/versioning guarantees and prepare the first tagged pre-release.
5. Continue cross-platform confinement and compatibility testing.

**License status:** no open-source license has been selected or granted yet. Public source visibility alone is not a license to reuse or redistribute the code. An explicit license is required before describing a tagged release as open source.
