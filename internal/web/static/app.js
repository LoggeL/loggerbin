import { encryptText, decryptText } from "./crypto.js";

const $ = (id) => document.getElementById(id);
let settings;
let currentText = "";
let expiryTimer;
let expiresAt;
const status = (text, error = false) => {
  $("status").textContent = text;
  $("status").classList.toggle("error", error);
};

async function api(path, options = {}) {
  const response = await fetch(path, {
    credentials: "same-origin",
    cache: "no-store",
    referrerPolicy: "no-referrer",
    ...options,
  });
  let data;
  try {
    data = await response.json();
  } catch {
    throw new Error("The server returned an unexpected response.");
  }
  if (!response.ok) throw new Error(data.message || "The request failed.");
  return data;
}

function erase() {
  currentText = "";
  $("content").value = "";
  $("paste-text").textContent = "";
  $("share-link").value = "";
  clearTimeout(expiryTimer);
}

function checkExpiry() {
  if (!expiresAt) return;
  const remaining = expiresAt - Date.now();
  if (remaining <= 0) {
    erase();
    $("viewer").hidden = true;
    $("sharing").hidden = true;
    $("heading").textContent = "This paste has expired.";
    $("description").textContent =
      "Create a new paste to share something else.";
    status("This paste has expired.", true);
  } else {
    clearTimeout(expiryTimer);
    expiryTimer = setTimeout(checkExpiry, Math.min(remaining, 2147483000));
  }
}

function show(text, result, link) {
  const date = Date.parse(result.expires_at);
  if (!Number.isFinite(date) || date <= Date.now())
    throw new Error("This paste has expired.");
  currentText = text;
  expiresAt = date;
  $("content").value = "";
  $("composer").hidden = true;
  $("viewer").hidden = false;
  $("sharing").hidden = false;
  $("paste-text").textContent = text;
  $("share-link").value = link;
  $("expires").textContent = "Expires " + new Date(date).toLocaleString();
  $("mode").textContent = "Encrypted paste";
  $("byte-count").textContent =
    new TextEncoder().encode(text).length.toLocaleString() + " bytes";
  $("heading").textContent = "A private paste, ready to share.";
  $("description").textContent =
    "Decrypted in this browser. Keep the complete link private.";
  checkExpiry();
}

async function copy(text, what) {
  try {
    await navigator.clipboard.writeText(text);
    status(what + " copied to clipboard.");
  } catch {
    status(
      "Clipboard access is unavailable. Select the text and copy it manually.",
      true,
    );
  }
}

$("content").addEventListener("input", () => {
  const bytes = new TextEncoder().encode($("content").value).length;
  $("byte-count").textContent =
    bytes.toLocaleString() +
    " / " +
    (settings?.max_paste_bytes ?? 65536).toLocaleString() +
    " bytes";
});
$("composer").addEventListener("submit", async (event) => {
  event.preventDefault();
  if (!settings || $("create").disabled) return;
  $("create").disabled = true;
  status("Encrypting your paste...");
  try {
    const submittedText = $("content").value;
    const encrypted = await encryptText(
      submittedText,
      settings.max_paste_bytes,
    );
    const result = await api("/api/pastes", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        ...encrypted.envelope,
        expiration: Number($("expiration").value),
      }),
    });
    if (!/^[A-Za-z0-9_-]{22}$/.test(result.id))
      throw new Error("The server returned an invalid paste ID.");
    const link = location.origin + "/p/" + result.id + "#key=" + encrypted.key;
    show(submittedText, result, link);
    // The fragment stays in browser history; it is never part of an HTTP request.
    history.replaceState(null, "", link);
    status("Your paste is ready. Copy the private link to share it.");
  } catch (error) {
    status(error.message, true);
  } finally {
    $("create").disabled = false;
  }
});
$("copy-link").addEventListener("click", () =>
  copy($("share-link").value, "Private link"),
);
$("copy-content").addEventListener("click", () =>
  copy(currentText, "Paste text"),
);
$("download").addEventListener("click", () => {
  const href = URL.createObjectURL(
    new Blob([currentText], { type: "text/plain;charset=utf-8" }),
  );
  const anchor = document.createElement("a");
  anchor.href = href;
  anchor.download = "loggerbin-paste.txt";
  anchor.click();
  setTimeout(() => URL.revokeObjectURL(href), 1000);
});
window.addEventListener("pagehide", erase);
window.addEventListener("hashchange", () => {
  erase();
  location.reload();
});
window.addEventListener("pageshow", (event) => {
  if (event.persisted) location.reload();
});
document.addEventListener("visibilitychange", () => {
  if (!document.hidden) checkExpiry();
});

async function init() {
  // Remove only Vaultbin's legacy plaintext snapshots on a reused dedicated origin.
  try {
    localStorage.removeItem("htmx-history-cache");
  } catch {}
  if (!window.isSecureContext || !crypto.subtle) {
    status(
      "Open Loggerbin over HTTPS to use browser encryption. Localhost is supported for development.",
      true,
    );
    return;
  }
  try {
    settings = await api("/api/config");
    $("version").textContent = settings.version;
    const choices = [
      [600, "10 minutes"],
      [3600, "1 hour"],
      [86400, "1 day"],
      [604800, "7 days"],
      [2592000, "30 days"],
    ].filter(([seconds]) => seconds <= settings.max_ttl);
    if (!choices.some(([seconds]) => seconds === settings.default_ttl))
      choices.push([settings.default_ttl, settings.default_ttl + " seconds"]);
    $("expiration").replaceChildren(
      ...choices.map(([seconds, label]) => {
        const option = document.createElement("option");
        option.value = seconds;
        option.textContent = label;
        return option;
      }),
    );
    $("expiration").value = settings.default_ttl;
    const match = location.pathname.match(/^\/p\/([A-Za-z0-9_-]{22})$/);
    if (match) {
      $("composer").hidden = true;
      $("heading").textContent = "Opening your private paste...";
      const fragment = new URLSearchParams(location.hash.slice(1));
      const key = fragment.get("key");
      if (
        !key ||
        [...fragment.keys()].length !== 1 ||
        !/^[A-Za-z0-9_-]{43}$/.test(key)
      )
        throw new Error(
          "This link is missing a valid decryption key. Ask the sender for the complete link.",
        );
      const paste = await api("/api/pastes/" + match[1]);
      const text = await decryptText(paste, key, settings.max_paste_bytes);
      show(text, paste, location.href);
      status("");
    } else {
      $("create").disabled = false;
      $("expiration").disabled = false;
    }
  } catch (error) {
    erase();
    if (location.pathname.startsWith("/p/")) {
      $("heading").textContent = "This paste is unavailable.";
      $("description").textContent =
        "Check the complete link, or create a new paste.";
    }
    status(error.message, true);
  }
}
await init();
