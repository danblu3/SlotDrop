// Drives the real SlotDrop.exe the way a person would, for the machine tests
// in .github/workflows/machine-test.yml. It starts a fake RomM, opens the app
// with WebView2's debugging port on, picks a scratch card folder, signs in
// with an API token, adds two games, checks what landed on the card, then
// reopens the app to check the sign-in was remembered.
//
// Writes screenshots and result.json to SMOKE_OUT, and a pass/fail table to
// the job summary. Not part of the app.
//
// Environment:
//   SMOKE_SPEC     name of this machine, for the report
//   SMOKE_EXE      path to the test build of SlotDrop.exe (see the workflow)
//   SMOKE_FAKE     path to fakeromm.exe
//   SMOKE_CARD     scratch folder to use as the SD card (made if missing)
//   SMOKE_OUT      folder for screenshots and result.json
//   SMOKE_SCALE    optional WebView2 device scale factor, e.g. 1.5
//   SMOKE_PAGE_URL dry run only: drive this page in a browser instead of the exe
import { chromium } from "playwright-core";
import { spawn, execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

const env = process.env;
const spec = env.SMOKE_SPEC || "local";
const out = path.resolve(env.SMOKE_OUT || "smoke-out");
const card = path.resolve(env.SMOKE_CARD || path.join(out, "card"));
const rommAddr = "127.0.0.1:8737";
const rommURL = `http://${rommAddr}`;
const token = "ci-token";
const cdpPort = 9222;
const dryRun = !!env.SMOKE_PAGE_URL;

fs.mkdirSync(out, { recursive: true });
const checks = [];
const children = [];
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function check(name, ok, detail = "") {
  checks.push({ name, ok: !!ok, detail });
  console.log(`${ok ? "PASS" : "FAIL"}  ${name}${detail ? "  (" + detail + ")" : ""}`);
  return !!ok;
}

async function waitFor(what, fn, timeoutMs) {
  const end = Date.now() + timeoutMs;
  let last;
  while (Date.now() < end) {
    try {
      const v = await fn();
      if (v) return v;
    } catch (e) {
      last = e;
    }
    await sleep(500);
  }
  throw new Error(`timed out waiting for ${what}${last ? ": " + last.message : ""}`);
}

// desktopShot saves the whole Windows screen, so a crash dialog or a
// "WebView2 is missing" box shows up too.
function desktopShot(name) {
  if (process.platform !== "win32") return;
  try {
    execFileSync("powershell", ["-NoProfile", "-ExecutionPolicy", "Bypass", "-File",
      path.join(import.meta.dirname, "screenshot.ps1"), path.join(out, name)], { stdio: "inherit" });
  } catch (e) {
    console.log("desktop screenshot failed:", e.message);
  }
}

// machineInfo notes the Windows version, WebView2 version and region, for the report.
function machineInfo() {
  if (process.platform !== "win32") return {};
  const ps = `
    $os = Get-CimInstance Win32_OperatingSystem
    $wv = $null
    foreach ($k in 'HKLM:\\SOFTWARE\\WOW6432Node\\Microsoft\\EdgeUpdate\\Clients\\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}',
                   'HKLM:\\SOFTWARE\\Microsoft\\EdgeUpdate\\Clients\\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}',
                   'HKCU:\\SOFTWARE\\Microsoft\\EdgeUpdate\\Clients\\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}') {
      if (-not $wv) { $wv = (Get-ItemProperty -Path $k -Name pv -ErrorAction SilentlyContinue).pv }
    }
    @{ windows = "$($os.Caption) $($os.Version)"; cpu = $env:PROCESSOR_ARCHITECTURE;
       webview2 = $(if ($wv) { $wv } else { 'not found' }); region = (Get-Culture).Name } | ConvertTo-Json -Compress`;
  try {
    return JSON.parse(execFileSync("powershell", ["-NoProfile", "-Command", ps], { encoding: "utf8" }));
  } catch (e) {
    return { error: e.message };
  }
}

function makeCard() {
  for (const d of ["System", "Games/GBA", "Games/GB", "Games/GBC", "Labels/GBA", "BIOS"]) {
    fs.mkdirSync(path.join(card, d), { recursive: true });
  }
  fs.writeFileSync(path.join(card, "System", "slot"), "not really slot");
}

function files(dir) {
  try { return fs.readdirSync(path.join(card, dir)); } catch { return []; }
}

async function startFakeRomm() {
  if (dryRun) return; // the dry run starts it itself
  const p = spawn(env.SMOKE_FAKE, ["-addr", rommAddr, "-token", token], { stdio: ["ignore", "inherit", "inherit"] });
  children.push(p);
  await waitFor("the fake RomM", async () => (await fetch(rommURL + "/health")).ok, 30000);
}

let app = null;
async function launchApp() {
  const args = [`--remote-debugging-port=${cdpPort}`];
  if (env.SMOKE_SCALE) args.push(`--force-device-scale-factor=${env.SMOKE_SCALE}`);
  app = spawn(env.SMOKE_EXE, [], {
    env: { ...env, SLOTDROP_TEST_WEBVIEW_ARGS: args.join(" ") }, // see "Build the test copy" in the workflow
    stdio: "ignore",
  });
  children.push(app);
  const started = Date.now();
  await waitFor("WebView2 to start inside SlotDrop", async () => {
    if (app.exitCode !== null) throw new Error(`SlotDrop quit with code ${app.exitCode}`);
    return (await fetch(`http://127.0.0.1:${cdpPort}/json/version`)).ok;
  }, 90000);
  return Date.now() - started;
}

function stopApp() {
  if (app && app.exitCode === null) {
    try { execFileSync("taskkill", ["/PID", String(app.pid), "/T", "/F"], { stdio: "ignore" }); } catch {}
  }
  app = null;
}

// connect returns the SlotDrop page, either inside the real app or (dry run) in a browser.
async function connect() {
  if (dryRun) {
    const browser = await chromium.launch();
    const page = await browser.newPage({ viewport: { width: 1180, height: 780 } });
    await page.goto(env.SMOKE_PAGE_URL);
    return { browser, page };
  }
  const browser = await chromium.connectOverCDP(`http://127.0.0.1:${cdpPort}`);
  const page = await waitFor("the SlotDrop window", async () => browser.contexts().flatMap((c) => c.pages())[0], 30000);
  return { browser, page };
}

// press clicks an element through the page itself (the app's own click handler runs).
async function press(page, selector) {
  const el = page.locator(selector).first();
  await el.waitFor({ state: "attached", timeout: 15000 });
  await el.evaluate((e) => { e.scrollIntoView({ block: "center" }); e.click(); });
}

async function shot(page, name) {
  try {
    await page.screenshot({ path: path.join(out, name) });
  } catch (e) {
    console.log(`screenshot ${name} failed: ${e.message}`);
  }
}

async function firstRun() {
  const startMs = await (dryRun ? 0 : launchApp());
  check("SlotDrop opens", true, dryRun ? "dry run" : `window ready in ${(startMs / 1000).toFixed(1)}s`);
  const { browser, page } = await connect();
  page.on("pageerror", (e) => check("No script errors in the window", false, e.message));
  await page.waitForSelector(".setup, #search", { timeout: 30000 });
  await shot(page, "1-first-screen.png");

  // Step 1: the card. "Choose folder…" opens Windows' folder picker, which a
  // script can't click through, so answer it with the scratch card's path.
  // Everything after the picker is the app's own code.
  if (await page.locator("text=Choose folder…").count()) {
    await page.evaluate((p) => { window.go.main.App.BrowseForCard = async () => p; }, card);
    await page.click("text=Choose folder…");
    // The scratch folder isn't a removable drive, so the app asks first.
    const anyway = page.locator("button", { hasText: "Use it anyway" });
    if (await anyway.waitFor({ timeout: 10000 }).then(() => true, () => false)) await anyway.click();
    await page.waitForSelector(".step.done", { timeout: 15000 });
  }
  check("Picks the SD card folder", await page.locator(".step.done").count() >= 1 || dryRun);

  // Step 2: RomM, with an API token.
  await page.click("button:has-text('API token')");
  await page.fill("input[placeholder='romm.example.com']", rommURL);
  await page.fill("input[type=password]", token);
  await page.click("button[type=submit]");
  const signedIn = await page.waitForSelector("#search", { timeout: 60000 }).then(() => true, () => false);
  if (!check("Signs in to RomM with a token", signedIn)) {
    await shot(page, "2-sign-in-failed.png");
    return browser;
  }
  await page.waitForSelector(".tile[data-rom]", { timeout: 30000 }).catch(() => {});
  const tiles = await page.locator(".tile[data-rom]").count();
  check("Lists the RomM games", tiles >= 6, `${tiles} games shown`);
  await sleep(1500); // let the pictures load
  await shot(page, "2-library.png");

  // Add a GBA game with a cartridge photo: the game, its label and a shell colour.
  // The + button only shows on hover, and on a small or scaled screen the
  // tile can be off to the side, so press it directly rather than by mouse.
  await press(page, `.tile[data-rom="1"] button.add`);
  const toast = await page.waitForSelector("text=is on your card, label and all", { timeout: 60000 }).then(() => true, () => false);
  if (!check("Says the game was added with its label", toast)) await shot(page, "3-add-failed.png");
  if (!dryRun) {
    const stem = "Test Quest (World)";
    check("Copies the game to Games/GBA", files("Games/GBA").includes(stem + ".gba"), files("Games/GBA").join(", ") || "folder empty");
    const lbl = path.join(card, "Labels", "GBA", stem + ".png");
    const lblOK = fs.existsSync(lbl) && fs.readFileSync(lbl).subarray(1, 4).toString() === "PNG";
    check("Makes the label in Labels/GBA", lblOK, lblOK ? `${Math.round(fs.statSync(lbl).size / 1024)} KB` : "missing");
    let shells = "";
    try { shells = fs.readFileSync(path.join(card, "Labels", "cart_shell.txt"), "utf8"); } catch {}
    check("Writes the shell colour to Labels/cart_shell.txt", shells.includes(stem + " ="), shells.trim().split("\n").find((l) => l.startsWith(stem)) || "no line for the game");
  }

  // Add a Game Boy Color game that RomM stores as a zip.
  await press(page, `.tile[data-rom="3"] button.add`);
  if (!dryRun) {
    const unzipped = await waitFor("the GBC game", async () => files("Games/GBC").includes("Pocket Garden (World).gbc"), 60000).catch(() => false);
    check("Unzips a zipped game into Games/GBC", unzipped, files("Games/GBC").join(", ") || "folder empty");
  }
  await sleep(1500);
  await shot(page, "3-games-added.png");

  // The game panel with the label on the drawn cartridge.
  await press(page, `.tile[data-rom="1"]`);
  await page.waitForSelector(".drawer.open", { timeout: 10000 }).catch(() => {});
  await sleep(1000);
  await shot(page, "4-game-panel.png");
  desktopShot("desktop.png");
  return browser;
}

// secondRun reopens the app: the saved token should sign in by itself.
async function secondRun() {
  await launchApp();
  const { browser, page } = await connect();
  const ok = await page.waitForFunction((url) => [...document.querySelectorAll(".step.done .step-summary")]
    .some((e) => e.textContent.includes(url)), rommAddr, { timeout: 60000 }).then(() => true, () => false);
  check("Remembers the sign-in after reopening", ok);
  await shot(page, "5-reopened.png");
  return browser;
}

function report() {
  const passed = checks.length > 0 && checks.every((c) => c.ok);
  fs.writeFileSync(path.join(out, "result.json"), JSON.stringify({ spec, passed, info, checks }, null, 2));
  if (env.GITHUB_STEP_SUMMARY) {
    const about = Object.entries(info).map(([k, v]) => `${k}: ${v}`).join(" · ");
    const rows = checks.map((c) => `| ${c.ok ? "✅" : "❌"} | ${c.name} | ${c.detail.replace(/\|/g, "/")} |`);
    fs.appendFileSync(env.GITHUB_STEP_SUMMARY, `### ${passed ? "✅" : "❌"} ${spec}\n\n${about}\n\n| | Check | Detail |\n|---|---|---|\n${rows.join("\n")}\n\n`);
  }
  return passed;
}

let passed = false;
const info = machineInfo();
console.log("machine:", JSON.stringify(info));
try {
  makeCard();
  await startFakeRomm();
  let browser = await firstRun();
  await browser.close().catch(() => {});
  if (!dryRun && checks.every((c) => c.ok)) {
    stopApp();
    await sleep(2000);
    browser = await secondRun();
    await browser.close().catch(() => {});
  }
} catch (e) {
  check("Test ran to the end", false, e.message.split("\n")[0]);
  if (process.platform === "win32") { // what WebView2 was started with, and what windows are open
    try {
      console.log(execFileSync("powershell", ["-NoProfile", "-Command",
        "Get-CimInstance Win32_Process -Filter \"name='msedgewebview2.exe' or name='SlotDrop-test.exe'\" | Select-Object -First 3 Name, CommandLine | Format-List | Out-String -Width 400;" +
        "Get-Process | Where-Object MainWindowTitle | Format-Table Name, MainWindowTitle -AutoSize | Out-String"], { encoding: "utf8" }));
    } catch {}
  }
  desktopShot("desktop-at-failure.png");
} finally {
  stopApp();
  for (const c of children) if (c.exitCode === null) c.kill();
  passed = report();
}
process.exit(passed ? 0 : 1);
