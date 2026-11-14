"use strict";

const sessionToken = window.location.hash.slice(1);
if (sessionToken) {
  window.history.replaceState(null, "", window.location.pathname);
}

const plan = document.getElementById("plan");
const role = document.getElementById("role");
const advice = document.getElementById("planAdvice");
const scopedFields = document.getElementById("scopedFields");
const allFields = document.getElementById("allFields");
const paths = document.getElementById("paths");
const pickDirectory = document.getElementById("pickDirectory");
const pickerStatus = document.getElementById("pickerStatus");
const ack = document.getElementById("ack");
const generate = document.getElementById("generate");
const stateBadge = document.getElementById("state");
const error = document.getElementById("validationError");
const results = document.getElementById("results");
const command = document.getElementById("command");
const client = document.getElementById("client");
const loadSaved = document.getElementById("loadSaved");
const revokeSaved = document.getElementById("revokeSaved");
const saveScope = document.getElementById("saveScope");
const savedStatus = document.getElementById("savedStatus");
const checkTunnel = document.getElementById("checkTunnel");
const tunnelStatus = document.getElementById("tunnelStatus");
const tunnelPath = document.getElementById("tunnelPath");
const checkChosenTunnel = document.getElementById("checkChosenTunnel");
const browseTunnel = document.getElementById("browseTunnel");
const fingerprintTunnel = document.getElementById("fingerprintTunnel");
const fingerprintStatus = document.getElementById("fingerprintStatus");
const chosenTunnelStatus = document.getElementById("chosenTunnelStatus");
let savedRevision = "";
let savedChoice = null;
let validatedScope = null;

function activeMode() {
  return document.querySelector('input[name="mode"]:checked').value;
}
function updateMode() {
  const all = activeMode() === "all";
  scopedFields.classList.toggle("hidden", all);
  allFields.classList.toggle("hidden", !all);
  resetResult();
}
function updatePlan() {
  const notes = {
    enterprise: "Enterprise / Edu: full custom MCP support is documented. Admins and owners can create connections; selected members need workspace authorization through RBAC. Write actions depend on action-level permissions.",
    business: "Business: full custom MCP support is documented. Only workspace admins and owners can create custom MCP connections.",
    pro: "Pro: custom MCP connections are currently limited to read and fetch. Do not expect Dirhaven's write or edit tools to work in ChatGPT Web. The local MCP server itself still supports writes in other permitted clients.",
    unknown: "For other plans, full custom MCP support in ChatGPT Web is not currently confirmed. Dirhaven still works with compatible local stdio MCP clients. Check official OpenAI guidance before continuing."
  };
  let message = notes[plan.value];
  if (role.value === "restricted") {
    message += " Your selected workspace role cannot create a custom MCP connection. Ask an administrator to review access.";
  } else if (role.value === "unsure") {
    message += " Account plan alone does not prove workspace authorization.";
  }
  advice.textContent = message;
  advice.classList.toggle("warning", plan.value === "pro" || plan.value === "unknown" || role.value === "restricted");
}
function resetResult() {
  validatedScope = null;
  stateBadge.textContent = "NOT VALIDATED";
  stateBadge.classList.remove("ready");
  error.classList.add("hidden");
  results.classList.add("hidden");
}
function showError(message) {
  validatedScope = null;
  error.textContent = message;
  error.classList.remove("hidden");
  stateBadge.textContent = "CHECK REQUIRED";
  stateBadge.classList.remove("ready");
  results.classList.add("hidden");
}
for (const radio of document.querySelectorAll('input[name="mode"]')) {
  radio.addEventListener("change", updateMode);
}
paths.addEventListener("input", resetResult);
ack.addEventListener("change", resetResult);
plan.addEventListener("change", updatePlan);
role.addEventListener("change", updatePlan);
updateMode();
updatePlan();

