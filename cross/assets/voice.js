// 声での入力。Vosk (WebAssembly) を 2 つ並べて動かす。
//  - 呼びかけ用: 「ルミ」などの決まった言葉だけを聞き分ける (短い呼びかけでも聞き間違えにくい)
//  - 書き起こし用: 話した内容を文字にする (単語ごとの時刻つき)
// 呼びかけが聞こえたら、その後ろの言葉 (一息で続けた分も、少し間をあけた次の発言も) を受け取る。
// 確認の質問中は「実行して」「やめて」などだけを受け付ける。ルミが喋っている間などは paused にして聞かない。

const FOLLOW_UP_MS = 6000;
const WAKE_MIN_CONF = 0.5;

function loadScript(src) {
  return new Promise((resolve, reject) => {
    const s = document.createElement("script");
    s.src = src;
    s.onload = resolve;
    s.onerror = () => reject(new Error("failed to load " + src));
    document.head.appendChild(s);
  });
}

export class Voice {
  // handlers: { woke(), heard(text), timeout(), confirm(yes), state(state, message), debug(text) }
  constructor(handlers) {
    this.h = handlers;
    this.mode = "off";        // off / wake / listening / confirm
    this.paused = false;
    this.listenUntil = 0;
    this.wakeEnd = null;      // 呼びかけが終わった時刻 (秒、認識を始めてから)
    this.pending = null;      // 呼びかけの判定を待っている書き起こし
  }

  // msgs: 画面に渡された今の言語の文言 (voice.wakeAliases など)
  async start({ lang, model, wake }, msgs, withMic = true) {
    this.stop();
    // 読み込み中に別の start / stop が来たら、この start は途中でやめる
    const gen = this.gen = (this.gen || 0) + 1;
    const stale = () => gen !== this.gen;
    this.lang = lang;
    const raw = key => (msgs[key] || "").split(",").map(s => s.trim()).filter(Boolean);
    const phrases = [...new Set([wake, ...raw("voice.wakeAliases")].filter(Boolean))];
    this.aliases = [...new Set(phrases.map(p => this.norm(p)))].sort((a, b) => b.length - a.length);
    this.yes = raw("voice.yes").map(s => this.norm(s));
    this.no = raw("voice.no").map(s => this.norm(s));
    try {
      if (!window.Vosk) await loadScript("/voice/vosk.js");
      const loaded = await window.Vosk.createModel(model);
      if (stale()) { loaded.terminate(); return; }
      this.model = loaded;
      this.ctx = new AudioContext();
      const rate = this.ctx.sampleRate;
      this.free = new this.model.KaldiRecognizer(rate);
      this.free.setWords(true);
      this.free.on("result", m => this.onFree(m.result));
      this.free.on("partialresult", m => this.onPartial(m.result.partial));
      // 呼びかけ用は、呼びかけの言葉と「それ以外」だけを聞き分ける
      this.wake = new this.model.KaldiRecognizer(rate, JSON.stringify([...phrases, "[unk]"]));
      this.wake.setWords(true);
      this.wake.on("result", m => this.onWake(m.result));
      this.wake.on("error", m => this.h.debug && this.h.debug("(wake error) " + JSON.stringify(m)));
      this.free.on("error", m => this.h.debug && this.h.debug("(free error) " + JSON.stringify(m)));
      if (withMic) await this.openMic(stale);
      if (stale()) return;   // 新しい start / stop がこの分を片付けている
      this.mode = "wake";
      this.timer = setInterval(() => this.tick(), 200);
      this.h.state("ready");
    } catch (e) {
      if (stale()) return;
      this.stop();
      this.h.state("error", e && e.message ? e.message : String(e));
    }
  }

  // 同じ音を両方の認識に渡す (渡したデータは手放されるので、それぞれに複製する)
  feed(data, rate) {
    if (this.paused || this.mode === "off") return;
    this.free.acceptWaveformFloat(data.slice(), rate);
    this.wake.acceptWaveformFloat(data.slice(), rate);
  }

  async openMic(stale) {
    const stream = await navigator.mediaDevices.getUserMedia({
      audio: { echoCancellation: true, noiseSuppression: true, channelCount: 1 },
    });
    if (stale()) { stream.getTracks().forEach(t => t.stop()); return; }
    this.stream = stream;
    const src = this.ctx.createMediaStreamSource(this.stream);
    this.node = this.ctx.createScriptProcessor(4096, 1, 1);
    this.node.onaudioprocess = e => this.feed(e.inputBuffer.getChannelData(0), e.inputBuffer.sampleRate);
    src.connect(this.node);
    this.node.connect(this.ctx.destination);
  }

  stop() {
    this.gen = (this.gen || 0) + 1;   // 読み込み中の start があれば無効にする
    const was = this.mode !== "off";
    this.mode = "off";
    clearInterval(this.timer);
    if (this.stream) this.stream.getTracks().forEach(t => t.stop());
    if (this.node) this.node.disconnect();
    if (this.ctx) this.ctx.close().catch(() => {});
    for (const r of [this.free, this.wake]) if (r) try { r.remove(); } catch {}
    if (this.model) try { this.model.terminate(); } catch {}
    this.stream = this.node = this.ctx = this.free = this.wake = this.model = null;
    this.wakeEnd = this.pending = null;
    if (was) this.h.state("stopped");
  }

