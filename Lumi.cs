// ターミナル風の顔つきアシスタント「ルミ」
// 普通のコンソールと同じ 120x30 の文字画面で、背景にうっすら顔を描く。
// .NET Framework 4.8 だけで動く単体 exe。

using System;
using System.Collections.Concurrent;
using System.Collections.Generic;
using System.Drawing;
using System.Drawing.Drawing2D;
using System.Linq;
using System.Runtime.InteropServices;
using System.Speech.Synthesis;
using System.Text;
using System.Threading;
using System.Windows.Forms;

// Windows Terminal の標準配色 (Campbell)
static class Pal
{
    public static readonly Color Bg = Color.FromArgb(12, 12, 12);
    public static readonly Color Fg = Color.FromArgb(204, 204, 204);
    public static readonly Color White = Color.FromArgb(242, 242, 242);
    public static readonly Color Dim = Color.FromArgb(118, 118, 118);
    public static readonly Color Red = Color.FromArgb(231, 72, 86);
    public static readonly Color Yellow = Color.FromArgb(249, 241, 165);
    public static readonly Color BGreen = Color.FromArgb(22, 198, 12);

    // 背景の顔 (文字の邪魔にならない暗さ)
    public static readonly Color FaceLine = Color.FromArgb(34, 62, 66);
    public static readonly Color FaceFill = Color.FromArgb(40, 78, 84);
    public static readonly Color FaceCheek = Color.FromArgb(70, 30, 60);
    // 飛び出す顔 (明るい)
    public static readonly Color FaceBright = Color.FromArgb(97, 214, 214);
}

class Seg
{
    public readonly string Text;
    public readonly Color Color;
    public Seg(string text, Color color) { Text = text; Color = color; }
}

// ---------------- 背景の顔 ----------------

class Face
{
    public volatile string Expression = "normal";   // normal / happy / think / listen
    public volatile bool Speaking;

    // 顔の大きさ (描く場所に対する割合) と色。背景の顔は暗く、飛び出す顔は明るく描く
    public float SizeRatio = 0.6f;
    public Color LineColor = Pal.FaceLine, FillColor = Pal.FaceFill, CheekColor = Pal.FaceCheek, BackColor = Pal.Bg;
    // これだけ何もしないと眠る
    static readonly TimeSpan SleepAfter = TimeSpan.FromSeconds(90);

    readonly Random rnd = new Random();
    readonly DateTime born = DateTime.Now;
    float open, target;                         // 口の開き
    float lookX, lookY, lookXT, lookYT;         // 目線 (-1〜1)
    float tilt, tiltT;                          // 首のかしげ (度)
    float eyeScale = 1;                         // 目の大きさ
    DateTime nextMouth, lastViseme, blinkStart, nextBlink = DateTime.Now.AddSeconds(2);
    DateTime nextGlance = DateTime.Now.AddSeconds(4), glanceUntil;
    DateTime lastActivity = DateTime.Now, typingUntil;
    DateTime flashUntil;
    string flashExpr;
    string lastState = "";

    // SAPI の viseme 番号ごとの口の開き具合
    static readonly float[] VisemeOpen = {
        0f, .6f, 1f, .8f, .5f, .6f, .4f, .5f, .7f, .8f, .7f,
        .8f, .3f, .4f, .3f, .2f, .3f, .3f, .2f, .3f, .4f, 0f };

    public void OnViseme(int v)
    {
        lastViseme = DateTime.Now;
        if (v >= 0 && v < VisemeOpen.Length) target = VisemeOpen[v];
    }

    // キー入力などがあったとき (目線を入力行に向け、眠っていたら起きる)
    public void Poke(bool typing)
    {
        lastActivity = DateTime.Now;
        if (typing) typingUntil = DateTime.Now.AddMilliseconds(900);
    }

    // しばらくだけ表情を変える (happy / sad)
    public void Flash(string expression, double seconds)
    {
        flashExpr = expression;
        flashUntil = DateTime.Now.AddSeconds(seconds);
    }

    string Current
    {
        get
        {
            DateTime now = DateTime.Now;
            if (Speaking) lastActivity = now;
            if (now < flashUntil) return flashExpr;
            string ex = Expression;
            if (ex == "normal" && now - lastActivity > SleepAfter) return "sleep";
            return ex;
        }
    }

    float Blink(DateTime now)
    {
        double t = (now - blinkStart).TotalMilliseconds;
        return t < 160 ? (float)(1 - Math.Abs(t - 80) / 80) : 0;
    }

    // 状態を進め、見た目が変わったら true を返す
    public bool Tick()
    {
        DateTime now = DateTime.Now;
        double t = (now - born).TotalSeconds;
        string ex = Current;

        // まばたき (ときどき 2 回続けて)
        if (now >= nextBlink)
        {
            blinkStart = now;
            nextBlink = rnd.NextDouble() < 0.15 ? now.AddMilliseconds(300) : now.AddSeconds(2.5 + rnd.NextDouble() * 3.5);
        }

        // 目線と首のかしげ
        if (ex == "think") { lookXT = 0.6f; lookYT = -0.7f; tiltT = 5; }
        else if (now < typingUntil) { lookXT = -0.7f; lookYT = 0.6f; tiltT = -3; }
        else if (ex == "listen" || ex == "sleep" || Speaking) { lookXT = 0; lookYT = 0; tiltT = 0; }
        else if (now >= nextGlance)
        {
            // ときどきよそ見をする
            lookXT = (float)(rnd.NextDouble() * 1.6 - 0.8);
            lookYT = (float)(rnd.NextDouble() * 0.8 - 0.4);
            tiltT = lookXT * 5;
            glanceUntil = now.AddSeconds(0.8 + rnd.NextDouble());
            nextGlance = now.AddSeconds(4 + rnd.NextDouble() * 5);
        }
        else if (now >= glanceUntil) { lookXT = 0; lookYT = 0; tiltT = 0; }
        lookX += (lookXT - lookX) * 0.2f;
        lookY += (lookYT - lookY) * 0.2f;
        tilt += (tiltT - tilt) * 0.12f;
        eyeScale += ((ex == "listen" ? 1.25f : 1f) - eyeScale) * 0.25f;

        // 口 (viseme が来ない声・ミュート時はランダムに動かす)
        if (!Speaking) target = 0;
        else if ((now - lastViseme).TotalMilliseconds > 250 && now >= nextMouth)
        {
            target = 0.15f + (float)rnd.NextDouble() * 0.85f;
            nextMouth = now.AddMilliseconds(80 + rnd.Next(70));
        }
        open += (target - open) * 0.45f;
        if (open < 0.01f) open = 0;

        // 見た目に関わる値を丸めて比べ、変わったときだけ描き直す
        double breathe = ex == "sleep" ? 0.8 : 1.6;
        string state = ex + Speaking + Math.Round(Math.Sin(t * breathe) * 4) + Math.Round(Blink(now) * 6) +
                       Math.Round(lookX * 12) + "," + Math.Round(lookY * 12) + "," + Math.Round(tilt) + "," +
                       Math.Round(eyeScale * 20) + "," + Math.Round(open * 30) +
                       (ex == "think" || ex == "listen" || ex == "sleep" ? ((int)(t * 12)).ToString() : "");
        bool changed = state != lastState;
        lastState = state;
        return changed;
    }

