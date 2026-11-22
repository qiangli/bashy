"use strict";

const role = location.pathname.split("/").filter(Boolean).at(-1);
const api = path => new URL("./api/" + path, location.href);
const sessionRoles = ["control", "present", "projector"];
const $ = id => document.getElementById(id);
const videoType = "video/webm;codecs=vp8";
const audioType = "video/webm;codecs=vp8,opus";
let state = {};
let eventStream;
let key = "";
let viewer = "";

let target = "";
if (sessionRoles.includes(role)) {
  const storage = "demo-queue:" + location.pathname;
  const link = decodeURIComponent(location.hash.slice(1)) || sessionStorage.getItem(storage) || "";
  if (link.startsWith("http://")) {
    const native = parseNativeLink(link);
    if (native.role === role) {
      target = native.addr;
      key = native.key;
    }
  } else {
    key = link;
  }
  if (key) {
    sessionStorage.setItem(storage, link);
    history.replaceState(null, "", location.pathname);
  }
}

function parseNativeLink(value) {
  const url = new URL(value);
  const role = url.pathname.slice(1);
  const key = url.hash.slice(1);
  if (url.protocol !== "http:" || url.hostname !== "127.0.0.1" || !url.port ||
      url.search || url.username || url.password || !sessionRoles.includes(role) ||
      !/^[a-zA-Z0-9_-]{32}$/.test(key)) throw new Error("Enter a private Demo Queue link.");
  return {addr: url.host, role, key};
}

function connectPage() {
  document.body.innerHTML = `
    <main><header><h1>Demo Queue</h1></header>
      <section><h2>Connect to a session</h2>
        <form id="attachment">
          <label>Private session link<input id="local-link" type="password" required autocomplete="off"></label>
          <button>Connect</button>
        </form>
        <p id="error" role="alert"></p>
      </section>
    </main>`;
  $("attachment").onsubmit = async event => {
    event.preventDefault();
    clearError();
    const link = $("local-link").value.trim();
    try {
      const {addr, role, key} = parseNativeLink(link);
      const probe = await fetch(api("demo-link"), {headers: {
        Authorization: "Bearer " + key, "X-Demo-Target": addr, "X-Demo-Role": role
      }});
      if (!probe.ok) throw new Error("The local session rejected this link.");
      const next = new URL("./" + role, location.href);
      next.hash = encodeURIComponent(link);
      location.assign(next);
    } catch (error) { failure(error); }
  };
}

async function request(path, {method = "GET", data, type, signal} = {}) {
  const headers = {};
  if (key) headers.Authorization = "Bearer " + key;
  if (viewer) headers["X-Demo-Viewer"] = viewer;
  if (target) {
    headers["X-Demo-Target"] = target;
    headers["X-Demo-Role"] = role;
  }
  if (type) headers["Content-Type"] = type;
  if (data && !type) {
    data = JSON.stringify(data);
    headers["Content-Type"] = "application/json";
  }
  const result = await fetch(api(path), {method, body: data, headers, signal, cache: "no-store"});
  if (!result.ok) {
    const {error} = await result.json().catch(() => ({}));
    throw new Error(error || "Connection failed. Try again.");
  }
  return result;
}

const post = (path, data) => request(path, {method: "POST", data});
function events(render, onLost, params = {}) {
  eventStream?.close();
  const query = new URLSearchParams({...params, key});
  if (target) { query.set("target", target); query.set("role", role); }
  eventStream = new EventSource(api("events?" + query));
  eventStream.onmessage = event => {
    state = JSON.parse(event.data);
    render(state);
    if (state.ended) eventStream.close();
  };
  eventStream.onerror = onLost;
}

function failure(error) { $("error").textContent = error.message; }
function clearError() { $("error").textContent = ""; }

