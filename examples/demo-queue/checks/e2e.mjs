// Runs the same native binaries and browser actions on macOS and Windows.
import assert from "node:assert/strict";
import { spawn, execFileSync } from "node:child_process";
import { createServer } from "node:net";
import { once } from "node:events";
import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import { delimiter, join } from "node:path";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const { chromium } = require(process.env.DEMO_PLAYWRIGHT || "playwright");
const project = fileURLToPath(new URL("../", import.meta.url));
const temp = await mkdtemp(join(tmpdir(), "demo-queue-e2e-"));
const suffix = process.platform === "win32" ? ".exe" : "";
const app = join(temp, "demo-queue" + suffix);
const mesh = join(temp, "outpost" + suffix);
const env = { ...process.env, GOWORK: "off" };
for (const [name, value] of Object.entries(env)) {
  if (name !== "PATH" && name.toUpperCase() === "PATH") {
    env.PATH = value;
    delete env[name];
  }
}
env.PATH = temp + delimiter + env.PATH;
env.DEMO_QUEUE_TEST_MESH = join(temp, "forward");
const children = new Set();
const browsers = [];

function build(target, source) {
  execFileSync("go", ["build", "-trimpath", "-o", target, source], { cwd: project, env, stdio: "inherit" });
}

function run(args, stdin) {
  const p = spawn(app, args, { env, stdio: ["pipe", "pipe", "pipe"] });
  children.add(p);
  p.once("exit", () => children.delete(p));
  if (stdin !== undefined) p.stdin.end(stdin);
  return p;
}

async function line(p) {
  let data = "";
  let errors = "";
  p.stderr.on("data", chunk => { errors += chunk; });
  return await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error("Demo Queue process produced no JSON: " + errors)), 12000);
    p.on("exit", code => { clearTimeout(timer); reject(new Error("Demo Queue exited with " + code + ": " + errors)); });
    p.stdout.on("data", chunk => {
      data += chunk;
      if (data.includes("\n")) {
        clearTimeout(timer);
        try { resolve(JSON.parse(data.slice(0, data.indexOf("\n")))); }
        catch (error) { reject(error); }
      }
    });
  });
}

function cli(verb, url, ...flags) {
  const args = [verb, ...(url ? ["--url", url] : []), ...flags];
  return JSON.parse(execFileSync(app, args, { env, encoding: "utf8", timeout: 12000 }));
}

async function freePort() {
  const socket = createServer();
  socket.listen(0, "127.0.0.1");
  await once(socket, "listening");
  const { port } = socket.address();
  await new Promise(resolve => socket.close(resolve));
  return port;
}

// Deterministic source for CI. Chromium still runs its real encoder, muxer,
// autoplay policy, and decoder. The OS picker is covered in the live rehearsal.
function fakePicker() {
  navigator.mediaDevices.getDisplayMedia = async () => {
    const canvas = document.createElement("canvas");
    canvas.width = 640;
    canvas.height = 360;
    const brush = canvas.getContext("2d");
    let index = 0;
    const color = () => sessionStorage.getItem("demo-color") || "#1478bd";
    const draw = () => {
      brush.fillStyle = color();
      brush.fillRect(0, 0, 640, 360);
      brush.font = "48px sans-serif";
      brush.fillStyle = "white";
      brush.fillText("Demo " + index++, 80, 150);
    };
    draw();
    const timer = setInterval(draw, 33);
    const capture = canvas.captureStream(30);
    const [video] = capture.getVideoTracks();
    video.getSettings = () => ({ displaySurface: "browser" });
    const source = new AudioContext();
    const tone = source.createOscillator();
    const output = source.createMediaStreamDestination();
    tone.connect(output);
    tone.start();
    await source.resume();
    capture.addTrack(output.stream.getAudioTracks()[0]);
    const stop = video.stop.bind(video);
    video.stop = () => { clearInterval(timer); stop(); source.close(); };
    return capture;
  };
}

async function projector(browser, url) {
  const page = await browser.newPage();
  await page.goto(url);
  await page.getByRole("button", { name: "Enable sound" }).click();
  return page;
}

async function openPresenter(browser, host, name) {
  const { invite } = cli("invite", host);
  const launcher = run(["join", "--name", name, "--invite-file", "-", "--json"], invite + "\n");
  const { presenter: url } = await line(launcher);
  const page = await browser.newPage();
  await page.addInitScript(fakePicker);
  await page.goto(url);
  return { page, url, launcher };
}

async function ready(presenter, position) {
  await presenter.page.getByRole("button", { name: "Prepare to present" }).click();
  await presenter.page.getByText("Ready · queue position " + position, { exact: true }).waitFor();
  assert.equal(cli("status", presenter.url).me.status, "Ready");
}

async function current(page, name) {
  await page.getByRole("heading", { name }).waitFor();
  await page.locator("#screen.showing").waitFor();
  await page.waitForFunction(() => document.querySelector("#video").webkitAudioDecodedByteCount > 0);
}

