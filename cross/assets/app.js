// ルミの画面。Go (lumi.go) とはイベントでやり取りする。
// URL が #peek のときは、右下から出てくる小さな窓として動く。

import * as wails from "/wails/runtime.js";
import { Face } from "./face.js";
import { Voice } from "./voice.js";

const emit = (name, data) => wails.Events.Emit(name, data);
const on = (name, fn) => wails.Events.On(name, e => fn(e.data));

// 顔を Canvas に描き続ける (見た目が変わったときだけ描き直す)
function animate(canvas, face) {
  const ctx = canvas.getContext("2d");
  let w = 0, h = 0;
  const frame = () => {
    const dpr = window.devicePixelRatio || 1;
    const cw = canvas.clientWidth, ch = canvas.clientHeight;
    const resized = cw !== w || ch !== h;
    if (resized) { w = cw; h = ch; canvas.width = cw * dpr; canvas.height = ch * dpr; }
    if (face.tick() || resized) {
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.clearRect(0, 0, w, h);
      face.draw(ctx, w, h);
    }
    setTimeout(() => requestAnimationFrame(frame), 40);
  };
  frame();
}

// ---- 顔の色 ----
// 名前か #rrggbb で指定する。背景の顔は暗く (背景に混ぜる)、右下の顔は明るいまま使う
const COLORS = { cyan: "#61d6d6", green: "#5fd16a", pink: "#ff8fc8", amber: "#ffa94d", purple: "#b59cff", white: "#e6e6e6", red: "#ff6b6b" };
const BG = "#0c0c0c";

function mix(a, b, k) {
  const p = s => [1, 3, 5].map(i => parseInt(s.slice(i, i + 2), 16));
  const [x, y] = [p(a), p(b)];
  return "#" + x.map((v, i) => Math.round(v * k + y[i] * (1 - k)).toString(16).padStart(2, "0")).join("");
}