function control() {
  document.body.innerHTML = `
    <main>
      <header><h1>Demo Queue</h1><p id="session">Connecting…</p></header>
      <section aria-labelledby="session-heading">
        <h2 id="session-heading">Session</h2>
        <form id="settings">
          <label>Turn length (seconds)<input id="limit" type="number" min="1" max="3600" value="120" required></label>
          <label>Warning (seconds)<input id="warning" type="number" min="0" max="3599" value="15" required></label>
          <button id="start" disabled>Start</button>
        </form>
        <div class="actions"><button id="invite">Create invite</button><button id="skip" disabled>Skip</button><button id="end" disabled>End session</button></div>
        <div class="invite" id="invite-panel" hidden><label>Private invite<input id="invite-text" readonly></label><button id="copy">Copy</button></div>
        <p id="error" role="alert"></p><p id="notice" role="status"></p>
      </section>
      <section aria-labelledby="roster-heading"><h2 id="roster-heading">Presenters</h2><ul id="roster" class="roster"></ul></section>
    </main>`;
  const run = async (action, fields = {}) => {
    clearError();
    try { return await post("control", {action, ...fields}); } catch (err) { failure(err); }
  };
  $("settings").onsubmit = event => {
    event.preventDefault();
    run("start", {limit: Number($("limit").value), warning: Number($("warning").value)});
  };
  $("invite").onclick = async () => {
    const result = await run("invite");
    if (!result) return;
    const {invite} = await result.json();
    $("invite-text").value = invite;
    $("invite-panel").hidden = false;
    $("copy").textContent = "Copy";
    $("invite-text").select();
  };
  $("copy").onclick = async () => {
    try {
      await navigator.clipboard.writeText($("invite-text").value);
      $("copy").textContent = "Copied";
    } catch {
      $("invite-text").select();
    }
  };
  $("skip").onclick = () => run("skip", {turn: state.turn?.ID});
  $("end").onclick = () => run("end");
  let rosterVersion = "";
  events(s => {
    $("session").textContent = s.ended ? "Session ended" : s.turn ?
      s.turn.Presenter + (s.turn.Live ? " is presenting" : " is connecting") : s.started ? "Waiting for a ready presenter" :
      s.projector ? "Projector connected" : "Open the projector view";
    $("settings").hidden = s.started || s.ended;
    $("start").disabled = !s.projector || !key;
    $("invite").disabled = s.ended || !key;
    $("invite-panel").hidden = s.ended || !$("invite-text").value;
    $("skip").disabled = s.ended || !s.turn;
    $("end").disabled = s.ended || !key;
    $("notice").textContent = s.assigned ? "Waiting for " + s.assigned + " to prepare." : s.notice;
    const version = JSON.stringify([s.roster, s.ended]);
    if (version === rosterVersion) return;
    rosterVersion = version;
    const roster = (s.roster || []).slice().sort((a, b) => (a.position || Infinity) - (b.position || Infinity));
    $("roster").replaceChildren(...roster.map(p => {
      const item = document.createElement("li");
      const label = document.createElement("span");
      const status = document.createElement("span");
      const assign = document.createElement("button");
      label.textContent = p.position ? p.position + ". " + p.name : p.name;
      status.className = p.status === "Ready" ? "ready status" : "status";
      status.textContent = p.status;
      assign.textContent = "Assign turn";
      assign.setAttribute("aria-label", "Assign turn to " + p.name);
      assign.disabled = s.ended || ["Offline", "Starting", "Presenting"].includes(p.status);
      assign.onclick = () => run("assign", {id: p.id});
      item.append(label, status, assign);
      return item;
    }));
  }, () => { $("session").textContent = "Projector session connection lost"; }, {key});
}

function mediaPath(turn, seq) {
  return new URLSearchParams({turn: turn.ID, generation: turn.Generation, seq});
}

