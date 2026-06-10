"use strict";

// Wird beim Container-Start durch den echten Schluessel ersetzt (entrypoint).
const API_KEY = "REPLACE_ME";

const canvas = document.getElementById("pad");
const ctx = canvas.getContext("2d");

function reset() {
  ctx.fillStyle = "black";
  ctx.fillRect(0, 0, canvas.width, canvas.height);
}
ctx.strokeStyle = "white";
ctx.lineWidth = 18;
ctx.lineCap = "round";
ctx.lineJoin = "round";
reset();

let drawing = false;
function pos(e) {
  const r = canvas.getBoundingClientRect();
  const t = e.touches ? e.touches[0] : e;
  return { x: (t.clientX - r.left) * (canvas.width / r.width),
           y: (t.clientY - r.top) * (canvas.height / r.height) };
}
function start(e) { drawing = true; const p = pos(e); ctx.beginPath(); ctx.moveTo(p.x, p.y); e.preventDefault(); }
function move(e) { if (!drawing) return; const p = pos(e); ctx.lineTo(p.x, p.y); ctx.stroke(); e.preventDefault(); }
function end() { drawing = false; }
canvas.addEventListener("mousedown", start);
canvas.addEventListener("mousemove", move);
window.addEventListener("mouseup", end);
canvas.addEventListener("touchstart", start, { passive: false });
canvas.addEventListener("touchmove", move, { passive: false });
canvas.addEventListener("touchend", end);

// 280x280-Zeichnung auf 28x28 herunterrechnen; weisse Striche auf schwarz
// entsprechen direkt dem MNIST-Format (heller Vordergrund auf dunklem Grund).
function getPixels() {
  const tmp = document.createElement("canvas");
  tmp.width = 28; tmp.height = 28;
  const tctx = tmp.getContext("2d");
  tctx.drawImage(canvas, 0, 0, 28, 28);
  const data = tctx.getImageData(0, 0, 28, 28).data;
  const px = [];
  for (let y = 0; y < 28; y++) {
    const row = [];
    for (let x = 0; x < 28; x++) row.push(data[(y * 28 + x) * 4]); // R-Kanal
    px.push(row);
  }
  return px;
}

document.getElementById("clear").onclick = () => {
  reset();
  document.getElementById("result").textContent = "";
};

document.getElementById("classify").onclick = async () => {
  const res = await fetch("/api/classify", {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-API-Key": API_KEY },
    body: JSON.stringify({ pixels: getPixels() }),
  });
  const out = document.getElementById("result");
  if (!res.ok) {
    out.textContent = "Fehler: HTTP " + res.status;
    return;
  }
  const d = await res.json();
  out.innerHTML = `Erkannt: <span class="digit">${d.prediction}</span> ` +
    `(${(d.confidence * 100).toFixed(1)} %)`;
  loadHistory();
};

async function loadHistory() {
  const res = await fetch("/api/results");
  if (!res.ok) return;
  const d = await res.json();
  const ul = document.getElementById("history");
  ul.innerHTML = "";
  for (const r of d.results) {
    const li = document.createElement("li");
    li.innerHTML = `<span>Ziffer ${r.prediction}</span>` +
      `<span class="conf">${(r.confidence * 100).toFixed(0)} %</span>`;
    ul.appendChild(li);
  }
}

document.getElementById("refresh").onclick = loadHistory;
loadHistory();