    public void Draw(Graphics g, Rectangle area)
    {
        DateTime now = DateTime.Now;
        double t = (now - born).TotalSeconds;
        string ex = Current;
        g.SmoothingMode = SmoothingMode.AntiAlias;
        var saved = g.Save();
        float s = Math.Min(area.Width / 640f, area.Height / 440f) * SizeRatio;
        float bob = (float)Math.Sin(t * (ex == "sleep" ? 0.8 : 1.6)) * 6 + (Speaking ? open * 5 : 0);
        g.TranslateTransform(area.X + area.Width / 2f, area.Y + area.Height / 2f);
        g.ScaleTransform(s, s);
        g.TranslateTransform(0, bob);
        g.RotateTransform(tilt);

        using (var line = new Pen(LineColor, 7) { StartCap = LineCap.Round, EndCap = LineCap.Round })
        using (var fill = new SolidBrush(FillColor))
        using (var bg = new SolidBrush(BackColor))
        {
            using (var p = RoundRect(-300, -200, 600, 400, 90)) g.DrawPath(line, p);

            // 呼ばれて聞いているとき: 両側に音の波
            if (ex == "listen")
                for (int k = 0; k < 3; k++)
                {
                    float ph = (float)((t * 1.2 + k / 3.0) % 1);
                    using (var wave = new Pen(Lerp(FillColor, BackColor, ph), 6) { StartCap = LineCap.Round, EndCap = LineCap.Round })
                    {
                        float r = 330 + ph * 70;
                        g.DrawArc(wave, -r, -r * 0.7f, r * 2, r * 1.4f, 160, 40);
                        g.DrawArc(wave, -r, -r * 0.7f, r * 2, r * 1.4f, -20, 40);
                    }
                }

            // 目
            float blink = Blink(now);
            foreach (float ex0 in new float[] { -120, 120 })
            {
                float x = ex0 + lookX * 22, y = -45 + lookY * 16;
                if (ex == "happy") g.DrawArc(line, x - 34, y - 20, 68, 60, 200, 140);
                else if (ex == "sleep") g.DrawArc(line, x - 32, y - 30, 64, 44, 30, 120);
                else
                {
                    float r = 30 * eyeScale, h = r * (1 - blink);
                    if (h < 4) g.DrawLine(line, x - r, y, x + r, y);
                    else
                    {
                        g.FillEllipse(fill, x - r, y - h, r * 2, h * 2);
                        if (h > r * 0.5f) g.FillEllipse(bg, x + r * 0.25f, y - h * 0.55f, r * 0.32f, r * 0.32f);   // 目のハイライト
                    }
                    if (ex == "sad")   // 困り眉 (内側を上げる)
                    {
                        if (ex0 < 0) g.DrawLine(line, x - 30, y - 48, x + 25, y - 62);
                        else g.DrawLine(line, x - 25, y - 62, x + 30, y - 48);
                    }
                }
            }

            // ほっぺ
            if (ex == "happy" || Speaking)
                using (var pk = new Pen(CheekColor, 6) { StartCap = LineCap.Round, EndCap = LineCap.Round })
                    foreach (float bx in new float[] { -215, 165 })
                        for (int i = 0; i < 3; i++) g.DrawLine(pk, bx + i * 18, 40, bx + i * 18 + 12, 18);

            // 口
            const float my = 95;
            if (Speaking || open > 0)
            {
                float h = 6 + open * 60;
                g.DrawEllipse(line, -48, my - h / 2, 96, h);
            }
            else if (ex == "think") g.DrawBezier(line, -24, my, -8, my - 14, 8, my + 14, 24, my);
            else if (ex == "sad") g.DrawArc(line, -40, my - 4, 80, 50, 210, 120);
            else if (ex == "sleep") g.DrawEllipse(line, -10, my - 6, 20, 16);
            else if (ex == "happy") g.DrawArc(line, -64, my - 62, 128, 90, 25, 130);
            else g.DrawArc(line, -52, my - 50, 104, 72, 30, 120);

            // 考え中: 跳ねる 3 つの点
            if (ex == "think")
                for (int i = 0; i < 3; i++)
                {
                    float jump = (float)Math.Max(0, Math.Sin(t * 6 - i * 0.9)) * 18;
                    g.FillEllipse(fill, 250 + i * 36, -180 - jump, 20, 20);
                }

            // 眠っている: 浮かんでいく z
            if (ex == "sleep")
                using (var font = new Font("Consolas", 40, FontStyle.Bold))
                    for (int i = 0; i < 2; i++)
                    {
                        float ph = (float)((t * 0.35 + i * 0.5) % 1);
                        using (var zb = new SolidBrush(Lerp(FillColor, BackColor, ph)))
                            g.DrawString("z", font, zb, 230 + ph * 60, -150 - ph * 90);
                    }
        }
        g.Restore(saved);
        g.SmoothingMode = SmoothingMode.None;
    }

    static Color Lerp(Color a, Color b, float k)
    {
        k = Math.Max(0, Math.Min(1, k));
        return Color.FromArgb((int)(a.R + (b.R - a.R) * k), (int)(a.G + (b.G - a.G) * k), (int)(a.B + (b.B - a.B) * k));
    }

    static GraphicsPath RoundRect(float x, float y, float w, float h, float r)
    {
        var p = new GraphicsPath();
        float d = r * 2;
        p.AddArc(x, y, d, d, 180, 90);
        p.AddArc(x + w - d, y, d, d, 270, 90);
        p.AddArc(x + w - d, y + h - d, d, d, 0, 90);
        p.AddArc(x, y + h - d, d, d, 90, 90);
        p.CloseFigure();
        return p;
    }
}

// ---------------- 端末ビュー ----------------

class TermView : Control
{
    const string Prompt = @"C:\Lumi> ";

    public readonly Face Face = new Face();
    public event Action<string> Submitted;
    public event Action Interrupted;
    public bool Busy;
    public bool Thinking;
    public string Hint = "";
    public bool LastWasVoice;   // 最後の入力が声だったか
    public string[] CommandNames = new string[0];   // Tab で補完する / コマンド
    string tabPrefix;
    int tabIndex;

    // 入力中の / コマンドの候補 (先頭一致)
    string[] Matches(string prefix)
    {
        return CommandNames.Where(c => c.StartsWith(prefix, StringComparison.OrdinalIgnoreCase)).ToArray();
    }

    // 入力の後ろに薄く出す補完候補の残り部分
    string Ghost()
    {
        string s = input.ToString();
        if (askPrompt != null || !s.StartsWith("/") || s.Contains(" ")) return "";
        var m = Matches(s);
        return m.Length > 0 && m[0].Length > s.Length ? m[0].Substring(s.Length) : "";
    }   // 入力行の後ろに薄く出す案内 (「聞いています…」など)

    // 確認の質問中は、プロンプトの代わりに質問を出し、Enter の内容を onAnswer に渡す
    string askPrompt;
    Action<string> askCallback;

    public void Ask(string prompt, Action<string> onAnswer)
    {
        if (InvokeRequired) { BeginInvoke(new Action<string, Action<string>>(Ask), prompt, onAnswer); return; }
        askPrompt = prompt;
        askCallback = onAnswer;
        input.Clear();
        Busy = false;
        Thinking = false;
        ShowCursorNow();
    }

    // 確認の質問に声で答えたとき
    public void AnswerVoice(string text)
    {
        if (askCallback != null) Answer(text, false, "  (音声)");
    }

    void Answer(string text, bool interrupted, string note = "")
    {
        var cb = askCallback;
        Write(askPrompt, Pal.Yellow);
        Write(text + (interrupted ? "^C" : ""), Pal.White);
        Write(note + "\n", Pal.Dim);
        askPrompt = null;
        askCallback = null;
        input.Clear();
        Busy = true;
        cb(interrupted ? "" : text.Trim());
    }

    string CurrentPrompt { get { return askPrompt ?? Prompt; } }

    // 音声で聞き取った内容を、打ち込まれたのと同じように送る (確認の質問には音声では答えられない)
    public void SubmitVoice(string text)
    {
        Face.Poke(false);
        if (Busy || askCallback != null) return;
        input.Clear();
        Write(Prompt, Pal.Fg);
        Write(text, Pal.White);
        Write("  (音声)\n", Pal.Dim);
        history.Add(text);
        historyIndex = history.Count;
        LastWasVoice = true;
        if (Submitted != null) Submitted(text.Trim());
    }