function presenter() {
  document.body.innerHTML = `
    <main>
      <header><h1>Demo Queue</h1><p id="session">Joining…</p></header>
      <section>
        <p id="guidance">Choose a tab, window, or screen. Share a browser tab with audio for videos.</p>
        <div class="actions">
          <button id="prepare" class="primary" disabled>Prepare to present</button>
          <button id="leave" hidden>Leave queue</button>
          <button id="finish" hidden>Finish turn</button>
        </div>
        <p id="error" role="alert"></p>
        <div id="preview" hidden><h2>Local preview</h2><video id="source" autoplay muted playsinline></video></div>
      </section>
    </main>`;
  let stream;
  let current;
  let preparing = false;
  let cancellation;
  let ready = false;
  let lastEvent = Date.now();

  function stopSending() {
    if (!current) return;
    current.abort.abort();
    if (current.recorder.state !== "inactive") current.recorder.stop();
    current = undefined;
  }

  function release() {
    stopSending();
    for (const track of stream?.getTracks() || []) track.stop();
    $("source").srcObject = null;
    $("preview").hidden = true;
    stream = undefined;
    ready = false;
    cancellation?.abort();
  }

  async function withdraw() {
    release();
    await post("member", {action: "withdraw"}).catch(failure);
  }

  function begin(turn) {
    if (current?.turn.ID === turn.ID && current.turn.Generation === turn.Generation) return;
    stopSending();
    const mimeType = stream.getAudioTracks().length ? audioType : videoType;
    let recorder;
    try {
      recorder = new MediaRecorder(stream, {mimeType, videoBitsPerSecond: 3000000, audioBitsPerSecond: 96000});
    } catch (err) {
      failure(err);
      withdraw();
      return;
    }
    const run = {turn, recorder, abort: new AbortController()};
    current = run;
    let seq = 0;
    let pending = 0;
    let bytes = 0;
    let send = Promise.resolve();
    const broken = err => {
      if (current !== run || run.abort.signal.aborted) return;
      failure(err);
      withdraw();
    };
    recorder.ondataavailable = ({data}) => {
      if (current !== run || run.abort.signal.aborted || !data.size) return;
      if (data.size > 2 * 1024 * 1024 || pending >= 8 || bytes + data.size > 4 * 1024 * 1024) {
        broken(new Error("The connection cannot keep up. Prepare again to retry."));
        return;
      }
      const query = mediaPath(turn, seq++);
      pending++;
      bytes += data.size;
      send = send.then(async () => {
        if (run.abort.signal.aborted) return;
        try {
          await request("upload?" + query, {
            method: "POST", data, type: mimeType,
            signal: AbortSignal.any([run.abort.signal, AbortSignal.timeout(3500)])
          });
        } catch (err) {
          if (state.turn?.Generation === turn.Generation && state.turn?.ID === turn.ID) {
            if (err.message === "The turn has changed.") stopSending();
            else broken(err);
          }
        } finally {
          pending--;
          bytes -= data.size;
        }
      });
    };
    recorder.onerror = () => broken(new Error("Screen encoding stopped. Prepare again to retry."));
    try { recorder.start(250); } catch (err) { broken(err); }
  }

  $("prepare").onclick = async () => {
    clearError();
    preparing = true;
    $("prepare").disabled = true;
    const attempt = new AbortController();
    cancellation = attempt;
    try {
      if (!navigator.mediaDevices?.getDisplayMedia || !window.MediaRecorder?.isTypeSupported(videoType)) {
        throw new Error("Screen sharing requires a current desktop version of Chrome or Edge.");
      }
      const picked = await navigator.mediaDevices.getDisplayMedia({
        video: {width: {max: 1920}, height: {max: 1080}, frameRate: {max: 30}},
        audio: true, systemAudio: "exclude", windowAudio: "exclude", selfBrowserSurface: "exclude"
      });
      if (attempt.signal.aborted) {
        picked.getTracks().forEach(track => track.stop());
        return;
      }
      stream = picked;
      const track = stream.getVideoTracks()[0];
      if (!track || track.readyState !== "live") throw new Error("Choose an available capture source.");
      if (track.getSettings().displaySurface !== "browser") {
        for (const audio of stream.getAudioTracks()) {
          stream.removeTrack(audio);
          audio.stop();
        }
      }
      if (stream.getAudioTracks().length && !MediaRecorder.isTypeSupported(audioType)) {
        throw new Error("Tab audio requires WebM and Opus support.");
      }
      track.addEventListener("ended", withdraw, {once: true});
      $("source").srcObject = stream;
      $("preview").hidden = false;
      const sample = new Uint8Array(512 * 1024);
      // A synthetic payload exercises the very same HTTP forward as media.
      for (let i = 0; i < sample.length; i += 65536) crypto.getRandomValues(sample.subarray(i, i + 65536));
      await request("prepare", {
        method: "POST", data: sample, type: "application/octet-stream",
        signal: AbortSignal.any([attempt.signal, AbortSignal.timeout(3000)])
      });
      ready = true;
      await request("member", {method: "POST", data: {action: "ready"}, signal: attempt.signal});
      $("session").textContent = "Ready. Sharing starts automatically when it’s your turn.";
    } catch (err) {
      if (!attempt.signal.aborted) failure(err.name === "NotAllowedError" ? new Error("Choose a source and allow sharing to prepare.") : err);
      release();
    } finally {
      preparing = false;
      $("prepare").disabled = !state.me || state.ended;
    }
  };
  $("leave").onclick = withdraw;
  $("finish").onclick = () => {
    if (!state.turn) return;
    const turn = state.turn.ID;
    release();
    post("member", {action: "finish", turn}).catch(failure);
  };

  events(s => {
    lastEvent = Date.now();
    const status = s.me?.status;
    if (s.ended) release();
    if (s.projector && s.turn && stream && ready) begin(s.turn);
    else if (current) stopSending();
    if (!preparing && stream && !["Ready", "Starting", "Presenting"].includes(status)) release();
    const isQueued = status === "Ready";
    $("prepare").disabled = preparing || !s.me || s.ended || (isQueued && !!stream) || !!s.turn;
    $("leave").hidden = !isQueued;
    $("finish").hidden = !s.turn;
    $("session").textContent = s.ended ? "Session ended" : s.turn ? (s.turn.Live ? "You’re presenting" : "Starting your share…") :
      isQueued ? "Ready · queue position " + s.me.position :
      s.assigned ? "The organizer is waiting for you to prepare." :
      preparing ? "Checking your source and connection…" : "Joined as " + (s.me?.name || "presenter");
    const left = s.turn ? remaining(s.turn, s.now) : 0;
    $("guidance").classList.toggle("presenter-warning", !!s.turn?.Live && s.warning > 0 && left <= s.warning);
    $("guidance").textContent = s.turn?.Live ? "Time remaining " + formatTime(left) :
      "Choose a tab, window, or screen. Share a browser tab with audio for videos.";
  }, () => {
    release();
    $("session").textContent = "Connection lost. Prepare again once reconnected.";
    $("prepare").disabled = true;
    // Clear stale readiness if the control stream reconnects before the host expires it.
    post("member", {action: "withdraw"}).catch(() => {});
  });
  setInterval(() => {
    if (stream && Date.now() - lastEvent >= 5000) {
      release();
      $("session").textContent = "Connection lost. Prepare again once reconnected.";
      post("member", {action: "withdraw"}).catch(() => {});
    }
  }, 1000);
  addEventListener("pagehide", () => {
    release();
    const query = new URLSearchParams({key});
    if (target) { query.set("target", target); query.set("role", role); }
    navigator.sendBeacon(api("member?" + query), JSON.stringify({action: "withdraw"}));
  });
}