  get running() { return this.mode !== "off"; }

  // 日本語は単語の間の空白を取り、英語は小文字にそろえる
  norm(s) {
    s = (s || "").trim().toLowerCase();
    return this.lang === "ja" ? s.replace(/\s+/g, "") : s.replace(/\s+/g, " ");
  }

  join(words) { return words.map(w => w.word).join(this.lang === "ja" ? "" : " "); }

  // 呼びかけなしで、しばらく話しかけを受け付ける
  listen() {
    if (this.mode === "off") return;
    this.mode = "listening";
    this.listenUntil = performance.now() + FOLLOW_UP_MS;
    this.h.woke();
  }

  confirming(on) {
    if (this.mode === "off") return;
    this.mode = on ? "confirm" : "wake";
  }

  tick() {
    if (this.mode === "listening" && performance.now() > this.listenUntil) {
      this.mode = "wake";
      this.wakeEnd = null;
      this.partialWake = false;
      this.h.timeout();
    }
  }

  findAlias(t) {
    for (const a of this.aliases) {
      const i = t.indexOf(a);
      if (i >= 0) return { i, a };
    }
    return null;
  }

  heard(text) {
    this.mode = "wake";
    this.wakeEnd = null;
    this.pending = null;
    this.partialWake = false;
    this.h.heard(text);
  }

  // 書き起こしの途中で呼びかけに気づいたら、すぐ聞く顔にする
  // (確定した文では呼びかけが聞き間違えられていることがあるので、印を付けておく)
  onPartial(text) {
    if (this.mode === "wake" && this.findAlias(this.norm(text))) {
      this.partialWake = true;
      this.listen();
    }
  }

  // 呼びかけ用の認識の結果
  onWake(result) {
    if (this.mode === "off" || this.mode === "confirm") return;
    const words = (result.result || []).filter(w => w.word !== "[unk]");
    if (!words.length || !this.findAlias(this.norm(this.join(words)))) return;
    if (Math.min(...words.map(w => w.conf)) < WAKE_MIN_CONF) return;
    if (this.h.debug) this.h.debug("(wake) " + this.join(words));
    this.wakeEnd = words[words.length - 1].end;
    // 同じ発言の書き起こしが先に届いていれば、呼びかけより後ろを話しかけた内容にする
    if (this.pending && performance.now() - this.pending.at < 3000) {
      const rest = this.afterWake(this.pending.words);
      this.pending = null;
      if (rest) { this.heard(rest); return; }
    }
    this.listen();
  }

  // 書き起こしのうち、呼びかけが終わった後の単語だけ
  afterWake(words) {
    if (this.wakeEnd === null) return "";
    return this.join(words.filter(w => w.start >= this.wakeEnd - 0.05)).replace(/^[\s、,。.!！?？]+/, "");
  }

  // 書き起こし用の認識の結果
  onFree(result) {
    const t = this.norm(result.text);
    const words = result.result || [];
    if (!t || this.mode === "off") return;
    if (this.h.debug) this.h.debug(t);
    if (this.mode === "confirm") {
      if (this.yes.some(w => t.includes(w))) this.h.confirm(true);
      else if (this.no.some(w => t.includes(w))) this.h.confirm(false);
      return;
    }
    const fromPartial = this.partialWake;
    this.partialWake = false;
    const found = this.findAlias(t);
    if (found) {
      // 書き起こしにも呼びかけが入っていれば、その後ろを話しかけた内容にする
      const rest = t.slice(found.i + found.a.length).replace(/^[\s、,。.!！?？]+/, "");
      if (rest) this.heard(rest); else this.listen();
      return;
    }
    if (fromPartial) {
      // 途中では呼びかけだったのに確定で別の言葉になった: 最初の単語を呼びかけとみなし、残りを内容にする
      const rest = this.join(words.slice(1)).replace(/^[\s、,。.!！?？]+/, "");
      if (rest) this.heard(rest); else this.listen();
      return;
    }
    if (this.wakeEnd !== null) {
      // 呼びかけ用の認識だけが聞き取った: 時刻で呼びかけの後ろを取り出す
      const rest = this.afterWake(words);
      if (rest) this.heard(rest); else this.wakeEnd = null;
      return;
    }
    if (this.mode === "listening") { this.heard(t); return; }
    this.pending = { words, at: performance.now() };   // 呼びかけ用の結果が後から届くかもしれない
  }

  // テスト用: マイクの代わりに音声ファイルを聞かせる
  async feedFile(url) {
    const buf = await (await fetch(url)).arrayBuffer();
    const audio = await this.ctx.decodeAudioData(buf);
    const data = audio.getChannelData(0);
    for (let i = 0; i < data.length; i += 4096) {
      this.feed(data.subarray(i, i + 4096), audio.sampleRate);
      await new Promise(r => setTimeout(r, 20));
    }
    // 終わりに無音を足して、最後の発言を確定させる
    const silence = new Float32Array(audio.sampleRate);
    for (let i = 0; i < 3; i++) { this.feed(silence, audio.sampleRate); await new Promise(r => setTimeout(r, 50)); }
  }
}