    readonly Font mono, cjk;
    public readonly int CellW, CellH;
    readonly int cjkOffset, cjkDy;
    readonly List<List<Seg>> log = new List<List<Seg>>();
    readonly StringBuilder input = new StringBuilder();
    readonly List<string> history = new List<string>();
    int historyIndex, scroll;
    DateTime lastBlink = DateTime.Now;
    bool cursorOn = true;
    const TextFormatFlags Flags = TextFormatFlags.NoPadding | TextFormatFlags.NoPrefix | TextFormatFlags.SingleLine;

    public TermView()
    {
        SetStyle(ControlStyles.OptimizedDoubleBuffer | ControlStyles.AllPaintingInWmPaint |
                 ControlStyles.UserPaint | ControlStyles.ResizeRedraw | ControlStyles.Selectable, true);
        BackColor = Pal.Bg;
        ImeMode = ImeMode.On;
        mono = new Font(FirstFont("Cascadia Mono", "Consolas"), 12f);
        Size m = TextRenderer.MeasureText(new string('M', 20), mono, Size.Empty, Flags);
        CellW = (int)Math.Round(m.Width / 20.0);
        CellH = (int)(m.Height * 1.15);

        // 日本語は 2 セルにほぼ収まる大きさにし、ベースラインを英数字にそろえる
        string jp = FirstFont("BIZ UDGothic", "BIZ UDゴシック", "MS Gothic");
        float size = 12f;
        using (var probe = new Font(jp, size))
        {
            int w = TextRenderer.MeasureText("あ", probe, Size.Empty, Flags).Width;
            size = Math.Min(size * CellW * 2 * 0.9f / w, size * 1.25f);
        }
        cjk = new Font(jp, size);
        cjkOffset = Math.Max(0, (CellW * 2 - TextRenderer.MeasureText("あ", cjk, Size.Empty, Flags).Width) / 2);
        cjkDy = (int)Math.Round(AscentPx(mono) - AscentPx(cjk));
        log.Add(new List<Seg>());
    }

    static float AscentPx(Font f)
    {
        FontFamily ff = f.FontFamily;
        float px = f.Size * f.FontFamily.GetCellAscent(f.Style) / ff.GetEmHeight(f.Style);
        using (var bmp = new Bitmap(1, 1))
        using (Graphics g = Graphics.FromImage(bmp))
            return px * g.DpiY / 72f;
    }

    static string FirstFont(params string[] names)
    {
        foreach (string n in names)
            try { using (var f = new FontFamily(n)) return f.Name; } catch (ArgumentException) { }
        return FontFamily.GenericMonospace.Name;
    }

    protected override bool CanEnableIme { get { return true; } }

    // ---- 出力 ----

    public void Write(string text, Color color)
    {
        if (InvokeRequired) { BeginInvoke(new Action<string, Color>(Write), text, color); return; }
        string[] parts = text.Split('\n');
        for (int i = 0; i < parts.Length; i++)
        {
            if (i > 0) log.Add(new List<Seg>());
            if (parts[i].Length > 0) log[log.Count - 1].Add(new Seg(parts[i], color));
        }
        if (log.Count > 2000) log.RemoveRange(0, log.Count - 2000);
        scroll = 0;
        Invalidate();
    }

    public void ClearScreen()
    {
        log.Clear();
        log.Add(new List<Seg>());
        scroll = 0;
        Invalidate();
    }

    // ---- 入力 ----

    protected override bool IsInputKey(Keys k) { return true; }

    protected override void OnMouseDown(MouseEventArgs e) { Focus(); base.OnMouseDown(e); }

    protected override void OnMouseWheel(MouseEventArgs e)
    {
        scroll = Math.Max(0, scroll + (e.Delta > 0 ? 3 : -3));
        Invalidate();
    }

    protected override void OnKeyDown(KeyEventArgs e)
    {
        Face.Poke(true);
        if (e.KeyCode == Keys.Tab && !Busy && askCallback == null)
        {
            // Tab を押すたびに候補を順に切り替える
            if (tabPrefix == null) { tabPrefix = input.ToString(); tabIndex = -1; }
            var m = tabPrefix.StartsWith("/") ? Matches(tabPrefix) : new string[0];
            if (m.Length > 0)
            {
                tabIndex = (tabIndex + 1) % m.Length;
                input.Clear().Append(m[tabIndex]);
            }
            e.SuppressKeyPress = true;
            ShowCursorNow();
            return;
        }
        tabPrefix = null;
        bool ctrl = e.Control;
        if (askCallback != null && ((ctrl && e.KeyCode == Keys.C) || e.KeyCode == Keys.Escape))
            Answer(input.ToString(), true);   // 確認中の中断は「いいえ」
        else if (askCallback != null && e.KeyCode == Keys.Enter)
            Answer(input.ToString(), false);
        else if ((ctrl && e.KeyCode == Keys.C) || e.KeyCode == Keys.Escape)
        {
            if (Interrupted != null) Interrupted();
            if (!Busy && input.Length > 0) { Write(Prompt, Pal.Fg); Write(input + "^C\n", Pal.White); input.Clear(); }
        }
        else if (ctrl && e.KeyCode == Keys.V)
        {
            if (Clipboard.ContainsText()) input.Append(Clipboard.GetText().Replace("\r", "").Replace("\n", " "));
        }
        else if (ctrl && e.KeyCode == Keys.L) ClearScreen();
        else if (e.KeyCode == Keys.PageUp) scroll += 10;
        else if (e.KeyCode == Keys.PageDown) scroll = Math.Max(0, scroll - 10);
        else if (Busy) { }
        else if (e.KeyCode == Keys.Enter)
        {
            string text = input.ToString();
            input.Clear();
            Write(Prompt, Pal.Fg);
            Write(text + "\n", Pal.White);
            if (text.Trim().Length > 0)
            {
                history.Add(text);
                historyIndex = history.Count;
            }
            LastWasVoice = false;
            if (Submitted != null) Submitted(text.Trim());
        }
        else if (e.KeyCode == Keys.Back && input.Length > 0)
        {
            int n = input.Length >= 2 && char.IsLowSurrogate(input[input.Length - 1]) ? 2 : 1;
            input.Remove(input.Length - n, n);
        }
        else if (e.KeyCode == Keys.Up && history.Count > 0)
        {
            historyIndex = Math.Max(0, historyIndex - 1);
            input.Clear().Append(history[historyIndex]);
        }
        else if (e.KeyCode == Keys.Down && history.Count > 0)
        {
            historyIndex = Math.Min(history.Count, historyIndex + 1);
            input.Clear();
            if (historyIndex < history.Count) input.Append(history[historyIndex]);
        }
        else return;
        e.SuppressKeyPress = ctrl || e.KeyCode == Keys.Enter || e.KeyCode == Keys.Escape;
        ShowCursorNow();
    }

    protected override void OnKeyPress(KeyPressEventArgs e)
    {
        if (Busy || e.KeyChar < ' ' || ModifierKeys == Keys.Control) return;
        input.Append(e.KeyChar);
        ShowCursorNow();
    }

    void ShowCursorNow()
    {
        scroll = 0;
        cursorOn = true;
        lastBlink = DateTime.Now;
        Invalidate();
    }

    // ---- 毎フレーム: 顔が動いたとき・カーソル点滅・スピナーのときだけ再描画 ----

    public void Tick()
    {
        bool dirty = Face.Tick() || Thinking;
        if ((DateTime.Now - lastBlink).TotalMilliseconds >= 530)
        {
            cursorOn = !cursorOn;
            lastBlink = DateTime.Now;
            dirty = true;
        }
        if (dirty) Invalidate();
    }

