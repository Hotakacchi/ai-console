// ターミナル風の顔つきアシスタント「ルミ」
// 普通のコンソールと同じ 120x30 の文字画面で、背景にうっすら顔を描く。
// .NET Framework 4.8 だけで動く単体 exe。

using System;
using System.Collections.Concurrent;
using System.Collections.Generic;
using System.Drawing;
using System.Drawing.Drawing2D;
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
    public static readonly Color BGreen = Color.FromArgb(22, 198, 12);

    // 背景の顔 (文字の邪魔にならない暗さ)
    public static readonly Color FaceLine = Color.FromArgb(34, 62, 66);
    public static readonly Color FaceFill = Color.FromArgb(40, 78, 84);
    public static readonly Color FaceCheek = Color.FromArgb(70, 30, 60);
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
    public volatile string Expression = "normal";   // normal / happy / think
    public volatile bool Speaking;

    float open, target;
    DateTime nextMouth, blinkUntil, nextBlink = DateTime.Now.AddSeconds(2), lastViseme;
    readonly Random rnd = new Random();
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

    // 状態を進め、見た目が変わったら true を返す
    public bool Tick()
    {
        DateTime now = DateTime.Now;
        if (now >= nextBlink)
        {
            blinkUntil = now.AddMilliseconds(140);
            nextBlink = now.AddSeconds(2.5 + rnd.NextDouble() * 3);
        }
        if (!Speaking) target = 0;
        else if ((now - lastViseme).TotalMilliseconds > 250 && now >= nextMouth)
        {
            // viseme が来ない声・ミュート時はランダムに動かす
            target = 0.15f + (float)rnd.NextDouble() * 0.85f;
            nextMouth = now.AddMilliseconds(80 + rnd.Next(70));
        }
        open += (target - open) * 0.45f;
        if (open < 0.01f) open = 0;

        string state = Expression + Speaking + (now < blinkUntil) + Math.Round(open, 2) +
                       (Expression == "think" ? ((int)(now.TimeOfDay.TotalSeconds * 3) % 4).ToString() : "");
        bool changed = state != lastState;
        lastState = state;
        return changed;
    }

    public void Draw(Graphics g, Rectangle area)
    {
        g.SmoothingMode = SmoothingMode.AntiAlias;
        var saved = g.Save();
        float s = Math.Min(area.Width / 640f, area.Height / 440f);
        g.TranslateTransform(area.X + area.Width / 2f, area.Y + area.Height / 2f);
        g.ScaleTransform(s, s);
        DateTime now = DateTime.Now;
        string ex = Expression;

        using (var line = new Pen(Pal.FaceLine, 6) { StartCap = LineCap.Round, EndCap = LineCap.Round })
        using (var fill = new SolidBrush(Pal.FaceFill))
        {
            // 輪郭
            using (var p = RoundRect(-300, -200, 600, 400, 90)) g.DrawPath(line, p);

            // 目
            float dx = ex == "think" ? 18 : 0, dy = ex == "think" ? -18 : 0;
            foreach (float x in new float[] { -120, 120 })
            {
                const float y = -45;
                if (ex == "happy") g.DrawArc(line, x - 34, y - 20, 68, 60, 200, 140);
                else if (now < blinkUntil) g.DrawLine(line, x - 32, y, x + 32, y);
                else g.FillEllipse(fill, x - 30 + dx, y - 30 + dy, 60, 60);
            }

            // ほっぺ
            if (ex == "happy" || Speaking)
                using (var pk = new Pen(Pal.FaceCheek, 5) { StartCap = LineCap.Round, EndCap = LineCap.Round })
                    foreach (float bx in new float[] { -215, 165 })
                        for (int i = 0; i < 3; i++) g.DrawLine(pk, bx + i * 18, 40, bx + i * 18 + 12, 18);

            // 口
            const float my = 95;
            if (Speaking || open > 0)
            {
                float h = 6 + open * 60;
                g.DrawEllipse(line, -48, my - h / 2, 96, h);
            }
            else if (ex == "think")
                g.DrawBezier(line, -24, my, -8, my - 14, 8, my + 14, 24, my);
            else
                g.DrawArc(line, -52, my - 50, 104, 72, 30, 120);

            // 考え中の点々
            if (ex == "think")
            {
                int n = (int)(now.TimeOfDay.TotalSeconds * 3) % 4;
                for (int i = 0; i < n; i++) g.FillEllipse(fill, 230 + i * 34, -175, 18, 18);
            }
        }
        g.Restore(saved);
        g.SmoothingMode = SmoothingMode.None;
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
        bool ctrl = e.Control;
        if ((ctrl && e.KeyCode == Keys.C) || e.KeyCode == Keys.Escape)
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
        if (!Busy) { tail.Add(new Seg(Prompt, Pal.Fg)); tail.Add(new Seg(input.ToString(), Pal.White)); }
        else if (Thinking) tail.Add(new Seg("|/-\\"[(int)(DateTime.Now.TimeOfDay.TotalSeconds * 8) % 4].ToString(), Pal.Dim));

        int endCol;
        var visual = Wrap(tail, cols, out endCol);
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
        int cursorRow = visual.Count - 1 - start;
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

    [DllImport("dwmapi.dll")] static extern int DwmSetWindowAttribute(IntPtr hwnd, int attr, ref int value, int size);

    public MainForm(bool startMuted)
    {
        muted = startMuted;
        BackColor = Pal.Bg;
        Icon = MakeIcon();
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

        var timer = new System.Windows.Forms.Timer { Interval = 33 };
        timer.Tick += (s, e) => term.Tick();
        timer.Start();

        term.Submitted += OnSubmit;
        term.Interrupted += Interrupt;
        UpdateTitle();
        term.Write("Lumi Assistant [Version 1.0.0]\n", Pal.Fg);
        term.Write("話しかけると声で返事をします。help でコマンド一覧。\n\n", Pal.Dim);
        if (settings.Error != null) term.Write("settings.json を読めませんでした: " + settings.Error + "\n\n", Pal.Red);
        Shown += (s, e) => term.Focus();
        FormClosing += (s, e) => Interrupt();
    }

    protected override void OnHandleCreated(EventArgs e)
    {
        base.OnHandleCreated(e);
        int on = 1;
        try { DwmSetWindowAttribute(Handle, 20, ref on, 4); } catch { }  // ダークなタイトルバー
    }

    static Icon MakeIcon()
    {
        using (var bmp = new Bitmap(32, 32))
        {
            using (Graphics g = Graphics.FromImage(bmp))
            {
                g.Clear(Pal.Bg);
                using (var p = new Pen(Pal.BGreen, 3)) g.DrawLines(p, new[] { new Point(6, 9), new Point(14, 16), new Point(6, 23) });
                using (var b = new SolidBrush(Pal.Fg)) g.FillRectangle(b, 16, 22, 11, 3);
            }
            return Icon.FromHandle(bmp.GetHicon());
        }
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
        Text = "ルミ  —  " + mode + " | " + voice;
    }

    void Interrupt()
    {
        if (!term.Busy) return;
        cancel = true;
        ai.Abort();
        if (synth != null) synth.SpeakAsyncCancelAll();
    }

    void OnSubmit(string text)
    {
        string cmd = text.ToLowerInvariant().TrimStart('/');
        if (text.Length == 0) return;
        if (cmd == "exit" || cmd == "quit") { Close(); return; }
        if (cmd == "cls" || cmd == "clear") { ai.Clear(); term.ClearScreen(); return; }
        if (cmd == "mute")
        {
            muted = !muted;
            UpdateTitle();
            term.Write(muted ? "音声をオフにしました。\n\n" : "音声をオンにしました。\n\n", Pal.Dim);
            return;
        }
        if (cmd == "reload")
        {
            LoadSettings();
            if (settings.Error != null) term.Write("settings.json を読めませんでした: " + settings.Error + "\n\n", Pal.Red);
            else term.Write("設定を読み直しました。（" + ai.Label + "）\n\n", Pal.Dim);
            return;
        }
        if (cmd == "config")
        {
            try { System.Diagnostics.Process.Start("notepad.exe", "\"" + settings.Path + "\""); } catch { }
            term.Write("settings.json を開きました。保存したら reload と入力してください。\n\n", Pal.Dim);
            return;
        }
        if (cmd == "help")
        {
            term.Write("  cls     画面と会話をリセット\n  mute    音声のオン/オフ\n  config  設定ファイルを開く\n" +
                       "  reload  設定を読み直す\n  exit    終了\n" +
                       "  Ctrl+C / Esc  返事を止める     ↑↓  入力履歴     PageUp/Down  スクロール\n\n", Pal.Dim);
            return;
        }

        term.Busy = true;
        term.Thinking = true;
        cancel = false;
        var th = new Thread(() => Respond(text)) { IsBackground = true };
        th.Start();
    }

    // ---- バックグラウンド: 頭脳から文を受け取り順に喋る ----

    void Respond(string userText)
    {
        var queue = new BlockingCollection<string>();
        var speaker = new Thread(() =>
        {
            foreach (string s in queue.GetConsumingEnumerable())
                if (!cancel) Speak(s);
        }) { IsBackground = true };
        speaker.Start();

        term.Face.Expression = "think";
        var splitter = new SentenceSplitter();
        try
        {
            ai.Reply(userText, chunk =>
            {
                foreach (string s in splitter.Push(chunk)) queue.Add(s);
            });
            string rest = splitter.Flush();
            if (rest.Trim().Length > 0) queue.Add(rest);
        }
        finally { queue.CompleteAdding(); }
        speaker.Join();

        term.Face.Expression = "normal";
        term.Face.Speaking = false;
        BeginInvoke(new Action(() =>
        {
            term.Thinking = false;
            term.Write(cancel ? "^C\n\n" : "\n\n", Pal.Dim);
            term.Busy = false;
            UpdateTitle();
        }));
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
    [DllImport("user32.dll")] static extern bool SetProcessDPIAware();

    [STAThread]
    static void Main(string[] args)
    {
        System.Net.ServicePointManager.SecurityProtocol = System.Net.SecurityProtocolType.Tls12;
        try { SetProcessDPIAware(); } catch { }
        Application.EnableVisualStyles();
        Application.SetCompatibleTextRenderingDefault(false);
        Application.Run(new MainForm(Array.IndexOf(args, "--mute") >= 0));
    }
}
