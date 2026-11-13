# Third-party Go module license inventory

This inventory is based on `go list -m all` in the Dirhaven Go module
on November 13, 2026. It lists the **complete module graph**, which may
include dependencies used only by tests or tooling. It is not a claim that
all modules are linked into every release binary.

| Go module | Version | License noted in module |
| --- | --- | --- |
| `cloud.google.com/go/compute/metadata` | v0.3.0 | Apache-2.0 |
| `github.com/bmatcuk/doublestar/v4` | v4.10.4 | MIT |
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | MIT |
| `github.com/google/go-cmp` | v0.7.0 | BSD-3-Clause |
| `github.com/google/jsonschema-go` | v0.4.3 | MIT |
| `github.com/modelcontextprotocol/go-sdk` | v1.8.0 | Apache-2.0 / MIT transition; CC-BY-4.0 for its documentation |
| `github.com/pmezard/go-difflib` | v1.0.0 | BSD-3-Clause |
| `github.com/segmentio/asm` | v1.1.3 | MIT |
| `github.com/segmentio/encoding` | v0.5.4 | MIT |
| `github.com/yosida95/uritemplate/v3` | v3.0.2 | BSD-3-Clause |
| `golang.org/x/oauth2` | v0.35.0 | BSD-3-Clause |
| `golang.org/x/sync` | v0.20.0 | BSD-3-Clause; separate PATENTS file |
| `golang.org/x/sys` | v0.41.0 | BSD-3-Clause; separate PATENTS file |
| `golang.org/x/time` | v0.15.0 | BSD-3-Clause; separate PATENTS file |
| `golang.org/x/tools` | v0.42.0 | BSD-3-Clause; separate PATENTS file |

Verbatim module-level LICENSE and PATENTS files were preserved in
`third_party/licenses/`. The MIT/Apache transition text of the MCP
SDK is kept intact; its code must **not** be misrepresented as uniformly
Apache-2.0. Third-party files remain governed by their own licenses.

Before creating an official release, compare this inventory with the
**exact tagged dependency graph**, check for nested license notices,
and verify that all required third-party notices are inside the
platform-specific downloadable artifacts.