    // ---- 描画 ----

    static bool IsWide(int c)
    {
        return (c >= 0x1100 && c <= 0x115F) || (c >= 0x2E80 && c <= 0xA4CF) || (c >= 0xAC00 && c <= 0xD7A3) ||
               (c >= 0xF900 && c <= 0xFAFF) || (c >= 0xFE30 && c <= 0xFE4F) || (c >= 0xFF00 && c <= 0xFF60) ||
               (c >= 0xFFE0 && c <= 0xFFE6) || c >= 0x1F300;
    }

    // 文字列を「1グリフ」単位に分けて、幅 (1 or 2 セル) と一緒に列挙する
    static IEnumerable<KeyValuePair<string, int>> Glyphs(string s)
    {
        for (int i = 0; i < s.Length; i++)
        {
            if (char.IsHighSurrogate(s[i]) && i + 1 < s.Length)
            {
                yield return new KeyValuePair<string, int>(s.Substring(i, 2), 2);
                i++;
            }
            else yield return new KeyValuePair<string, int>(s[i].ToString(), IsWide(s[i]) ? 2 : 1);
        }
    }

    // 1行を cols 幅で折り返す。endCol に最後の文字の次の位置を返す
    static List<List<Seg>> Wrap(List<Seg> line, int cols, out int endCol)
    {
        var rows = new List<List<Seg>> { new List<Seg>() };
        int col = 0;
        foreach (Seg seg in line)
        {
            var sb = new StringBuilder();
            foreach (var g in Glyphs(seg.Text))
            {
                if (col + g.Value > cols)
                {
                    if (sb.Length > 0) rows[rows.Count - 1].Add(new Seg(sb.ToString(), seg.Color));
                    sb.Clear();
                    rows.Add(new List<Seg>());
                    col = 0;
                }
                sb.Append(g.Key);
                col += g.Value;
            }
            if (sb.Length > 0) rows[rows.Count - 1].Add(new Seg(sb.ToString(), seg.Color));
        }
        endCol = col;
        return rows;
    }

    // ASCII はまとめて描き、それ以外は 1 文字ずつセル位置に置く (代替フォントで幅がずれないように)
    void DrawRow(Graphics g, List<Seg> segs, int row)
    {
        int y = row * CellH, col = 0;
        foreach (Seg seg in segs)
        {
            var run = new StringBuilder();
            int runCol = col;
            foreach (var gl in Glyphs(seg.Text))
            {
                if (gl.Key.Length == 1 && gl.Key[0] >= ' ' && gl.Key[0] <= '~')
                {
                    if (run.Length == 0) runCol = col;
                    run.Append(gl.Key);
                    col++;
                    continue;
                }
                if (run.Length > 0) TextRenderer.DrawText(g, run.ToString(), mono, new Point(runCol * CellW, y), seg.Color, Flags);
                run.Clear();
                if (gl.Value == 2)
                    TextRenderer.DrawText(g, gl.Key, cjk, new Point(col * CellW + cjkOffset, y + cjkDy), seg.Color, Flags);
                else
                    TextRenderer.DrawText(g, gl.Key, mono, new Point(col * CellW, y), seg.Color, Flags);
                col += gl.Value;
            }
            if (run.Length > 0) TextRenderer.DrawText(g, run.ToString(), mono, new Point(runCol * CellW, y), seg.Color, Flags);
        }
    }

    protected override void OnPaint(PaintEventArgs e)
    {
        Graphics g = e.Graphics;
        g.Clear(Pal.Bg);
        Face.Draw(g, ClientRectangle);

        int cols = Math.Max(10, Width / CellW), rows = Math.Max(3, Height / CellH);

        // ログ + 入力行 (末尾から必要な行数だけ折り返す)
        var tail = new List<Seg>(log[log.Count - 1]);
        if (!Busy) { tail.Add(new Seg(CurrentPrompt, askPrompt != null ? Pal.Yellow : Pal.Fg)); tail.Add(new Seg(input.ToString(), Pal.White)); }
        else if (Thinking) tail.Add(new Seg("|/-\\"[(int)(DateTime.Now.TimeOfDay.TotalSeconds * 8) % 4].ToString(), Pal.Dim));

        // カーソル位置は案内文を足す前の末尾。案内文は表示だけ
        int endCol;
        int cursorRows = Wrap(tail, cols, out endCol).Count;
        string ghost = Busy ? "" : Ghost();
        if (ghost.Length > 0) tail.Add(new Seg(ghost, Pal.Dim));
        else if (!Busy && Hint.Length > 0 && input.Length == 0) tail.Add(new Seg(" " + Hint, Pal.Dim));
        int dummyCol;
        var visual = Wrap(tail, cols, out dummyCol);
        int tailRows = visual.Count;
        for (int i = log.Count - 2; i >= 0 && visual.Count < rows + scroll; i--)
        {
            int dummy;
            visual.InsertRange(0, Wrap(log[i], cols, out dummy));
        }
        scroll = Math.Min(scroll, Math.Max(0, visual.Count - rows));
        int start = Math.Max(0, visual.Count - rows - scroll);
        for (int r = 0; r < rows && start + r < visual.Count; r++)
            DrawRow(g, visual[start + r], r);

        // カーソル
        int cursorRow = visual.Count - tailRows + cursorRows - 1 - start;
        if (endCol >= cols) { endCol = 0; cursorRow++; }
        if (!Busy && scroll == 0 && Focused && cursorOn && cursorRow < rows)
            using (var b = new SolidBrush(Pal.Fg))
                g.FillRectangle(b, endCol * CellW, cursorRow * CellH + 1, CellW, CellH - 2);
        if (!Busy) PlaceIme(endCol * CellW, cursorRow * CellH);
    }

    // ---- IME の変換窓をカーソル位置に出す ----

    [StructLayout(LayoutKind.Sequential)] struct POINT { public int X, Y; }
    [StructLayout(LayoutKind.Sequential)] struct RECT { public int L, T, R, B; }
    [StructLayout(LayoutKind.Sequential)] struct COMPOSITIONFORM { public int Style; public POINT Pos; public RECT Area; }
    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
    class LOGFONT
    {
        public int lfHeight, lfWidth, lfEscapement, lfOrientation, lfWeight;
        public byte lfItalic, lfUnderline, lfStrikeOut, lfCharSet, lfOutPrecision, lfClipPrecision, lfQuality, lfPitchAndFamily;
        [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 32)] public string lfFaceName;
    }
    [DllImport("imm32.dll")] static extern IntPtr ImmGetContext(IntPtr hwnd);
    [DllImport("imm32.dll")] static extern bool ImmReleaseContext(IntPtr hwnd, IntPtr himc);
    [DllImport("imm32.dll")] static extern bool ImmSetCompositionWindow(IntPtr himc, ref COMPOSITIONFORM form);
    [DllImport("imm32.dll", CharSet = CharSet.Unicode)] static extern bool ImmSetCompositionFontW(IntPtr himc, LOGFONT lf);

    Point imePos = new Point(-1, -1);
    void PlaceIme(int x, int y)
    {
        if (imePos.X == x && imePos.Y == y) return;
        imePos = new Point(x, y);
        IntPtr himc = ImmGetContext(Handle);
        if (himc == IntPtr.Zero) return;
        var form = new COMPOSITIONFORM { Style = 2, Pos = new POINT { X = x, Y = y } };
        ImmSetCompositionWindow(himc, ref form);
        var lf = new LOGFONT();
        cjk.ToLogFont(lf);
        ImmSetCompositionFontW(himc, lf);
        ImmReleaseContext(Handle, himc);
    }
}

// ---------------- ウィンドウ ----------------

class MainForm : Form
{
    const double CharsPerSec = 7.5;
    const int Cols = 120, Rows = 30;   // Windows Terminal の既定サイズ

