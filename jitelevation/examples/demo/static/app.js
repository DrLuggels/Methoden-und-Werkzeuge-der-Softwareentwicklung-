"use strict";

// Kleiner Helfer: JSON-fetch mit gemeinsamem Fehler-Handling.
async function api(method, path, body) {
  const opts = { method, headers: {} };
  if (body !== undefined) {
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(path, opts);
  let data = {};
  try { data = await res.json(); } catch (_) { /* leere Antwort */ }
  return { ok: res.ok, status: res.status, data };
}

function toast(msg, kind = "info") {
  const t = document.getElementById("toast");
  t.textContent = msg;
  t.className = kind;
  setTimeout(() => { t.className = "hidden"; }, 4000);
}

const $ = (id) => document.getElementById(id);
let pendingGrantId = null;

// --- Sitzungszustand --------------------------------------------------------

async function refreshSession() {
  const { ok, data } = await api("GET", "/api/me");
  if (ok) {
    $("whoami").textContent = "angemeldet als " + data.user;
    $("logout").classList.remove("hidden");
    $("login-card").classList.add("hidden");
    $("app").classList.remove("hidden");
    loadStatus();
  } else {
    $("whoami").textContent = "nicht angemeldet";
    $("logout").classList.add("hidden");
    $("login-card").classList.remove("hidden");
    $("app").classList.add("hidden");
  }
}

$("login").onclick = async () => {
  const { ok, data } = await api("POST", "/api/login", { username: $("username").value.trim() });
  if (ok) { toast("Angemeldet als " + data.user, "ok"); refreshSession(); }
  else { toast(data.error || "Anmeldung fehlgeschlagen", "err"); }
};

$("logout").onclick = async () => {
  await api("POST", "/api/logout");
  pendingGrantId = null;
  refreshSession();
};

// --- TOTP-Einrichtung -------------------------------------------------------

$("setup").onclick = async () => {
  const { ok, data } = await api("GET", "/api/totp/setup");
  if (!ok) { toast(data.error || "Fehler", "err"); return; }
  $("totp-setup").classList.remove("hidden");
  $("secret").textContent = data.secret;
  $("otpauth").textContent = data.otpauth;
  const box = $("qrcode");
  box.innerHTML = "";
  if (window.QRCode) {
    new QRCode(box, { text: data.otpauth, width: 180, height: 180 });
  } else {
    box.textContent = "(QR-Bibliothek nicht geladen – Secret manuell eingeben)";
  }
};

// --- Antrag stellen ---------------------------------------------------------

$("request").onclick = async () => {
  const body = {
    role: $("role").value,
    reason: $("reason").value,
    duration_minutes: parseInt($("duration").value, 10) || 0,
  };
  const { ok, data } = await api("POST", "/api/elevate/request", body);
  if (ok) {
    pendingGrantId = data.grant_id;
    $("pending-id").textContent = data.grant_id.slice(0, 12) + "…";
    toast("Antrag angelegt – jetzt mit TOTP bestätigen", "ok");
    loadStatus();
  } else {
    toast(data.error || "Antrag abgelehnt", "err");
  }
};

// --- Bestätigen (Step-Up) ---------------------------------------------------

$("confirm").onclick = async () => {
  if (!pendingGrantId) { toast("Erst einen Antrag stellen", "err"); return; }
  const { ok, data } = await api("POST", "/api/elevate/confirm", {
    grant_id: pendingGrantId, code: $("code").value.trim(),
  });
  if (ok) {
    toast("Erhöhung aktiv bis " + new Date(data.expires_at).toLocaleTimeString(), "ok");
    pendingGrantId = null;
    $("pending-id").textContent = "–";
    $("code").value = "";
    loadStatus();
  } else {
    toast(data.error || "Bestätigung fehlgeschlagen", "err");
  }
};

// --- Geschützte Aktion ------------------------------------------------------

$("admin").onclick = async () => {
  const { ok, status, data } = await api("GET", "/api/admin/data");
  const out = $("admin-result");
  out.classList.remove("hidden");
  if (ok) {
    out.textContent = "200 OK\n" + JSON.stringify(data, null, 2);
    out.className = "ok";
  } else {
    out.textContent = status + " – " + (data.error || "verweigert");
    out.className = "err";
  }
};

// --- Statusliste ------------------------------------------------------------

async function loadStatus() {
  const { ok, data } = await api("GET", "/api/elevate/status");
  if (!ok) return;
  const tbody = $("grants").querySelector("tbody");
  tbody.innerHTML = "";
  for (const g of data) {
    const tr = document.createElement("tr");
    const rest = g.state === "active" ? g.seconds_left + " s" : "–";
    tr.innerHTML =
      `<td>${g.role || "–"}</td><td><span class="badge ${g.state}">${g.state}</span></td>` +
      `<td>${rest}</td><td>${g.reason || ""}</td><td></td>`;
    if (g.state === "active") {
      const btn = document.createElement("button");
      btn.textContent = "Widerrufen";
      btn.className = "small";
      btn.onclick = async () => {
        await api("POST", "/api/elevate/revoke", { grant_id: g.id });
        toast("Widerrufen", "ok");
        loadStatus();
      };
      tr.lastElementChild.appendChild(btn);
    }
    tbody.appendChild(tr);
  }
}

$("refresh").onclick = loadStatus;

// Statusliste regelmäßig aktualisieren, damit die Restzeit herunterzählt.
setInterval(() => { if (!$("app").classList.contains("hidden")) loadStatus(); }, 5000);

refreshSession();
