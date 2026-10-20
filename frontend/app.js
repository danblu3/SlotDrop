// SlotDrop: the window.
//
// Everything shown here that came from RomM or the SD card is put on the page
// as text (textContent), never as HTML, so a strange game name can't run code.
"use strict";

const api = () => window.go.main.App;
const rt = () => window.runtime;

// ---- tiny DOM helpers -------------------------------------------------------

function h(tag, attrs, ...children) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v === undefined || v === null || v === false) continue;
    if (k === "class") el.className = v;
    else if (k === "style" && typeof v === "object") {
      for (const [sk, sv] of Object.entries(v)) sk.startsWith("--") ? el.style.setProperty(sk, sv) : (el.style[sk] = sv);
    }
    else if (k.startsWith("on")) el.addEventListener(k.slice(2), v);
    else if (k === "html") el.innerHTML = v; // only ever used with our own icon strings
    else if (k in el && k !== "list") el[k] = v;
    else el.setAttribute(k, v === true ? "" : v);
  }
  for (const c of children.flat()) {
    if (c === null || c === undefined || c === false) continue;
    el.append(c instanceof Node ? c : document.createTextNode(String(c)));
  }
  return el;
}
const icon = (name, cls = "") => h("span", { class: "icon-wrap " + cls, html: ICONS[name].replace("<svg", '<svg class="icon"') }).firstChild;
// Console icons: RomM's own when the server has them, else our drawings.
const rommIcons = {}; // key -> "loading" | true | false
const rommIconImg = (key) => h("img", { src: `/img/platform/${key}`, alt: "", draggable: false });
function platformIcon(key, cls = "") {
  const span = h("span", { class: `platform-icon p-${key} ${cls}` });
  if (rommIcons[key] === true) span.append(rommIconImg(key));
  else span.innerHTML = PLATFORM_ICONS[key];
  if (key !== "ALL" && state.connected && rommIcons[key] === undefined) {
    rommIcons[key] = "loading";
    const probe = new Image();
    probe.onload = () => {
      rommIcons[key] = true;
      document.querySelectorAll(`.platform-icon.p-${key}`).forEach((el) => el.replaceChildren(rommIconImg(key)));
    };
    probe.onerror = () => { rommIcons[key] = false; };
    probe.src = `/img/platform/${key}`;
  }
  return span;
}

function mb(bytes) {
  if (!bytes) return "";
  if (bytes >= 1 << 30) return (bytes / (1 << 30)).toFixed(1) + " GB";
  if (bytes >= 1 << 20) return (bytes / (1 << 20)).toFixed(bytes >= 10 << 20 ? 0 : 1) + " MB";
  return Math.max(1, Math.round(bytes / 1024)) + " KB";
}
const errText = (e) => String(e && e.message ? e.message : e || "Something went wrong.");
const plural = (n, one, many) => `${n} ${n === 1 ? one : many || one + "s"}`;

function toast(text, bad = false) {
  const t = h("div", { class: "toast" + (bad ? " bad" : "") }, icon(bad ? "warn" : "check"), h("span", {}, text));
  document.getElementById("toasts").append(t);
  setTimeout(() => t.remove(), bad ? 7000 : 4000);
}

// ---- state ------------------------------------------------------------------

const PLATFORMS = ["GBA", "GB", "GBC"];
const PAGE = 120;
const RECENT = 12; // the row shows as many as fit

const state = {
  start: null,
  card: null,
  connected: false,
  connection: null,
  connecting: false,
  games: [],
  platform: "ALL",
  query: "",
  shown: PAGE,
  busy: {},          // romId -> {step, frac}
  labelVersion: Date.now(),
  drawer: null,      // the game open in the drawer
  popover: null,     // "card" | "romm" | null
  choosingCard: false,
  selected: new Map(), // romId -> game, ticked to add together
  cartColours: {},     // romId -> the plastic's colour in RomM's photo ("" if grey or none)
};

// One add at a time, so two games never write the card at once.
let queue = Promise.resolve();

// ---- start --------------------------------------------------------------------

async function boot() {
  document.getElementById("win-min").onclick = () => rt().WindowMinimise();
  document.getElementById("win-max").onclick = () => rt().WindowToggleMaximise();
  document.getElementById("win-close").onclick = () => rt().Quit();
  document.querySelector(".titlebar").addEventListener("dblclick", (e) => {
    if (!e.target.closest(".win-buttons")) rt().WindowToggleMaximise();
  });

  rt().EventsOn("add-progress", ({ romId, step, frac }) => {
    const label = step === "download" ? `Downloading ${Math.round(frac * 100)}%` : step === "label" ? "Making label" : "Done";
    state.busy[romId] = { step: label, frac: step === "download" ? frac : 1 };
    updateBusy(romId);
  });

  state.start = await api().Start();
  state.card = state.start.card;
  if (state.start.savedLogin) {
    state.connecting = true;
    render();
    try {
      await api().ConnectSaved();
      await afterConnect();
    } catch (e) {
      state.connectError = errText(e);
    }
    state.connecting = false;
  }
  render();
}

async function afterConnect() {
  state.connected = true;
  for (const k in rommIcons) delete rommIcons[k]; // a different server may have different icons
  state.connection = await api().Connection();
  await reload();
}

async function reload() {
  const [games, card] = await Promise.all([api().Library(), state.card ? api().Card() : null]);
  state.games = games || [];
  if (card) state.card = card;
  if (state.drawer) {
    const d = state.drawer;
    state.drawer = state.games.find((g) => (d.stem && g.onCard && g.stem === d.stem && g.platform === d.platform) || (!d.onCard && g.romId && g.romId === d.romId)) || d;
  }
}

// ---- rendering --------------------------------------------------------------------

function render() {
  const app = document.getElementById("app");
  app.replaceChildren(!state.card || !state.connected || state.choosingCard ? setupScreen() : mainScreen());
}