    readonly TermView term = new TermView();
    Settings settings;
    IProvider ai;
    SpeechSynthesizer synth;
    bool muted;
    volatile bool cancel;
    VoiceInput listener;
    bool micAllowed;
    bool wasBusy;
    DateTime quietUntil;

    [DllImport("dwmapi.dll")] static extern int DwmSetWindowAttribute(IntPtr hwnd, int attr, ref int value, int size);

    public MainForm(bool startMuted, bool noMic, bool startHidden)
    {
        this.startHidden = startHidden;
        muted = startMuted;
        micAllowed = !noMic;
        BackColor = Pal.Bg;
        Icon = Icon.ExtractAssociatedIcon(Application.ExecutablePath);
        int pad = term.CellW;
        Padding = new Padding(pad, pad / 2, pad, pad / 2);
        StartPosition = FormStartPosition.CenterScreen;
        Rectangle wa = Screen.PrimaryScreen.WorkingArea;
        int rows = Math.Min(Rows, (wa.Height - 80) / term.CellH);
        int cols = Math.Min(Cols, (wa.Width - 40) / term.CellW);
        ClientSize = new Size(cols * term.CellW + pad * 2, rows * term.CellH + pad);
        MinimumSize = new Size(term.CellW * 40, term.CellH * 12);
        term.Dock = DockStyle.Fill;
        Controls.Add(term);

        try
        {
            synth = new SpeechSynthesizer();
            synth.SetOutputToDefaultAudioDevice();
            synth.VisemeReached += (s, e) => term.Face.OnViseme(e.Viseme);
        }
        catch { synth = null; }
        LoadSettings();

        var timer = new System.Windows.Forms.Timer { Interval = 40 };
        timer.Tick += (s, e) => OnTick();
        timer.Start();

        term.Submitted += OnSubmit;
        term.CommandNames = Commands.Select(c => c[0]).ToArray();
        term.Interrupted += Interrupt;
        UpdateTitle();
        term.Write("Lumi Assistant [Version " + Program.Version + "]\n", Pal.Fg);
        term.Write("話しかけると声で返事をします。/help でコマンド一覧。\n\n", Pal.Dim);
        if (settings.Error != null) term.Write("settings.json を読めませんでした: " + settings.Error + "\n\n", Pal.Red);
        StartListening(false);
        WarmupLocal();
        term.Face.Flash("happy", 2.5);   // 起動のあいさつ
        Shown += (s, e) => term.Focus();
        SetupTray();
        ApplyStartup();
        FormClosing += OnClosing;
    }

    // ---- バックグラウンド (タスクトレイ) ----

    readonly bool startHidden;
    readonly PeekWindow peek = new PeekWindow();
    NotifyIcon tray;
    bool exiting, toldAboutTray;

    // --background で起動したときは、ウィンドウを出さずにトレイから始める
    protected override void SetVisibleCore(bool value)
    {
        if (startHidden && !IsHandleCreated)
        {
            CreateHandle();
            value = false;
        }
        base.SetVisibleCore(value);
    }

    void SetupTray()
    {
        var menu = new ContextMenuStrip();
        menu.Items.Add("表示", null, (s, e) => ShowFromTray());
        menu.Items.Add("マイクのオン/オフ", null, (s, e) => OnSubmit("/mic"));
        menu.Items.Add(new ToolStripSeparator());
        menu.Items.Add("終了", null, (s, e) => ExitApp());
        tray = new NotifyIcon { Icon = Icon, Text = "ルミ", ContextMenuStrip = menu, Visible = true };
        tray.DoubleClick += (s, e) => ShowFromTray();
        peek.Clicked += () => { peek.Retract(false); FadeIn(); };
    }

    // 透明から少しずつ出てくる
    void FadeIn()
    {
        if (Visible) { ShowFromTray(); return; }
        Opacity = 0;
        ShowFromTray();
        var start = DateTime.Now;
        var t = new System.Windows.Forms.Timer { Interval = 15 };
        t.Tick += (s, e) =>
        {
            double k = Math.Min(1, (DateTime.Now - start).TotalMilliseconds / 280);
            Opacity = k;
            if (k >= 1) { t.Stop(); t.Dispose(); }
        };
        t.Start();
    }

    // 呼ばれたときのアニメーションを試す (ウィンドウを隠して、右下から顔を出す)
    void DemoPeek()
    {
        Hide();
        var steps = new System.Windows.Forms.Timer { Interval = 700 };
        int n = 0;
        steps.Tick += (s, e) =>
        {
            n++;
            if (n == 1) peek.Pop("なあに？");
            if (n == 5) { peek.Retract(false, "はーい！"); FadeIn(); steps.Stop(); steps.Dispose(); }
        };
        steps.Start();
    }

    public void ShowFromTray()
    {
        if (!Visible) Show();
        if (WindowState == FormWindowState.Minimized) WindowState = FormWindowState.Normal;
        // ほかのアプリを使っている最中でも前に出るよう、一瞬だけ最前面にする
        TopMost = true;
        TopMost = false;
        SetForegroundWindow(Handle);
        Activate();
        term.Focus();
    }

    [DllImport("user32.dll")] static extern bool SetForegroundWindow(IntPtr hwnd);

    void ExitApp()
    {
        exiting = true;
        Close();
    }

    void OnClosing(object sender, FormClosingEventArgs e)
    {
        // × で閉じたときは、設定が on ならトレイに隠れて動き続ける
        if (!exiting && e.CloseReason == CloseReason.UserClosing && settings.Get("background", "on") != "off")
        {
            e.Cancel = true;
            Hide();
            if (!toldAboutTray)
            {
                tray.ShowBalloonTip(4000, "ルミ", "バックグラウンドで動いています。「" + WakeWord + "」と呼ぶか、アイコンをダブルクリックすると出てきます。", ToolTipIcon.Info);
                toldAboutTray = true;
            }
            return;
        }
        Interrupt();
        StopListening();
        LocalServer.Stop();
        tray.Visible = false;
        tray.Dispose();
        peek.Close();
    }

    // startup の設定に合わせて、Windows 起動時の自動起動 (HKCU の Run) を登録・解除する
    void ApplyStartup()
    {
        try
        {
            using (var key = Microsoft.Win32.Registry.CurrentUser.CreateSubKey(@"Software\Microsoft\Windows\CurrentVersion\Run"))
            {
                if (settings.Get("startup", "off") == "on") key.SetValue("Lumi", "\"" + Application.ExecutablePath + "\" --background");
                else if (key.GetValue("Lumi") != null) key.DeleteValue("Lumi");
            }
        }
        catch { }
    }

    void OnTick()
    {
        term.Tick();
        if (listener == null) return;
        // 喋り終わった直後は自分の声の残りを拾わないよう少し待つ
        if (wasBusy && !term.Busy) quietUntil = DateTime.Now.AddMilliseconds(800);
        wasBusy = term.Busy;
        listener.Paused = term.Busy || DateTime.Now < quietUntil;
        listener.Tick();
    }

    // ---- 呼びかけによる音声入力 ----

    string WakeWord { get { return settings.Get("wake_word", "ルミ"); } }