async function rehearsal(brand, peer) {
  const { control, projector: display } = await line(run(["host", "--json"]));
  const controlPage = await brand.newPage();
  await controlPage.goto(control);
  const screen = await projector(brand, display);
  const first = await openPresenter(brand, control, "First presenter");
  const second = await openPresenter(peer, control, "Second presenter");
  await ready(first, 1);
  await ready(second, 2);
  await controlPage.waitForFunction(() => document.querySelectorAll(".ready.status").length === 2);
  cli("start", control, "--limit", "5", "--warning", "2", "--dry-run");
  assert.equal(cli("status", control).started, false);
  assert.equal(await screen.locator("#video").getAttribute("src"), null);
  cli("start", control, "--limit", "5", "--warning", "2");
  await current(screen, "First presenter");
  const firstStream = await screen.locator("#video").getAttribute("src");
  await screen.locator("#clock.warning").waitFor();
  await current(screen, "Second presenter");
  assert.notEqual(await screen.locator("#video").getAttribute("src"), firstStream);
  await second.page.evaluate(() => sessionStorage.setItem("demo-color", "#be3355"));
  await screen.waitForFunction(() => {
    const sample = document.createElement("canvas").getContext("2d");
    sample.drawImage(document.querySelector("#video"), 0, 0, 160, 90);
    const [red, , blue] = sample.getImageData(6, 6, 1, 1).data;
    return red > blue + 25;
  });

  const source = await screen.locator("#video").evaluate(async () => {
    const context = new AudioContext();
    const node = context.createOscillator();
    const audio = context.createMediaStreamDestination();
    node.connect(audio);
    node.start();
    await context.resume();
    const canvas = document.createElement("canvas");
    canvas.width = 320;
    canvas.height = 180;
    const brush = canvas.getContext("2d");
    brush.fillRect(1, 1, 280, 160);
    const stream = canvas.captureStream(30);
    stream.addTrack(audio.stream.getAudioTracks()[0]);
    const recorder = new MediaRecorder(stream, { mimeType: "video/webm;codecs=vp8,opus" });
    const chunks = [];
    recorder.ondataavailable = event => chunks.push(event.data);
    const result = new Promise(resolve => recorder.onstop = resolve);
    recorder.start();
    const drawing = setInterval(() => brush.fillRect(1, 1, 280, 160), 33);
    setTimeout(() => recorder.stop(), 750);
    await result;
    clearInterval(drawing);
    stream.getTracks().forEach(track => track.stop());
    await context.close();
    return Array.from(new Uint8Array(await new Blob(chunks).arrayBuffer()));
  });
  await cli("finish", second.url);
  await screen.getByText("Waiting for a ready presenter").waitFor();
  assert.equal(await screen.locator("#video").getAttribute("src"), null);

  const firstID = cli("status", control).roster.find(p => p.name === "First presenter").id;
  cli("assign", control, "--id", firstID, "--dry-run");
  assert.equal(cli("status", control).assigned, undefined);
  cli("assign", control, "--id", firstID);
  assert.equal(cli("status", control).assigned, "First presenter");
  const file = join(temp, "agent.webm");
  await writeFile(file, Buffer.from(source));
  cli("prepare", first.url, "--input", file, "--audio", "--dry-run");
  assert.equal(cli("status", first.url).me.status, "Finished");
  await screen.close();
  cli("view", display, "--dry-run");
  const consumer = run(["view", "--url", display]);
  const received = [];
  consumer.stdout.on("data", chunk => received.push(chunk));
  const consumerDone = once(consumer, "exit");
  const producer = run(["prepare", "--url", first.url, "--input", file, "--audio"]);
  assert.equal((await line(producer)).status, "Ready");
  await controlPage.getByText("First presenter is presenting").waitFor();
  cli("skip", control);
  assert.equal((await consumerDone)[0], 0);
  assert.deepEqual(Buffer.concat(received).subarray(0, 4), Buffer.from([0x1a, 0x45, 0xdf, 0xa3]));
  cli("end", control);
  assert.equal(cli("status", control).ended, true);
  await first.page.close();
  await second.page.close();
  await controlPage.close();
  return control;
}

async function shapedLink(browser, control) {
  const port = await freePort();
  const data = join(temp, "principal");
  await mkdir(data);
  const addr = "127.0.0.1:" + port;
  run(["serve", "--addr", addr, "--data", data]);
  for (let attempt = 0; attempt < 100; attempt++) {
    if ((await fetch("http://" + addr + "/healthz").catch(() => null))?.ok) break;
    await new Promise(resolve => setTimeout(resolve, 50));
  }
  const page = await browser.newPage();
  await page.goto("http://" + addr + "/");
  await page.getByLabel("Private session link").fill(control);
  await page.getByRole("button", { name: "Connect" }).click();
  await page.getByText("Session ended", { exact: true }).waitFor();
  const reject = await fetch("http://" + addr + "/api/events?target=other.example:9&role=control");
  assert.equal(reject.status, 400);
  await page.close();
}

try {
  build(app, ".");
  build(mesh, "./checks/meshfixture");
  const args = ["--autoplay-policy=no-user-gesture-required"];
  const chrome = await chromium.launch({ channel: "chrome", args });
  browsers.push(chrome);
  const edge = await chromium.launch({ channel: "msedge", args });
  browsers.push(edge);
  const control = await rehearsal(chrome, edge);
  await shapedLink(chrome, control);
  console.log(process.platform + ": Chrome projector, Edge presenter, CLI media I/O and shaped relay passed");
  await rehearsal(edge, chrome);
  console.log(process.platform + ": Edge projector and Chrome presenter passed");
} finally {
  for (const browser of browsers) await browser.close().catch(() => {});
  for (const child of children) {
    const done = once(child, "exit");
    child.kill();
    await done.catch(() => {});
  }
  await rm(temp, { recursive: true, force: true });
}