function remaining(turn, now) {
  return Math.max(0, Math.ceil((Date.parse(turn.Deadline) - now) / 1000));
}

function formatTime(seconds) {
  return Math.floor(seconds / 60) + ":" + String(seconds % 60).padStart(2, "0");
}

function waitFor(target, name, signal) {
  return new Promise((resolve, reject) => {
    const controller = new AbortController();
    const complete = (callback, value) => {
      controller.abort();
      signal.removeEventListener("abort", stop);
      callback(value);
    };
    const stop = () => complete(reject, signal.reason);
    if (signal.aborted) { stop(); return; }
    signal.addEventListener("abort", stop, {once: true});
    target.addEventListener(name, event => complete(resolve, event), {once: true, signal: controller.signal});
    target.addEventListener("error", () => complete(reject, new Error("The browser could not decode the share.")), {once: true, signal: controller.signal});
  });
}

function projector() {
  document.body.className = "projection";
  document.body.innerHTML = `
    <main id="stage">
      <header><div><h1 id="name">Demo Queue</h1><p id="next"></p></div><div id="clock" aria-hidden="true"></div></header>
      <div id="screen"><video id="video" playsinline muted></video><p id="waiting" role="status">Connecting…</p></div>
      <footer><p id="error" role="alert"></p><button id="sound">Enable sound</button><button id="full">Fullscreen</button></footer>
    </main>`;
  const media = $("video");
  let active;
  let referenceTime = 0;
  let clockTime = 0;
  let userSound = false;

  function play() {
    media.muted = !userSound;
    media.play().catch(() => {
      if (!media.paused || !media.src || !userSound) return;
      userSound = false;
      media.muted = true;
      $("sound").textContent = "Enable sound";
      if (media.src) media.play().catch(() => {});
    });
  }

  $("sound").onclick = () => {
    userSound = !userSound;
    $("sound").textContent = userSound ? "Mute" : "Enable sound";
    if (media.src) play();
  };
  $("full").onclick = () => {
    const change = document.fullscreenElement ? document.exitFullscreen() : $("stage").requestFullscreen();
    change.catch(failure);
  };
  document.addEventListener("fullscreenchange", () => {
    $("full").textContent = document.fullscreenElement ? "Exit fullscreen" : "Fullscreen";
  });

  function closeMedia() {
    if (!active) return;
    active.abort.abort();
    URL.revokeObjectURL(active.url);
    media.pause();
    media.removeAttribute("src");
    media.load();
    $("screen").classList.remove("showing");
    active = undefined;
  }

  async function display(turn) {
    closeMedia();
    clearError();
    const source = new MediaSource();
    const job = {turn, abort: new AbortController(), url: URL.createObjectURL(source)};
    const {signal} = job.abort;
    active = job;
    try {
      const loaded = waitFor(source, "sourceopen", signal);
      media.src = job.url;
      await loaded;
      let seq = 0;
      let buffer;
      while (!signal.aborted) {
        const result = await request("chunk?" + mediaPath(turn, seq), {signal});
        if (result.status === 204) continue;
        const chunk = await result.arrayBuffer();
        if (signal.aborted) return;
        if (!buffer) {
          buffer = source.addSourceBuffer(result.headers.get("Content-Type"));
          media.requestVideoFrameCallback(() => {
            if (active !== job || signal.aborted) return;
            $("screen").classList.add("showing");
            $("waiting").textContent = "";
            post("rendered", {turn: turn.ID, generation: turn.Generation}).catch(err => {
              if (active === job) failure(err);
            });
          });
        }
        const appended = waitFor(buffer, "updateend", signal);
        buffer.appendBuffer(chunk);
        await appended;
        if (seq === 0) play();
        if (media.currentTime > 12 && buffer.buffered.length &&
            buffer.buffered.start(0) < media.currentTime - 10) {
          const trimmed = waitFor(buffer, "updateend", signal);
          buffer.remove(0, media.currentTime - 6);
          await trimmed;
        }
        seq++;
      }
    } catch (err) {
      if (active === job && !signal.aborted) {
        if (err.message !== "The turn has changed.") failure(err);
        closeMedia();
      }
    }
  }

  function updateClock() {
    const turn = state.turn;
    const elapsed = performance.now() - clockTime;
    const timed = turn && Date.parse(turn.Deadline) > 0;
    const left = timed ? remaining(turn, referenceTime + elapsed) : 0;
    $("clock").textContent = timed ? formatTime(left) : "";
    $("clock").classList.toggle("warning", timed && state.warning > 0 && left <= state.warning);
    if (timed && left === 0 && active) closeMedia();
  }

  async function connect() {
    if (!window.MediaSource?.isTypeSupported(videoType) || !MediaSource.isTypeSupported(audioType) || !media.requestVideoFrameCallback) {
      failure(new Error("Open the projector in current desktop Chrome or Edge."));
      return;
    }
    try {
      const result = await post("viewer", {});
      ({viewer} = await result.json());
      events(s => {
        referenceTime = s.now;
        clockTime = performance.now();
        const turn = s.turn;
        const expired = turn && Date.parse(turn.Deadline) > 0 && remaining(turn, s.now) === 0;
        $("name").textContent = turn?.Presenter || "Demo Queue";
        $("next").textContent = s.next ? "Up next · " + s.next : "";
        $("waiting").textContent = s.ended ? "Session ended" :
          turn ? "Connecting to " + turn.Presenter + "…" : s.started ? "Waiting for a ready presenter" : "Waiting for the organizer";
        if (!turn || expired || s.ended) closeMedia();
        else if (!active || active.turn.ID !== turn.ID || active.turn.Generation !== turn.Generation) display(turn);
        updateClock();
      }, () => {
        closeMedia();
        $("waiting").textContent = "Reconnecting projector…";
      }, {key, viewer});
    } catch (err) {
      failure(err);
    }
  }

  setInterval(updateClock, 100);
  connect();
}

if (role === "control") control();
else if (role === "projector") projector();
else if (role === "present") presenter();
else connectPage();