pickDirectory.addEventListener("click", async () => {
  if (!sessionToken) {
    pickerStatus.textContent = "Setup session expired. Restart 'dirhaven setup'.";
    return;
  }
  pickDirectory.disabled = true;
  pickerStatus.textContent = "Waiting for the desktop folder chooser...";
  try {
    const response = await fetch("/api/pick-directory", {
      method: "POST",
      mode: "same-origin",
      cache: "no-store",
      headers: { "X-Dirhaven-Setup-Token": sessionToken }
    });
    if (!response.ok) {
      throw new Error(response.status === 409
        ? "A directory chooser is already open."
        : "Desktop chooser unavailable. Paste an absolute path instead.");
    }
    const data = await response.json();
    if (data.cancelled) {
      pickerStatus.textContent = "Selection cancelled. No access was granted.";
      return;
    }
    if (!data.ok || !data.path) {
      pickerStatus.textContent = data.error || "Could not select a folder.";
      return;
    }
    const existing = paths.value.split(/\r?\n/).map(item => item.trim()).filter(Boolean);
    if (!existing.includes(data.path)) existing.push(data.path);
    paths.value = existing.join("\n");
    resetResult();
    pickerStatus.textContent = "Folder added to the list. Validate to prepare access.";
  } catch (reason) {
    pickerStatus.textContent = reason.message || "Folder chooser could not be opened.";
  } finally {
    pickDirectory.disabled = false;
  }
});

generate.addEventListener("click", async () => {
  if (!sessionToken) {
    showError("This setup session has expired. Close this tab and start 'dirhaven setup' again.");
    return;
  }
  const mode = activeMode();
  const chosen = mode === "scoped"
    ? paths.value.split(/\r?\n/).map((item) => item.trim()).filter(Boolean)
    : [];
  if (mode === "scoped" && chosen.length === 0) {
    showError("Enter at least one existing absolute folder path.");
    return;
  }
  generate.disabled = true;
  stateBadge.textContent = "CHECKING LOCALLY...";
  error.classList.add("hidden");
  try {
    const response = await fetch("/api/validate", {
      method: "POST",
      mode: "same-origin",
      cache: "no-store",
      headers: {
        "Content-Type": "application/json",
        "X-Dirhaven-Setup-Token": sessionToken
      },
      body: JSON.stringify({
        mode,
        paths: chosen,
        acknowledgeBroadAccess: mode === "all" && ack.checked
      })
    });
    if (!response.ok) {
      throw new Error(response.status === 401
        ? "Setup authorization expired. Restart 'dirhaven setup'."
        : "The local validation request failed (" + response.status + ").");
    }
    const data = await response.json();
    if (!data.ok) {
      showError(data.error || "Your selected scope could not be validated.");
      return;
    }
    command.textContent = data.command;
    client.textContent = data.mcpClientJSON;
    validatedScope = { mode, paths: chosen, acknowledgeBroadAccess: mode === "all" && ack.checked };
    results.classList.remove("hidden");
    stateBadge.textContent = "LOCAL SCOPE READY";
    stateBadge.classList.add("ready");
    results.scrollIntoView({ behavior: "smooth", block: "nearest" });
  } catch (reason) {
    showError(reason.message || "Unable to communicate with the local setup process.");
  } finally {
    generate.disabled = false;
  }
});

async function refreshSavedPreferences() {
  if (!sessionToken) {
    savedStatus.textContent = "Saved preferences unavailable until setup is restarted.";
    return;
  }
  try {
    const response = await fetch("/api/saved", {
      method: "GET", mode: "same-origin", cache: "no-store",
      headers: { "X-Dirhaven-Setup-Token": sessionToken }
    });
    if (!response.ok) throw new Error("Saved configuration could not be read. Existing data was not changed.");
    const saved = await response.json();
    savedRevision = saved.revision || "";
    savedChoice = saved.exists ? { mode: saved.mode, roots: saved.roots || [] } : null;
    loadSaved.disabled = !saved.exists;
    revokeSaved.disabled = !saved.exists;
    savedStatus.textContent = saved.exists
      ? "Saved locally: " + (saved.mode === "all" ? "broad access" : saved.roots.length + " selected folder(s)") + ". Nothing is applied automatically."
      : "No saved choices yet. Saving is optional.";
  } catch (cause) {
    savedRevision = "";
    savedChoice = null;
    loadSaved.disabled = true;
    revokeSaved.disabled = true;
    savedStatus.textContent = cause.message || "Saved configuration unavailable.";
  }
}

loadSaved.addEventListener("click", () => {
  if (!savedChoice) return;
  document.querySelector('input[name="mode"][value="' + savedChoice.mode + '"]').checked = true;
  // Broad access is never reacknowledged based on a saved preference.
  ack.checked = false;
  paths.value = savedChoice.mode === "scoped" ? savedChoice.roots.join("\n") : "";
  updateMode();
  savedStatus.textContent = "Loaded into the form. Review and validate before using. Broad access needs fresh confirmation.";
});

