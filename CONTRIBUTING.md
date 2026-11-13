# Contributing

Dirhaven is an early-development project focused on local filesystem access
from ordinary ChatGPT Web conversations through an external, private MCP
integration. Windows, macOS and Linux are first-class targets.

Before proposing major changes, open a GitHub issue explaining the user
problem, operating systems affected, expected MCP behavior and security
implications. Keep pull requests focused and include tests for platform
specific behavior when appropriate.

Run these checks before opening a pull request:

```sh
go test ./...
go vet ./...
```

Only submit code and assets you have the right to contribute. Unless
explicitly stated otherwise, intentionally submitted code contributions
are handled under Section 5 of the Apache License 2.0. Third-party
libraries retain their own licenses. Brand graphics are excluded from
the source-code license; see `assets/BRANDING.md`.

Do not commit credentials, private filesystem paths, logs, prompts,
conversation transcripts, or personal information. Report potential
security vulnerabilities privately as described in `SECURITY.md`.
