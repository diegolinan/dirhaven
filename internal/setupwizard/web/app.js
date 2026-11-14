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
  stateBadge.textContent = "NOT VALIDATED";
  stateBadge.classList.remove("ready");
  error.classList.add("hidden");
  results.classList.add("hidden");
}
function showError(message) {
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
