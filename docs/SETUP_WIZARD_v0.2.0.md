# Dirhaven 0.2.0-dev - setup wizard, first working slice

**Status:** development only. This does not replace the published `v0.1.0` pre-release and is not yet a native OS installer.

## Product boundary

Dirhaven is the filesystem MCP server. Each user owns their local installation and their private MCP connection. We do **not** distribute the project owner's personal ChatGPT plugin, tunnel ID, keys, workspace configuration or private runtime profile.

The intended experience is:
1. Download and install the correct native Dirhaven package for the user's OS.
2. Open a graphical setup wizard, choose authorized folders or intentionally grant broad access.
3. Confirm which ChatGPT MCP features the user's **plan and workspace permissions** can support.
4. Set up or reuse a privately owned Secure MCP Tunnel, managed using OpenAI's supported tooling.
5. Create the user's own custom MCP connection in ChatGPT Web where permitted.
6. Validate the **three distinct states**: Dirhaven ready, tunnel healthy, ChatGPT tool call verified. Never collapse these into a single "Connected" badge.

Users on plans without full ChatGPT Web MCP capability must still be able to use Dirhaven with another compatible local stdio MCP client.

## Visual identity (direct Work page reuse)

The setup page is now built on the **actual** `diegolinan.me/work/` structure: original `.work-frame`, `.work-header`, `.work-intro`, `.work-sections`, `.work-list`, `.work-entry`, and `.work-footer` markup conventions. These files were copied **verbatim** from the owner's local personal-site repository:
- `internal/setupwizard/web/foundation.css` - original `D:/Projects/diegolinan.me/foundation.css`
- `internal/setupwizard/web/work.css` - original `D:/Projects/diegolinan.me/pages/work-index/work.css`

The new `setup.css` contains only functional adaptations needed for forms, steps and responsive layout. It is **not** a separate reconstruction of the owner's visual system. The approved layered-folder/doorway logo concept is represented by a scalable SVG interpretation.

The owner authorized using the purchased Die Grotesk family. For local preview only, the three original `.woff2` binaries are copied from the personal website into `internal/setupwizard/web/assets/` and embedded in the development binary. Browser checks confirmed HTTP 200 and loaded font faces for `Die Grotesk`, `Die Grotesk Reading` and `Space Mono`. **The binaries are gitignored and are not included in the public Apache-2.0 source tree.** Public builds currently fall back to system fonts; before shipping a publicly downloadable executable with embedded font files, verify that the owner's license includes the required application embedding and distribution scope.

The direct page reuse changes frontend assets, their static routes and in-page navigation only; root validation, deliberate `--allow-all` acknowledgment, sensitive credentials and MCP semantics are unchanged. Local Chromium checks cover 1440, 1024, 768, 390 and 320 CSS-pixel widths with original fonts loaded and tested UI interactions. Native Go tests validate the local HTTP static assets and authorization boundary.

## First working slice (this branch)

Run `dirhaven setup` (after building `./cmd/dirhaven`) to launch a local browser wizard.

- Embedded visual UI, same Go binary on Windows, macOS and Linux.
- The setup listener binds exclusively to an ephemeral `127.0.0.1` port. A random authorization token is carried in the initial URL fragment, then removed from the address bar. API requests must present the token and be from the correct host and origin.
- The wizard reads **directory metadata only**, validating that chosen absolute paths exist and are directories. It never scans their contents.
- A user-triggered **Choose folder** action opens a native desktop folder dialog through the OS (Windows Forms dialog, macOS AppleScript, Linux Zenity or KDialog). It appends a chosen absolute path to the editable list, but never grants access by itself. The list must still be validated separately. Headless Linux systems without a picker retain manual entry. Only one picker may be active per wizard session, and requests require the same local token and host/origin checks as validation.
- `--allow-all` requires a separate explicit acknowledgement.
- Outputs a correctly quoted local shell command and a **generic local stdio MCP client JSON example**. The JSON example is not a ChatGPT Web plugin configuration.
- Links to authoritative current OpenAI instructions for plan compatibility, tunnel setup and plugin creation.
- No credentials are requested or stored. No files, services, tunnels, plugins or OS settings are created or changed by setup.
- Closing the process immediately stops the local setup listener.

**Limitations:** the native folder picker API, cancellation/error handling and browser interactions have automated tests, but the actual interactive OS dialog still requires manual acceptance on each supported desktop environment. The browser wizard is a *configuration assistant*, not yet an installer. It does not install `tunnel-client`, automate OpenAI account actions, discover the user's plan or workspace permissions, track tunnel health, or verify a ChatGPT tool call.

## Stable requirements

- Preserve `dirhaven serve --allow PATH`, repeatable on every supported OS. Never assume drive letters, user homes or mount points.
- Preserve intentional `--allow-all` (OS privileges still apply); do not silently enable it.
- Keep file content, API keys and tunnel profiles local. No telemetry or centralized accounts by default.
- Never collect OpenAI account credentials in Dirhaven.
- No fixed CSS selectors, browser automation or hardcoded coordinates for ChatGPT UI. Use official docs links. Do not assume a selected plan proves effective authorization.
- Never claim "Connected" based solely on a healthy local server or tunnel; actual ChatGPT plugin discoverability and a test call are independent gates.
- Upgrades and uninstallers must not replace or disrupt existing, unrelated OpenAI tunnels or user plugins.
- Every release must test macOS, Linux and Windows natively and publish clearly documented OS support.

## Next milestones

1. **Native install/uninstall:** Windows installation package, signed/notarized macOS package and Linux distribution packaging, with shortcuts and clean removal. Decide signing/build tool after reviewing reproducibility and signing constraints.
2. **Native folder selection:** platform-specific folder picker with manual entry fallback is implemented; complete manual GUI acceptance on Windows/macOS/Linux desktop sessions and tighten accessibility as needed.
3. **Config persistence:** store non-secret roots under per-user OS configuration directories, with explicit review, edit and revoke; move secret references to OS-approved secure storage.
4. **Tunnel integration:** detection, official-client installation by verified source/checksum, profile generation, `doctor`, explicit run/start/stop and health monitoring. Always reuse rather than overwrite existing tunnels.
5. **Account capability diagnostics:** separate local status from independently verified platform tunnel access, workspace authorization and ChatGPT plugin usability. Track feature changes via versioned guidance grounded in official documentation.
6. **Acceptance:** real external-user clean installation, including a scoped-root denial check, without project-owner assistance.

## Reference docs and platform capability caveats

OpenAI sources consulted on November 13, 2026:

- https://help.openai.com/en/articles/12584461-connecting-custom-mcp-servers-in-chatgpt
- https://developers.openai.com/api/docs/guides/secure-mcp-tunnels
- https://platform.openai.com/settings/organization/tunnels
- https://chatgpt.com/plugins

OpenAI documents full ChatGPT Web MCP for Business and Enterprise/Edu subject to role and workspace permissions. Pro currently supports custom MCP read/fetch rather than full write access. Other plans have no verified full-MCP support in these sources. These are **time-dependent claims**, not runtime feature checks, and should be reviewed before release.
