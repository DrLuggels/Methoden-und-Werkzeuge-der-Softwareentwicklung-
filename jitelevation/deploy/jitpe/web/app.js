"use strict";

async function api(method, path, body) {
  const opts = { method, headers: {} };
  if (body !== undefined) {
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(path, opts);
  let data = {};
  try { data = await res.json(); } catch (_) {}
  return { ok: res.ok, status: res.status, data };
}

const $ = (id) => document.getElementById(id);
let pendingGrantId = null;

function toast(msg, kind = "info") {
  const t = $("toast");
  t.textContent = msg;
  t.className = kind;
  setTimeout(() => { t.className = "hidden"; }, 4000);
}

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

$("setup").onclick = async () => {
  const { ok, data } = await api("GET", "/api/totp/setup");
  if (!ok) { toast(data.error || "Fehler", "err"); return; }
  $("totp-setup").classList.remove("hidden");
  $("secret").textContent = data.secret;
  const box = $("qrcode");
  box.innerHTML = "";
  if (window.QRCode) new QRCode(box, { text: data.otpauth, width: 160, height: 160 });
};

$("request").onclick = async () => {
  const body = {
    scope: $("scope").value,
    role: $("role").value,
    reason: $("reason").value,
    duration_minutes: parseInt($("duration").value, 10) || 0,
  };
  const { ok, data } = await api("POST", "/api/elevate/request", body);
  if (ok) {
    pendingGrantId = data.grant_id;
    $("pending-id").textContent = data.grant_id.slice(0, 12) + "…";
    toast("Antrag für " + body.scope + " angelegt – jetzt mit TOTP bestätigen", "ok");
    loadStatus();
  } else {
    toast(data.error || "Antrag abgelehnt", "err");
  }
};

$("confirm").onclick = async () => {
  if (!pendingGrantId) { toast("Erst einen Antrag stellen", "err"); return; }
  const { ok, data } = await api("POST", "/api/elevate/confirm", {
    grant_id: pendingGrantId, code: $("code").value.trim(),
  });
  if (ok) {
    toast(data.scope + ": Erhöhung aktiv bis " + new Date(data.expires_at).toLocaleTimeString(), "ok");
    pendingGrantId = null;
    $("pending-id").textContent = "–";
    $("code").value = "";
    loadStatus();
  } else {
    toast(data.error || "Bestätigung fehlgeschlagen", "err");
  }
};

async function loadStatus() {
  const { ok, data } = await api("GET", "/api/elevate/status");
  if (!ok) return;
  const tbody = $("grants").querySelector("tbody");
  tbody.innerHTML = "";
  for (const g of data) {
    const tr = document.createElement("tr");
    const rest = g.state === "active" ? g.seconds_left + " s" : "–";
    const selected = g.id === pendingGrantId ? " ✓" : "";
    tr.innerHTML =
      `<td>${g.scope}</td><td>${g.role || "–"}</td>` +
      `<td><span class="badge ${g.state}">${g.state}${selected}</span></td><td>${rest}</td><td></td>`;
    const actions = tr.lastElementChild;

    if (g.state === "pending") {
      // Diesen Antrag zum Bestätigen auswählen.
      const pick = document.createElement("button");
      pick.textContent = "Auswählen";
      pick.className = "small";
      pick.onclick = () => {
        pendingGrantId = g.id;
        $("pending-id").textContent = g.id.slice(0, 12) + "…";
        $("code").focus();
        toast("Antrag ausgewählt – jetzt TOTP-Code bestätigen", "info");
        loadStatus();
      };
      actions.appendChild(pick);

      // Antrag ablehnen/abbrechen.
      const deny = document.createElement("button");
      deny.textContent = "Ablehnen";
      deny.className = "small danger";
      deny.onclick = async () => {
        const { ok, data } = await api("POST", "/api/elevate/cancel", { grant_id: g.id });
        if (ok) {
          if (g.id === pendingGrantId) { pendingGrantId = null; $("pending-id").textContent = "–"; }
          toast("Antrag abgelehnt", "ok");
        } else { toast(data.error || "Fehler", "err"); }
        loadStatus();
      };
      actions.appendChild(deny);
    }

    if (g.state === "active") {
      const btn = document.createElement("button");
      btn.textContent = "Widerrufen";
      btn.className = "small danger";
      btn.onclick = async () => {
        await api("POST", "/api/elevate/revoke", { grant_id: g.id });
        toast("Widerrufen", "ok");
        loadStatus();
      };
      actions.appendChild(btn);
    }
    tbody.appendChild(tr);
  }
}

$("refresh").onclick = loadStatus;
setInterval(() => { if (!$("app").classList.contains("hidden")) loadStatus(); }, 5000);
refreshSession();
