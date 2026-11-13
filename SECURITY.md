# Security policy

Dirhaven is an early-development local filesystem MCP server. It can read and
modify real files inside authorized roots, so treat the MCP host and its
credentials as security-sensitive. No formal independent audit or security
certification has been completed.

## Supported versions

No stable or tagged releases are supported yet. The current `main` branch
receives best-effort maintenance and should not be described as hardened
production infrastructure.

## Reporting vulnerabilities

Please **do not** publish exploit details, sensitive paths, tokens, or
private file content in a public GitHub issue.

Use GitHub's private vulnerability reporting for this repository if it is
available. If that feature is unavailable, contact the maintainer through
a pre-established private channel before sharing sensitive details.
A reliable private contact channel must be confirmed before v0.1.0.

For non-sensitive defects and feature requests, GitHub issues are suitable.

## Scope

Dirhaven enforces access roots in its Go implementation, but cannot replace
OS user permissions, a trusted MCP client, or account-level tool approvals.
`--allow-all` and Windows `--allow-all-drives` intentionally give broad
access within the process's OS privileges. The user/operator chooses the
access scope; broad access is never the default.
