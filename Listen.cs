// 声での入力。Windows 標準の日本語音声認識を使う (音声は外に出ない)
//  - 「ルミ」と呼びかけると、続けて話した内容を受け付ける (「ルミ、今何時？」と一息でも可)
//  - コマンド実行の確認中は「実行して」「やめて」などの決まった言葉だけを受け付ける

using System;
using System.Linq;
using System.Speech.Recognition;

class VoiceInput : IDisposable
{
    // 呼びかけの後 (や返事の後) に、続きの言葉を待つ時間
    static readonly TimeSpan FollowUp = TimeSpan.FromSeconds(6);
    // 確認の言葉は聞き間違いで実行されないよう、自信が高いときだけ受け付ける
    const float ConfirmConfidence = 0.8f;
    static readonly string[] YesWords = { "実行して", "実行", "はい実行して", "お願い", "お願いします" };
    static readonly string[] NoWords = { "やめて", "やめる", "いいえ", "キャンセル", "実行しないで" };

    public event Action Woke;              // 聞く状態になった
    public event Action<string> Heard;     // 話しかけられた内容
    public event Action TimedOut;          // 聞く状態で、何も言われなかった
    public event Action<bool> Confirmed;   // 確認への答え (true = 実行)

    readonly SpeechRecognitionEngine engine;
    readonly Grammar wakeOnly, wakeWithText, dictation, confirm;
    readonly string[] wakeWords;
    readonly float minConfidence;
    DateTime listeningUntil = DateTime.MinValue;

    // 喋っている間などは true にして、聞こえた内容を無視する
    volatile bool paused;
    public bool Paused { get { return paused; } set { paused = value; } }

    // waveFile を渡すとマイクの代わりにその音声ファイルを聞く (テスト用)
    public VoiceInput(string wakeWord, float minConfidence, string waveFile = null)
    {
        var info = SpeechRecognitionEngine.InstalledRecognizers()
            .FirstOrDefault(r => r.Culture.Name == "ja-JP");
        if (info == null) throw new Exception("日本語の音声認識が入っていません（Windows の設定 → 時刻と言語 → 音声認識）");

        this.minConfidence = minConfidence;
        wakeWords = new[] { wakeWord, "ねえ" + wakeWord, wakeWord + "ちゃん", wakeWord + "さん" };
        engine = new SpeechRecognitionEngine(info);
        if (waveFile != null) engine.SetInputToWaveFile(waveFile);
        else engine.SetInputToDefaultAudioDevice();   // マイクがないとここで例外

        wakeOnly = new Grammar(new GrammarBuilder(new Choices(wakeWords))) { Name = "wake" };
        var gb = new GrammarBuilder(new Choices(wakeWords));
        gb.AppendDictation();
        wakeWithText = new Grammar(gb) { Name = "wake+text" };
        dictation = new DictationGrammar { Name = "dictation", Enabled = false };
        confirm = new Grammar(new GrammarBuilder(new Choices(YesWords.Concat(NoWords).ToArray()))) { Name = "confirm", Enabled = false };

        engine.LoadGrammar(wakeOnly);
        engine.LoadGrammar(wakeWithText);
        engine.LoadGrammar(dictation);
        engine.LoadGrammar(confirm);
        engine.EndSilenceTimeout = TimeSpan.FromMilliseconds(700);
        engine.SpeechRecognized += OnRecognized;
    }

    // 聞き取りを始める (イベントを登録してから呼ぶ)
    public void Start() { engine.RecognizeAsync(RecognizeMode.Multiple); }

    enum Mode { Wake, Listening, Confirm }
    Mode mode = Mode.Wake;

    void SetMode(Mode m)
    {
        mode = m;
        listeningUntil = m == Mode.Listening ? DateTime.Now + FollowUp : DateTime.MinValue;
        engine.RequestRecognizerUpdate();
        wakeOnly.Enabled = wakeWithText.Enabled = m == Mode.Wake;
        dictation.Enabled = m == Mode.Listening;
        confirm.Enabled = m == Mode.Confirm;
    }

    // 呼びかけなしで、しばらく話しかけを受け付ける (返事の後に続けて話せるように)
    public void Listen()
    {
        SetMode(Mode.Listening);
        if (Woke != null) Woke();
    }

    // コマンド実行の確認を声で受け付ける。EndConfirm で元に戻す
    public void BeginConfirm() { SetMode(Mode.Confirm); }
    public void EndConfirm() { if (mode == Mode.Confirm) SetMode(Mode.Wake); }

    // 待ち時間を過ぎたら呼びかけ待ちに戻す (UI のタイマーから定期的に呼ぶ)
    public void Tick()
    {
        if (mode == Mode.Listening && DateTime.Now > listeningUntil)
        {
            SetMode(Mode.Wake);
            if (TimedOut != null) TimedOut();
        }
    }

    // 認識結果をそのまま知らせる (調整・テスト用)
    public event Action<string, string, float> Raw;

    void OnRecognized(object sender, SpeechRecognizedEventArgs e)
    {
        if (Raw != null && e.Result != null) Raw(e.Result.Grammar.Name, e.Result.Text, e.Result.Confidence);
        if (paused || e.Result == null || e.Result.Confidence < minConfidence) return;
        string text = e.Result.Text.Trim();
        switch (e.Result.Grammar.Name)
        {
            case "wake":
                Listen();
                break;
            case "wake+text":
                // 先頭の呼びかけを取り除いて、残りを話しかけた内容にする
                string rest = text;
                foreach (string w in wakeWords.OrderByDescending(w => w.Length))
                    if (rest.StartsWith(w)) { rest = rest.Substring(w.Length); break; }
                rest = rest.TrimStart('、', '，', ',', ' ', '　');
                if (rest.Length == 0) Listen();
                else if (Heard != null) Heard(rest);
                break;
            case "dictation":
                SetMode(Mode.Wake);
                if (text.Length > 0 && Heard != null) Heard(text);
                break;
            case "confirm":
                if (e.Result.Confidence < ConfirmConfidence) return;
                SetMode(Mode.Wake);
                if (Confirmed != null) Confirmed(YesWords.Contains(text));
                break;
        }
    }

    public void Dispose()
    {
        try { engine.RecognizeAsyncCancel(); engine.Dispose(); } catch { }
    }
}