function baseColor(name) {
  name = (name || "cyan").toLowerCase();
  return COLORS[name] || (/^#[0-9a-f]{6}$/.test(name) ? name : COLORS.cyan);
}

// dim: 背景の顔用 (文字の邪魔にならない暗さ)
function palette(color, dim) {
  return dim
    ? { line: mix(color, BG, 0.22), fill: mix(color, BG, 0.3), cheek: "#461e3c", bg: BG }
    : { line: color, fill: color, cheek: "#e7488c", bg: BG };
}

// 管理者権限で動いている間の色 (ふだんの色がオレンジならかぶらないよう赤)
function adminColor(base) { return base === COLORS.amber ? COLORS.red : COLORS.amber; }

if (location.hash === "#peek") peekMain();
else if (location.hash === "#settings") import("./settings.js").then(m => m.settingsMain(emit, on));   // 設定画面 (/settings window)
else termMain();

// ================= メインの窓 =================

function termMain() {
  const term = document.getElementById("term"), log = document.getElementById("log");
  const keys = document.getElementById("keys");
  term.hidden = false;

  // ファイルを持ってきて、落とさずに窓の外へ出したときに枠が残らないように
  // (Wails は窓の外へ出たときの dragleave を無視するので、こちらで消す)
  {
    const off = () => term.classList.remove("file-drop-target-active");
    let idle = 0;
    document.addEventListener("dragover", e => {
      if (!e.dataTransfer?.types.includes("Files")) return;
      clearTimeout(idle);
      idle = setTimeout(off, 400);   // dragover が止まった = 外へ出たか、やめた
    });
    document.addEventListener("dragleave", e => {
      if (e.relatedTarget) return;
      if (e.clientX <= 0 || e.clientY <= 0 || e.clientX >= innerWidth || e.clientY >= innerHeight) off();
    });
    document.addEventListener("drop", () => clearTimeout(idle));
  }

  // 管理者権限で動いている間は、顔をオレンジ系にする
  let base = COLORS.cyan, admin = false;
  const face = new Face(palette(base, true));
  const recolor = () => { face.c = palette(admin ? adminColor(base) : base, true); face.lastState = ""; };
  animate(document.getElementById("face"), face);
  face.effect("boot", 2.6);   // 起動の演出

  // ---- 出力 ----
  let current = newLine();
  function newLine() {
    const div = document.createElement("div");
    div.className = "line";
    log.appendChild(div);
    return div;
  }
  function atBottom() { return log.scrollHeight - log.scrollTop - log.clientHeight < 40; }
  function write(text, color = "fg") {
    const stick = atBottom();
    text.split("\n").forEach((part, i) => {
      if (i > 0) current = newLine();
      if (part) {
        const span = document.createElement("span");
        span.className = color;
        span.textContent = part;
        current.appendChild(span);
      }
    });
    placeInput();
    if (stick) log.scrollTop = log.scrollHeight;
  }

  // ---- タブ ----
  // 最初のタブはルミ (AI と話す)。+ でシェルのタブを開ける (Go の tabs.go が動かす)。
  // 入力中の文字・カーソル・履歴はタブごとに持ち、切り替えるときに入れ替える
  const tabsEl = document.getElementById("tabs");
  const lumiTab = { id: 0, kind: "lumi", log, title: "" };
  const tabs = [lumiTab];
  let active = lumiTab, nextTabId = 1, shellList = [], lumiShell = false;
  const isLumi = () => active === lumiTab;
  // 今のタブで文字を打てるか (返事やコマンドの途中は打てない。確認の質問には答えられる)
  const canType = () => isLumi() ? (!busy || askPrompt !== null) : !active.busy;
  const findTab = id => tabs.find(t => t.id === id);

  function writeTab(t, text, color = "fg") {
    const stick = t.log.scrollHeight - t.log.scrollTop - t.log.clientHeight < 40;
    text.split("\n").forEach((part, i) => {
      if (i > 0) t.current = newLineIn(t.log);
      if (part) {
        const span = document.createElement("span");
        span.className = color;
        span.textContent = part;
        t.current.appendChild(span);
      }
    });
    if (t === active) placeInput();
    if (stick) t.log.scrollTop = t.log.scrollHeight;
  }
  function newLineIn(el) {
    const div = document.createElement("div");
    div.className = "line";
    el.appendChild(div);
    return div;
  }
  function clearTab(t) { t.log.textContent = ""; t.current = newLineIn(t.log); placeInput(); }

  function updateFaceShell() { face.shell = !isLumi() || lumiShell; face.lastState = ""; }

  function switchTab(t) {
    if (!t || t === active) return;
    // 今のタブの入力を覚えておき、切り替え先の入力に入れ替える
    Object.assign(active, { buffer, caret, history, historyIndex });
    active = t;
    buffer = t.buffer || ""; caret = t.caret || 0; history = t.history || []; historyIndex = t.historyIndex ?? history.length;
    for (const x of tabs) x.log.hidden = x !== active;
    updateFaceShell();
    renderTabs();
    placeInput();
    active.log.scrollTop = active.log.scrollHeight;
    keys.focus();
  }

  function openTab(kind) {
    const el = document.createElement("div");
    el.className = "log";
    term.insertBefore(el, keys);
    const t = { id: nextTabId++, kind: "shell", log: el, title: "…", prompt: "", busy: true, buffer: "", caret: 0, history: [], historyIndex: 0 };
    t.current = newLineIn(el);
    tabs.push(t);
    switchTab(t);
    emit("tabOpen", { tab: t.id, shell: kind || "" });
  }

  function closeTab(t) {
    if (!t || t === lumiTab) return;
    emit("tabClose", { tab: t.id });
    const i = tabs.indexOf(t);
    if (i < 0) return;
    if (active === t) switchTab(tabs[i - 1] || lumiTab);
    tabs.splice(i, 1);
    t.log.remove();
    renderTabs();
  }

  function renderTabs() {
    tabsEl.textContent = "";
    lumiTab.title = msgs["tab.lumi"] || "Lumi";
    for (const t of tabs) {
      const el = document.createElement("div");
      el.className = "tab" + (t === active ? " active" : "");
      const title = document.createElement("span");
      title.className = "title";
      title.textContent = t.title;
      el.appendChild(title);
      if (t !== lumiTab) {
        const x = document.createElement("span");
        x.className = "close";
        x.textContent = "×";
        x.title = msgs["tab.close"] || "";
        x.addEventListener("click", ev => { ev.stopPropagation(); closeTab(t); });
        el.appendChild(x);
      }
      el.addEventListener("click", () => switchTab(t));
      el.addEventListener("auxclick", ev => { if (ev.button === 1) closeTab(t); });   // 中ボタンでも閉じる
      tabsEl.appendChild(el);
    }
    const plus = document.createElement("div");
    plus.className = "tabbtn";
    plus.textContent = "+";
    plus.title = msgs["tab.new"] || "";
    plus.addEventListener("click", () => openTab(""));
    tabsEl.appendChild(plus);
    if (shellList.length > 1) {
      const more = document.createElement("div");
      more.className = "tabbtn";
      more.textContent = "▾";
      more.title = msgs["tab.choose"] || "";
      more.addEventListener("click", ev => { ev.stopPropagation(); toggleShellMenu(more); });
      tabsEl.appendChild(more);
    }
  }

  // ▾: シェルを選んでタブを開く
  let shellMenu = null;
  function toggleShellMenu(anchor) {
    if (shellMenu) { shellMenu.remove(); shellMenu = null; return; }
    shellMenu = document.createElement("div");
    shellMenu.id = "shellmenu";
    for (const s of shellList) {
      const item = document.createElement("div");
      item.textContent = s.label;
      item.addEventListener("click", () => { shellMenu.remove(); shellMenu = null; openTab(s.kind); });
      shellMenu.appendChild(item);
    }
    shellMenu.style.left = (anchor.offsetLeft) + "px";
    term.appendChild(shellMenu);
  }
  document.addEventListener("click", () => { if (shellMenu) { shellMenu.remove(); shellMenu = null; } });

  on("shells", list => { shellList = list || []; renderTabs(); });
  on("tabInfo", d => {
    const t = findTab(d.tab);
    if (!t) return;
    t.title = d.title;
    t.prompt = d.prompt;
    renderTabs();
    if (t === active) render();
  });
  on("tabWrite", d => { const t = findTab(d.tab); if (t) writeTab(t, d.text, d.color); });
  on("tabBusy", d => { const t = findTab(d.tab); if (t) { t.busy = d.on; if (t === active) render(); } });
  on("tabClear", d => { const t = findTab(d.tab); if (t) clearTab(t); });
  on("tabClosed", d => closeTab(findTab(d.tab)));
  // --script の "#tab ..." (テスト用): タブを開く・打つ・切り替える・閉じる
  on("testTab", d => {
    if (d.op === "open") openTab(d.arg);
    else if (d.op === "next") switchTab(tabs[(tabs.indexOf(active) + 1) % tabs.length]);
    else if (d.op === "close") closeTab(active);
    else if (d.op === "dump") {
      // タブの様子と、それぞれの画面の文字を Go に送る (Go がファイルに書く)
      emit("testDump", tabs.map(t => ({
        title: t.title, active: t === active, busy: t === lumiTab ? busy : t.busy,
        prompt: t === lumiTab ? PROMPT : t.prompt, faceShell: face.shell,
        text: t.log.innerText.slice(-1500),
      })));
    }
    else if (d.op === "run" && !isLumi()) {
      writeTab(active, active.prompt, "fg");
      writeTab(active, d.arg + "\n", "white");
      active.busy = true;
      render();
      emit("tabSubmit", { tab: active.id, text: d.arg });
    }
  });

  // ---- 入力行 (今の行の最後に置く) ----
  // シェルモードでは Go から送られたプロンプト (PS C:\Users\…> など) に変わる
  // ふだんのプロンプトは OS ごとに Go から届く (Windows は C:\Lumi>、Mac は lumi@Mac ~ %、Linux は lumi@linux:~$)
  let NORMAL_PROMPT = "C:\\Lumi> ";
  let PROMPT = NORMAL_PROMPT;
  const input = document.createElement("span");
  input.id = "input";
  input.innerHTML = '<span class="prompt"></span><span class="typed"></span><span class="compose"></span>' +
    '<span class="cursor"></span><span class="after"></span><span class="ghost"></span><span class="hint"></span><span class="spinner"></span>';
  const [promptEl, typedEl, composeEl, cursorEl, afterEl, ghostEl, hintEl, spinnerEl] = input.children;
  let buffer = "", compose = "", busy = false, thinking = false, askPrompt = null;
  let caret = 0;   // カーソルの位置 (何文字目か。日本語も 1 文字と数える)
  const chars = () => Array.from(buffer);
  // 入力を丸ごと入れ替える (カーソルは最後に)
  function setBuffer(s) { buffer = s; caret = Array.from(s).length; }
  // カーソルの位置に文字を入れる
  function insert(text) {
    const a = chars(), add = Array.from(text);
    a.splice(caret, 0, ...add);
    buffer = a.join("");
    caret += add.length;
  }
  let hint = "", lastWasVoice = false, msgs = {};
  let commands = [], history = [], historyIndex = 0, tabPrefix = null, tabIndex = -1;

  function placeInput() { (active === lumiTab ? current : active.current).appendChild(input); render(); }

  function render() {
    const editing = canType();
    promptEl.textContent = editing ? (isLumi() ? (askPrompt ?? PROMPT) : active.prompt) : "";
    input.classList.toggle("asking", isLumi() && askPrompt !== null);
    // カーソルより前・カーソルの上の 1 文字・後ろに分けて出す (コンソールのように、カーソルは文字に重なる)
    const a = chars(), at = editing && !compose ? (a[caret] || "") : "";
    typedEl.textContent = editing ? a.slice(0, caret).join("") : "";
    composeEl.textContent = editing ? compose : "";
    cursorEl.style.display = editing ? "" : "none";
    cursorEl.textContent = at;
    cursorEl.classList.toggle("char", !!at);
    afterEl.textContent = editing ? a.slice(caret + (at ? 1 : 0)).join("") : "";
    ghostEl.textContent = editing && !compose && caret >= a.length ? ghost() : "";
    hintEl.textContent = editing && isLumi() && !buffer && !compose && hint ? " " + hint : "";
    spinnerEl.textContent = !editing && isLumi() && thinking ? "|/-\\"[Math.floor(performance.now() / 125) % 4] : "";
    // 日本語変換の窓がカーソルの位置に出るよう、見えない入力欄を動かす
    const r = cursorEl.getBoundingClientRect(), t = term.getBoundingClientRect();
    keys.style.left = (r.left - t.left) + "px";
    keys.style.top = (r.top - t.top) + "px";
  }
  setInterval(() => { if (thinking) render(); }, 125);

  // 入力中の / コマンドの候補 (先頭一致) の残り部分
  function ghost() {
    if (!isLumi() || askPrompt !== null || !buffer.startsWith("/") || buffer.includes(" ")) return "";
    const m = commands.find(c => c.startsWith(buffer));
    return m ? m.slice(buffer.length) : "";
  }

  // カーソルの点滅
  let lastKey = performance.now();
  setInterval(() => {
    const on = (performance.now() - lastKey) < 530 || Math.floor(performance.now() / 530) % 2 === 0;
    cursorEl.classList.toggle("off", !on);
  }, 100);
  keys.addEventListener("focus", () => term.classList.remove("blur"));
  keys.addEventListener("blur", () => term.classList.add("blur"));
  term.addEventListener("mouseup", () => { if (!window.getSelection().toString()) keys.focus(); });

  // 打った文字 (日本語変換の確定も含む)
  keys.addEventListener("input", e => {
    if (e.isComposing) return;
    if (canType()) insert(keys.value.replace(/\r?\n/g, " "));
    keys.value = "";
    tabPrefix = null;
    render();
  });
  keys.addEventListener("compositionupdate", e => { compose = e.data || ""; render(); });
  keys.addEventListener("compositionend", e => {
    compose = "";
    if (canType()) insert(e.data || "");
    keys.value = "";
    render();
  });

  keys.addEventListener("keydown", e => {
    lastKey = performance.now();
    face.poke(true);
    if (e.isComposing || e.keyCode === 229) return;
    const ctrl = e.ctrlKey || e.metaKey;
    // タブ: Ctrl+T で開く、Ctrl+W で閉じる、Ctrl+Tab で次へ
    if (ctrl && e.key.toLowerCase() === "t") { e.preventDefault(); openTab(""); return; }
    if (ctrl && e.key.toLowerCase() === "w") { e.preventDefault(); closeTab(active); return; }
    if (ctrl && e.key === "Tab") {
      e.preventDefault();
      const i = tabs.indexOf(active);
      switchTab(tabs[(i + (e.shiftKey ? tabs.length - 1 : 1)) % tabs.length]);
      return;
    }
    if (isLumi() && askPrompt !== null && (e.key === "Enter" || e.key === "Escape" || (ctrl && e.key === "c"))) {
      e.preventDefault();
      if (e.key === "Enter") answerAsk(buffer, buffer, "");
      else answerAsk("", buffer + "^C", "");
      return;
    }
    if (e.key === "Escape" || (ctrl && e.key === "c" && !window.getSelection().toString())) {
      e.preventDefault();
      if (!isLumi()) {
        if (active.busy) emit("tabInterrupt", { tab: active.id });
        else if (buffer) { writeTab(active, active.prompt, "fg"); writeTab(active, buffer + "^C\n", "white"); setBuffer(""); render(); }
        return;
      }
      if (busy) emit("interrupt");
      else if (buffer) { write(PROMPT, "fg"); write(buffer + "^C\n", "white"); setBuffer(""); render(); }
      return;
    }
    if (ctrl && e.key === "l") { e.preventDefault(); if (isLumi()) clear(); else clearTab(active); return; }
    if (!canType()) { if (e.key !== "Tab") return; e.preventDefault(); return; }
    // Shift+Tab: 自動モード (確認せずにコマンドを実行する) の切り替え
    if (e.key === "Tab" && e.shiftKey) { e.preventDefault(); emit("toggleAuto"); return; }
    if (e.key === "Tab") {
      // Tab を押すたびに候補を順に切り替える
      e.preventDefault();
      if (!isLumi()) return;
      if (tabPrefix === null) { tabPrefix = buffer; tabIndex = -1; }
      const m = tabPrefix.startsWith("/") ? commands.filter(c => c.startsWith(tabPrefix)) : [];
      if (m.length) { tabIndex = (tabIndex + 1) % m.length; setBuffer(m[tabIndex]); render(); }
      return;
    }
    tabPrefix = null;
    if (e.key === "Enter" && !isLumi()) {
      // シェルのタブ: 打った行をそのタブのシェルで実行する
      e.preventDefault();
      const text = buffer;
      setBuffer("");
      writeTab(active, active.prompt, "fg");
      writeTab(active, text + "\n", "white");
      if (text.trim()) { history.push(text); historyIndex = history.length; }
      active.busy = true;
      render();
      emit("tabSubmit", { tab: active.id, text });
      return;
    }
    if (e.key === "Enter") {
      e.preventDefault();
      const text = buffer;
      setBuffer("");
      write(PROMPT, "fg");
      write(text + "\n", "white");
      if (text.trim()) { history.push(text); historyIndex = history.length; }
      lastWasVoice = false;
      emit("submit", text);
    } else if (e.key === "Backspace") {
      // カーソルの前の 1 文字を消す
      e.preventDefault();
      if (caret > 0) { const a = chars(); a.splice(caret - 1, 1); buffer = a.join(""); caret--; render(); }
    } else if (e.key === "Delete") {
      // カーソルの上の 1 文字を消す
      e.preventDefault();
      const a = chars();
      if (caret < a.length) { a.splice(caret, 1); buffer = a.join(""); render(); }
    } else if (e.key === "ArrowLeft" || e.key === "ArrowRight") {
      // 1 文字ずつ動く (Ctrl を押していれば単語ごと)
      e.preventDefault();
      const a = chars(), dir = e.key === "ArrowLeft" ? -1 : 1;
      if (ctrl) {
        let i = caret;
        if (dir < 0) { while (i > 0 && a[i - 1] === " ") i--; while (i > 0 && a[i - 1] !== " ") i--; }
        else { while (i < a.length && a[i] !== " ") i++; while (i < a.length && a[i] === " ") i++; }
        caret = i;
      } else caret = Math.max(0, Math.min(a.length, caret + dir));
      render();
    } else if (e.key === "Home") {
      e.preventDefault(); caret = 0; render();
    } else if (e.key === "End") {
      e.preventDefault(); caret = chars().length; render();
    } else if (e.key === "ArrowUp" && history.length) {
      e.preventDefault();
      historyIndex = Math.max(0, historyIndex - 1);
      setBuffer(history[historyIndex]);
      render();
    } else if (e.key === "ArrowDown" && history.length) {
      e.preventDefault();
      historyIndex = Math.min(history.length, historyIndex + 1);
      setBuffer(history[historyIndex] ?? "");
      render();
    }
  });

  function clear() { log.textContent = ""; current = newLine(); placeInput(); }

  // ---- 読み上げ (OS の音声合成。使えなければ文字だけ) ----
  let voiceName = "", rate = 1, muted = false, voices = [];
  const synth = window.speechSynthesis;
  const loadVoices = () => { voices = synth ? synth.getVoices() : []; };
  if (synth) { loadVoices(); synth.addEventListener?.("voiceschanged", loadVoices); }

  function pickVoice() {
    if (!voices.length) loadVoices();
    if (voiceName) return voices.find(v => v.name === voiceName) || null;
    // 今の言語の声を選ぶ (端末にある声を優先、日本語なら女性の声を優先)
    const lang = (msgs.lang || "ja").toLowerCase();
    const same = voices.filter(v => v.lang && v.lang.toLowerCase().startsWith(lang));
    const local = same.filter(v => v.localService);
    const pool = local.length ? local : same;
    return pool.find(v => /Haruka|Nanami|Kyoko|Ayumi|Sayaka|Zira|Aria|Jenny|Samantha/.test(v.name)) || pool[0] || null;
  }

  let speaking = null;   // 今喋っている文 { id, text, shown, done }
  function speak(id, text) {
    face.speaking = true;
    const s = { id, text, shown: 0, finished: false };
    speaking = s;
    const reveal = upTo => {
      upTo = Math.min(upTo, text.length);
      if (upTo > s.shown) { write(text.slice(s.shown, upTo), "fg"); s.shown = upTo; }
    };
    const finish = (stopped) => {
      if (s.finished) return;
      s.finished = true;
      if (!stopped) reveal(text.length);
      face.speaking = false;
      if (speaking === s) speaking = null;
      emit("spoken", id);
    };
    s.stop = () => finish(true);
    const voice = muted || !synth ? null : pickVoice();
    if (voice) {
      const u = new SpeechSynthesisUtterance(text);
      u.voice = voice;
      u.lang = voice.lang;
      u.rate = Math.max(0.5, Math.min(2, 1 + rate * 0.1));
      u.onboundary = e => reveal(e.charIndex + (e.charLength || 1));
      u.onend = () => finish(false);
      u.onerror = () => finish(false);
      synth.speak(u);
      setTimeout(() => finish(false), text.length * 400 + 8000);   // 終わりの知らせが来ないときの保険
    } else {
      // 声なし: 読み上げの速さに合わせて 1 文字ずつ出す
      const start = performance.now();
      const step = () => {
        if (s.finished) return;
        reveal(Math.floor((performance.now() - start) / 1000 * 7.5));
        if (s.shown >= text.length) setTimeout(() => finish(false), 200);
        else setTimeout(step, 30);
      };
      step();
    }
  }

  // ---- 確認の質問への答え (キーボードでも声でも) ----
  function answerAsk(answer, shown, note) {
    write(askPrompt, "yellow");
    write(shown, "white");
    write((note ? "  " + note : "") + "\n", "dim");
    askPrompt = null; setBuffer("");
    voice.confirming(false);
    emit("answer", answer);
    render();
  }

  // ---- 声での入力 ----
  // whisper.cpp (Go 側) に音を送って書き起こしてもらう: 16kHz の音を 16bit にして base64 で
  const nativeWaiting = new Map();
  let nativeId = 0;
  on("whisperResult", d => {
    const w = nativeWaiting.get(d.id);
    if (!w) return;
    nativeWaiting.delete(d.id);
    if (d.error) w.reject(new Error(d.error)); else w.resolve(d.text || "");
  });
  function pcmBase64(audio) {
    const pcm = new Int16Array(audio.length);
    for (let i = 0; i < audio.length; i++) pcm[i] = Math.max(-32768, Math.min(32767, Math.round(audio[i] * 32767)));
    const bytes = new Uint8Array(pcm.buffer);
    let s = "";
    for (let i = 0; i < bytes.length; i += 0x8000) s += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
    return btoa(s);
  }

  const voice = new Voice({
    nativeWhisper(audio, lang, prompt) {
      const id = ++nativeId;
      return new Promise((resolve, reject) => {
        nativeWaiting.set(id, { resolve, reject });
        emit("whisperNative", { id, pcm: pcmBase64(audio), lang, prompt });
      });
    },
    woke() {
      face.expression = "listen";
      face.poke(false);
      hint = msgs["voice.listening"] || "";
      render();
      emit("voiceWoke");
    },
    heard(text) {
      hint = "";
      if (face.expression === "listen") face.expression = "normal";
      if (busy || askPrompt !== null) { render(); return; }
      switchTab(lumiTab);   // 声で話しかけた内容は、ルミのタブで受ける
      write(PROMPT, "fg");
      write(text, "white");
      write("  " + (msgs["voice.mark"] || "") + "\n", "dim");
      history.push(text); historyIndex = history.length;
      lastWasVoice = true;
      emit("voiceHeard", text);
    },
    timeout() {
      hint = "";
      if (face.expression === "listen") face.expression = "normal";
      render();
      emit("voiceTimeout");
    },
    // answer: y / a / n
    confirm(answer) { if (askPrompt !== null) answerAsk(answer, answer, msgs["voice.mark"]); },
    state(state, message) { emit("voiceState", { state, message: message || "" }); },
    debug(text) { emit("voiceDebug", text); },
    whisperError(message) { emit("voiceWhisperError", message); },
  });
  // 喋っている間・考えている間は聞かない。喋り終わった直後も自分の声の残りを拾わないよう少し待つ
  let quietUntil = 0;
  setInterval(() => {
    const talking = !!speaking || (synth && synth.speaking);
    if (talking) quietUntil = performance.now() + 800;
    voice.paused = talking || performance.now() < quietUntil || (busy && askPrompt === null);
  }, 100);

  // VOICEVOX の声 (WAV) を鳴らし、時刻表どおりに口を動かし、文字を出す
  let audioCtx = null;
  async function speakAudio(id, text, wavB64, keys) {
    face.speaking = true;
    const s = { id, finished: false, shown: 0 };
    speaking = s;
    const reveal = upTo => {
      upTo = Math.min(upTo, text.length);
      if (upTo > s.shown) { write(text.slice(s.shown, upTo), "fg"); s.shown = upTo; }
    };
    let src = null, timer = null, guard = null;
    const finish = stopped => {
      if (s.finished) return;
      s.finished = true;
      clearInterval(timer);
      clearTimeout(guard);
      if (src) try { src.stop(); } catch {}
      if (!stopped) reveal(text.length);
      face.speaking = false;
      if (speaking === s) speaking = null;
      emit("spoken", id);
    };
    s.stop = () => finish(true);
    try {
      audioCtx = audioCtx || new AudioContext();
      // 止まった状態で作られることがある (そのままだと音が出ず、終わりも来ない)
      if (audioCtx.state === "suspended") await Promise.race([audioCtx.resume(), new Promise(r => setTimeout(r, 1000))]);
      const bytes = Uint8Array.from(atob(wavB64), c => c.charCodeAt(0));
      const buffer = await audioCtx.decodeAudioData(bytes.buffer);
      if (s.finished) return;
      src = audioCtx.createBufferSource();
      src.buffer = buffer;
      src.connect(audioCtx.destination);
      const start = audioCtx.currentTime;
      src.onended = () => finish(false);
      src.start();
      // 音が進まなくても、長さぶん待ったら終わりにする
      guard = setTimeout(() => finish(false), (buffer.duration + 3) * 1000);
      timer = setInterval(() => {
        const t = audioCtx.currentTime - start;
        let open = 0;
        for (const k of keys || []) { if (k.t <= t) open = k.open; else break; }
        face.mouth(open);
        reveal(Math.floor(text.length * Math.min(1, t / buffer.duration)));
      }, 30);
    } catch (e) {
      finish(false);
    }
  }

  // ---- Go からのイベント ----
  on("speakAudio", d => speakAudio(d.id, d.text, d.wav, d.keys));
  on("write", d => write(d.text, d.color));
  on("clear", () => clear());
  on("busy", b => {
    busy = b;
    voice.setBusy(b);   // 返事の最中に聞こえた声は、終わったあとに入力しない
    if (!b) {
      thinking = false; askPrompt = null;
      // 声で話しかけられていたら、返事のあと呼びかけなしで続けて話せるようにする
      if (lastWasVoice) { lastWasVoice = false; setTimeout(() => voice.listen(), 900); }
    }
    render();
  });
  on("thinking", b => { thinking = b; render(); });
  on("face", expr => { face.expression = expr; });
  on("flash", d => face.flash(d.expr, d.seconds));
  on("ask", q => { switchTab(lumiTab); askPrompt = q; setBuffer(""); thinking = false; voice.confirming(true); render(); keys.focus(); });
  on("i18n", m => { msgs = m; renderTabs(); });
  on("prompt", p => { PROMPT = p || NORMAL_PROMPT; render(); });
  on("basePrompt", p => { if (PROMPT === NORMAL_PROMPT) PROMPT = p; NORMAL_PROMPT = p; render(); });
  // コマンドの実行中とシェルモードは、顔の見た目を変える
  on("running", d => face.setRunning(!!d.on, d.cmd || ""));
  on("fx", d => face.effect(d.name, d.seconds));
  on("activity", d => face.setActivity(d ? d.name : null, d ? d.progress || 0 : 0));
  // 目がマウスを追いかける
  term.addEventListener("mousemove", e => {
    const r = document.getElementById("face").getBoundingClientRect();
    face.lookAt((e.clientX - r.left - r.width / 2) / (r.width / 2), (e.clientY - r.top - r.height / 2) / (r.height / 2));
  });
  on("shellMode", on => { lumiShell = !!on; updateFaceShell(); });
  // QR コードなどの画像を、ログの中に出す
  on("image", src => {
    const img = document.createElement("img");
    img.src = src;
    img.className = "logimg";
    current.appendChild(img);
    current = newLine();
    placeInput();
    log.scrollTop = log.scrollHeight;
  });
  // スマホで確認に答えたとき (答えは Go に届いているので、表示だけ整える)
  on("askAnswered", a => {
    if (askPrompt === null) return;
    write(askPrompt, "yellow");
    write(a + "  📱\n", "white");
    askPrompt = null; setBuffer("");
    voice.confirming(false);
    render();
  });
  // --script で流し込まれた行を、打ち込んだのと同じように表示する
  on("echo", text => { write(PROMPT, "fg"); write(text + "\n", "white"); if (!text.startsWith("/")) lastWasVoice = false; });
  on("admin", on => { admin = on; recolor(); });
  // 見た目の設定 (顔の色・大きさ、文字の大きさ・フォント)
  on("appearance", d => {
    base = baseColor(d.faceColor);
    face.sizeRatio = Math.max(0.2, Math.min(1, (d.faceSize || 60) / 100));
    recolor();
    document.body.style.fontSize = Math.max(10, Math.min(28, d.fontSize || 15)) + "px";
    document.body.style.fontFamily = d.font ? `"${d.font.replace(/"/g, "")}", var(--mono)` : "";
  });
  on("voiceStart", d => voice.start(d, msgs));
  on("voiceStop", () => voice.stop());
  on("whisperUnload", () => voice.unloadWhisper());
  // Whisper に見せる単語帳 (呼びかけのたびに Go から最新のものが届く)
  on("voiceVocab", v => { voice.vocab = v || ""; });
  // テスト用: マイクの代わりに音声ファイルを聞かせる
  on("voiceTest", async d => { await voice.start(d, msgs, false); await voice.feedFile(d.file); });
  on("speak", d => speak(d.id, d.text));
  on("stopSpeaking", () => { if (synth) synth.cancel(); if (speaking) speaking.stop(); });
  on("commands", list => { commands = list; });
  on("voiceSettings", d => { voiceName = d.voice || ""; rate = d.rate || 0; muted = !!d.muted; });
  on("listVoices", () => {
    loadVoices();
    const v = pickVoice();
    emit("voices", { names: voices.map(x => x.name + (x.lang ? "  (" + x.lang + ")" : "")), current: v ? v.name + (v.lang ? "  (" + v.lang + ")" : "") : "" });
  });

  placeInput();
  keys.focus();
  emit("ready");
}

// ================= 右下から出てくる窓 =================

function peekMain() {
  document.body.classList.add("peek");
  const peek = document.getElementById("peek"), bubble = document.getElementById("bubble");
  const textEl = document.getElementById("peekText");
  peek.hidden = false;
  let base = COLORS.cyan, admin = false;
  const face = new Face(palette(base, false), 0.9);
  animate(document.getElementById("peekFace"), face);
  const recolor = () => {
    face.c = palette(admin ? adminColor(base) : base, false);
    face.lastState = "";
    bubble.style.borderColor = face.c.line;
  };
  on("admin", on => { admin = on; recolor(); });
  on("appearance", d => { base = baseColor(d.faceColor); recolor(); });

  let doneTimer = null;
  let moving = false;
  on("peek", d => {
    textEl.textContent = d.text || "";
    clearTimeout(doneTimer);
    if (d.from) peek.classList.toggle("top", d.from === "top");   // 上の場所なら、上から出てくる
    if (d.mode === "pop" || d.mode === "move") {
      moving = d.mode === "move";
      face.expression = moving ? "happy" : "listen";
      face.poke(false);
      bubble.className = "";
      void bubble.offsetWidth;   // いったん引っ込めてから飛び出させる
      bubble.className = moving ? "pop moving" : "pop";
      return;
    }
    moving = false;
    face.expression = d.mode === "sleepy" ? "sleep" : "happy";
    bubble.className = d.mode === "sleepy" ? "sleepy" : "retract";
    doneTimer = setTimeout(() => emit("peekDone"), d.mode === "sleepy" ? 1650 : 350);
  });
  bubble.addEventListener("click", () => { if (!moving) emit("peekClicked"); });
  // 場所を決めている途中: ドラッグで動かし、ダブルクリックで決める
  bubble.addEventListener("dblclick", () => { if (moving) { moving = false; emit("peekMoved"); } });
}
