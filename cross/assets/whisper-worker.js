// Whisper の書き起こし (画面を止めないように別スレッドで動かす)。
// transformers.js とモデルはデータフォルダの /voice/whisper/ から読み、ネットには出ない。
//   受け取る: { id, type: "load" | "transcribe", model, audio (16kHz の Float32Array), lang, prompt }
//   返す:     { id, ok, text?, error? }
// prompt (単語帳) を渡すと「前に話した文」として Whisper に見せ、その言葉を書き起こしやすくする。

let asr = null, loaded = "";

async function load(model) {
  if (asr && loaded === model) return asr;
  const { pipeline, env } = await import("/voice/whisper/transformers.min.js");
  env.allowRemoteModels = false;
  env.allowLocalModels = true;
  env.localModelPath = "/voice/whisper/models/";
  env.useBrowserCache = false;
  env.backends.onnx.wasm.wasmPaths = location.origin + "/voice/whisper/";
  env.backends.onnx.wasm.numThreads = 1;
  if (asr) await asr.dispose?.();
  asr = null;
  asr = await pipeline("automatic-speech-recognition", model, { dtype: "q8", device: "wasm" });
  loaded = model;
  return asr;
}

// 単語帳つきで書き起こす: <|startofprev|> 単語帳 <|startoftranscript|><|ja|><|transcribe|><|notimestamps|> から続きを作らせる
async function transcribeWithPrompt(p, audio, lang, prompt) {
  const tok = p.tokenizer;
  const id = t => {
    const v = tok.convert_tokens_to_ids(t);
    if (v === undefined || v === null) throw new Error("no token " + t);
    return v;
  };
  // 単語帳は長すぎると逆効果なので、後ろの 200 トークンまで
  const words = tok.encode(" " + prompt, { add_special_tokens: false }).slice(-200);
  const start = [id("<|startofprev|>"), ...words, id("<|startoftranscript|>"), id(`<|${lang}|>`), id("<|transcribe|>"), id("<|notimestamps|>")];
  const inputs = await p.processor(audio);
  const out = await p.model.generate({ ...inputs, decoder_input_ids: [start], max_new_tokens: 200 });
  const seq = Array.from(out.tolist ? out.tolist()[0] : out[0], Number).slice(start.length);
  return tok.decode(seq, { skip_special_tokens: true });
}

// 読み込みと書き起こしは 1 つずつ順番に行う
let queue = Promise.resolve();

self.onmessage = ({ data }) => {
  queue = queue.then(async () => {
    const { id, type, model, audio, lang, prompt } = data;
    try {
      const p = await load(model);
      if (type === "load") { postMessage({ id, ok: true }); return; }
      const code = lang === "ja" ? "ja" : "en";
      let text;
      if (prompt) {
        text = await transcribeWithPrompt(p, audio, code, prompt);
      } else {
        const out = await p(audio, { language: lang === "ja" ? "japanese" : "english", task: "transcribe" });
        text = out.text;
      }
      postMessage({ id, ok: true, text: (text || "").trim() });
    } catch (e) {
      postMessage({ id, ok: false, error: e && e.message ? e.message : String(e) });
    }
  });
};