    void StartListening(bool announce, bool force = false)
    {
        StopListening();
        if (!micAllowed || (!force && settings.Get("voice_input", "on").ToLowerInvariant() == "off")) { UpdateTitle(); return; }
        float confidence;
        if (!float.TryParse(settings.Get("wake_confidence", "0.6"), System.Globalization.NumberStyles.Float,
                            System.Globalization.CultureInfo.InvariantCulture, out confidence)) confidence = 0.6f;
        try
        {
            var l = new VoiceInput(WakeWord, confidence);
            l.Woke += () => BeginInvoke(new Action(() =>
            {
                if (!Visible) peek.Pop("なあに？");
                term.Face.Expression = "listen";
                term.Face.Poke(false);
                term.Hint = "（聞いています…）";
                term.Invalidate();
            }));
            l.Heard += text => BeginInvoke(new Action(() =>
            {
                if (peek.Popped) { peek.Retract(false, "はーい！"); FadeIn(); }
                else ShowFromTray();
                term.Hint = "";
                term.Face.Expression = "normal";
                term.SubmitVoice(VoiceCommand(text));
            }));
            l.Confirmed += yes => BeginInvoke(new Action(() => term.AnswerVoice(yes ? "y" : "n")));
            l.TimedOut += () => BeginInvoke(new Action(() =>
            {
                if (peek.Popped) peek.Retract(true, "…またね");
                term.Hint = "";
                if (term.Face.Expression == "listen") term.Face.Expression = "normal";
                term.Invalidate();
            }));
            l.Start();
            listener = l;
            if (announce) term.Write("音声入力をオンにしました。「" + WakeWord + "」と呼びかけてください。\n\n", Pal.Dim);
        }
        catch (Exception e)
        {
            term.Write("音声入力を使えません: " + e.Message + "\n\n", Pal.Dim);
        }
        UpdateTitle();
    }

    // 声で言われたアプリのコマンドを、打ち込むコマンドに置き換える
    static string VoiceCommand(string text)
    {
        string t = text.Trim().TrimEnd('。', '！', '!', '？', '?').Replace(" ", "").Replace("　", "");
        if (t == "ミュート" || t == "ミュート解除" || t == "静かにして" || t == "喋って") return "/mute";
        if (t == "画面を消して" || t == "画面をクリア" || t == "クリア" || t == "リセット") return "/cls";
        if (t == "マイクをオフ" || t == "マイクオフ" || t == "マイクを切って") return "/mic";
        if (t == "設定を読み直して" || t == "リロード") return "/reload";
        if (t == "ヘルプ" || t == "コマンド一覧") return "/help";
        if (t == "終了して" || t == "アプリを終了して" || t == "おやすみ") return "/exit";
        return text;
    }

    void StopListening()
    {
        if (listener != null) listener.Dispose();
        listener = null;
        term.Hint = "";
        UpdateTitle();
    }

    // ローカルAIなら、最初の返事を待たせないように先にモデルを読み込んでおく
    void WarmupLocal()
    {
        var local = ai as LocalProvider;
        if (local == null) return;
        if (!local.Installed)
        {
            term.Write("ローカルAIがまだ入っていません。/install-local と入力するとダウンロードします（約" +
                       (LocalAI.TotalSize / 1000000000.0).ToString("0.0") + "GB）。\n\n", Pal.Yellow);
            return;
        }
        term.Write("ローカルAIを起動しています…\n", Pal.Dim);
        new Thread(() =>
        {
            try { local.Warmup(); term.Write("ローカルAIの準備ができました。\n\n", Pal.Dim); }
            catch (Exception e) { term.Write(e.Message + "\n\n", Pal.Red); }
        }) { IsBackground = true }.Start();
    }

    // llama.cpp と標準モデルをダウンロードして、provider を local に切り替える
    void InstallLocal()
    {
        term.Busy = true;
        cancel = false;
        term.Write("ローカルAIをダウンロードします。Ctrl+C で中断できます（次回は続きから再開します）。\n", Pal.Dim);
        new Thread(() =>
        {
            string baseDir = AppDomain.CurrentDomain.BaseDirectory;
            string lastStep = null;
            int lastPct = -1;
            try
            {
                LocalAI.Install(baseDir, (step, ratio) =>
                {
                    int pct = (int)(ratio * 100);
                    if (step == lastStep && pct / 5 == lastPct / 5) return;
                    term.Write("  [" + pct.ToString().PadLeft(3) + "%] " + step + "\n", Pal.Dim);
                    lastStep = step;
                    lastPct = pct;
                }, () => cancel);
                BeginInvoke(new Action(() =>
                {
                    if (settings.Get("provider", "offline") == "offline")
                    {
                        string json = System.IO.File.ReadAllText(settings.Path);
                        json = System.Text.RegularExpressions.Regex.Replace(json, "\"provider\"\\s*:\\s*\"offline\"", "\"provider\": \"local\"");
                        System.IO.File.WriteAllText(settings.Path, json, new UTF8Encoding(false));
                    }
                    LoadSettings();
                    term.Write("ローカルAIを入れました。（" + ai.Label + "）\n", Pal.Dim);
                    term.Busy = false;
                    WarmupLocal();
                }));
            }
            catch (OperationCanceledException)
            {
                term.Write("^C\nダウンロードを中断しました。もう一度 /install-local で続きから再開します。\n\n", Pal.Dim);
                BeginInvoke(new Action(() => term.Busy = false));
            }
            catch (Exception e)
            {
                term.Write("ダウンロードに失敗しました: " + e.Message + "\n\n", Pal.Red);
                BeginInvoke(new Action(() => term.Busy = false));
            }
        }) { IsBackground = true }.Start();
    }

    protected override void OnHandleCreated(EventArgs e)
    {
        base.OnHandleCreated(e);
        int on = 1;
        try { DwmSetWindowAttribute(Handle, 20, ref on, 4); } catch { }  // ダークなタイトルバー
    }

    // settings.json を読み直して、AI と声の設定を反映する
    void LoadSettings()
    {
        settings = Settings.Load();
        ai = Providers.Create(settings);
        if (synth != null)
        {
            string voice = settings.Get("voice", "");
            try
            {
                if (voice.Length > 0) synth.SelectVoice(voice);
                else synth.SelectVoiceByHints(VoiceGender.Female, VoiceAge.Adult, 0, new System.Globalization.CultureInfo("ja-JP"));
            }
            catch { }
            synth.Rate = Math.Max(-10, Math.Min(10, settings.GetInt("voice_rate", 0)));
        }
        UpdateTitle();
    }

    void UpdateTitle()
    {
        string mode = ai.Label;
        string voice = muted ? "mute" : synth != null ? synth.Voice.Name.Replace("Microsoft ", "").Replace(" Desktop", "") : "no voice";
        Text = "ルミ  —  " + mode + " | " + voice + (listener != null ? " | mic" : "");
    }

    void Interrupt()
    {
        if (!term.Busy) return;
        cancel = true;
        ai.Abort();
        if (synth != null) synth.SpeakAsyncCancelAll();
    }

    // ---- / で始まるアプリのコマンド ----

    // 名前, 引数, 説明
    static readonly string[][] Commands =
    {
        new[] { "/help", "", "コマンド一覧" },
        new[] { "/settings", "", "今の設定を一覧する" },
        new[] { "/set", "<項目> <値>", "設定を変える (例: /set voice_rate 3)" },
        new[] { "/voices", "", "使える声の一覧" },
        new[] { "/config", "", "設定ファイル (settings.json) をメモ帳で開く" },
        new[] { "/reload", "", "設定ファイルを読み直す" },
        new[] { "/mute", "", "読み上げのオン/オフ" },
        new[] { "/mic", "", "音声入力のオン/オフ" },
        new[] { "/install-local", "", "ローカルAIをダウンロードする" },
        new[] { "/peek", "", "バックグラウンドで呼ばれたときの動きを試す" },
        new[] { "/cls", "", "画面と会話をリセット" },
        new[] { "/exit", "", "終了" },
    };

