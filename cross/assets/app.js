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

if (location.hash === "#peek") peekMain(); else termMain();

// ================= メインの窓 =================

function termMain() {
  const term = document.getElementById("term"), log = document.getElementById("log");
  const keys = document.getElementById("keys");
  term.hidden = false;

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

  // ---- 入力行 (今の行の最後に置く) ----
  // シェルモードでは Go から送られたプロンプト (PS C:\Users\…> など) に変わる
  const NORMAL_PROMPT = "C:\\Lumi> ";
  let PROMPT = NORMAL_PROMPT;
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
  on("prompt", p => { PROMPT = p || NORMAL_PROMPT; render(); });
  // コマンドの実行中とシェルモードは、顔の見た目を変える
  on("running", d => face.setRunning(!!d.on, d.cmd || ""));
  on("fx", d => face.effect(d.name, d.seconds));
  on("activity", d => face.setActivity(d ? d.name : null, d ? d.progress || 0 : 0));
  // 目がマウスを追いかける
  term.addEventListener("mousemove", e => {
    const r = document.getElementById("face").getBoundingClientRect();
    face.lookAt((e.clientX - r.left - r.width / 2) / (r.width / 2), (e.clientY - r.top - r.height / 2) / (r.height / 2));
  });
  on("shellMode", on => { face.shell = !!on; face.lastState = ""; });
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
    askPrompt = null; buffer = "";
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
