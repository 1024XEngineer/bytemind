const state = {
  config: null,
  sessions: [],
  currentSession: null,
  messages: [],
  events: [],
  plan: {},
  mode: "build",
  activeRun: null,
  streamingIndex: -1,
  approval: null,
  eventSource: null,
  status: "Ready.",
};

const els = {};

document.addEventListener("DOMContentLoaded", () => {
  bindElements();
  bindActions();
  void boot();
});

window.addEventListener("unhandledrejection", (event) => {
  const reason = event.reason instanceof Error ? event.reason.message : String(event.reason || "request failed");
  setStatus(reason, "danger");
  addSystemMessage(`Error: ${reason}`);
  event.preventDefault();
});

function bindElements() {
  els.workspace = document.getElementById("workspace");
  els.model = document.getElementById("model");
  els.approvalPolicy = document.getElementById("approvalPolicy");
  els.newSessionBtn = document.getElementById("newSessionBtn");
  els.refreshSessionsBtn = document.getElementById("refreshSessionsBtn");
  els.sessionsList = document.getElementById("sessionsList");
  els.statusLine = document.getElementById("chatStatus");
  els.messages = document.getElementById("messages");
  els.promptInput = document.getElementById("promptInput");
  els.sendBtn = document.getElementById("sendBtn");
  els.btwBtn = document.getElementById("btwBtn");
  els.stopBtn = document.getElementById("stopBtn");
  els.modeBuildBtn = document.getElementById("modeBuildBtn");
  els.modePlanBtn = document.getElementById("modePlanBtn");
  els.runMeta = document.getElementById("runMeta");
  els.planView = document.getElementById("planView");
  els.events = document.getElementById("events");
  els.approvalModal = document.getElementById("approvalModal");
  els.approvalReason = document.getElementById("approvalReason");
  els.approvalCommand = document.getElementById("approvalCommand");
  els.approveBtn = document.getElementById("approveBtn");
  els.rejectBtn = document.getElementById("rejectBtn");
}

function bindActions() {
  els.newSessionBtn.addEventListener("click", () => {
    void createSession();
  });
  els.refreshSessionsBtn.addEventListener("click", () => {
    void loadSessions();
  });
  els.sendBtn.addEventListener("click", () => {
    void submitInput();
  });
  els.btwBtn.addEventListener("click", () => {
    void submitBTWFromInput();
  });
  els.stopBtn.addEventListener("click", () => {
    void cancelActiveRun();
  });
  els.modeBuildBtn.addEventListener("click", () => {
    void setMode("build");
  });
  els.modePlanBtn.addEventListener("click", () => {
    void setMode("plan");
  });
  els.approveBtn.addEventListener("click", () => {
    void resolveApproval(true);
  });
  els.rejectBtn.addEventListener("click", () => {
    void resolveApproval(false);
  });
  els.promptInput.addEventListener("keydown", (event) => {
    if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
      event.preventDefault();
      void submitInput();
    }
  });
}

async function boot() {
  setStatus("Loading workspace context...");
  await loadConfig();
  await loadSessions();
  renderModeButtons();
  setStatus("Ready. Enter a prompt or slash command.");
}

async function loadConfig() {
  const payload = await api("/api/config");
  state.config = payload;
  els.workspace.textContent = payload.workspace || "-";
  els.model.textContent = payload.provider?.model || "-";
  els.approvalPolicy.textContent = payload.approval_policy || "-";
}

async function loadSessions() {
  const payload = await api("/api/sessions?limit=50");
  state.sessions = Array.isArray(payload.sessions) ? payload.sessions : [];
  renderSessions();

  if (state.sessions.length === 0) {
    await createSession();
    return;
  }

  const currentID = state.currentSession?.id;
  const stillExists = currentID && state.sessions.some((item) => item.id === currentID);
  if (stillExists) {
    return;
  }
  await selectSession(state.sessions[0].id);
}

async function createSession() {
  const payload = await api("/api/sessions", {
    method: "POST",
    body: {},
  });
  if (!payload.session) {
    throw new Error("session creation failed");
  }
  await loadSessions();
  await selectSession(payload.session.id);
  addSystemMessage("Created a new session.");
}

async function selectSession(sessionID) {
  const payload = await api(`/api/sessions/${encodeURIComponent(sessionID)}`);
  if (!payload.session) {
    throw new Error("session not found");
  }
  applySession(payload.session);
  renderSessions();
  connectEventStream();
}