    // 設定項目, 説明, 選べる値 (空なら自由)
    static readonly string[][] SettingKeys =
    {
        new[] { "provider", "使うAI", "local,offline,anthropic,openai,command" },
        new[] { "model", "モデル名 (local では .gguf のファイル名)", "" },
        new[] { "endpoint", "API の URL", "" },
        new[] { "api_key_env", "API キーが入っている環境変数の名前", "" },
        new[] { "command", "command で実行するコマンド", "" },
        new[] { "max_tokens", "返答の最大トークン数 (0 で既定)", "#int:0:1000000" },
        new[] { "effort", "Anthropic の effort", ",low,medium,high,xhigh,max" },
        new[] { "local_gpu", "ローカルAIで GPU を使うか", "auto,off" },
        new[] { "pc_control", "PC の操作 (毎回確認あり)", "on,off" },
        new[] { "voice_input", "音声入力", "on,off" },
        new[] { "wake_word", "呼びかけの言葉", "" },
        new[] { "wake_confidence", "呼びかけの聞き取りの厳しさ (0〜1)", "#num:0:1" },
        new[] { "system_prompt", "キャラクター設定 (空なら既定)", "" },
        new[] { "voice", "声の名前 (/voices で一覧)", "" },
        new[] { "voice_rate", "読み上げの速さ (-10〜10)", "#int:-10:10" },
        new[] { "background", "× で閉じてもトレイで動き続ける", "on,off" },
        new[] { "startup", "Windows の起動時にトレイで起動する", "on,off" },
    };

    void OnSubmit(string text)
    {
        if (text.Length == 0) return;
        if (!text.StartsWith("/")) { StartReply(text); return; }

        string[] parts = text.Split(new[] { ' ', '　' }, 3, StringSplitOptions.RemoveEmptyEntries);
        string cmd = parts[0].ToLowerInvariant();
        switch (cmd)
        {
            case "/exit": case "/quit": ExitApp(); return;
            case "/cls": case "/clear": ai.Clear(); term.ClearScreen(); return;
            case "/mute":
                muted = !muted;
                UpdateTitle();
                Info(muted ? "読み上げをオフにしました。" : "読み上げをオンにしました。");
                return;
            case "/mic":
                if (listener != null) { StopListening(); Info("音声入力をオフにしました。"); }
                else { micAllowed = true; StartListening(true, true); }
                return;
            case "/reload": Reload(true); return;
            case "/install-local": InstallLocal(); return;
            case "/config":
                try { System.Diagnostics.Process.Start("notepad.exe", "\"" + settings.Path + "\""); } catch { }
                Info("settings.json を開きました。保存したら /reload で反映されます。");
                return;
            case "/settings": ShowSettings(); return;
            case "/set": SetCommand(parts.Length > 1 ? parts[1] : null, parts.Length > 2 ? parts[2] : null); return;
            case "/voices": ShowVoices(); return;
            case "/peek": DemoPeek(); return;
            case "/help":
                var sb = new StringBuilder();
                foreach (var c in Commands) sb.Append("  " + (c[0] + " " + c[1]).PadRight(24) + c[2] + "\n");
                sb.Append("\n  「" + WakeWord + "」と呼びかけると声で話しかけられます。Tab でコマンドを補完できます。\n");
                sb.Append("  Ctrl+C / Esc  返事を止める     ↑↓  入力履歴     PageUp/Down  スクロール\n");
                Info(sb.ToString().TrimEnd('\n'));
                return;
            default:
                term.Write("知らないコマンドです: " + cmd + "（/help で一覧）\n\n", Pal.Red);
                return;
        }
    }

    void Info(string text) { term.Write(text + "\n\n", Pal.Dim); }

    // 設定を読み直して、AI・声・音声入力に反映する
    void Reload(bool announce)
    {
        LoadSettings();
        if (settings.Error != null) term.Write("settings.json を読めませんでした: " + settings.Error + "\n\n", Pal.Red);
        else if (announce) Info("設定を読み直しました。（" + ai.Label + "）");
        if (!(ai is LocalProvider)) LocalServer.Stop();
        StartListening(false);
        WarmupLocal();
        ApplyStartup();
    }

    void ShowSettings()
    {
        var sb = new StringBuilder();
        foreach (var k in SettingKeys)
        {
            string v = settings.Get(k[0], "");
            if (v.Length > 40) v = v.Substring(0, 40) + "…";
            sb.Append("  " + k[0].PadRight(16) + (v.Length > 0 ? v : "(既定)").PadRight(28) + k[1] + "\n");
        }
        sb.Append("\n  変えるには /set <項目> <値>。空に戻すには /set <項目> \"\"");
        Info(sb.ToString());
    }

    void ShowVoices()
    {
        if (synth == null) { Info("音声合成が使えません。"); return; }
        var sb = new StringBuilder();
        foreach (var v in synth.GetInstalledVoices())
            if (v.Enabled) sb.Append("  " + v.VoiceInfo.Name + "  (" + v.VoiceInfo.Culture.Name + ")" + (v.VoiceInfo.Name == synth.Voice.Name ? "  ← 使用中" : "") + "\n");
        sb.Append("\n  変えるには /set voice <名前>");
        Info(sb.ToString());
    }

    void SetCommand(string key, string value)
    {
        if (key == null) { ShowSettings(); return; }
        var def = SettingKeys.FirstOrDefault(k => k[0] == key.ToLowerInvariant());
        if (def == null)
        {
            term.Write("そんな設定項目はありません: " + key + "（/settings で一覧）\n\n", Pal.Red);
            return;
        }
        if (value == null)
        {
            Info(def[0] + " = " + (settings.Get(def[0], "").Length > 0 ? settings.Get(def[0], "") : "(既定)") + "    " + def[1] +
                 (def[2].Length > 0 && !def[2].StartsWith("#") ? "\n  選べる値: " + def[2].Trim(',').Replace(",", " / ") : ""));
            return;
        }
        if (settings.Error != null) { term.Write("settings.json が壊れているので変更できません。/config で直してください。\n\n", Pal.Red); return; }

        value = value.Trim();
        if (value == "\"\"") value = "";
        object stored = value;
        string rule = def[2];
        if (rule.StartsWith("#"))
        {
            string[] r = rule.Split(':');
            double n, min = double.Parse(r[1], System.Globalization.CultureInfo.InvariantCulture), max = double.Parse(r[2], System.Globalization.CultureInfo.InvariantCulture);
            if (!double.TryParse(value, System.Globalization.NumberStyles.Float, System.Globalization.CultureInfo.InvariantCulture, out n) ||
                n < min || n > max || (r[0] == "#int" && n != Math.Floor(n)))
            {
                term.Write(def[0] + " は " + r[1] + "〜" + r[2] + " の" + (r[0] == "#int" ? "整数" : "数") + "で指定してください。\n\n", Pal.Red);
                return;
            }
            stored = r[0] == "#int" ? (object)(int)n : (object)n;
        }
        else if (rule.Length > 0)
        {
            value = value.ToLowerInvariant();
            if (Array.IndexOf(rule.Split(','), value) < 0)
            {
                term.Write(def[0] + " に使える値: " + rule.Trim(',').Replace(",", " / ") + "\n\n", Pal.Red);
                return;
            }
            stored = value;
        }
        if (def[0] == "voice" && value.Length > 0 && synth != null &&
            !synth.GetInstalledVoices().Any(v => v.VoiceInfo.Name == value))
        {
            term.Write("その声は見つかりません。/voices で一覧を見られます。\n\n", Pal.Red);
            return;
        }

        try { settings.Set(def[0], stored); }
        catch (Exception e) { term.Write("保存できませんでした: " + e.Message + "\n\n", Pal.Red); return; }
        Info(def[0] + " を " + (value.Length > 0 ? value : "(既定)") + " にしました。");
        Reload(false);
    }

    void StartReply(string text)
    {
        term.Busy = true;
        term.Thinking = true;
        cancel = false;
        var th = new Thread(() => Respond(text)) { IsBackground = true };
        th.Start();
    }

    // ---- バックグラウンド: 頭脳から文を受け取り順に喋る ----

    const int MaxCommandRounds = 5;

