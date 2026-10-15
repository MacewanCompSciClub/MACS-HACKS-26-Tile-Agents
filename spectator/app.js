"use strict";

// ---------------------------------------------------------------------------
// Config: fill these in
// ---------------------------------------------------------------------------

const SERVERS = {
  local: { label: "Local", host: "localhost",    port: 8080, token: "FFFFFFFF" },
  vps:   { label: "VPS",   host: "YOUR_VPS_IP",  port: 8080, token: "FFFFFFFF" }, // TODO: real VPS values
};

// Path of the spectator websocket endpoint on the Go server.
const WS_PATH = "/ws";

// TODO: set these to match the Go MessageType constants.
// Messages are also recognised by shape (see detectType), so it works until then.
const MessageType = {
  SPECTATOR_ROUND_INFO:  -1,
  SPECTATOR_ROUND_DELTA: -2,
  WORLD_STATE:           2,
  SPECTATOR_LOG: 10,
};

// Tile byte layout: bits 0-2 are health (0-7), bit 3 is set when a player is on the tile.
const HEALTH_MASK = 0b0111;
const PLAYER_BIT  = 0b1000;
const MAX_HEALTH  = 7;      // full-health tile (white)
const MAX_LOG_LINES = 500;  // keeps the log from growing forever

// Latest data received from the server.
let worldSize  = null; // { x, y } from the round info message
let worldTick  = 0;
let worldCells = null; // Uint8Array, length world_size_x * world_size_y
let worldStateCount = 0;
let players    = [];   // [{ pos: [x, y], dead, score }]

const CRC32_TABLE = (() => {
  const table = new Uint32Array(256);
  for (let i = 0; i < 256; i++) {
    let c = i;
    for (let k = 0; k < 8; k++) {
      c = c & 1 ? (c >>> 1) ^ 0xedb88320 : c >>> 1;
    }
    table[i] = c >>> 0;
  }
  return table;
})();

// Equivalent of Go's crc32.ChecksumIEEE(data []byte) uint32
function crc32IEEE(bytes) {
  let crc = 0xffffffff;
  for (let i = 0; i < bytes.length; i++) {
    crc = CRC32_TABLE[(crc ^ bytes[i]) & 0xff] ^ (crc >>> 8);
  }
  return (crc ^ 0xffffffff) >>> 0;
}

// ---------------------------------------------------------------------------
// DOM
// ---------------------------------------------------------------------------

const $ = (id) => document.getElementById(id);

const startScreen = $("start-screen");
const logScreen   = $("log-screen");
const statusEl    = $("status");
const targetLabel = $("target-label");
const logEl       = $("log");
const canvasWrap  = $("canvas-wrap");
const canvas      = $("world-canvas");
const ctx         = canvas.getContext("2d");

function showScreen(name) {
  startScreen.classList.toggle("hidden", name !== "start");
  logScreen.classList.toggle("hidden", name !== "log");
}

// ---------------------------------------------------------------------------
// Logging
// ---------------------------------------------------------------------------

