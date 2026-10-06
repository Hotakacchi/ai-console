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

if (location.hash === "#peek") peekMain(); else termMain();

// ================= メインの窓 =================

function termMain() {
  const term = document.getElementById("term"), log = document.getElementById("log");
  const keys = document.getElementById("keys");
  term.hidden = false;

  // 管理者権限で動いている間は、顔をオレンジ系にする
  const NORMAL = { line: "#223e42", fill: "#284e54", cheek: "#461e3c", bg: "#0c0c0c" };
  const ADMIN = { line: "#4e3214", fill: "#6a4418", cheek: "#5a1e2a", bg: "#0c0c0c" };
  const face = new Face(NORMAL);
  animate(document.getElementById("face"), face);

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

  // ---- 入力行 (今の行の最後に置く) ----
  const PROMPT = "C:\\Lumi> ";
  const input = document.createElement("span");
  input.id = "input";
  input.innerHTML = '<span class="prompt"></span><span class="typed"></span><span class="compose"></span>' +
    '<span class="cursor"></span><span class="ghost"></span><span class="hint"></span><span class="spinner"></span>';
  const [promptEl, typedEl, composeEl, cursorEl, ghostEl, hintEl, spinnerEl] = input.children;
  let buffer = "", compose = "", busy = false, thinking = false, askPrompt = null;
  let hint = "", lastWasVoice = false, msgs = {};
  let commands = [], history = [], historyIndex = 0, tabPrefix = null, tabIndex = -1;

  function placeInput() { current.appendChild(input); render(); }

  function render() {
    const editing = !busy || askPrompt !== null;
    promptEl.textContent = editing ? (askPrompt ?? PROMPT) : "";
    input.classList.toggle("asking", askPrompt !== null);
    typedEl.textContent = editing ? buffer : "";
    composeEl.textContent = editing ? compose : "";
    cursorEl.style.display = editing ? "" : "none";
    ghostEl.textContent = editing && !compose ? ghost() : "";
    hintEl.textContent = editing && !buffer && !compose && hint ? " " + hint : "";
    spinnerEl.textContent = !editing && thinking ? "|/-\\"[Math.floor(performance.now() / 125) % 4] : "";
    // 日本語変換の窓がカーソルの位置に出るよう、見えない入力欄を動かす
    const r = cursorEl.getBoundingClientRect(), t = term.getBoundingClientRect();
    keys.style.left = (r.left - t.left) + "px";
    keys.style.top = (r.top - t.top) + "px";
  }
  setInterval(() => { if (thinking) render(); }, 125);

  // 入力中の / コマンドの候補 (先頭一致) の残り部分
  function ghost() {
    if (askPrompt !== null || !buffer.startsWith("/") || buffer.includes(" ")) return "";
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
    if (!busy || askPrompt !== null) buffer += keys.value.replace(/\r?\n/g, " ");
    keys.value = "";
    tabPrefix = null;
    render();
  });
  keys.addEventListener("compositionupdate", e => { compose = e.data || ""; render(); });
  keys.addEventListener("compositionend", e => {
    compose = "";
    if (!busy || askPrompt !== null) buffer += e.data || "";
    keys.value = "";
    render();
  });

  keys.addEventListener("keydown", e => {
    lastKey = performance.now();
    face.poke(true);
    if (e.isComposing || e.keyCode === 229) return;
    const ctrl = e.ctrlKey || e.metaKey;
    if (askPrompt !== null && (e.key === "Enter" || e.key === "Escape" || (ctrl && e.key === "c"))) {
      e.preventDefault();
      if (e.key === "Enter") answerAsk(buffer, buffer, "");
      else answerAsk("", buffer + "^C", "");
      return;
    }
    if (e.key === "Escape" || (ctrl && e.key === "c" && !window.getSelection().toString())) {
      e.preventDefault();
      if (busy) emit("interrupt");
      else if (buffer) { write(PROMPT, "fg"); write(buffer + "^C\n", "white"); buffer = ""; render(); }
      return;
    }
    if (ctrl && e.key === "l") { e.preventDefault(); clear(); return; }
    if (busy) { if (e.key !== "Tab") return; e.preventDefault(); return; }
    if (e.key === "Tab") {
      // Tab を押すたびに候補を順に切り替える
      e.preventDefault();
      if (tabPrefix === null) { tabPrefix = buffer; tabIndex = -1; }
      const m = tabPrefix.startsWith("/") ? commands.filter(c => c.startsWith(tabPrefix)) : [];
      if (m.length) { tabIndex = (tabIndex + 1) % m.length; buffer = m[tabIndex]; render(); }
      return;
    }
    tabPrefix = null;
    if (e.key === "Enter") {
      e.preventDefault();
      const text = buffer;
      buffer = "";
      write(PROMPT, "fg");
      write(text + "\n", "white");
      if (text.trim()) { history.push(text); historyIndex = history.length; }
      lastWasVoice = false;
      emit("submit", text);
    } else if (e.key === "Backspace" && buffer) {
      e.preventDefault();
      buffer = Array.from(buffer).slice(0, -1).join("");
      render();
    } else if (e.key === "ArrowUp" && history.length) {
      e.preventDefault();
      historyIndex = Math.max(0, historyIndex - 1);
      buffer = history[historyIndex];
      render();
    } else if (e.key === "ArrowDown" && history.length) {
      e.preventDefault();
      historyIndex = Math.min(history.length, historyIndex + 1);
      buffer = history[historyIndex] ?? "";
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
    askPrompt = null; buffer = "";
    voice.confirming(false);
    emit("answer", answer);
    render();
  }

  // ---- 声での入力 ----
  const voice = new Voice({
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
    confirm(yes) { if (askPrompt !== null) answerAsk(yes ? "y" : "n", yes ? "y" : "n", msgs["voice.mark"]); },
    state(state, message) { emit("voiceState", { state, message: message || "" }); },
    debug(text) { emit("voiceDebug", text); },
  });
  // 喋っている間・考えている間は聞かない。喋り終わった直後も自分の声の残りを拾わないよう少し待つ
  let quietUntil = 0;
  setInterval(() => {
    const talking = !!speaking || (synth && synth.speaking);
    if (talking) quietUntil = performance.now() + 800;
    voice.paused = talking || performance.now() < quietUntil || (busy && askPrompt === null);
  }, 100);

  // ---- Go からのイベント ----
  on("write", d => write(d.text, d.color));
  on("clear", () => clear());
  on("busy", b => {
    busy = b;
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
  on("ask", q => { askPrompt = q; buffer = ""; thinking = false; voice.confirming(true); render(); keys.focus(); });
  on("i18n", m => { msgs = m; });
  on("admin", on => { face.c = on ? ADMIN : NORMAL; face.lastState = ""; });   // lastState を消して描き直させる
  on("voiceStart", d => voice.start(d, msgs));
  on("voiceStop", () => voice.stop());
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
  const NORMAL = { line: "#61d6d6", fill: "#61d6d6", cheek: "#e7488c", bg: "#0c0c0c" };
  const ADMIN = { line: "#ffa94d", fill: "#ffa94d", cheek: "#e7488c", bg: "#0c0c0c" };
  const face = new Face(NORMAL, 0.9);
  animate(document.getElementById("peekFace"), face);
  on("admin", on => {
    face.c = on ? ADMIN : NORMAL;
    face.lastState = "";
    bubble.style.borderColor = face.c.line;
  });

  let doneTimer = null;
  on("peek", d => {
    textEl.textContent = d.text || "";
    clearTimeout(doneTimer);
    if (d.mode === "pop") {
      face.expression = "listen";
      face.poke(false);
      bubble.className = "";
      void bubble.offsetWidth;   // いったん下に戻してから飛び出させる
      bubble.className = "pop";
      return;
    }
    face.expression = d.mode === "sleepy" ? "sleep" : "happy";
    bubble.className = d.mode === "sleepy" ? "sleepy" : "retract";
    doneTimer = setTimeout(() => emit("peekDone"), d.mode === "sleepy" ? 1650 : 350);
  });
  bubble.addEventListener("click", () => emit("peekClicked"));
}
