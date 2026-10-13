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
};

// ---------------------------------------------------------------------------
// DOM
// ---------------------------------------------------------------------------

const $ = (id) => document.getElementById(id);

const startScreen = $("start-screen");
const logScreen   = $("log-screen");
const statusEl    = $("status");
const targetLabel = $("target-label");
const logEl       = $("log");

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
  logEl.scrollTop = logEl.scrollHeight;

  console.log(`[${level}]`, msg);
}

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

  const ws = new WebSocket(url);
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

  if ("world_size_x" in msg) return "round_info";
  if ("delta_updates" in msg) return "round_delta";
  return "unknown";
}

function handleRaw(data) {
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
    default:
      log(`Unknown message type ${msg.message_type}`, "warn");
  }
}

function handleRoundInfo(msg) {
  log(`Round info: world size ${msg.world_size_x} x ${msg.world_size_y}, ${msg.num_players} players`, "ok");
}

let deltaCount = 0;
function handleRoundDelta(msg) {
  // Not rendered yet. Only note once that data is flowing.
  deltaCount++;
  if (deltaCount === 1) log("Receiving delta messages (not logged individually).");
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

showScreen("start");
setStatus("disconnected", "Disconnected");