saveScope.addEventListener("click", async () => {
  if (!validatedScope) return;
  saveScope.disabled = true;
  try {
    const response = await fetch("/api/save", {
      method: "POST", mode: "same-origin", cache: "no-store",
      headers: { "Content-Type": "application/json", "X-Dirhaven-Setup-Token": sessionToken },
      body: JSON.stringify({ ...validatedScope, expectedRevision: savedRevision })
    });
    const data = await response.json();
    if (!response.ok || !data.ok) throw new Error(data.error || "Could not save choices.");
    await refreshSavedPreferences();
    savedStatus.textContent = "Saved locally. This does not change an existing MCP process or tunnel.";
  } catch (cause) {
    savedStatus.textContent = cause.message || "Could not save choices.";
  } finally {
    saveScope.disabled = false;
  }
});

revokeSaved.addEventListener("click", async () => {
  if (!savedChoice || !savedRevision) return;
  if (!window.confirm("Forget saved Dirhaven access choices? This does not stop or change a running MCP server.")) return;
  revokeSaved.disabled = true;
  try {
    const response = await fetch("/api/revoke", {
      method: "POST", mode: "same-origin", cache: "no-store",
      headers: { "Content-Type": "application/json", "X-Dirhaven-Setup-Token": sessionToken },
      body: JSON.stringify({ expectedRevision: savedRevision })
    });
    const data = await response.json();
    if (!response.ok || !data.ok) throw new Error(data.error || "Could not forget choices.");
    await refreshSavedPreferences();
    savedStatus.textContent = "Saved choices forgotten. Running connections are unchanged.";
  } catch (cause) {
    savedStatus.textContent = cause.message || "Could not forget choices.";
    revokeSaved.disabled = false;
  }
});
void refreshSavedPreferences();

checkTunnel.addEventListener("click", async () => {
  if (!sessionToken) {
    tunnelStatus.textContent = "Setup authorization expired. Restart Dirhaven Setup.";
    return;
  }
  checkTunnel.disabled = true;
  tunnelStatus.textContent = "Checking only for the tunnel-client command on PATH...";
  try {
    const response = await fetch("/api/tunnel-diagnostic", {
      method: "GET", mode: "same-origin", cache: "no-store",
      headers: { "X-Dirhaven-Setup-Token": sessionToken }
    });
    if (!response.ok) throw new Error("Local discovery request failed.");
    const data = await response.json();
    tunnelStatus.textContent = (data.clientOnPath
      ? "Found " + (data.executableName || "tunnel-client") + " on PATH. "
      : "No tunnel-client executable found on PATH. ") +
      (data.nextStep || "") +
      " Connection status: NOT CHECKED.";
  } catch (cause) {
    tunnelStatus.textContent = cause.message || "Cannot check local executable availability.";
  } finally {
    checkTunnel.disabled = false;
  }
});

browseTunnel.addEventListener("click", async () => {
  if (!sessionToken) {
    chosenTunnelStatus.textContent = "Setup authorization expired. Restart Dirhaven Setup.";
    return;
  }
  browseTunnel.disabled = true;
  chosenTunnelStatus.textContent = "Waiting for the desktop file chooser...";
  try {
    const response = await fetch("/api/pick-tunnel-file", {
      method: "POST", mode: "same-origin", cache: "no-store",
      headers: { "X-Dirhaven-Setup-Token": sessionToken }
    });
    if (!response.ok) throw new Error(response.status === 409
      ? "Another file or folder chooser is already open."
      : "Desktop chooser unavailable. Paste the absolute path instead.");
    const data = await response.json();
    if (data.cancelled) {
      chosenTunnelStatus.textContent = "Selection cancelled. Nothing was inspected.";
    } else if (data.ok && data.path) {
      tunnelPath.value = data.path;
      chosenTunnelStatus.textContent = "File selected. Click Inspect selected file to check metadata. No executable was run.";
    } else {
      chosenTunnelStatus.textContent = data.error || "Could not choose a file.";
    }
  } catch (cause) {
    chosenTunnelStatus.textContent = cause.message || "File chooser unavailable.";
  } finally {
    browseTunnel.disabled = false;
  }
});