function applySession(session) {
  state.currentSession = session;
  state.mode = (session.mode || "build").toLowerCase() === "plan" ? "plan" : "build";
  state.plan = session.plan || {};
  state.messages = convertStoredMessages(session.messages || []);
  state.streamingIndex = -1;
  state.activeRun = null;
  state.approval = null;
  renderModeButtons();
  renderMessages();
  renderPlan();
  renderRunMeta();
  renderApproval();
}

function convertStoredMessages(rawMessages) {
  const list = [];
  for (const raw of rawMessages) {
    if (Array.isArray(raw.tool_calls) && raw.tool_calls.length > 0) {
      for (const call of raw.tool_calls) {
        list.push({
          kind: "tool",
          title: `Tool Call | ${call.function?.name || "unknown"}`,
          body: call.function?.arguments || "",
        });
      }
      continue;
    }
    if (raw.tool_call_id) {
      list.push({
        kind: "tool",
        title: "Tool Result",
        body: raw.content || "",
      });
      continue;
    }
    if (!raw.content) {
      continue;
    }
    list.push({
      kind: raw.role === "assistant" ? "assistant" : "user",
      title: raw.role === "assistant" ? "Bytemind" : "You",
      body: raw.content,
    });
  }
  return list;
}

async function submitInput() {
  const value = (els.promptInput.value || "").trim();
  if (!value) {
    return;
  }
  if (value.startsWith("/")) {
    await handleCommand(value);
    els.promptInput.value = "";
    return;
  }

  if (!state.currentSession) {
    await createSession();
  }
  addUserMessage(value);
  els.promptInput.value = "";

  const payload = await api("/api/runs", {
    method: "POST",
    body: {
      session_id: state.currentSession.id,
      prompt: value,
      mode: state.mode,
    },
  });
  state.activeRun = payload.run || null;
  renderRunMeta();
  setStatus("Run started.");
}

async function submitBTWFromInput() {
  const value = (els.promptInput.value || "").trim();
  if (!value) {
    return;
  }
  await sendBTW(value);
  els.promptInput.value = "";
}

async function sendBTW(message) {
  if (!state.activeRun?.id) {
    setStatus("No active run. Send normally instead.", "warn");
    return;
  }
  addUserMessage(`/btw ${message}`);
  await api(`/api/runs/${encodeURIComponent(state.activeRun.id)}/btw`, {
    method: "POST",
    body: { message },
  });
  setStatus("BTW update queued.");
}

async function cancelActiveRun() {
  if (!state.activeRun?.id) {
    return;
  }
  await api(`/api/runs/${encodeURIComponent(state.activeRun.id)}/cancel`, {
    method: "POST",
    body: {},
  });
  setStatus("Cancel requested.");
}

async function handleCommand(command) {
  const fields = command.trim().split(/\s+/);
  const name = fields[0];
  const args = fields.slice(1);

  switch (name) {
    case "/help":
      addSystemMessage([
        "Supported commands:",
        "/new",
        "/sessions",
        "/resume <session-id>",
        "/session",
        "/mode build|plan",
        "/btw <message>",
        "/skills",
        "/skill clear",
      ].join("\n"));
      break;
    case "/new":
      await createSession();
      break;
    case "/sessions":
      await loadSessions();
      addSystemMessage(`Loaded ${state.sessions.length} session(s).`);
      break;
    case "/resume":
      if (args.length < 1) {
        addSystemMessage("usage: /resume <session-id>");
        break;
      }
      await selectSession(args[0]);
      addSystemMessage(`Resumed session ${args[0]}.`);
      break;
    case "/session":
      if (!state.currentSession) {
        addSystemMessage("No active session.");
      } else {
        addSystemMessage([
          `Session: ${state.currentSession.id}`,
          `Workspace: ${state.currentSession.workspace}`,
          `Mode: ${state.mode}`,
        ].join("\n"));
      }
      break;
    case "/mode":
      if (args.length < 1) {
        addSystemMessage("usage: /mode build|plan");
        break;
      }
      await setMode(args[0]);
      break;
    case "/btw":
      if (args.length < 1) {
        addSystemMessage("usage: /btw <message>");
        break;
      }
      await sendBTW(args.join(" "));
      break;
    case "/skills":
      await showSkills();
      break;
    case "/skill":
      if (args.length === 1 && args[0] === "clear") {
        await clearSkill();
      } else {
        addSystemMessage("usage: /skill clear");
      }
      break;
    default:
      addSystemMessage(`Unknown command: ${name}`);
  }
}

async function setMode(mode) {
  if (!state.currentSession) {
    addSystemMessage("No active session.");
    return;
  }
  const normalized = mode.toLowerCase() === "plan" ? "plan" : "build";
  await api("/api/mode", {
    method: "POST",
    body: {
      session_id: state.currentSession.id,
      mode: normalized,
    },
  });
  state.mode = normalized;
  renderModeButtons();
  setStatus(`Mode set to ${normalized.toUpperCase()}.`);
}