// The first-run screen: pick the card, then sign in to RomM.
function setupScreen() {
  const s = state.start || {};
  const cardDone = !!state.card && !state.choosingCard;

  // Step 1: the SD card.
  let cardBody;
  if (cardDone) {
    cardBody = null;
  } else {
    const drives = (s.drives || []);
    const anySlot = drives.some((d) => d.isSlot && d.removable && !d.system);
    const what = (d) => d.isSlot && d.removable ? "Slot card" : d.isSlot ? "Has Slot, but isn't removable" : (d.name ? d.name + " · " : "") + "no Slot on it";
    cardBody = h("div", {},
      !anySlot && h("div", { class: "noslot" }, icon("sd"), h("div", {},
        h("b", {}, "No Slot card found"),
        h("small", {}, "Put your SD card with Slot on it in the PC, then press Refresh."))),
      drives.length ? h("div", { class: "drives" }, drives.map((d) =>
        h("button", { class: "drive" + (d.isSlot && d.removable ? " slot" : ""), onclick: () => chooseCard(d.path) },
          icon("sd"),
          h("div", {}, h("b", {}, d.path.replace(/\\$/, "")), h("small", {}, what(d)))))) : null,
      h("div", { class: "row", style: { marginTop: "12px" } },
        h("button", { class: "btn", onclick: browseCard }, "Choose folder…"),
        h("button", { class: "btn ghost small", onclick: async () => { s.drives = await api().Drives(); render(); } }, "Refresh")),
      state.cardError && h("div", { class: "error" }, state.cardError));
  }
  const step1 = h("section", { class: "step" + (cardDone ? " done" : "") },
    h("div", { class: "step-head" },
      h("div", { class: "step-num" }, cardDone ? icon("check") : "1"),
      h("div", { class: "step-title" }, "Your SD card"),
      cardDone && h("span", { class: "step-summary" }, `${state.card.path} · ${plural(state.card.games, "game")}`),
      cardDone && h("button", { class: "btn ghost small", onclick: () => { state.choosingCard = true; render(); } }, "Change")),
    cardBody);

  // Step 2: RomM.
  const form = { mode: s.useToken ? "token" : "password" };
  const url = h("input", { class: "input", placeholder: "romm.example.com", value: s.rommUrl || "", spellcheck: false, autocomplete: "off" });
  const user = h("input", { class: "input", placeholder: "Username", value: s.username || "", spellcheck: false, autocomplete: "username" });
  const secret = h("input", { class: "input", type: "password", placeholder: "Password", autocomplete: "current-password" });
  const remember = h("input", { type: "checkbox", checked: true });
  const httpWarn = h("div", { class: "warnline", hidden: true }, icon("warn"), "This address isn't encrypted (http), so your password or token (including one from phone sign-in) is sent as plain text on your network. Use the https address if you can.");
  url.oninput = () => { httpWarn.hidden = !/^\s*http:\/\//i.test(url.value); };
  url.oninput();
  const userField = h("div", { class: "field" }, h("label", {}, "Username"), user);
  const secretLabel = h("label", {}, "Password");
  const secretField = h("div", { class: "field" }, secretLabel, secret);
  const phoneNote = h("div", { class: "note", hidden: true }, icon("lock"), "SlotDrop shows a QR code. Scan it with your phone, approve SlotDrop in RomM, and you're in. Nothing to type. Needs RomM 5.3 or newer.");
  const modes = ["password", "token", "phone"];
  const setMode = (m) => {
    form.mode = m;
    userField.hidden = m !== "password";
    secretField.hidden = m === "phone";
    phoneNote.hidden = m !== "phone";
    secretLabel.textContent = m === "token" ? "API token" : "Password";
    secret.placeholder = m === "token" ? "Paste your RomM API token" : "Password";
    go.replaceChildren(m === "phone" ? "Show QR code" : "Connect");
    seg.querySelectorAll("button").forEach((b, i) => b.classList.toggle("on", modes[i] === m));
  };
  const seg = h("div", { class: "segmented" },
    h("button", { type: "button", onclick: () => setMode("password") }, "Password"),
    h("button", { type: "button", onclick: () => setMode("token") }, "API token"),
    h("button", { type: "button", onclick: () => setMode("phone") }, "Phone (QR)"));
  const err = h("div", { class: "error", hidden: !state.connectError }, state.connectError || "");
  const go = h("button", { class: "btn primary", type: "submit", style: { width: "100%", height: "44px", marginTop: "14px" } }, "Connect");

  const fail = (ex) => {
    err.textContent = errText(ex);
    err.hidden = false;
    go.disabled = false;
    setMode(form.mode);
  };
  const submit = async (e) => {
    e.preventDefault();
    err.hidden = true;
    go.disabled = true;
    if (form.mode === "phone") {
      go.replaceChildren(h("span", { class: "spinner" }), "Asking RomM for a code…");
      try {
        const info = await api().StartPairing(url.value, remember.checked);
        go.disabled = false;
        setMode("phone");
        waitForPhone(info);
      } catch (ex) { fail(ex); }
      return;
    }
    go.replaceChildren(h("span", { class: "spinner" }), "Connecting and loading your library…");
    try {
      await api().Connect(url.value, user.value, secret.value, form.mode === "token", remember.checked);
      secret.value = "";
      state.connectError = "";
      await afterConnect();
      render();
    } catch (ex) { fail(ex); }
  };

  // While pairing: the QR code takes the form's place until the phone says yes.
  const pairBox = h("div", { class: "pair", hidden: true });
  const waitForPhone = async (info) => {
    formEl.hidden = true;
    pairBox.hidden = false;
    const status = h("div", { class: "row muted" }, h("span", { class: "spinner" }), "Waiting for you to approve it in RomM…");
    let cancelled = false;
    pairBox.replaceChildren(
      h("img", { class: "qr", src: info.qr, alt: "QR code that opens RomM's pairing page" }),
      h("div", { class: "pair-text" },
        h("b", {}, "Scan this with your phone"),
        h("p", {}, "It opens your RomM. Sign in there if asked, then approve SlotDrop."),
        h("p", { class: "muted" }, "Or open ", h("span", { class: "mono" }, info.link.replace(/\?.*$/, "")), " and enter the code"),
        h("div", { class: "pair-code" }, info.code),
        status,
        h("button", { class: "btn ghost small", type: "button", onclick: () => { cancelled = true; api().CancelPairing(); pairBox.hidden = true; formEl.hidden = false; } }, "Cancel")));
    try {
      await api().WaitForPairing();
      state.connectError = "";
      status.replaceChildren(h("span", { class: "spinner" }), "Approved. Loading your library…");
      await afterConnect();
      render();
    } catch (ex) {
      if (cancelled) return;
      pairBox.hidden = true;
      formEl.hidden = false;
      fail(ex);
    }
  };

  let step2Body, formEl;
  if (state.connecting) {
    step2Body = h("div", { class: "row muted" }, h("span", { class: "spinner" }), "Signing in with your saved login…");
  } else if (state.connected) {
    step2Body = null;
  } else {
    formEl = h("form", { onsubmit: submit },
      h("div", { class: "field" }, h("label", {}, "RomM address"), url, httpWarn),
      h("div", { style: { marginBottom: "12px" } }, seg),
      userField,
      secretField,
      phoneNote,
      h("label", { class: "check" }, remember, "Remember me on this PC"),
      h("div", { class: "note" }, icon("lock"), "Saved encrypted with Windows' own protection. Only your Windows account on this PC can unlock it."),
      err, go);
    step2Body = h("div", {}, formEl, pairBox);
  }
  const step2 = h("section", { class: "step" + (state.connected ? " done" : "") + (!cardDone ? " locked" : "") },
    h("div", { class: "step-head" },
      h("div", { class: "step-num" }, state.connected ? icon("check") : "2"),
      h("div", { class: "step-title" }, "Your RomM library"),
      state.connected && h("span", { class: "step-summary" }, state.connection.url)),
    step2Body);
  setTimeout(() => setMode(form.mode), 0);

  return h("div", { class: "setup" },
    h("div", { class: "setup-inner" },
      h("h1", {}, "Pick it in RomM. Play it on Slot. Label included."),
      h("p", { class: "lead" }, "Pick a game from RomM and it lands on your card with its cartridge label."),
      step1, step2,
      state.choosingCard && state.card && h("button", { class: "btn ghost", onclick: () => { state.choosingCard = false; render(); } }, "Back")));
}

async function chooseCard(path) {
  state.cardError = "";
  // Anything that doesn't look like a Slot SD card gets a second look first.
  const warning = await api().CardWarning(path);
  if (warning && !(await confirmBox("Use this drive?", warning, "Use it anyway"))) return;
  try {
    state.card = await api().OpenCard(path);
    state.choosingCard = false;
    if (state.connected) await reload();
  } catch (e) {
    state.cardError = errText(e);
  }
  render();
}

async function browseCard() {
  const path = await api().BrowseForCard();
  if (path) chooseCard(path);
}

// ---- main screen ------------------------------------------------------------------

let gridEl = null;
let moreObserver = null; // adds more RomM tiles as the grid scrolls

function mainScreen() {
  const card = state.card;
  const search = h("input", {
    id: "search", type: "search", placeholder: "Search your card and RomM…", value: state.query,
    spellcheck: false, autocomplete: "off",
    oninput: (e) => { state.query = e.target.value; state.shown = PAGE; renderGrid(); },
    onkeydown: (e) => { if (e.key === "Escape") { e.target.value = ""; state.query = ""; renderGrid(); } },
  });

  const counts = { ALL: state.games.length };
  for (const p of PLATFORMS) counts[p] = state.games.filter((g) => g.platform === p).length;
  const chips = ["ALL", ...PLATFORMS].map((p) =>
    h("button", { class: "chip" + (state.platform === p ? " on" : ""), onclick: () => { state.platform = p; state.shown = PAGE; render(); } },
      platformIcon(p), p === "ALL" ? "All" : p, h("span", { class: "count" }, counts[p])));

  const used = card.totalBytes ? card.totalBytes - card.freeBytes : 0;
  const meter = card.totalBytes ? h("div", { class: "space-meter", title: `${mb(card.freeBytes)} free` },
    h("span", {}, `${plural(card.games, "game")} on the card · ${mb(used)} of ${mb(card.totalBytes)} used`),
    h("div", { class: "meter" }, h("i", { style: { width: Math.min(100, (used / card.totalBytes) * 100).toFixed(1) + "%" } }))) :
    h("div", { class: "space-meter" }, plural(card.games, "game") + " on the card");

  const cardPill = h("button", { class: "pill", onclick: (e) => { e.stopPropagation(); togglePopover("card"); } },
    icon("sd"), h("b", {}, card.path.replace(/\\$/, "")));
  const rommPill = h("button", { class: "pill", onclick: (e) => { e.stopPropagation(); togglePopover("romm"); } },
    h("span", { class: "dot" }), "RomM");

  gridEl = h("div", { class: "content" });
  const root = h("div", { style: { display: "contents" } },
    h("div", { class: "topbar" },
      h("div", { style: { width: "150px" } }),
      h("div", { class: "search" }, icon("search"), search, h("kbd", {}, "Ctrl K")),
      h("div", { class: "pills" }, cardPill,
        card.canEject && h("button", { class: "pill icon-only", title: "Eject the SD card", "aria-label": "Eject the SD card", onclick: (e) => eject(e.currentTarget) }, icon("eject")),
        rommPill)),
    h("div", { class: "filterbar" }, chips, h("div", { class: "spacer" }), meter),
    gridEl,
    popoverEl(),
    drawerEl());
  renderGrid();
  return root;
}

function matches(g, terms) {
  const hay = fold(`${g.title} ${g.file} ${g.stem || ""} ${g.platform}`);
  return terms.every((t) => hay.includes(t));
}
const fold = (s) => s.toLowerCase().normalize("NFD").replace(/[\u0300-\u036f]/g, "").replace(/[^a-z0-9 ]+/g, " ");

function renderGrid() {
  if (!gridEl) return;
  const terms = fold(state.query).split(/\s+/).filter(Boolean);
  let list = state.games.filter((g) => state.platform === "ALL" || g.platform === state.platform);
  if (terms.length) list = list.filter((g) => matches(g, terms));
  const onCard = list.filter((g) => g.onCard);
  const fromRomm = list.filter((g) => !g.onCard);
  const missing = onCard.filter((g) => !g.hasLabel);

  const parts = [];
  // A little shelf of your favourites, like Slot's own.
  const favs = onCard.filter((g) => g.favourite);
  if (favs.length && !terms.length) {
    parts.push(h("div", { class: "section-head" }, h("h2", { class: "fav-head" }, icon("star"), "Favourites"), h("span", { class: "muted" }, favs.length)));
    parts.push(h("div", { class: "shelf" }, favs.map(tile)));
  }
  if (onCard.length) {
    parts.push(h("div", { class: "section-head" }, h("h2", {}, "On your card"), h("span", { class: "muted" }, onCard.length)));
    if (missing.length && !terms.length) {
      const btn = h("button", { class: "btn small primary", onclick: () => makeMissing(btn) }, icon("wand"), "Make labels");
      parts.push(h("div", { class: "banner" }, icon("warn"),
        h("span", {}, `${plural(missing.length, "game")} on your card ${missing.length === 1 ? "has" : "have"} no label yet.`), btn));
    }
    parts.push(h("div", { class: "grid" }, onCard.map(tile)));
  }
  // The newest games in RomM, so new hacks are easy to spot.
  if (!terms.length) {
    const recent = fromRomm.filter((g) => g.added).sort((a, b) => (a.added < b.added ? 1 : -1)).slice(0, RECENT);
    if (recent.length) {
      parts.push(h("div", { class: "section-head" }, h("h2", {}, "Recently added to RomM")));
      parts.push(h("div", { class: "grid one-row" }, recent.map(tile)));
    }
  }
  parts.push(h("div", { class: "section-head" },
    h("h2", {}, terms.length ? "In RomM" : "Add from RomM"), h("span", { class: "muted" }, fromRomm.length)));
  if (moreObserver) { moreObserver.disconnect(); moreObserver = null; }
  if (fromRomm.length) {
    const grid = h("div", { class: "grid" }, fromRomm.slice(0, state.shown).map(tile));
    parts.push(grid);
    if (fromRomm.length > state.shown) {
      // Keep adding tiles as you scroll, well before you reach the end.
      const sentinel = h("div", { class: "more-sentinel", "aria-hidden": "true" });
      parts.push(sentinel);
      moreObserver = new IntersectionObserver((entries) => {
        if (!entries.some((e) => e.isIntersecting)) return;
        const next = fromRomm.slice(state.shown, state.shown + PAGE);
        state.shown += next.length;
        grid.append(...next.map(tile));
        if (state.shown >= fromRomm.length) { moreObserver.disconnect(); moreObserver = null; sentinel.remove(); return; }
        // Look again, in case the end is still in reach after this batch.
        moreObserver.unobserve(sentinel);
        moreObserver.observe(sentinel);
      }, { root: gridEl, rootMargin: "0px 0px 1200px 0px" });
      moreObserver.observe(sentinel);
    }
  } else {
    parts.push(h("div", { class: "empty" },
      h("div", { class: "big" }, terms.length ? "Nothing in RomM matches that" : "Nothing else to add"),
      terms.length ? "Try fewer words, or part of the file name." : "Every GBA, GB and GBC game in RomM is already on your card."));
  }
  gridEl.replaceChildren(...parts);
}

// ---- the drawn cartridge -----------------------------------------------------------
//
// A clean cartridge in the shell colour with the whole label filling its recess,
// drawn like Cart Studio's preview (proportions measured from it).

const SVGNS = "http://www.w3.org/2000/svg";
function sv(tag, attrs, ...kids) {
  const el = document.createElementNS(SVGNS, tag);
  for (const [k, v] of Object.entries(attrs || {})) el.setAttribute(k, v);
  for (const k of kids) el.append(k instanceof Node ? k : document.createTextNode(String(k)));
  return el;
}
function shade(hex, amount) {
  const c = [0, 2, 4].map((i) => parseInt(hex.slice(i, i + 2), 16));
  const out = amount >= 1
    ? c.map((v) => Math.min(255, Math.round(v + (255 - v) * (amount - 1) + 8 * (amount - 1))))
    : c.map((v) => Math.round(v * amount));
  return `rgb(${out.join(",")})`;
}
let svgIds = 0;

// labelSrc is a /img/... address, or null for an empty recess.
function cartSVG(platform, shell, labelSrc) {
  const hex = /^[0-9a-f]{6}$/i.test(shell.colour) ? shell.colour : GBA_BLACK.colour;
  // A clear shell reads darker and richer than solid, the way Studio shows it.
  const housing = shell.finish === "clear" ? shade(hex, 0.78) : "#" + hex, edge = shade(hex, 0.62), dark = shade(hex, 0.55), light = shade(hex, 1.35);
  const portrait = platform !== "GBA";
  const W = portrait ? 88 : 166, H = 100;
  const id = "c" + ++svgIds;
  const svg = sv("svg", { viewBox: `0 0 ${W} ${H}`, class: "cart-svg" + (portrait ? " portrait" : ""), role: "img" });
  const seeThrough = shell.finish === "clear" || shell.finish === "glitter";
  const shellGroup = sv("g", seeThrough ? { opacity: shell.finish === "clear" ? "0.88" : "0.86" } : {});
  const bases = []; // the plain shell shapes, for the glitter layer
  const base = (el) => { bases.push(el); return el; };
  let recess;
  if (portrait) {
    // The Game Boy cartridge's own moulding, as Cart Studio draws it: grip ridges
    // at the top corners, the embossed "Nintendo GAME BOY" plate, grooves down
    // both sides and the arrow under the label.
    // Studio's "notched" outline cuts the top right corner, like a real DMG cart.
    const r = 3.2, cut = shell.outline === "notched" ? W * 0.075 : 0;
    shellGroup.append(base(sv("path", {
      d: cut ? `M${r} 0H${W - cut}L${W} ${cut}V${H - r * 1.6}Q${W} ${H} ${W - r * 1.6} ${H}H${r * 1.6}Q0 ${H} 0 ${H - r * 1.6}V${r}Q0 0 ${r} 0Z`
        : `M${r} 0H${W - r}Q${W} 0 ${W} ${r}V${H - r * 1.6}Q${W} ${H} ${W - r * 1.6} ${H}H${r * 1.6}Q0 ${H} 0 ${H - r * 1.6}V${r}Q0 0 ${r} 0Z`,
      fill: housing, stroke: edge, "stroke-width": 0.6,
    })));
    // The lip along the top edge.
    shellGroup.append(sv("line", { x1: 1.2, x2: W - 1.2 - cut, y1: H * 0.03, y2: H * 0.03, stroke: dark, "stroke-width": 0.5 }));
    shellGroup.append(sv("line", { x1: 1.2, x2: W - 1.2 - cut, y1: H * 0.03 + 0.5, y2: H * 0.03 + 0.5, stroke: light, "stroke-width": 0.35, opacity: 0.6 }));
    // Grip ridges in the top corners.
    for (const [x1, x2] of [[W * 0.015, W * 0.085], [W * 0.915, W * 0.985]]) {
      for (let n = 0; n < 8; n++) {
        const y = H * (0.055 + n * 0.024);
        if (cut && x1 > W / 2 && y < cut + 1.5) continue;
        shellGroup.append(sv("line", { x1, x2, y1: y, y2: y, stroke: dark, "stroke-width": 0.55 }));
        shellGroup.append(sv("line", { x1, x2, y1: y + 0.5, y2: y + 0.5, stroke: light, "stroke-width": 0.3, opacity: 0.5 }));
      }
    }
    // Grooves running down both sides.
    for (const x of [W * 0.035, W * 0.965]) {
      shellGroup.append(sv("line", { x1: x - 0.6, x2: x - 0.6, y1: H * 0.28, y2: H * 0.97, stroke: dark, "stroke-width": 0.55 }));
      shellGroup.append(sv("line", { x1: x + 0.6, x2: x + 0.6, y1: H * 0.28, y2: H * 0.97, stroke: light, "stroke-width": 0.35, opacity: 0.55 }));
    }
    // The embossed plate and its lettering.
    const plate = { x: W * 0.13, y: H * 0.065, width: W * 0.74, height: H * 0.14, rx: H * 0.07 };
    shellGroup.append(sv("rect", { ...plate, x: plate.x + 0.4, y: plate.y + 0.4, fill: "none", stroke: light, "stroke-width": 0.7, opacity: 0.55 }));
    shellGroup.append(sv("rect", { ...plate, fill: "none", stroke: dark, "stroke-width": 0.7 }));
    const word = (dy, fill, op) => sv("text", { x: W / 2, y: plate.y + plate.height / 2 + dy, "text-anchor": "middle", "dominant-baseline": "central", fill, opacity: op, class: "cart-logo gb" },
      sv("tspan", { "font-size": W * 0.052 }, "Nintendo "),
      sv("tspan", { "font-size": W * 0.078 }, "GAME BOY"),
      sv("tspan", { "font-size": W * 0.03, dy: -W * 0.025 }, "\u2122"));
    shellGroup.append(word(0.35, light, 0.5), word(0, dark, 0.9));
    // The arrow under the label, pressed in.
    const ax = W / 2, at = H * 0.893, ab = H * 0.95, aw = W * 0.085;
    shellGroup.append(sv("polygon", { points: `${ax - aw},${at} ${ax + aw},${at} ${ax},${ab}`, fill: shade(hex, 0.85) }));
    shellGroup.append(sv("polyline", { points: `${ax - aw},${at} ${ax + aw},${at}`, stroke: dark, "stroke-width": 0.6, fill: "none" }));
    shellGroup.append(sv("polyline", { points: `${ax - aw},${at} ${ax},${ab} ${ax + aw},${at}`, stroke: light, "stroke-width": 0.45, fill: "none", opacity: 0.6, "stroke-linejoin": "round" }));
    recess = [W * 0.12, H * 0.255, W * 0.88, H * 0.875];
  } else {
    // The wide grip at the top, then the slightly narrower body.
    // One outline for both, so a clear shell shows a single inner edge.
    const t = 0.3, gy = H * 0.27 + t, R = W / 20, R2 = W * 0.02, bx0 = W * 0.025, bx1 = W * 0.975, rb = W / 34, by = H - t;
    shellGroup.append(base(sv("path", {
      d: `M${t + R} ${t}H${W - t - R}A${R} ${R} 0 0 1 ${W - t} ${t + R}V${gy - R2}A${R2} ${R2} 0 0 1 ${W - t - R2} ${gy}H${bx1}` +
        `V${by - rb}A${rb} ${rb} 0 0 1 ${bx1 - rb} ${by}H${bx0 + rb}A${rb} ${rb} 0 0 1 ${bx0} ${by - rb}V${gy}H${t + R2}` +
        `A${R2} ${R2} 0 0 1 ${t} ${gy - R2}V${t + R}A${R} ${R} 0 0 1 ${t + R} ${t}Z`,
      fill: housing, stroke: edge, "stroke-width": 0.6,
    })));
    shellGroup.append(sv("path", { d: `M${W * 0.12} ${H * 0.2}Q${W / 2} ${H * 0.0} ${W * 0.88} ${H * 0.2}`, fill: "none", stroke: light, "stroke-width": 0.6, opacity: 0.7 }));
    const text = shade(hex, 0.75);
    shellGroup.append(sv("text", { x: W / 2, y: H * 0.155, "text-anchor": "middle", "dominant-baseline": "middle", fill: text, class: "cart-logo" },
      sv("tspan", { "font-weight": 700, "font-style": "italic", "font-size": W * 0.055 }, "GAME BOY "),
      sv("tspan", { "font-size": W * 0.042, "letter-spacing": W * 0.012 }, "ADVANCE")));
    const cx = W / 2, cy = H * 0.955;
    shellGroup.append(sv("polyline", { points: `${cx - W * 0.07},${cy - H * 0.02} ${cx},${cy + H * 0.012} ${cx + W * 0.07},${cy - H * 0.02}`, fill: "none", stroke: dark, "stroke-width": 0.8, "stroke-linejoin": "round", "stroke-linecap": "round" }));
    recess = [W * 0.1, H * 0.215, W * 0.913, H * 0.92];
  }
  if (shell.finish === "glitter") {
    // Fine flakes scattered at random over a big tile, so no grid shows:
    // mostly small and dim, a few bright, some darker than the plastic.
    let seed = 7;
    const rnd = () => ((seed = (seed * 16807) % 2147483647) / 2147483647);
    const T = 34, flakes = [];
    const tints = [light, shade(hex, 1.6), dark, "#fff"];
    for (let n = 0; n < 120; n++) {
      const tint = tints[Math.min(3, Math.floor(rnd() * rnd() * 5))];
      flakes.push(sv("circle", { cx: (rnd() * T).toFixed(2), cy: (rnd() * T).toFixed(2), r: (0.1 + rnd() * rnd() * 0.32).toFixed(2), fill: tint, opacity: (0.25 + rnd() * 0.65).toFixed(2) }));
    }
    const pat = sv("pattern", { id: id + "g", width: T, height: T, patternUnits: "userSpaceOnUse" }, ...flakes);
    svg.append(sv("defs", {}, pat));
    for (const b of bases) {
      const sparkle = b.cloneNode();
      sparkle.setAttribute("fill", `url(#${id}g)`);
      sparkle.removeAttribute("stroke");
      shellGroup.append(sparkle);
    }
  }
  if (seeThrough) svg.append(circuitBoard(W, H, recess));
  if (shell.finish === "clear") {
    // The inside edge of the plastic, seen through the shell. Stroking the
    // silhouette clipped to itself leaves just the inner half of the line.
    const clip = sv("clipPath", { id: id + "k" }, ...bases.map((b) => b.cloneNode()));
    svg.append(sv("defs", {}, clip));
    for (const b of bases) {
      const wall = b.cloneNode();
      wall.setAttribute("fill", "none");
      wall.setAttribute("stroke", dark);
      wall.setAttribute("stroke-width", W * 0.045);
      wall.setAttribute("clip-path", `url(#${id}k)`);
      wall.setAttribute("opacity", "0.55");
      shellGroup.append(wall);
    }
  }
  svg.append(shellGroup);
  // The recess: dark on the top and left, light on the bottom and right, like it's pressed in.
  const [rl, rt, rr, rb] = recess, bev = W * 0.016;
  svg.append(sv("polygon", { points: `${rl},${rt} ${rr},${rt} ${rr - bev},${rt + bev} ${rl + bev},${rb - bev} ${rl},${rb}`, fill: dark }));
  svg.append(sv("polygon", { points: `${rr},${rt} ${rr},${rb} ${rl},${rb} ${rl + bev},${rb - bev} ${rr - bev},${rt + bev}`, fill: light }));
  const inner = { x: rl + bev, y: rt + bev, width: rr - rl - 2 * bev, height: rb - rt - 2 * bev };
  if (labelSrc) {
    svg.append(sv("image", { ...inner, href: labelSrc, preserveAspectRatio: "xMidYMid slice" }));
  } else {
    svg.append(sv("rect", { ...inner, fill: shade(hex, 0.7) }));
    svg.append(sv("text", { x: inner.x + inner.width / 2, y: inner.y + inner.height / 2, "text-anchor": "middle", "dominant-baseline": "middle", class: "cart-nolabel", "font-size": W * 0.05 }, "NO LABEL"));
  }
  return svg;
}

// The board inside the cartridge. Like Cart Studio, only its contacts show,
// in the strip between the label and the bottom edge.
function circuitBoard(W, H, recess) {
  const [rl, , rr, rb] = recess;
  const x0 = rl, x1 = rr, y0 = rb - H * 0.01, y1 = H * 0.985;
  const g = sv("g", { class: "pcb" });
  g.append(sv("rect", { x: x0, y: y0, width: x1 - x0, height: y1 - y0, fill: "#1d4a33" }));
  const fingers = Math.round((x1 - x0) / (W * 0.028)), pitch = (x1 - x0) / fingers;
  for (let n = 0; n < fingers; n++) {
    g.append(sv("rect", { x: x0 + pitch * (n + 0.22), y: y0 + (y1 - y0) * 0.15, width: pitch * 0.56, height: (y1 - y0) * 0.8, fill: "#a48a3a" }));
  }
  return g;
}

function cardLabelURL(g) {
  return `/img/label/${g.platform}/${encodeURIComponent(g.stem)}?v=${state.labelVersion}`;
}
const cartDrawing = (g) => cartSVG(g.platform, g.shell || GBA_BLACK, g.hasLabel ? cardLabelURL(g) : null);

function tile(g) {
  const id = g.romId;
  let art;
  if (g.onCard) {
    art = h("div", { class: "art" }, cartDrawing(g));
  } else {
    const ph = platformIcon(g.platform, "placeholder");
    // RomM's cartridge photo if it has one, otherwise the box art.
    art = h("div", { class: "art" }, g.hasCart || g.hasCover ? h("img", {
      class: "cover" + (g.hasCart ? " cartphoto" : ""), src: `/img/thumb/${id}`, alt: "", loading: "lazy", decoding: "async",
      onerror: (e) => e.target.replaceWith(ph),
    }) : ph);
  }
  if (g.onCard) {
    art.append(g.hasLabel ? h("div", { class: "badge" }, icon("check"), "On card") : h("div", { class: "badge warn" }, icon("warn"), "No label"));
    art.append(h("button", {
      class: "star" + (g.favourite ? " on" : ""), title: g.favourite ? "Take off Slot's favourites" : "Add to Slot's favourites",
      "aria-label": (g.favourite ? "Unfavourite " : "Favourite ") + g.title, "aria-pressed": String(!!g.favourite),
      onclick: (e) => { e.stopPropagation(); toggleFavourite(g, e.currentTarget); },
    }, icon("star")));
  }
  // While some games are ticked, clicking a game ticks it too.
  const open = () => (state.selected.size && !g.onCard && !state.busy[id] ? toggleSelected(g) : openDrawer(g));
  const el = h("div", { class: "tile" + (state.selected.has(id) ? " selected" : ""), tabindex: 0, "data-rom": id || "", onclick: open, onkeydown: (e) => { if (e.key === "Enter") open(); } },
    art,
    h("div", { class: "meta" },
      h("div", { class: "title", title: g.title }, g.title),
      h("div", { class: "sub" }, platformIcon(g.platform), g.platform, g.size ? " · " + mb(g.size) : "")));
  if (!g.onCard) {
    art.append(h("button", { class: "add", title: "Add to card", "aria-label": "Add to card", onclick: (e) => { e.stopPropagation(); addGame(g); } }, icon("plus")));
    art.append(h("button", { class: "pick", title: "Tick to add several", "aria-label": "Tick " + g.title, onclick: (e) => { e.stopPropagation(); toggleSelected(g); } }, icon("check")));
  }
  if (state.busy[id]) paintBusy(el, state.busy[id]);
  return el;
}

function paintBusy(el, b) {
  el.classList.add("busy");
  const art = el.querySelector(".art");
  art.dataset.step = b.step;
  let bar = el.querySelector(".progress");
  if (!bar) { bar = h("div", { class: "progress" }, h("i")); art.append(bar); }
  bar.firstChild.style.width = Math.round(b.frac * 100) + "%";
  const add = el.querySelector(".add");
  if (add) add.remove();
}

function updateBusy(romId) {
  document.querySelectorAll(`.tile[data-rom="${romId}"]`).forEach((el) => paintBusy(el, state.busy[romId]));
  const dbtn = document.getElementById("drawer-add");
  if (dbtn && state.drawer && state.drawer.romId === romId) dbtn.replaceChildren(h("span", { class: "spinner" }), state.busy[romId].step);
}

// ---- actions ----------------------------------------------------------------------

// addGame queues one game. With quiet set it reports nothing itself and
// resolves to "label", "nolabel" or an error message, for addSelected's summary.
function addGame(g, quiet = false, favourite = false) {
  if (state.busy[g.romId]) return Promise.resolve(null);
  state.busy[g.romId] = { step: "Waiting", frac: 0 };
  updateBusy(g.romId);
  let result;
  queue = queue.then(async () => {
    try {
      const added = await api().AddGame(g.romId);
      state.labelVersion = Date.now();
      if (favourite) await api().SetFavourite(g.platform, added.stem, true).catch(() => {});
      result = added.label ? "label" : "nolabel";
      if (!quiet) toast(added.label ? `${g.title} is on your card, label and all.`
        : `${g.title} is on your card, without a label: ${added.labelNote}. Click it to use your own image.`);
    } catch (e) {
      result = `${g.title}: ${errText(e)}`;
      if (!quiet) toast(`Couldn't add ${g.title}: ${errText(e)}`, true);
    }
    delete state.busy[g.romId];
    await reload();
    refreshMain();
  });
  return queue.then(() => result);
}

// ---- picking several ---------------------------------------------------------------

function toggleSelected(g) {
  if (state.selected.has(g.romId)) state.selected.delete(g.romId);
  else state.selected.set(g.romId, g);
  document.querySelectorAll(`.tile[data-rom="${g.romId}"]`).forEach((el) => el.classList.toggle("selected", state.selected.has(g.romId)));
  paintSelectBar();
}

function clearSelected() {
  state.selected.clear();
  document.querySelectorAll(".tile.selected").forEach((el) => el.classList.remove("selected"));
  paintSelectBar();
}

// The bar along the bottom while games are ticked: how many, how big, and one button.
function paintSelectBar() {
  document.body.classList.toggle("selecting", state.selected.size > 0);
  let bar = document.getElementById("selectbar");
  if (!state.selected.size) { if (bar) bar.remove(); return; }
  const games = [...state.selected.values()];
  const bytes = games.reduce((n, g) => n + (g.size || 0), 0);
  const free = state.card && state.card.freeBytes;
  const tooBig = free > 0 && bytes > free;
  const body = [
    h("span", { class: "count" }, plural(games.length, "game") + " ticked"),
    h("span", { class: "faint" }, bytes ? mb(bytes) + (tooBig ? " · more than the card has free" : "") : ""),
    h("button", { class: "btn ghost small", onclick: clearSelected }, "Clear"),
    h("label", { class: "check small", title: "Add them to Slot's favourites too" },
      h("input", { type: "checkbox", checked: !!state.addAsFavourite, onchange: (e) => { state.addAsFavourite = e.target.checked; } }),
      icon("star"), "Favourite"),
    h("button", { class: "btn primary small", disabled: tooBig, onclick: addSelected }, icon("plus"), "Add " + plural(games.length, "game")),
  ];
  if (!bar) { bar = h("div", { id: "selectbar", role: "toolbar", "aria-label": "Ticked games" }); document.body.append(bar); }
  bar.replaceChildren(...body);
}

async function addSelected() {
  const games = [...state.selected.values()];
  clearSelected();
  const fav = state.addAsFavourite;
  const results = await Promise.all(games.map((g) => addGame(g, true, fav)));
  const labelled = results.filter((r) => r === "label").length;
  const unlabelled = results.filter((r) => r === "nolabel").length;
  const failed = results.filter((r) => r && r !== "label" && r !== "nolabel");
  const added = labelled + unlabelled;
  if (added) {
    toast(`Added ${plural(added, "game")} to your card` + (!unlabelled ? ", labels and all."
      : unlabelled === added ? ", but none had a cartridge photo for a label. Click one to use your own image."
      : `, ${unlabelled} without a label. Click one to use your own image.`));
  }
  if (failed.length) toast(`Couldn't add ${failed.length}: ${failed.join("; ")}`, true);
}

async function makeMissing(btn) {
  btn.disabled = true;
  btn.replaceChildren(h("span", { class: "spinner" }), "Making labels…");
  const res = await api().MakeMissingLabels();
  state.labelVersion = Date.now();
  if (res.made) toast(`Made ${plural(res.made, "label")}.`);
  if (res.problems && res.problems.length) toast(`Couldn't label ${res.problems.length}: ${res.problems.join("; ")}`, true);
  await reload();
  refreshMain();
}

// A small "are you sure?" box. Resolves true if confirmed.
function confirmBox(title, text, okLabel) {
  return new Promise((resolve) => {
    const done = (ok) => { overlay.remove(); document.removeEventListener("keydown", onKey, true); resolve(ok); };
    const onKey = (e) => { if (e.key === "Escape") { e.stopPropagation(); done(false); } };
    const ok = h("button", { class: "btn danger-fill", onclick: () => done(true) }, okLabel);
    const overlay = h("div", { class: "modal-scrim", onclick: (e) => { if (e.target === overlay) done(false); } },
      h("div", { class: "modal", role: "alertdialog", "aria-label": title },
        h("h3", {}, title), h("p", {}, text),
        h("div", { class: "row", style: { justifyContent: "flex-end" } },
          h("button", { class: "btn ghost", onclick: () => done(false) }, "Cancel"), ok)));
    document.body.append(overlay);
    document.addEventListener("keydown", onKey, true);
    ok.focus();
  });
}

async function removeGame(g, btn) {
  const ok = await confirmBox(`Remove ${g.title}?`,
    "The game and its label are deleted from the card. Your save and save states stay, so they're there if you add it again.", "Remove");
  if (!ok) return;
  btn.disabled = true;
  try {
    await api().RemoveGame(g.platform, g.stem, g.file);
    toast(`Removed ${g.title} from your card.`);
    state.drawer = null;
    await reload();
  } catch (e) {
    toast(errText(e), true);
  }
  refreshMain();
}

async function eject(btn) {
  if (Object.keys(state.busy).length) {
    toast("Wait for the games being added to finish, then eject.", true);
    return;
  }
  btn.disabled = true;
  btn.replaceChildren(h("span", { class: "spinner" }));
  const where = state.card.path.replace(/\\$/, "");
  try {
    await queue; // nothing else writing to the card
    await api().Eject();
    state.card = null;
    state.drawer = null;
    state.popover = null;
    state.start.drives = await api().Drives();
    render();
    toast(`${where} is safe to remove. Enjoy!`);
  } catch (e) {
    toast(errText(e), true);
    btn.disabled = false;
    btn.replaceChildren(icon("eject"));
  }
}

// Re-render the main screen but keep the search box focused and the grid and
// drawer scrolled where they were, so finishing a download doesn't jump to the top.
const SCROLLERS = [".content", ".drawer-body"];
function refreshMain() {
  if (!state.card || !state.connected || state.choosingCard) return;
  const focused = document.activeElement && document.activeElement.id === "search";
  const scrolled = SCROLLERS.map((sel) => { const el = document.querySelector(sel); return el ? el.scrollTop : 0; });
  render();
  SCROLLERS.forEach((sel, i) => { const el = document.querySelector(sel); if (el) el.scrollTop = scrolled[i]; });
  if (focused) {
    const s = document.getElementById("search");
    s.focus();
    s.setSelectionRange(s.value.length, s.value.length);
  }
}

// ---- popovers ---------------------------------------------------------------------

function togglePopover(which) {
  state.popover = state.popover === which ? null : which;
  refreshMain();
}

function popoverEl() {
  if (!state.popover) return null;
  const card = state.card;
  let body;
  if (state.popover === "card") {
    const labelled = card.platforms.reduce((n, p) => n + p.labels, 0);
    body = [
      h("h4", {}, "SD card"),
      h("p", {}, `${card.path}\n`),
      h("dl", { class: "facts" },
        h("dt", {}, "Games"), h("dd", {}, card.platforms.filter((p) => p.games).map((p) => `${p.games} ${p.key}`).join(", ") || "none yet"),
        h("dt", {}, "Labels"), h("dd", {}, `${labelled} of ${card.games}`),
        h("dt", {}, "Space"), h("dd", {}, card.totalBytes ? `${mb(card.freeBytes)} free of ${mb(card.totalBytes)}` : "unknown"),
        h("dt", {}, "Slot"), h("dd", {}, card.slotInstalled ? "installed" + (card.bios ? ", GBA BIOS present" : "") : "not found in System")),
      card.canEject && h("button", { class: "btn primary", onclick: (e) => eject(e.currentTarget) }, icon("eject"), "Eject card"),
      h("button", { class: "btn", onclick: async () => { state.popover = null; state.choosingCard = true; state.start.drives = await api().Drives(); render(); } }, "Use a different card"),
    ];
  } else {
    body = [
      h("h4", {}, "RomM"),
      h("p", {}, state.connection.url + (state.connection.secure ? "" : "  (not https)")),
      h("button", { class: "btn", onclick: async (e) => { e.target.disabled = true; await reload(); state.popover = null; refreshMain(); toast("Library refreshed."); } }, "Refresh library"),
      h("button", { class: "btn ghost", onclick: async () => {
        await api().SignOut();
        Object.assign(state, { connected: false, popover: null, games: [], drawer: null });
        state.start.savedLogin = false;
        render();
      } }, "Sign out and forget login"),
    ];
  }
  return h("div", { class: "popover", onclick: (e) => e.stopPropagation() }, body);
}

document.addEventListener("click", () => { if (state.popover) { state.popover = null; refreshMain(); } });
document.addEventListener("keydown", (e) => {
  if ((e.ctrlKey && (e.key === "k" || e.key === "f")) || (e.key === "/" && document.activeElement.tagName !== "INPUT")) {
    const s = document.getElementById("search");
    if (s) { e.preventDefault(); s.focus(); s.select(); }
  }
  if (e.key === "Escape" && state.drawer && document.activeElement.id !== "search") closeDrawer();
  else if (e.key === "Escape" && state.selected.size && document.activeElement.id !== "search") clearSelected();
});

// ---- the game drawer --------------------------------------------------------------

function openDrawer(g) {
  state.drawer = g;
  closeEditor();
  state.pendingShell = null;
  refreshMain();
  const body = document.querySelector(".drawer-body");
  if (body) body.scrollTop = 0;
}
function closeDrawer() {
  state.drawer = null;
  closeEditor();
  refreshMain();
}

function drawerEl() {
  const open = !!state.drawer;
  const scrim = h("div", { class: "scrim" + (open ? " open" : ""), onclick: closeDrawer });
  if (!open) return h("div", { style: { display: "contents" } }, scrim, h("aside", { class: "drawer" }));
  const g = state.drawer;
  // A game not on the card yet is shown in the colour it'll get: its cart
  // photo's plastic if that's coloured, otherwise GBA grey.
  const photoHex = cartColourFor(g);
  const shell = state.pendingShell || g.shell || (!g.onCard && photoHex ? { colour: photoHex, finish: "solid" } : GBA_BLACK);

  // The big picture: the finished cartridge, drawn like Slot shows it.
  const hero = h("div", { class: "hero" });
  const showHero = (sh = shell) => {
    const ed = state.editor;
    let src = null;
    if (ed && ed.token) src = `/img/owncrop/${ed.token}?box=${ed.box.join(",")}`;
    else if (ed) src = `/img/crop/${g.romId}?box=${ed.box.join(",")}`;
    else if (g.onCard && g.hasLabel) src = cardLabelURL(g);
    else if (g.romId && g.hasCart) src = `/img/crop/${g.romId}`;
    if (!src && !g.onCard && g.hasCover) {
      hero.replaceChildren(h("img", { class: "cover-big", src: `/img/cover/${g.romId}`, alt: "" }));
    } else {
      hero.replaceChildren(cartSVG(g.platform, sh, src));
    }
  };
  showHero();

  const body = [hero];

  // The one big action.
  if (g.onCard) {
    body.push(h("div", { class: "primary-action" },
      h("div", { class: "oncard" }, icon("check"), g.hasLabel ? "On your card with its label" : "On your card")));
  } else {
    const busy = state.busy[g.romId];
    body.push(h("div", { class: "primary-action" },
      h("button", { id: "drawer-add", class: "btn primary", disabled: !!busy, onclick: () => addGame(g) },
        busy ? [h("span", { class: "spinner" }), busy.step] : [icon("plus"), "Add to card"])));
  }

  // Label and shell, for games on the card that RomM has a photo of.
  if (g.onCard) body.push(labelGroup(g, shell, showHero));

  // Card games RomM didn't recognise: let you pick the right one.
  if (g.onCard && !g.romId) body.push(matchGroup(g));

  const details = h("div", { class: "group" }, h("h4", {}, "Details"),
    h("dl", { class: "facts" },
      h("dt", {}, "File"), h("dd", {}, g.file),
      g.size ? [h("dt", {}, "Size"), h("dd", {}, mb(g.size))] : null,
      g.regions && g.regions.length ? [h("dt", {}, "Region"), h("dd", {}, g.regions.join(", "))] : null,
      g.onCard ? [h("dt", {}, "In RomM"), h("dd", {}, g.romId ? (g.match === "checksum" ? "found by checksum" : "yes") : "not found")] : null,
      g.romId && !g.hasCart ? [h("dt", {}, "Label"), h("dd", {}, "RomM has no cartridge photo for this game")] : null));
  body.push(details);
  if (g.onCard) {
    body.push(h("div", { class: "group" },
      h("button", { class: "btn ghost danger", onclick: (e) => removeGame(g, e.currentTarget) }, icon("trash"), "Remove from card")));
  }

  return h("div", { style: { display: "contents" } }, scrim,
    h("aside", { class: "drawer open", role: "dialog", "aria-label": g.title },
      h("div", { class: "drawer-head" },
        platformIcon(g.platform),
        h("div", {}, h("h3", {}, g.title), h("div", { class: "muted" }, g.file)),
        g.onCard && h("button", {
          class: "x fav" + (g.favourite ? " on" : ""), title: g.favourite ? "Take off Slot's favourites" : "Add to Slot's favourites",
          "aria-pressed": String(!!g.favourite), onclick: (e) => toggleFavourite(g, e.currentTarget),
        }, icon("star")),
        h("button", { class: "x", title: "Close", onclick: closeDrawer }, icon("close"))),
      h("div", { class: "drawer-body" }, body)));
}

// Stars or unstars a game in Slot's favourites (Config/favorites.txt).
async function toggleFavourite(g, btn) {
  btn.disabled = true;
  try {
    await api().SetFavourite(g.platform, g.stem, !g.favourite);
    toast(g.favourite ? `Took ${g.title} off your favourites.` : `${g.title} is in your favourites on Slot.`);
    await reload();
    refreshMain();
  } catch (e) {
    toast(`Couldn't change the favourite: ${errText(e)}`, true);
    btn.disabled = false;
  }
}

// A real GBA cartridge's grey (Cart Studio's "Automatic"). Slot draws a game
// with no colour in plain black, so SlotDrop writes this one out.
const GBA_BLACK = { colour: "3a3431", finish: "solid" };
const PRESETS = ["9a9aa0", "e9e7e1", "5b4bd8", "c0282c", "2f7d4f", "1f6fd1", "e8b72e"];
const FINISHES = [["solid", "Solid"], ["clear", "Clear"], ["glitter", "Glitter"]];

// cartColourFor is the plastic's colour in a game's RomM cart photo, for
// carts like FireRed's red: "" until it's known, or if it's grey. The first
// call looks it up and redraws the drawer when it arrives.
function cartColourFor(g) {
  if (!g.romId || !g.hasCart) return "";
  if (!(g.romId in state.cartColours)) {
    state.cartColours[g.romId] = "";
    api().PlanLabel(g.romId).then((plan) => {
      state.cartColours[g.romId] = plan.cartColour || "";
      if (plan.cartColour && state.drawer && state.drawer.romId === g.romId) refreshMain();
    }).catch(() => {});
  }
  return state.cartColours[g.romId];
}

function labelGroup(g, shell, showHero) {
  const group = h("div", { class: "group" }, h("h4", {}, "Label and shell"));
  const ed = state.editor;
  const dirty = !!state.pendingShell || (ed && ed.moved);

  // Shell colour. Most carts are the plain GBA grey, so that's one click.
  const pick = (colour, finish = shell.finish) => { state.pendingShell = { colour, finish, outline: shell.outline }; refreshMain(); };
  const black = h("button", { class: "btn small" + (shell.colour === GBA_BLACK.colour ? " on" : ""), onclick: () => pick(GBA_BLACK.colour, "solid") },
    h("span", { class: "dot-swatch", style: { background: "#" + GBA_BLACK.colour } }), "GBA Grey");
  const fromLabel = ed && ed.suggested ? h("button", { class: "btn small", title: "The label's main colour", onclick: () => pick(ed.suggested.colour) },
    h("span", { class: "dot-swatch", style: { background: "#" + ed.suggested.colour } }), "From label") : null;
  const cartHex = cartColourFor(g) || (ed && ed.cartColour);
  const fromCart = cartHex ? h("button", { class: "btn small" + (shell.colour === cartHex ? " on" : ""), title: "The cartridge's colour in RomM's photo", onclick: () => pick(cartHex, "solid") },
    h("span", { class: "dot-swatch", style: { background: "#" + cartHex } }), "Cart colour") : null;
  const swatches = PRESETS.map((c) => h("button", {
    class: "swatch" + (c === shell.colour ? " on" : ""), style: { background: "#" + c }, title: "#" + c, onclick: () => pick(c),
  }));
  // Any colour: the cart above recolours live while you drag or type, and the
  // choice sticks when you let go (or finish typing a full hex code).
  let frame = 0;
  const preview = (hex) => {
    cancelAnimationFrame(frame);
    frame = requestAnimationFrame(() => showHero({ colour: hex, finish: shell.finish, outline: shell.outline }));
  };
  const hexBox = h("input", { class: "hex", value: shell.colour, maxlength: 7, spellcheck: false, "aria-label": "Colour as a hex code" });
  const picker = h("input", { type: "color", value: "#" + shell.colour,
    oninput: (e) => { const hex = e.target.value.slice(1); hexBox.value = hex; preview(hex); },
    onchange: (e) => pick(e.target.value.slice(1)) });
  hexBox.oninput = () => {
    const hex = hexBox.value.replace(/^#/, "").toLowerCase();
    if (/^[0-9a-f]{6}$/.test(hex)) { picker.value = "#" + hex; preview(hex); }
  };
  hexBox.onchange = () => setTimeout(() => {
    // Wait a tick: a change fired by the panel closing must not redraw it.
    if (!hexBox.isConnected) return;
    const hex = hexBox.value.replace(/^#/, "").toLowerCase();
    if (/^[0-9a-f]{6}$/.test(hex)) pick(hex);
    else { hexBox.value = shell.colour; showHero(); }
  });
  hexBox.onkeydown = (e) => { if (e.key === "Enter") hexBox.blur(); };
  swatches.push(h("label", { class: "swatch-pick", title: "Any colour" }, picker, h("span", {}, "+")));
  const finish = h("div", { class: "segmented" }, FINISHES.map(([f, name]) =>
    h("button", { class: shell.finish === f ? "on" : "", onclick: () => pick(shell.colour, f) }, name)));
  group.append(h("div", { class: "shell-row" }, black, fromCart, fromLabel, finish));
  group.append(h("div", { class: "shell-row" }, swatches, h("label", { class: "hex-wrap" }, "#", hexBox)));
  if (!g.shell && !state.pendingShell) {
    group.append(h("p", { class: "faint", style: { margin: "-6px 0 12px", fontSize: "12.5px" } }, "No colour on the card yet, so Slot shows it in plain black. Click GBA Grey, then save, to fix that."));
  }

  // Label position on the photo.
  if (ed) {
    group.append(boxEditor(g, ed, showHero));
    group.append(h("p", { class: "faint", style: { margin: "0 0 12px", fontSize: "12.5px" } },
      "Drag the box to move it, or a corner or the mouse wheel to resize. The preview above updates when you let go.",
      ed.token ? " Past the picture's edge comes out white." : ""));
  }

  const fromPhoto = g.romId && g.hasCart; // RomM has a cartridge photo to cut the label from
  const actions = h("div", { class: "row", style: { flexWrap: "wrap" } });
  if (!ed && fromPhoto) {
    actions.append(h("button", { class: "btn", onclick: (e) => startEditor(g, e.currentTarget) }, g.hasLabel ? "Redo label from photo" : "Adjust label"));
  }
  if (!ed) {
    actions.append(h("button", { class: "btn" + (fromPhoto || g.hasLabel ? "" : " primary"), onclick: (e) => chooseLabel(g, e.currentTarget) },
      icon("image"), "Use my own image…"));
  }
  if (!g.hasLabel && !ed && fromPhoto) {
    actions.append(h("button", { class: "btn primary", onclick: (e) => saveLabel(g, null, state.pendingShell, e.currentTarget) }, icon("wand"), "Make label"));
  } else if (ed) {
    actions.append(h("button", { class: "btn ghost", onclick: () => { closeEditor(); refreshMain(); } }, "Cancel"));
    actions.append(h("button", { class: "btn primary", onclick: (e) => saveLabel(g, ed.box, state.pendingShell, e.currentTarget) }, "Save label"));
  } else if (dirty) {
    actions.append(h("button", { class: "btn primary", onclick: (e) => saveShell(g, shell, e.currentTarget) }, "Save colour"));
  }
  group.append(actions);
  return group;
}

async function startEditor(g, btn) {
  btn.disabled = true;
  btn.replaceChildren(h("span", { class: "spinner" }), "Loading photo…");
  try {
    const plan = await api().PlanLabel(g.romId);
    state.editor = { ...plan, box: plan.box.slice(), moved: false };
  } catch (e) {
    toast(errText(e), true);
  }
  refreshMain();
}

// closeEditor stops framing a label, and lets go of a picture you chose.
function closeEditor() {
  if (state.editor && state.editor.token) api().DropOwnLabel(state.editor.token).catch(() => {});
  state.editor = null;
}

// boxEditor frames the label on a picture: drag the box to move it, a corner
// or the wheel to resize it, always in the label's shape so it isn't
// stretched. On a picture you chose the box may go past its edges (by
// ed.marginX and ed.marginY) to leave white around it.
function boxEditor(g, ed, showHero) {
  const mx = ed.marginX || 0, my = ed.marginY || 0;
  const L = -mx, T = -my, R = ed.width + mx, B = ed.height + my; // where the box may go
  const ew = R - L, eh = B - T;
  const minW = Math.max(16, 16 * ed.aspect);
  const maxW = Math.min(ew, eh * ed.aspect);
  const boxEl = h("div", { class: "box" }, ["nw", "ne", "sw", "se"].map((k) => h("div", { class: "h", "data-h": k })));
  const pic = h("img", {
    src: ed.token ? `/img/own/${ed.token}` : `/img/cart/${g.romId}`, alt: "",
    style: { left: (mx / ew) * 100 + "%", top: (my / eh) * 100 + "%", width: (ed.width / ew) * 100 + "%", height: (ed.height / eh) * 100 + "%" },
  });
  const wrap = h("div", {
    class: "editor" + (ed.token ? " loose" : ""),
    style: { aspectRatio: `${ew} / ${eh}`, width: `min(100%, ${((55 * ew) / eh).toFixed(2)}vh)` },
  }, pic, boxEl);
  const place = () => {
    const [l, t, r, b] = ed.box;
    Object.assign(boxEl.style, {
      left: ((l - L) / ew) * 100 + "%", top: ((t - T) / eh) * 100 + "%",
      width: ((r - l) / ew) * 100 + "%", height: ((b - t) / eh) * 100 + "%",
    });
  };
  // fit puts a box of width w at (l, t), in the label's shape and inside the area.
  const fit = (l, t, w) => {
    w = Math.min(Math.max(w, minW), maxW);
    const hgt = w / ed.aspect;
    l = Math.min(Math.max(L, l), R - w);
    t = Math.min(Math.max(T, t), B - hgt);
    ed.box = [l, t, l + w, t + hgt].map(Math.round);
    place();
  };
  place();
  const done = () => {
    ed.moved = true;
    showHero();
  };
  let drag = null;
  boxEl.addEventListener("pointerdown", (e) => {
    e.preventDefault();
    boxEl.setPointerCapture(e.pointerId);
    drag = { handle: e.target.dataset.h || "move", x: e.clientX, y: e.clientY, box: ed.box.slice() };
  });
  boxEl.addEventListener("pointermove", (e) => {
    if (!drag) return;
    const rect = wrap.getBoundingClientRect();
    const dx = ((e.clientX - drag.x) / rect.width) * ew;
    const dy = ((e.clientY - drag.y) / rect.height) * eh;
    const [l, t, r, b] = drag.box;
    if (drag.handle === "move") {
      fit(l + dx, t + dy, r - l);
      return;
    }
    // A corner: the opposite corner stays put, and the box grows to reach
    // the pointer in whichever direction it went further.
    const east = drag.handle.includes("e"), south = drag.handle.includes("s");
    const ax = east ? l : r, ay = south ? t : b;
    const wide = east ? r + dx - ax : ax - (l + dx);
    const tall = south ? b + dy - ay : ay - (t + dy);
    let w = Math.max(wide, tall * ed.aspect);
    w = Math.min(Math.max(w, minW), east ? R - ax : ax - L, (south ? B - ay : ay - T) * ed.aspect);
    fit(east ? ax : ax - w, south ? ay : ay - w / ed.aspect, w);
  });
  const end = () => {
    if (!drag) return;
    drag = null;
    done();
  };
  boxEl.addEventListener("pointerup", end);
  boxEl.addEventListener("pointercancel", end);
  // The wheel zooms around the pointer: down shows more of the picture.
  let settle = 0;
  wrap.addEventListener("wheel", (e) => {
    e.preventDefault();
    const rect = wrap.getBoundingClientRect();
    const px = L + ((e.clientX - rect.left) / rect.width) * ew;
    const py = T + ((e.clientY - rect.top) / rect.height) * eh;
    const [l, t, r, b] = ed.box;
    const w = Math.min(Math.max((r - l) * Math.exp(e.deltaY * 0.0015), minW), maxW);
    const fx = Math.min(Math.max((px - l) / (r - l), 0), 1);
    const fy = Math.min(Math.max((py - t) / (b - t), 0), 1);
    fit(px - fx * w, py - fy * (w / ed.aspect), w);
    clearTimeout(settle);
    settle = setTimeout(done, 250);
  }, { passive: false });
  return wrap;
}

async function saveLabel(g, box, shell, btn) {
  btn.disabled = true;
  btn.replaceChildren(h("span", { class: "spinner" }), "Saving…");
  try {
    if (box && state.editor && state.editor.token) await api().SaveOwnLabel(g.platform, g.stem, state.editor.token, box, shell);
    else if (box) await api().SaveLabel(g.platform, g.stem, g.romId, box, shell);
    else {
      await api().SaveLabel(g.platform, g.stem, g.romId, (await api().PlanLabel(g.romId)).box, shell);
    }
    state.labelVersion = Date.now();
    state.editor = null;
    state.pendingShell = null;
    toast(`Saved the label for ${g.title}.`);
    await reload();
  } catch (e) {
    toast(errText(e), true);
  }
  refreshMain();
}

// Pick a picture on the PC (a label scan, or a whole cartridge photo) for a
// game's label, then frame it in the editor before saving.
async function chooseLabel(g, btn) {
  btn.disabled = true;
  try {
    const plan = await api().ChooseLabel(g.platform);
    if (plan) {
      closeEditor();
      state.editor = { ...plan, box: plan.box.slice(), moved: false };
      refreshMain();
      return;
    }
  } catch (e) {
    toast(errText(e), true);
  }
  btn.disabled = false;
}

async function saveShell(g, shell, btn) {
  btn.disabled = true;
  try {
    await api().SetShell(g.stem, shell);
    state.pendingShell = null;
    toast(`Saved the shell colour for ${g.title}.`);
    await reload();
  } catch (e) {
    toast(errText(e), true);
  }
  refreshMain();
}

function matchGroup(g) {
  const list = h("div", { class: "matchlist" });
  const input = h("input", { class: "input", placeholder: "Search RomM for this game…", spellcheck: false });
  const fill = () => {
    const terms = fold(input.value || g.stem).split(/\s+/).filter(Boolean);
    const found = state.games.filter((x) => x.romId && x.platform === g.platform && !x.onCard && matches(x, terms)).slice(0, 8);
    list.replaceChildren(...(found.length ? found.map((x) =>
      h("button", { onclick: async () => { await api().SetMatch(g.platform, g.stem, x.romId); await reload(); refreshMain(); } },
        x.title, h("span", { class: "faint" }, "  " + x.file))) : [h("div", { class: "faint", style: { fontSize: "13px" } }, "No matches. Try other words.")]));
  };
  input.oninput = fill;
  fill();
  return h("div", { class: "group" }, h("h4", {}, "Not found in RomM"),
    h("p", { class: "muted", style: { margin: "0 0 10px", fontSize: "13px" } }, "Pick the RomM game this is, so it can get a label."),
    input, list);
}

window.addEventListener("DOMContentLoaded", boot);
if (document.readyState !== "loading") boot();
