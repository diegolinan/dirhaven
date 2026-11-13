# Safety and compatibility boundaries

Dirhaven is a local stdio MCP filesystem server. The server does not create a network endpoint by itself, choose an AI model, authenticate remote callers, or authorize individual agent actions beyond the configured filesystem roots.

## Security-relevant behavior

- **Explicit roots** are required unless an operator explicitly opts into broad Windows drive discovery.
- Paths are resolved relative to the allowed roots, using Go root-constrained filesystem access. Protection depends on OS behavior and package implementation; do not claim a formal sandbox certification.
- Reads, writes, edits, moves and directory creation are supported. These are potentially destructive from the perspective of users and agent prompts.
- Symlink and Windows junction escapes have targeted test coverage, but no formal security audit has been completed.
- Optional local metrics are allowlisted and do not intentionally record raw request bodies, paths or file contents.
- No process supervisor, vendor tunnel client, cloud account, hosted proxy or arbitrary Claude Code executor is included.

## MCP compatibility

The code provides 14 tool entrypoints aligned with the Node filesystem MCP reference, plus three additional bounded retrieval tools. That is **17** tools in total.

The optional reference test only runs when a developer supplies `FS_NODE_REFERENCE` explicitly. It does not search private workspaces or rely on a fixed, personal machine path.

Native Go tests cover filesystem confinement and tool contracts. Compatibility with any particular MCP host, operating system version or reference-server release must be tested before it is claimed.

## Platform support status

| Target | Scope of claim |
| --- | --- |
| Windows | Local Go tests with explicit roots. Optional SUBST drive acceptance must be explicitly enabled. |
| Linux | Portable filesystem implementation and native GitHub CI tests. |
| macOS | Portable filesystem implementation and native GitHub CI tests. |

A green CI run is evidence of the tests in that workflow, not broad certification of every filesystem edge case or complete platform parity.

## Optional Windows wildcard

`--allow-all-drives` includes accessible drive-letter roots, including newly mounted or mapped drive letters. This deliberately widens access. It must never be enabled by default by a distributor or MCP host integration.

## Release gate

The repository must not be called a finished open-source release until copyright/provenance, dependency licenses, a root LICENSE, supported configurations and independently reproduced build/test evidence are approved. Source may be visible publicly before those gates, but it is not yet licensed as open source.
