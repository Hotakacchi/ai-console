// Whisper の書き起こし (画面を止めないように別スレッドで動かす)。
// transformers.js とモデルはデータフォルダの /voice/whisper/ から読み、ネットには出ない。
//   受け取る: { id, type: "load" | "transcribe", model, audio (16kHz の Float32Array), lang }
//   返す:     { id, ok, text?, error? }

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

// 読み込みと書き起こしは 1 つずつ順番に行う
let queue = Promise.resolve();

self.onmessage = ({ data }) => {
  queue = queue.then(async () => {
    const { id, type, model, audio, lang } = data;
    try {
      const p = await load(model);
      if (type === "load") { postMessage({ id, ok: true }); return; }
      const out = await p(audio, { language: lang === "ja" ? "japanese" : "english", task: "transcribe" });
      postMessage({ id, ok: true, text: (out.text || "").trim() });
    } catch (e) {
      postMessage({ id, ok: false, error: e && e.message ? e.message : String(e) });
    }
  });
};