async function showSkills() {
  if (!state.currentSession) {
    addSystemMessage("No active session.");
    return;
  }
  const payload = await api(`/api/skills?session_id=${encodeURIComponent(state.currentSession.id)}`);
  const names = (payload.skills || []).map((item) => item.name);
  const active = payload.active || "none";
  addSystemMessage([
    `Active skill: ${active}`,
    `Available skills (${names.length}):`,
    ...(names.length ? names : ["(none)"]),
  ].join("\n"));
}

async function clearSkill() {
  if (!state.currentSession) {
    addSystemMessage("No active session.");
    return;
  }
  await api("/api/skills/clear", {
    method: "POST",
    body: { session_id: state.currentSession.id },
  });
  addSystemMessage("Active skill cleared.");
}

function connectEventStream() {
  if (state.eventSource) {
    state.eventSource.close();
    state.eventSource = null;
  }
  if (!state.currentSession?.id) {
    return;
  }
  const sessionID = encodeURIComponent(state.currentSession.id);
  const es = new EventSource(`/api/events/stream?session_id=${sessionID}`);
  state.eventSource = es;

  const known = [
    "connected",
    "run_state",
    "run_started",
    "assistant_delta",
    "assistant_message",
    "tool_call_started",
    "tool_call_completed",
    "plan_updated",
    "approval_required",
    "approval_resolved",
    "run_finished",
    "btw_queued",
    "btw_restarted",
    "mode_changed",
    "skill_cleared",
  ];
  for (const type of known) {
    es.addEventListener(type, (event) => handleStreamEvent(type, event));
  }
  es.onerror = () => {
    setStatus("Event stream disconnected. Retrying...", "warn");
  };
}

function handleStreamEvent(fallbackType, event) {
  let payload;
  try {
    payload = JSON.parse(event.data);
  } catch (err) {
    return;
  }
  const type = payload.type || fallbackType;
  const data = payload.data || {};
  const runID = payload.run_id || data.run_id || "";

  switch (type) {
    case "connected":
      setStatus("Stream connected.");
      return;
    case "run_state":
      if (data.running) {
        state.activeRun = {
          id: data.run_id || runID || state.activeRun?.id,
          phase: data.phase || "thinking",
          mode: data.mode || state.mode,
        };
      } else if (!data.running) {
        state.activeRun = null;
      }
      renderRunMeta();
      return;
    case "run_started":
      state.activeRun = state.activeRun || { id: runID, phase: "thinking", mode: state.mode };
      state.activeRun.id = runID || state.activeRun.id;
      state.activeRun.phase = "thinking";
      setStatus("Run started.");
      pushEvent(type, data.user_input || "");
      renderRunMeta();
      return;
    case "assistant_delta":
      appendAssistantDelta(data.content || "");
      renderRunMeta();
      return;
    case "assistant_message":
      finishAssistantMessage(data.content || "");
      return;
    case "tool_call_started":
      addToolMessage(`Tool Call | ${data.tool_name || "unknown"}`, data.tool_arguments || "(no arguments)");
      if (state.activeRun) {
        state.activeRun.phase = "tool";
      }
      setStatus(`Running tool: ${data.tool_name || "unknown"}`);
      pushEvent(type, data.tool_name || "unknown");
      renderRunMeta();
      return;
    case "tool_call_completed":
      addToolMessage(`Tool Result | ${data.tool_name || "unknown"}`, summarizeToolResult(data.tool_result, data.error));
      if (state.activeRun) {
        state.activeRun.phase = "thinking";
      }
      renderRunMeta();
      return;
    case "plan_updated":
      state.plan = data.plan || {};
      renderPlan();
      return;
    case "approval_required":
      state.approval = {
        id: data.approval_id,
        runID: runID || state.activeRun?.id,
        reason: data.reason || "",
        command: data.command || "",
      };
      renderApproval();
      setStatus("Approval required.", "warn");
      return;
    case "approval_resolved":
      state.approval = null;
      renderApproval();
      return;
    case "btw_queued":
      setStatus(`BTW queued (${data.pending_count || 1} pending).`, "warn");
      pushEvent(type, `${data.pending_count || 1} pending`);
      return;
    case "btw_restarted":
      if (data.new_run_id) {
        state.activeRun = {
          id: data.new_run_id,
          phase: "thinking",
          mode: state.mode,
        };
      }
      setStatus("BTW accepted. Restarted run.");
      pushEvent(type, `${data.old_run_id || "-"} -> ${data.new_run_id || "-"}`);
      renderRunMeta();
      return;
    case "run_finished":
      state.streamingIndex = -1;
      state.activeRun = null;
      state.approval = null;
      renderApproval();
      renderRunMeta();
      if (data.error) {
        setStatus(`Run finished: ${data.error}`, "danger");
        addSystemMessage(`Run finished: ${data.error}`);
      } else {
        setStatus("Run finished.");
      }
      pushEvent(type, data.status || "ok");
      return;
    case "mode_changed":
      if (data.mode) {
        state.mode = data.mode;
      }
      renderModeButtons();
      return;
    case "skill_cleared":
      pushEvent(type, "active skill cleared");
      return;
    default:
      pushEvent(type, compactText(JSON.stringify(data), 120));
  }
}