    void Respond(string userText)
    {
        string message = userText;
        for (int round = 0; round < MaxCommandRounds && message != null && !cancel; round++)
        {
            var requests = ReplyOnce(message);
            message = null;
            if (requests.Count == 0 || cancel || !settings.PcControl) break;

            // 1 つずつ必ず確認してから実行し、結果をまとめて AI に返す
            var report = new StringBuilder("[コマンドの実行結果]\n");
            foreach (var r in requests)
            {
                if (cancel) break;
                report.Append("\n$ " + r.Command + (r.Admin ? "  (管理者)" : "") + "\n");
                string answer = Confirm(r);
                bool asAdmin = r.Admin || answer == "a";
                if (answer != "y" && answer != "a")
                {
                    term.Write("実行しませんでした。\n", Pal.Dim);
                    report.Append("(ユーザーが実行を許可しませんでした)\n");
                    continue;
                }
                term.Thinking = true;
                term.Face.Expression = "think";
                string result;
                try { result = CommandRunner.Run(r.Command, asAdmin, () => cancel); }
                catch (Exception e) { result = "(実行できませんでした: " + e.Message + ")"; }
                ShowOutput(result);
                term.Face.Flash(result.StartsWith("終了コード 0") && !result.Contains("Exception") ? "happy" : "sad", 2);
                report.Append(result + "\n");
            }
            if (!cancel) message = report.ToString();
        }

        term.Face.Expression = "normal";
        term.Face.Speaking = false;
        BeginInvoke(new Action(() =>
        {
            term.Thinking = false;
            term.Write(cancel ? "^C\n\n" : "\n\n", Pal.Dim);
            term.Busy = false;
            UpdateTitle();
            // 声で話しかけられていたら、呼びかけなしで続けて話せるようにする
            if (term.LastWasVoice && listener != null && !cancel) listener.Listen();
        }));
    }

    // AI に 1 回話しかけて返事を読み上げ、返事に含まれていたコマンドを返す
    List<RunRequest> ReplyOnce(string message)
    {
        var queue = new BlockingCollection<string>();
        var speaker = new Thread(() =>
        {
            foreach (string s in queue.GetConsumingEnumerable())
                if (!cancel) Speak(s);
        }) { IsBackground = true };
        speaker.Start();

        term.Thinking = true;
        term.Face.Expression = "think";
        var splitter = new SentenceSplitter();
        var runs = new RunExtractor();
        try
        {
            ai.Reply(message, chunk =>
            {
                foreach (string s in splitter.Push(runs.Push(chunk))) queue.Add(s);
            });
            foreach (string s in splitter.Push(runs.Flush())) queue.Add(s);
            string rest = splitter.Flush();
            if (rest.Trim().Length > 0) queue.Add(rest);
        }
        finally { queue.CompleteAdding(); }
        speaker.Join();
        term.Write("\n", Pal.Fg);
        return runs.Requests;
    }

    // コマンドを見せて、実行してよいかキーボードで答えてもらう (y / a / それ以外は実行しない)
    string Confirm(RunRequest r)
    {
        term.Face.Speaking = false;
        term.Face.Expression = "normal";
        term.Write("\nルミがコマンドを実行しようとしています" + (r.Admin ? "（管理者権限が必要です）" : "") + ":\n", Pal.Yellow);
        foreach (string line in r.Command.Split('\n')) term.Write("  " + line.TrimEnd('\r') + "\n", Pal.White);
        string question = r.Admin ? "管理者として実行しますか？ [y/N] " : "実行しますか？ [y=実行 / a=管理者として実行 / N=やめる] ";
        var l = listener;
        if (l != null) term.Write("  声なら「実行して」か「やめて」で答えられます。" + (r.Admin ? "このあと Windows の確認画面が出ます。" : "") + "\n", Pal.Dim);
        string answer = null;
        var done = new ManualResetEvent(false);
        term.Ask(question, a => { answer = a.ToLowerInvariant(); done.Set(); });
        if (l != null) l.BeginConfirm();
        done.WaitOne();
        if (l != null) l.EndConfirm();
        if (r.Admin && answer == "a") answer = "y";
        return answer;
    }

    void ShowOutput(string result)
    {
        string[] lines = result.Replace("\r", "").Split('\n');
        const int Max = 15;
        for (int i = 0; i < Math.Min(lines.Length, Max); i++) term.Write("  " + lines[i] + "\n", Pal.Dim);
        if (lines.Length > Max) term.Write("  …(" + (lines.Length - Max) + " 行省略)\n", Pal.Dim);
        term.Write("\n", Pal.Fg);
    }

    void Speak(string sentence)
    {
        string text = SentenceSplitter.Clean(sentence);
        if (text.Length == 0) return;
        term.Thinking = false;
        term.Face.Expression = "normal";
        term.Face.Speaking = true;
        int shown = 0;
        object gate = new object();
        Action<int> reveal = upTo =>
        {
            lock (gate)
            {
                upTo = Math.Min(upTo, text.Length);
                if (upTo > shown) { term.Write(text.Substring(shown, upTo - shown), Pal.Fg); shown = upTo; }
            }
        };

        if (synth != null && !muted)
        {
            var done = new ManualResetEvent(false);
            EventHandler<SpeakProgressEventArgs> progress = (s, e) => reveal(e.CharacterPosition + e.CharacterCount);
            EventHandler<SpeakCompletedEventArgs> completed = (s, e) => done.Set();
            synth.SpeakProgress += progress;
            synth.SpeakCompleted += completed;
            synth.SpeakAsync(text);
            done.WaitOne();
            synth.SpeakProgress -= progress;
            synth.SpeakCompleted -= completed;
        }
        else
        {
            DateTime start = DateTime.Now;
            while (shown < text.Length && !cancel)
            {
                reveal((int)((DateTime.Now - start).TotalSeconds * CharsPerSec));
                Thread.Sleep(30);
            }
        }
        if (!cancel) reveal(text.Length);
        term.Face.Speaking = false;
    }
}

static class Program
{
    public const string Version = "0.2.0-beta";

    [DllImport("user32.dll")] static extern bool SetProcessDPIAware();

    static void LogError(Exception e)
    {
        try
        {
            System.IO.File.AppendAllText(System.IO.Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "error.log"),
                                         DateTime.Now + "\r\n" + e + "\r\n\r\n");
        }
        catch { }
    }

    [STAThread]
    static void Main(string[] args)
    {
        System.Net.ServicePointManager.SecurityProtocol = System.Net.SecurityProtocolType.Tls12;
        try { SetProcessDPIAware(); } catch { }
        Application.EnableVisualStyles();
        Application.SetCompatibleTextRenderingDefault(false);
        // 想定外のエラーは error.log に残す
        Application.SetUnhandledExceptionMode(UnhandledExceptionMode.CatchException);
        Application.ThreadException += (s, e) => LogError(e.Exception);
        AppDomain.CurrentDomain.UnhandledException += (s, e) => LogError(e.ExceptionObject as Exception);

        // すでに起動していたら、そちらのウィンドウを出して終わる
        bool first;
        using (var mutex = new Mutex(true, "Lumi.SingleInstance", out first))
        using (var showSignal = new EventWaitHandle(false, EventResetMode.AutoReset, "Lumi.Show"))
        {
            if (!first) { showSignal.Set(); return; }
            var form = new MainForm(Array.IndexOf(args, "--mute") >= 0, Array.IndexOf(args, "--no-mic") >= 0,
                                    Array.IndexOf(args, "--background") >= 0);
            new Thread(() =>
            {
                while (true)
                {
                    showSignal.WaitOne();
                    try { form.BeginInvoke(new Action(form.ShowFromTray)); } catch { return; }
                }
            }) { IsBackground = true }.Start();
            Application.Run(form);
        }
    }
}
