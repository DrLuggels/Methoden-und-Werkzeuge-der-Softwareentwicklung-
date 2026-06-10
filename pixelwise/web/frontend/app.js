// frontend/app.js
const API_KEY = "REPLACE_ME";  // overwritten on deploy

const N = 28;       // the model's input grid
const SCALE = 10;   // on-screen pixels per grid cell (280 / 28)

const pad = document.getElementById("pad");
const view = pad.getContext("2d");
view.imageSmoothingEnabled = false;  // draw crisp blocks

// The real drawing happens on a hidden 28x28 grid. The
// visible canvas is that grid magnified ten times, so
// the user paints at the model's own resolution.
const grid = document.createElement("canvas");
grid.width = N; grid.height = N;
const gctx = grid.getContext("2d");
gctx.lineWidth = 2.5;
gctx.lineCap = "round"; gctx.lineJoin = "round";

let drawing = false;

function render() {
    // Magnify the grid onto the canvas, smoothing off.
    view.drawImage(grid, 0, 0, pad.width, pad.height);
}

function clearPad() {
    gctx.fillStyle = "#fff";
    gctx.fillRect(0, 0, N, N);
    render();
}
clearPad();

// Mouse positions map onto the 28x28 grid via SCALE.
pad.onmousedown = e => {
    drawing = true; gctx.beginPath();
    gctx.moveTo(e.offsetX / SCALE, e.offsetY / SCALE);
};
pad.onmousemove = e => {
    if (!drawing) return;
    gctx.lineTo(e.offsetX / SCALE, e.offsetY / SCALE);
    gctx.stroke(); render();
};
pad.onmouseup = pad.onmouseleave = () => { drawing = false; };

function getPixels() {
    // The grid is already 28x28: read it and invert.
    const data = gctx.getImageData(0, 0, N, N).data;
    const pixels = [];
    for (let y = 0; y < N; y++) {
        const row = [];
        for (let x = 0; x < N; x++)
            row.push(255 - data[(y * N + x) * 4]);
        pixels.push(row);
    }
    return pixels;
}

async function classify() {
    const r = await fetch("/api/classify", {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
            "X-API-Key": API_KEY
        },
        body: JSON.stringify({ pixels: getPixels() })
    });
    const out = document.getElementById("result");
    if (!r.ok) { out.textContent = "Error " + r.status; return; }
    const d = await r.json();
    out.textContent = `Prediction: ${d.prediction} ` +
        `(${(d.confidence * 100).toFixed(1)}%)`;
    refresh();
}

async function refresh() {
    const r = await fetch("/api/results");
    if (!r.ok) return;
    const ul = document.getElementById("history");
    ul.innerHTML = "";
    for (const row of (await r.json()).results) {
        const li = document.createElement("li");
        li.textContent = `${row.prediction}  ` +
            `${row.confidence.toFixed(2)}  ${row.created_at}`;
        ul.appendChild(li);
    }
}

document.getElementById("classify").onclick = classify;
document.getElementById("clear").onclick = () => {
    clearPad();
    document.getElementById("result").textContent = "";
};
refresh();
