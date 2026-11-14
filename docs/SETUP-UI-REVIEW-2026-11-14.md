# Guided Setup UI Review — 2026-11-14

## Design

A task-focused assistant replaces the portfolio-like landing page. Three stages cover folders, connection and trying an operation. A returning user can review a saved selection without repeating onboarding. The approved bookends symbol anchors the identity; two new raster illustrations accompany connection and inspection. Broad filesystem access is a secondary, explicitly acknowledged option. Manual entry and all native picker cancellation/error paths remain available.

The frontend retains the existing control IDs/API routes. `app.js` owns API requests and state publication; `setup-view.js` controls presentation. Advancing, going back, opening an overview or changing the destination does not save preferences or start a server/tunnel. The overview distinguishes saved choices, a prepared local command and an unverified external connection.

## Preserved Boundaries

- Go API handlers, request schemas, token comparison, host/Origin validation, filesystem confinement, persistence/revision checks and cryptographic verification code are unchanged. `server.go` adds only explicit static asset routes.
- Saving and forgetting are explicit. Loading a saved broad scope clears its acknowledgement. Forgetting preferences does not revoke a running process's access.
- Metadata discovery, binary hashing, ZIP matching and signed provenance remain separate, opt-in operations. Their success never certifies tunnel health or an MCP tool call.
- A real tool call must be performed in the connected client; the interface does not manufacture a Connected state.
- Existing credentials, tunnel profiles, client settings and user folders are not changed by this redesign.

## Frontend Corrections Found During Review

1. Folder validation and file-verification replies are ignored when they belong to an earlier selection. A new choice cannot inherit a stale ready/verified result.
2. API fetches use `referrerPolicy: same-origin`. With the page's global no-referrer policy, Firefox sent `Origin: null` on local POSTs and the strict server correctly rejected them. The frontend now sends the expected local origin without relaxing the server.
3. Copy actions preserve keyboard focus. Navigation focuses the new heading; reduced motion is honored.
4. Late saved-preference responses do not take over while a user is already entering a selection. Failed persistence is reported without claiming a successful save.

## Verification

- `go test -timeout=120s ./...`: passed across all five packages on Windows.
- `go vet ./...` and `go build ./cmd/dirhaven`: passed.
- 13 browser scenarios across desktop/mobile Chromium and Firefox: 39 passed, using the public-source font fallbacks.
- Browser scenarios use a test-only local server and disposable directories/preferences. Native pickers are deterministic fixtures. Real HTTP validation and persistence are exercised; frontend cryptographic result presentation is mocked. Existing Go tests exercise the cryptographic handlers with their existing verifier fixtures.
- WCAG A/AA checks cover each stage and the overview; viewport checks cover 320, 390, 768, 1024 and 1440 px, 200% zoom and forced colors.
- Linux Chromium UI checks are added to the existing Linux CI job; no additional parallel runner is created. Local evidence is not a claim that hosted CI ran.

## Review Limits

Actual interactive native OS dialogs, external OpenAI account/tunnel configuration, real signed-release acceptance and real ChatGPT tool calls have not been newly certified here. Those remain manual/integration acceptance work. This change is a reviewable setup redesign, not a new product release.

The owner's ignored proprietary fonts may be used for the private local review only. They remain untracked, and are not part of the public source or a distributable release. Public builds retain system-font fallbacks. Artwork is separately reserved under `assets/BRANDING.md`; prompts and hashes are in `SETUP-ART-2026-11-14.json`.

## Running The UI Checks

Use Go from `go.mod` and Node 24:

```sh
npm ci --ignore-scripts
npx playwright install chromium firefox
npm run test:ui
```

On Windows with installed Edge, set `PLAYWRIGHT_CHANNEL=msedge`. The opt-in Go browser fixture listens on loopback port 4189, uses temporary folders and a temporary preference store, and is not compiled into the CLI. Its shutdown is handled by the browser test teardown.