async function resolveApproval(approved) {
  if (!state.approval?.runID) {
    return;
  }
  await api(`/api/runs/${encodeURIComponent(state.approval.runID)}/approval`, {
    method: "POST",
    body: {
      approval_id: state.approval.id,
      approved,
    },
  });
}

function appendAssistantDelta(delta) {
  if (!delta) {
    return;
  }
  if (state.streamingIndex < 0 || state.streamingIndex >= state.messages.length) {
    state.messages.push({
      kind: "assistant",
      title: "Bytemind",
      body: delta,
      streaming: true,
    });
    state.streamingIndex = state.messages.length - 1;
    renderMessages(true);
    return;
  }
  const current = state.messages[state.streamingIndex];
  if (!current || current.kind !== "assistant") {
    state.streamingIndex = -1;
    appendAssistantDelta(delta);
    return;
  }
  const existing = current.body || "";
  if (delta.startsWith(existing)) {
    current.body = delta;
  } else if (!existing.endsWith(delta)) {
    current.body = existing + delta;
  }
  current.streaming = true;
  renderMessages(true);
}

function finishAssistantMessage(content) {
  content = (content || "").trim();
  if (state.streamingIndex >= 0 && state.streamingIndex < state.messages.length) {
    const current = state.messages[state.streamingIndex];
    if (current && current.kind === "assistant") {
      if (content) {
        current.body = content;
      }
      current.streaming = false;
      state.streamingIndex = -1;
      renderMessages(true);
      return;
    }
  }
  if (content) {
    const last = state.messages[state.messages.length - 1];
    if (last && last.kind === "assistant" && (last.body || "").trim() === content) {
      return;
    }
    state.messages.push({ kind: "assistant", title: "Bytemind", body: content });
    renderMessages(true);
  }
}

function addUserMessage(text) {
  state.messages.push({ kind: "user", title: "You", body: text });
  renderMessages(true);
}

function addSystemMessage(text) {
  state.messages.push({ kind: "system", title: "System", body: text });
  renderMessages(true);
}

function addToolMessage(title, body) {
  state.messages.push({ kind: "tool", title, body });
  renderMessages(true);
}

function pushEvent(name, meta) {
  state.events.unshift({
    name,
    meta,
    at: new Date(),
  });
  if (state.events.length > 120) {
    state.events.length = 120;
  }
  renderEvents();
}

function renderSessions() {
  els.sessionsList.innerHTML = "";
  for (const session of state.sessions) {
    const item = document.createElement("div");
    item.className = "session-item";
    if (session.id === state.currentSession?.id) {
      item.classList.add("active");
    }
    item.addEventListener("click", () => {
      void selectSession(session.id);
    });

    const main = document.createElement("div");
    main.className = "session-main";
    main.innerHTML = `<span>${compactText(session.id, 14)}</span><span>${formatTime(session.updated_at)}</span>`;

    const sub = document.createElement("div");
    sub.className = "session-sub";
    sub.textContent = session.last_user_message || session.workspace || "-";

    item.appendChild(main);
    item.appendChild(sub);
    els.sessionsList.appendChild(item);
  }
}

function renderMessages(scrollToBottom = false) {
  els.messages.innerHTML = "";
  for (const msg of state.messages) {
    const wrapper = document.createElement("div");
    wrapper.className = `msg ${msg.kind}`;
    if (msg.streaming) {
      wrapper.classList.add("streaming");
    }
    const title = document.createElement("div");
    title.className = "msg-title";
    title.textContent = msg.title || "";
    const body = document.createElement("div");
    body.textContent = msg.body || "";
    wrapper.appendChild(title);
    wrapper.appendChild(body);
    els.messages.appendChild(wrapper);
  }
  if (scrollToBottom) {
    els.messages.scrollTop = els.messages.scrollHeight;
  }
}

