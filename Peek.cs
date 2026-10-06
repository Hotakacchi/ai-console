// バックグラウンドで「ルミ」と呼ばれたときに、画面右下から顔がぴょこっと出てくる小さな窓。
// 前面には出るが、キーボードの入力先は奪わない。

using System;
using System.Drawing;
using System.Drawing.Drawing2D;
using System.Windows.Forms;

class PeekWindow : Form
{
    public readonly Face Face = new Face();
    public event Action Clicked;

    readonly System.Windows.Forms.Timer timer = new System.Windows.Forms.Timer { Interval = 15 };
    readonly float scale;
    readonly Font font;
    string message = "";

    // 位置のアニメーション
    DateTime animStart;
    double animMs;
    int fromY, toY;
    double fromOpacity, toOpacity;
    Func<double, double> ease;
    Action onDone;

    public bool Popped { get; private set; }

    public PeekWindow()
    {
        FormBorderStyle = FormBorderStyle.None;
        ShowInTaskbar = false;
        TopMost = true;
        StartPosition = FormStartPosition.Manual;
        BackColor = Pal.Bg;
        DoubleBuffered = true;
        using (var g = CreateGraphics()) scale = g.DpiX / 96f;
        Size = new Size((int)(240 * scale), (int)(190 * scale));
        font = new Font("Yu Gothic UI", 11f, FontStyle.Bold);
        using (var path = Rounded(new RectangleF(0, 0, Width, Height), 22 * scale)) Region = new Region(path);

        Face.SizeRatio = 0.9f;
        Face.LineColor = Pal.FaceBright;
        Face.FillColor = Pal.FaceBright;
        Face.CheekColor = Color.FromArgb(231, 72, 140);

        timer.Tick += (s, e) => Step();
        Click += (s, e) => { if (Clicked != null) Clicked(); };
    }

    // フォーカスを奪わない・タスクバーに出さない
    protected override bool ShowWithoutActivation { get { return true; } }
    protected override CreateParams CreateParams
    {
        get
        {
            var cp = base.CreateParams;
            cp.ExStyle |= 0x08000000 | 0x80 | 0x8;   // WS_EX_NOACTIVATE | WS_EX_TOOLWINDOW | WS_EX_TOPMOST
            return cp;
        }
    }

    Rectangle Area { get { return Screen.PrimaryScreen.WorkingArea; } }
    int RestY { get { return Area.Bottom - Height - (int)(16 * scale); } }
    int HiddenY { get { return Area.Bottom + (int)(10 * scale); } }

    // 下から弾むように飛び出して、聞く顔になる
    public void Pop(string text)
    {
        message = text;
        Face.Expression = "listen";
        Face.Poke(false);
        if (Popped && Visible) { Invalidate(); return; }
        Popped = true;
        Location = new Point(Area.Right - Width - (int)(16 * scale), HiddenY);
        Opacity = 0;
        Show();
        Animate(HiddenY, RestY, 0, 1, 420, EaseOutBack, null);
        timer.Start();
    }

    // 下に引っ込む。sleepy なら眠そうな顔で少し待ってから沈む
    public void Retract(bool sleepy, string text = "")
    {
        if (!Popped) return;
        Popped = false;
        message = text;
        Face.Expression = sleepy ? "sleep" : "happy";
        Invalidate();
        int wait = sleepy ? 900 : 150;
        var delay = new System.Windows.Forms.Timer { Interval = wait };
        delay.Tick += (s, e) =>
        {
            delay.Stop();
            delay.Dispose();
            if (Popped) return;   // 待っている間にまた呼ばれた
            Animate(Top, HiddenY, Opacity, 0, sleepy ? 650 : 300, EaseIn, () => { Hide(); timer.Stop(); });
        };
        delay.Start();
    }

    void Animate(int y0, int y1, double o0, double o1, double ms, Func<double, double> e, Action done)
    {
        animStart = DateTime.Now;
        fromY = y0; toY = y1; fromOpacity = o0; toOpacity = o1;
        animMs = ms; ease = e; onDone = done;
    }

    void Step()
    {
        if (ease != null)
        {
            double t = Math.Min(1, (DateTime.Now - animStart).TotalMilliseconds / animMs);
            double k = ease(t);
            Top = (int)(fromY + (toY - fromY) * k);
            Opacity = Math.Max(0, Math.Min(1, fromOpacity + (toOpacity - fromOpacity) * Math.Min(1, t * 1.6)));
            if (t >= 1)
            {
                ease = null;
                var done = onDone;
                onDone = null;
                if (done != null) done();
            }
        }
        if (Face.Tick() || ease != null) Invalidate();
    }

    // 行き過ぎてから戻る (ぴょこっ)
    static double EaseOutBack(double t)
    {
        const double c1 = 1.9, c3 = c1 + 1;
        return 1 + c3 * Math.Pow(t - 1, 3) + c1 * Math.Pow(t - 1, 2);
    }

    static double EaseIn(double t) { return t * t; }

    protected override void OnPaint(PaintEventArgs e)
    {
        Graphics g = e.Graphics;
        g.Clear(Pal.Bg);
        g.SmoothingMode = SmoothingMode.AntiAlias;
        float b = 2 * scale;
        using (var path = Rounded(new RectangleF(b / 2, b / 2, Width - b - 1, Height - b - 1), 21 * scale))
        using (var pen = new Pen(Pal.FaceBright, b))
            g.DrawPath(pen, path);

        int textH = (int)(30 * scale);
        Face.Draw(g, new Rectangle(0, (int)(6 * scale), Width, Height - textH - (int)(6 * scale)));

        if (message.Length > 0)
        {
            var size = TextRenderer.MeasureText(message, font);
            TextRenderer.DrawText(g, message, font,
                new Point((Width - size.Width) / 2, Height - textH - (int)(4 * scale)), Pal.Fg);
        }
    }

    static GraphicsPath Rounded(RectangleF r, float radius)
    {
        var p = new GraphicsPath();
        float d = radius * 2;
        p.AddArc(r.X, r.Y, d, d, 180, 90);
        p.AddArc(r.Right - d, r.Y, d, d, 270, 90);
        p.AddArc(r.Right - d, r.Bottom - d, d, d, 0, 90);
        p.AddArc(r.X, r.Bottom - d, d, d, 90, 90);
        p.CloseFigure();
        return p;
    }
}