tunnelPath.addEventListener("input", () => {
  fingerprintStatus.textContent = "Fingerprint not calculated for this selection.";
});
fingerprintTunnel.addEventListener("click", async () => {
  if (!sessionToken || !tunnelPath.value.trim()) {
    fingerprintStatus.textContent = "Select an executable before calculating a fingerprint.";
    return;
  }
  fingerprintTunnel.disabled = true;
  fingerprintStatus.textContent = "Calculating SHA-256 of the explicitly chosen file locally...";
  try {
    const response = await fetch("/api/tunnel-fingerprint", {
      method: "POST", mode: "same-origin", cache: "no-store",
      headers: { "Content-Type": "application/json", "X-Dirhaven-Setup-Token": sessionToken },
      body: JSON.stringify({ path: tunnelPath.value.trim() })
    });
    if (!response.ok) throw new Error("Fingerprint request failed.");
    const data = await response.json();
    fingerprintStatus.textContent = data.ok
      ? "SHA-256: " + data.sha256 + ". Compare against a trusted publisher checksum. Authenticity: NOT VERIFIED."
      : (data.error || "Fingerprint unavailable.") + " Authenticity: NOT VERIFIED.";
  } catch (cause) {
    fingerprintStatus.textContent = cause.message || "Fingerprint unavailable.";
  } finally {
    fingerprintTunnel.disabled = false;
  }
});

checkChosenTunnel.addEventListener("click", async () => {
  const chosen = tunnelPath.value.trim();
  if (!sessionToken) {
    chosenTunnelStatus.textContent = "Setup authorization expired. Restart Dirhaven Setup.";
    return;
  }
  if (!chosen) {
    chosenTunnelStatus.textContent = "Enter an absolute path to a tunnel-client executable.";
    return;
  }
  checkChosenTunnel.disabled = true;
  chosenTunnelStatus.textContent = "Inspecting only the chosen file's metadata...";
  try {
    const response = await fetch("/api/tunnel-selected", {
      method: "POST", mode: "same-origin", cache: "no-store",
      headers: { "Content-Type": "application/json", "X-Dirhaven-Setup-Token": sessionToken },
      body: JSON.stringify({ path: chosen })
    });
    if (!response.ok) throw new Error("Selected file inspection request failed.");
    const data = await response.json();
    chosenTunnelStatus.textContent = (data.executableName
      ? "Candidate found: " + data.executableName + ". "
      : "Candidate not confirmed. ") +
      (data.nextStep || "") + " Connection status: NOT CHECKED.";
  } catch (cause) {
    chosenTunnelStatus.textContent = cause.message || "Cannot inspect the chosen file.";
  } finally {
    checkChosenTunnel.disabled = false;
  }
});

for (const button of document.querySelectorAll("[data-copy]")) {
  button.addEventListener("click", async () => {
    const source = button.dataset.copy === "client" ? client : command;
    try {
      await navigator.clipboard.writeText(source.textContent);
      const previous = button.textContent;
      button.textContent = "Copied ✓";
      button.blur();
      setTimeout(() => { button.textContent = previous; }, 1700);
    } catch {
      source.focus();
      button.textContent = "Select and copy manually";
    }
  });
}

/* Navigation follows the actual Work page's editorial section links. */
const chapterLinks = Array.from(document.querySelectorAll(".work-sections a"));
const chapters = chapterLinks.map((link) => document.querySelector(link.getAttribute("href"))).filter(Boolean);
function activateChapter(id) {
  for (const link of chapterLinks) {
    const active = link.getAttribute("href") === "#" + id;
    link.classList.toggle("active", active);
    if (active) {
      link.setAttribute("aria-current", "location");
    } else {
      link.removeAttribute("aria-current");
    }
  }
}
for (const link of chapterLinks) {
  link.addEventListener("click", () => activateChapter(link.hash.slice(1)));
}
if ("IntersectionObserver" in window) {
  const chapterObserver = new IntersectionObserver(
    (entries) => {
      const visible = entries.filter((entry) => entry.isIntersecting)
        .sort((left, right) => left.boundingClientRect.top - right.boundingClientRect.top);
      if (visible.length > 0) activateChapter(visible[0].target.id);
    },
    { rootMargin: "-10% 0px -60% 0px" }
  );
  for (const chapter of chapters) chapterObserver.observe(chapter);
}
