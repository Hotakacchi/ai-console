// 声での入力。Vosk (WebAssembly) を 2 つ並べて動かす。
//  - 呼びかけ用: 「ルミ」などの決まった言葉だけを聞き分ける (短い呼びかけでも聞き間違えにくい)
//  - 書き起こし用: 話した内容を文字にする (単語ごとの時刻つき)
// 呼びかけが聞こえたら、その後ろの言葉 (一息で続けた分も、少し間をあけた次の発言も) を受け取る。
// 確認の質問中は「実行して」「やめて」などだけを受け付ける。ルミが喋っている間などは paused にして聞かない。
// Whisper が入っていれば、話しかけた内容の部分の音だけを Whisper で書き起こし直す (より正確)。

const FOLLOW_UP_MS = 6000;
const WAKE_MIN_CONF = 0.5;
const LETTER_MIN_CONF = 0.3;
const KEEP_SEC = 30;            // 書き起こし直すために覚えておく音の長さ
const WHISPER_TIMEOUT_MS = 20000;

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
  async start({ lang, model, wake, whisper, whisperNative }, msgs, withMic = true) {
    this.stop();
    // 読み込み中に別の start / stop が来たら、この start は途中でやめる
    const gen = this.gen = (this.gen || 0) + 1;
    const stale = () => gen !== this.gen;
    this.lang = lang;
    this.whisper = whisper || null;
    this.whisperNative = !!whisperNative;   // whisper.cpp (Go 側) で書き起こす
    this.chunks = [];
    this.fedSec = 0;
    if (this.whisper && !this.whisperNative) this.whisperCall("load").catch(e => this.whisperFailed(e));
    const raw = key => (msgs[key] || "").split(",").map(s => s.trim()).filter(Boolean);
    const phrases = [...new Set([wake, ...raw("voice.wakeAliases")].filter(Boolean))];
    this.aliases = [...new Set(phrases.map(p => this.norm(p)))].sort((a, b) => b.length - a.length);
    this.yes = raw("voice.yes").map(s => this.norm(s));
    this.no = raw("voice.no").map(s => this.norm(s));
    // y / a / n をアルファベットの読みで答える (「ワイ」など。発言全体がその読みのときだけ)
    this.letters = { y: raw("voice.letterY"), a: raw("voice.letterA"), n: raw("voice.letterN") };
    for (const k in this.letters) this.letters[k] = this.letters[k].map(s => this.norm(s));
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
      // 確認の答え用は、アルファベットの読み (「ワイ」「エー」「エヌ」など) だけを聞き分ける
      // (短い音は書き起こしでは「あ」などになりやすいので、決まった言葉の中から選ばせる)
      const letterWords = [...new Set(Object.values(this.letters).flat())];
      if (letterWords.length) {
        this.letter = new this.model.KaldiRecognizer(rate, JSON.stringify([...letterWords, "[unk]"]));
        this.letter.setWords(true);
        this.letter.on("result", m => this.onLetter(m.result));
        this.letter.on("error", m => this.h.debug && this.h.debug("(letter error) " + JSON.stringify(m)));
      }
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
    if (this.letter && this.mode === "confirm") this.letter.acceptWaveformFloat(data.slice(), rate);
    if (this.whisper) {
      // 認識に渡した音を時刻つきで覚えておく (Vosk の単語の時刻は、渡した音の通算の秒)
      this.chunks.push({ t0: this.fedSec, rate, data: data.slice() });
      this.fedSec += data.length / rate;
      while (this.chunks.length && this.chunks[0].t0 < this.fedSec - KEEP_SEC) this.chunks.shift();
    }
  }

  // 覚えている音のうち from〜to 秒を 16kHz にして返す
  async clip(from, to) {
    const parts = this.chunks.filter(c => c.t0 + c.data.length / c.rate > from && c.t0 < to);
    if (!parts.length) return null;
    const rate = parts[0].rate;
    const start = Math.max(0, Math.floor((from - parts[0].t0) * rate));
    const all = new Float32Array(parts.reduce((n, c) => n + c.data.length, 0));
    let n = 0;
    for (const c of parts) { all.set(c.data, n); n += c.data.length; }
    const pcm = all.subarray(start, Math.min(all.length, Math.ceil((to - parts[0].t0) * rate)));
    if (pcm.length < rate * 0.2) return null;
    if (rate === 16000) return pcm.slice();
    const off = new OfflineAudioContext(1, Math.ceil(pcm.length * 16000 / rate), 16000);
    const buf = off.createBuffer(1, pcm.length, rate);
    buf.copyToChannel(pcm, 0);
    const src = off.createBufferSource();
    src.buffer = buf;
    src.connect(off.destination);
    src.start();
    return (await off.startRendering()).getChannelData(0);
  }

  whisperCall(type, audio) {
    if (this.whisperNative) {
      if (type !== "transcribe") return Promise.resolve("");
      return this.h.nativeWhisper(audio, this.lang, this.vocab || "");
    }
    if (!this.worker) {
      this.worker = new Worker("/whisper-worker.js", { type: "module" });
      this.waiting = new Map();
      this.worker.onmessage = ({ data }) => {
        const w = this.waiting.get(data.id);
        if (!w) return;
        this.waiting.delete(data.id);
        if (data.ok) w.resolve(data.text); else w.reject(new Error(data.error));
      };
      this.worker.onerror = e => {
        for (const w of this.waiting.values()) w.reject(new Error(e.message || "worker error"));
        this.waiting.clear();
      };
    }
    const id = this.callId = (this.callId || 0) + 1;
    return new Promise((resolve, reject) => {
      this.waiting.set(id, { resolve, reject });
      this.worker.postMessage({ id, type, model: this.whisper, audio, lang: this.lang, prompt: this.vocab || "" });
    });
  }

  // 先頭の呼びかけ (「ルミ、」など) を取る
  stripAlias(s) {
    const n = this.norm(s);
    const a = this.aliases.find(a => n.startsWith(a));
    if (!a) return s;
    let i = 0;
    while (i < s.length && this.norm(s.slice(0, i)).length < a.length) i++;
    return s.slice(i);
  }

  // 止めたことによる失敗は知らせない
  whisperFailed(e) {
    if (e.message !== "stopped" && this.h.whisperError) this.h.whisperError(e.message);
  }

  // 話しかけた内容の単語の範囲を Whisper で書き起こし直す (だめなら Vosk の文のまま)
  async transcribe(text, words) {
    if (!this.whisper || !words || !words.length) return text;
    const gen = this.gen;
    try {
      const audio = await this.clip(words[0].start - 0.1, words[words.length - 1].end + 0.4);
      if (!audio) return text;
      let timer;
      const timeout = new Promise((_, reject) => { timer = setTimeout(() => reject(new Error("timeout")), WHISPER_TIMEOUT_MS); });
      const out = await Promise.race([this.whisperCall("transcribe", audio), timeout]).finally(() => clearTimeout(timer));
      if (gen !== this.gen) return text;
      if (this.h.debug) this.h.debug("(whisper) " + out);
      // 呼びかけの残りが入っていたら取る
      let rest = this.stripAlias(out || "");
      rest = rest.replace(/^[\s、,。.!！?？]+/, "");
      return rest.replace(/[\s、,。.!！?？]/g, "") ? rest : text;
    } catch (e) {
      this.whisperFailed(e);
      return text;
    }
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

  // しばらく使っていないとき: Whisper のモデルを外してメモリを空ける (次に使うときに読み込み直す)。
  // 書き起こしの途中なら外さない
  unloadWhisper() {
    if (!this.worker || this.waiting.size > 0) return;
    this.worker.terminate();
    this.worker = this.waiting = null;
  }

  stop() {
    this.gen = (this.gen || 0) + 1;   // 読み込み中の start があれば無効にする
    const was = this.mode !== "off";
    this.mode = "off";
    clearInterval(this.timer);
    if (this.stream) this.stream.getTracks().forEach(t => t.stop());
    if (this.node) this.node.disconnect();
    if (this.ctx) this.ctx.close().catch(() => {});
    for (const r of [this.free, this.wake, this.letter]) if (r) try { r.remove(); } catch {}
    if (this.worker) {
      this.worker.terminate();
      for (const w of this.waiting.values()) w.reject(new Error("stopped"));
    }
    this.worker = this.waiting = null;
    this.chunks = [];
    if (this.model) try { this.model.terminate(); } catch {}
    this.stream = this.node = this.ctx = this.free = this.wake = this.letter = this.model = null;
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

  // words: 話しかけた内容の単語 (時刻つき)。Whisper で書き起こし直すのに使う
  heard(text, words) {
    this.mode = "wake";
    this.wakeEnd = null;
    this.pending = null;
    this.partialWake = false;
    if (!this.whisper) { this.h.heard(text); return; }
    const gen = this.gen;
    this.transcribe(text, words).then(t => { if (gen === this.gen) this.h.heard(t); });
  }

  // 書き起こしの文の n 文字目より後ろの単語
  wordsAfter(words, n) {
    let pos = 0;
    const out = [];
    for (const w of words) {
      if (pos >= n) out.push(w);
      pos += this.norm(w.word).length + (this.lang === "ja" ? 0 : 1);
    }
    return out;
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
      const words = this.afterWake(this.pending.words);
      this.pending = null;
      const rest = this.text(words);
      if (rest) { this.heard(rest, words); return; }
    }
    this.listen();
  }

  // 書き起こしのうち、呼びかけが終わった後の単語だけ
  afterWake(words) {
    if (this.wakeEnd === null) return [];
    return words.filter(w => w.start >= this.wakeEnd - 0.05);
  }

  text(words) { return this.join(words).replace(/^[\s、,。.!！?？]+/, ""); }

  // 確認の答え用の認識の結果: 発言全体がアルファベットの読み 1 つのときだけ答えにする
  onLetter(result) {
    if (this.mode !== "confirm") return;
    const words = result.result || [];
    // 決まった言葉の中から選ぶので、確からしさは低めでも受け付ける (それ以外の言葉は [unk] になる)
    if (words.length !== 1 || words[0].word === "[unk]" || words[0].conf < LETTER_MIN_CONF) return;
    if (this.h.debug) this.h.debug("(letter) " + words[0].word);
    const w = this.norm(words[0].word);
    const letter = Object.keys(this.letters).find(k => this.letters[k].includes(w));
    if (letter) this.h.confirm(letter);
  }

  // 書き起こし用の認識の結果
  onFree(result) {
    const t = this.norm(result.text);
    const words = result.result || [];
    if (!t || this.mode === "off") return;
    if (this.h.debug) this.h.debug(t);
    if (this.mode === "confirm") {
      const bare = t.replace(/[\s、,。.!！?？]/g, "");
      const letter = Object.keys(this.letters).find(k => this.letters[k].includes(bare));
      if (letter) this.h.confirm(letter);
      else if (this.yes.some(w => t.includes(w))) this.h.confirm("y");
      else if (this.no.some(w => t.includes(w))) this.h.confirm("n");
      return;
    }
    const fromPartial = this.partialWake;
    this.partialWake = false;
    const found = this.findAlias(t);
    if (found) {
      // 書き起こしにも呼びかけが入っていれば、その後ろを話しかけた内容にする
      const rest = t.slice(found.i + found.a.length).replace(/^[\s、,。.!！?？]+/, "");
      if (rest) this.heard(rest, this.wordsAfter(words, found.i + found.a.length)); else this.listen();
      return;
    }
    if (fromPartial) {
      // 途中では呼びかけだったのに確定で別の言葉になった: 最初の単語を呼びかけとみなし、残りを内容にする
      const rest = this.text(words.slice(1));
      if (rest) this.heard(rest, words.slice(1)); else this.listen();
      return;
    }
    if (this.wakeEnd !== null) {
      // 呼びかけ用の認識だけが聞き取った: 時刻で呼びかけの後ろを取り出す
      const after = this.afterWake(words);
      const rest = this.text(after);
      if (rest) this.heard(rest, after); else this.wakeEnd = null;
      return;
    }
    if (this.mode === "listening") { this.heard(t, words); return; }
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