function log(msg, level = "info") {
  const line = document.createElement("div");
  line.className = level;

  const time = document.createElement("span");
  time.className = "time";
  time.textContent = new Date().toLocaleTimeString() + "  ";

  line.appendChild(time);
  line.appendChild(document.createTextNode(msg));
  logEl.appendChild(line);

  while (logEl.childElementCount > MAX_LOG_LINES) {
    logEl.removeChild(logEl.firstChild);
  }
  logEl.scrollTop = logEl.scrollHeight;

  console.log(`[${level}]`, msg);
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

// One pixel per tile is drawn here, then scaled up onto the visible canvas.
const tileCanvas = document.createElement("canvas");
const tileCtx = tileCanvas.getContext("2d");

let renderQueued = false;

// Draw at most once per animation frame, no matter how many messages arrive.
function requestRender() {
  if (renderQueued) return;
  renderQueued = true;
  requestAnimationFrame(render);
}

// Health 7 (max) is white, 0 is black, with grey in between.
function tileShade(value) {
  const health = value & HEALTH_MASK;
  return Math.round((health / MAX_HEALTH) * 255);
}

function render() {
  renderQueued = false;
  if (!worldCells || !worldSize) return;

  const w = worldSize.x;
  const h = worldSize.y;
  if (worldCells.length !== w * h) return; // size mismatch is already logged

  // Tiles -> one pixel each.
  if (tileCanvas.width !== w || tileCanvas.height !== h) {
    tileCanvas.width = w;
    tileCanvas.height = h;
  }
  const image = tileCtx.createImageData(w, h);
  const px = image.data;
  for (let i = 0; i < worldCells.length; i++) {
    const shade = tileShade(worldCells[i]);
    const o = i * 4;
    px[o] = shade;
    px[o + 1] = shade;
    px[o + 2] = shade;
    px[o + 3] = 255;
  }
  tileCtx.putImageData(image, 0, 0);

  // Whole-number scale so tiles stay crisp.
  const scale = Math.max(1, Math.floor(Math.min(canvasWrap.clientWidth / w, canvasWrap.clientHeight / h)));
  if (canvas.width !== w * scale || canvas.height !== h * scale) {
    canvas.width = w * scale;
    canvas.height = h * scale;
  }
  ctx.imageSmoothingEnabled = false;
  ctx.drawImage(tileCanvas, 0, 0, w * scale, h * scale);

  // Red circle on every tile that has the player bit set.
  ctx.fillStyle = "#ff2d2d";
  const radius = Math.max(2, scale * 0.4);
  for (let i = 0; i < worldCells.length; i++) {
    if (!(worldCells[i] & PLAYER_BIT)) continue;
    const x = i % w;
    const y = Math.floor(i / w);
    ctx.beginPath();
    ctx.arc((x + 0.5) * scale, (y + 0.5) * scale, radius, 0, Math.PI * 2);
    ctx.fill();
  }
}

window.addEventListener("resize", requestRender);

// ---------------------------------------------------------------------------
// WebSocket
// ---------------------------------------------------------------------------

let socket = null;

function setStatus(state, text) {
  statusEl.className = "status " + state;
  statusEl.textContent = text;
}

function connectTo(key) {
  const server = SERVERS[key];
  const url = `ws://${server.host}:${server.port}${WS_PATH}?token=${encodeURIComponent(server.token)}`;

  logEl.textContent = "";
  targetLabel.textContent = `${server.label} (${server.host}:${server.port})`;
  showScreen("log");

  log(`Connecting to ${url}`);
  setStatus("connecting", "Connecting...");

  worldSize = null;
  worldCells = null;
  worldTick = 0;
  worldStateCount = 0;
  deltaCount = 0;
  players = [];
  ctx.clearRect(0, 0, canvas.width, canvas.height);

  const ws = new WebSocket(url);
  ws.binaryType = "arraybuffer"; // binary frames arrive as ArrayBuffer, not Blob
  socket = ws;

  ws.addEventListener("open", () => {
    log("Connected.", "ok");
    setStatus("connected", "Connected");
  });

  ws.addEventListener("message", (event) => handleRaw(event.data));

  ws.addEventListener("error", () => {
    log("Websocket error (is the server running and the token valid?).", "error");
  });

  ws.addEventListener("close", (event) => {
    log(`Connection closed (code ${event.code}${event.reason ? ", " + event.reason : ""}).`, "warn");
    setStatus("disconnected", "Disconnected");
    if (socket === ws) socket = null;
  });
}

function disconnectAndGoBack() {
  if (socket) {
    socket.close();
    socket = null;
  }
  showScreen("start");
}

$("connect-local-btn").addEventListener("click", () => connectTo("local"));
$("connect-vps-btn").addEventListener("click", () => connectTo("vps"));
$("back-btn").addEventListener("click", disconnectAndGoBack);

// ---------------------------------------------------------------------------
// Message handling
// ---------------------------------------------------------------------------

function detectType(msg) {
  if (msg.message_type === MessageType.SPECTATOR_ROUND_INFO)  return "round_info";
  if (msg.message_type === MessageType.SPECTATOR_ROUND_DELTA) return "round_delta";
  if (msg.message_type === MessageType.SPECTATOR_LOG)		  return "spectator_log";

  if ("world_size_x" in msg) return "round_info";
  if ("delta_updates" in msg) return "round_delta";
  return "unknown";
}

function handleRaw(data) {
  // Binary frames are world state messages; text frames are JSON.
  if (data instanceof ArrayBuffer) {
    handleBinary(data);
    return;
  }

  let msg;
  try {
    msg = JSON.parse(data);
  } catch (err) {
    log(`Received non-JSON message: ${String(data).slice(0, 200)}`, "warn");
    return;
  }

  switch (detectType(msg)) {
    case "round_info":
      handleRoundInfo(msg);
      break;
    case "round_delta":
      handleRoundDelta(msg);
      break;
    case "spectator_log":
      handleSpectatorLog(msg);
      break;
    default:
      log(`Unknown message type ${msg.message_type}`, "warn");
  }
}

// Binary layout: [type: u8][tick: u32 little endian][cells: u8 * (w*h)]
function handleBinary(buffer) {
  if (buffer.byteLength < 5) {
    log(`Binary message too short (${buffer.byteLength} bytes).`, "warn");
    return;
  }

  const view = new DataView(buffer);
  const type = view.getUint8(0);

  if (MessageType.WORLD_STATE !== null && type !== MessageType.WORLD_STATE) {
    log(`Unknown binary message type ${type} (${buffer.byteLength} bytes).`, "warn");
    return;
  }

  worldTick  = view.getUint32(1, true); // true = little endian
  worldCells = new Uint8Array(buffer, 5);
  worldStateCount++;

  if (worldStateCount === 1) {
    log(`World state: type byte ${type}, tick ${worldTick}, ${worldCells.length} cells. Further world states are not logged individually.`, "ok");

    if (worldSize) {
      const expected = worldSize.x * worldSize.y;
      if (worldCells.length !== expected) {
        log(`World state size mismatch: got ${worldCells.length} cells, expected ${expected} (${worldSize.x} x ${worldSize.y}).`, "error");
      }
    } else {
      log("World state arrived before round info, so its size can't be checked yet.", "warn");
    }
  }

  requestRender();
}

function handleRoundInfo(msg) {
  worldSize = { x: msg.world_size_x, y: msg.world_size_y };
  players = msg.player_info || [];
  log(`Round info: world size ${msg.world_size_x} x ${msg.world_size_y}, ${msg.num_players} players`, "ok");
  requestRender();
}

let deltaCount = 0;
function handleRoundDelta(msg) {
  deltaCount++;
  const updates = msg.delta_updates || [];
  //log(`Received delta for current tick ${msg.current_tick}.  Delta count: ${updates.length}`);

  if (msg.player_info) players = msg.player_info;

  // Deltas can't be applied until a world state and the world size are known.
  if (!worldCells || !worldSize) return;

  for (const d of updates) {
    const [x, y] = d.pos;                // Vec2 marshals as [x, y]

    if (x < 0 || y < 0 || x >= worldSize.x || y >= worldSize.y) {
      log(`Delta out of bounds: (${x}, ${y})`, "warn");
      continue;
    }

    const index = y * worldSize.x + x;   // row-major; swap if the server uses x * height + y
    worldCells[index] = d.tile_value;
  }
  worldTick = msg.current_tick;

  const hash = crc32IEEE(worldCells);
  if (msg.world_hash !== hash) {
    log(`World hashes do not match expected: ${msg.world_hash} got: ${hash}`, "error");
  }

  requestRender();
}

function handleSpectatorLog(msg) {
  log(msg.text, "ok");
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

showScreen("start");
setStatus("disconnected", "Disconnected");