function renderModeButtons() {
  els.modeBuildBtn.classList.toggle("active", state.mode === "build");
  els.modePlanBtn.classList.toggle("active", state.mode === "plan");
}

function renderRunMeta() {
  const run = state.activeRun;
  if (!run) {
    els.runMeta.textContent = "No active run.";
    return;
  }
  els.runMeta.textContent = [
    `Run ID: ${run.id || "-"}`,
    `Mode: ${(run.mode || state.mode || "build").toUpperCase()}`,
    `Phase: ${run.phase || "thinking"}`,
  ].join("\n");
}

function renderPlan() {
  const plan = state.plan || {};
  const container = document.createElement("div");
  container.className = "side-content";

  const meta = document.createElement("div");
  meta.textContent = `Phase: ${plan.phase || "none"}`;
  container.appendChild(meta);

  if (plan.goal) {
    const goal = document.createElement("div");
    goal.style.marginTop = "8px";
    goal.textContent = `Goal: ${plan.goal}`;
    container.appendChild(goal);
  }

  const list = document.createElement("ul");
  list.className = "plan-list";
  const steps = Array.isArray(plan.steps) ? plan.steps : [];
  if (steps.length === 0) {
    const empty = document.createElement("div");
    empty.style.marginTop = "8px";
    empty.textContent = "No structured plan.";
    container.appendChild(empty);
  } else {
    for (const step of steps) {
      const li = document.createElement("li");
      li.className = "plan-item";
      if (step.status === "completed") {
        li.classList.add("done");
      }
      li.textContent = `${statusGlyph(step.status)} ${step.title || "-"}`;
      list.appendChild(li);
    }
    container.appendChild(list);
  }

  els.planView.innerHTML = "";
  els.planView.appendChild(container);
}

function renderEvents() {
  els.events.innerHTML = "";
  const list = document.createElement("div");
  list.className = "event-scroll";
  for (const evt of state.events) {
    const item = document.createElement("div");
    item.className = "event-item";

    const name = document.createElement("div");
    name.className = "event-name";
    name.textContent = evt.name;
    item.appendChild(name);

    const meta = document.createElement("div");
    meta.className = "event-meta";
    meta.textContent = `${formatClock(evt.at)} ${evt.meta ? " | " + evt.meta : ""}`;
    item.appendChild(meta);

    list.appendChild(item);
  }
  els.events.appendChild(list);
}

function renderApproval() {
  if (!state.approval) {
    els.approvalModal.classList.remove("visible");
    return;
  }
  els.approvalReason.textContent = state.approval.reason || "(no reason)";
  els.approvalCommand.textContent = state.approval.command || "(empty command)";
  els.approvalModal.classList.add("visible");
}

function setStatus(text, level = "info") {
  state.status = text;
  els.statusLine.textContent = text;
  els.statusLine.style.color = level === "danger" ? "var(--danger)" : level === "warn" ? "var(--warn)" : "var(--muted)";
}

function summarizeToolResult(rawResult, errText) {
  if (errText) {
    return `error: ${errText}`;
  }
  if (!rawResult) {
    return "(empty result)";
  }
  try {
    const parsed = JSON.parse(rawResult);
    if (parsed.error) {
      return `error: ${parsed.error}`;
    }
    if (typeof parsed.exit_code === "number") {
      return `exit code ${parsed.exit_code}`;
    }
    if (parsed.path) {
      return `path: ${parsed.path}`;
    }
    return compactText(JSON.stringify(parsed), 220);
  } catch (_err) {
    return compactText(String(rawResult), 220);
  }
}

async function api(path, options = {}) {
  const method = options.method || "GET";
  const headers = { "Content-Type": "application/json" };
  const init = { method, headers };
  if (options.body !== undefined) {
    init.body = JSON.stringify(options.body);
  }
  const response = await fetch(path, init);
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    const msg = payload.message || payload.error || `request failed (${response.status})`;
    throw new Error(msg);
  }
  return payload;
}

function statusGlyph(status) {
  switch ((status || "").toLowerCase()) {
    case "completed":
      return "[x]";
    case "in_progress":
      return "[>]";
    case "blocked":
      return "[!]";
    default:
      return "[ ]";
  }
}

function compactText(text, limit) {
  const clean = String(text || "").replace(/\s+/g, " ").trim();
  if (clean.length <= limit) {
    return clean;
  }
  return `${clean.slice(0, Math.max(0, limit - 3))}...`;
}

function formatTime(value) {
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) {
    return "-";
  }
  return d.toLocaleString();
}

function formatClock(value) {
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) {
    return "-";
  }
  return d.toLocaleTimeString();
}
