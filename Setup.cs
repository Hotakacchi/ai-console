// ルミのインストーラー (LumiSetup.exe)。Lumi.exe を中に埋め込んである。
// 管理者権限なしで %LOCALAPPDATA%\Programs\Lumi に入れ、必要ならローカルAIもダウンロードする。
// 同じ exe を /uninstall 付きで起動するとアンインストールする。

using System;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.Reflection;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;
using System.Windows.Forms;
using Microsoft.Win32;

class SetupForm : Form
{
    static readonly Color Bg = Color.FromArgb(12, 12, 12);
    static readonly Color Fg = Color.FromArgb(204, 204, 204);
    static readonly Color Dim = Color.FromArgb(118, 118, 118);
    static readonly Color Accent = Color.FromArgb(97, 214, 214);

    public static readonly string InstallDir = Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "Programs", "Lumi");
    const string UninstallKey = @"Software\Microsoft\Windows\CurrentVersion\Uninstall\Lumi";
    static string StartMenuLink { get { return Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.Programs), "ルミ.lnk"); } }
    static string DesktopLink { get { return Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.DesktopDirectory), "ルミ.lnk"); } }

    readonly CheckBox local = new CheckBox();
    readonly CheckBox desktop = new CheckBox();
    readonly ProgressBar bar = new ProgressBar();
    readonly Label status = new Label();
    readonly Button main = new Button();
    readonly Button sub = new Button();
    volatile bool cancel;
    bool running, done;

    public SetupForm()
    {
        Text = "ルミ セットアップ";
        Icon = Icon.ExtractAssociatedIcon(Application.ExecutablePath);
        BackColor = Bg;
        ForeColor = Fg;
        Font = new Font("Yu Gothic UI", 10f);
        FormBorderStyle = FormBorderStyle.FixedDialog;
        MaximizeBox = false;
        StartPosition = FormStartPosition.CenterScreen;
        AutoSize = true;
        AutoSizeMode = AutoSizeMode.GrowAndShrink;

        var flow = new FlowLayoutPanel
        {
            FlowDirection = FlowDirection.TopDown,
            AutoSize = true,
            WrapContents = false,
            Padding = new Padding(24, 20, 24, 20),
        };
        flow.Controls.Add(new Label { Text = "ルミ  v" + Program.Version, AutoSize = true, ForeColor = Accent,
                                      Font = new Font("Yu Gothic UI", 16f, FontStyle.Bold), Margin = new Padding(0, 0, 0, 6) });
        string installed = InstalledVersion();
        flow.Controls.Add(new Label
        {
            Text = installed == null ? "顔のあるコンソール風アシスタントをインストールします。"
                 : "インストール済みの v" + installed + " を v" + Program.Version + " に更新します。設定とローカルAIはそのまま残ります。",
            AutoSize = true,
        });
        flow.Controls.Add(new Label { Text = "インストール先: " + InstallDir, AutoSize = true, ForeColor = Dim, Margin = new Padding(0, 4, 0, 14) });

        local.AutoSize = true;
        if (LocalAI.IsInstalled(InstallDir))
        {
            // Enabled=false だと黒背景で文字が読めないので、チェックを変えられないようにするだけにする
            local.Text = "ローカルAI（インストール済み）";
            local.Checked = true;
            local.AutoCheck = false;
        }
        else
        {
            local.Text = "ローカルAIも入れる（" + LocalAI.ModelName + "、約" + (LocalAI.TotalSize / 1000000000.0).ToString("0.0") + "GB をダウンロード）";
            local.Checked = true;
        }
        flow.Controls.Add(local);
        flow.Controls.Add(new Label { Text = "インターネットなしで会話できます。メモリ 16GB 程度を推奨。あとから /install-local でも入れられます。",
                                      AutoSize = true, ForeColor = Dim, Margin = new Padding(20, 0, 0, 8) });
        desktop.Text = "デスクトップにショートカットを作る";
        desktop.AutoSize = true;
        flow.Controls.Add(desktop);

        bar.Width = 560;
        bar.Height = 14;
        bar.Margin = new Padding(0, 18, 0, 4);
        flow.Controls.Add(bar);
        status.AutoSize = true;
        status.ForeColor = Dim;
        status.Text = " ";
        flow.Controls.Add(status);

        var buttons = new FlowLayoutPanel { FlowDirection = FlowDirection.RightToLeft, Width = 560, AutoSize = true, Margin = new Padding(0, 14, 0, 0) };
        foreach (var b in new[] { sub, main })
        {
            b.AutoSize = true;
            b.FlatStyle = FlatStyle.Flat;
            b.FlatAppearance.BorderColor = Dim;
            b.BackColor = Color.FromArgb(30, 30, 30);
            b.Padding = new Padding(10, 2, 10, 2);
        }
        main.Text = installed == null ? "インストール" : "更新";
        sub.Text = "キャンセル";
        buttons.Controls.Add(sub);
        buttons.Controls.Add(main);
        flow.Controls.Add(buttons);
        Controls.Add(flow);

        main.Click += (s, e) => { if (done) { Launch(); Close(); } else Install(); };
        sub.Click += (s, e) => { if (running) cancel = true; else Close(); };
        FormClosing += (s, e) => { if (running) { cancel = true; e.Cancel = true; } };
        AcceptButton = main;
    }

    [DllImport("dwmapi.dll")] static extern int DwmSetWindowAttribute(IntPtr hwnd, int attr, ref int value, int size);

    protected override void OnHandleCreated(EventArgs e)
    {
        base.OnHandleCreated(e);
        int on = 1;
        try { DwmSetWindowAttribute(Handle, 20, ref on, 4); } catch { }  // ダークなタイトルバー
    }

    void SetStatus(string text, double ratio)
    {
        if (InvokeRequired) { BeginInvoke(new Action<string, double>(SetStatus), text, ratio); return; }
        status.Text = text;
        bar.Value = Math.Max(0, Math.Min(100, (int)(ratio * 100)));
    }

    static string InstalledVersion()
    {
        try
        {
            using (var key = Registry.CurrentUser.OpenSubKey(UninstallKey))
                if (key != null && File.Exists(Path.Combine(InstallDir, "Lumi.exe")))
                    return key.GetValue("DisplayVersion") as string ?? "?";
        }
        catch { }
        return null;
    }

    // インストール先で動いているルミ (と llama-server) を探す
    static Process[] RunningFromInstallDir()
    {
        var list = new System.Collections.Generic.List<Process>();
        foreach (string name in new[] { "Lumi", "llama-server" })
            foreach (var p in Process.GetProcessesByName(name))
                try { if (p.MainModule.FileName.StartsWith(InstallDir, StringComparison.OrdinalIgnoreCase)) list.Add(p); }
                catch { }
        return list.ToArray();
    }

    static void StopAll(Process[] procs)
    {
        foreach (var p in procs)
            try { p.Kill(); p.WaitForExit(5000); } catch { }
    }

    void Install()
    {
        var runningApps = RunningFromInstallDir();
        if (runningApps.Length > 0)
        {
            if (MessageBox.Show(this, "ルミが起動中です。終了して続けますか？", Text,
                                MessageBoxButtons.YesNo, MessageBoxIcon.Question) != DialogResult.Yes) return;
            StopAll(runningApps);
        }
        running = true;
        main.Enabled = false;
        local.AutoCheck = desktop.AutoCheck = false;   // 文字が読めるよう Enabled は変えない
        bool withLocal = local.Checked, withDesktop = desktop.Checked;   // 入っているファイルはダウンロードし直さない
        new Thread(() =>
        {
            string error = null;
            try
            {
                SetStatus("ルミをインストール中", 0);
                InstallCore(withLocal, withDesktop);
                if (withLocal)
                {
                    DateTime start = DateTime.Now;
                    LocalAI.Install(InstallDir, (step, ratio) =>
                    {
                        string eta = "";
                        double sec = (DateTime.Now - start).TotalSeconds;
                        if (ratio > 0.02 && ratio < 1 && sec > 3)
                            eta = string.Format("  残り約 {0} 分", Math.Ceiling(sec / ratio * (1 - ratio) / 60));
                        SetStatus(step + string.Format("  {0:0}%", ratio * 100) + eta, ratio);
                    }, () => cancel);
                }
                SetStatus("インストールが完了しました。", 1);
            }
            catch (OperationCanceledException)
            {
                error = "ローカルAIのダウンロードを中断しました。ルミ本体は使えます。続きはルミで /install-local と入力すると再開します。";
            }
            catch (IOException e)
            {
                error = "ファイルを書き込めませんでした。ルミが起動中なら終了してから、もう一度試してください。\n" + e.Message;
            }
            catch (Exception e)
            {
                error = e.Message;
            }
            BeginInvoke(new Action(() =>
            {
                running = false;
                if (error != null) status.Text = error;
                if (File.Exists(Path.Combine(InstallDir, "Lumi.exe")))
                {
                    done = true;
                    main.Text = "ルミを起動";
                    main.Enabled = true;
                }
                sub.Text = "閉じる";
            }));
        }) { IsBackground = true }.Start();
    }

    static void InstallCore(bool withLocal, bool withDesktop)
    {
        Directory.CreateDirectory(InstallDir);
        string exe = Path.Combine(InstallDir, "Lumi.exe");
        using (var res = Assembly.GetExecutingAssembly().GetManifestResourceStream("Lumi.exe"))
        using (var f = File.Create(exe))
            res.CopyTo(f);

        // 自分自身をアンインストーラーとして置いておく
        string self = Assembly.GetExecutingAssembly().Location;
        string uninstaller = Path.Combine(InstallDir, "uninstall.exe");
        if (!string.Equals(Path.GetFullPath(self), Path.GetFullPath(uninstaller), StringComparison.OrdinalIgnoreCase))
            File.Copy(self, uninstaller, true);

        string settings = Path.Combine(InstallDir, "settings.json");
        if (!File.Exists(settings))
            File.WriteAllText(settings, Settings.Template(withLocal ? "local" : "offline"), new UTF8Encoding(false));

        Shortcut(StartMenuLink, exe);
        if (withDesktop) Shortcut(DesktopLink, exe);

        using (var key = Registry.CurrentUser.CreateSubKey(UninstallKey))
        {
            key.SetValue("DisplayName", "ルミ (Lumi)");
            key.SetValue("DisplayVersion", Program.Version);
            key.SetValue("Publisher", "Hotakacchi");
            key.SetValue("DisplayIcon", exe);
            key.SetValue("InstallLocation", InstallDir);
            key.SetValue("UninstallString", "\"" + uninstaller + "\" /uninstall");
            key.SetValue("URLInfoAbout", "https://github.com/Hotakacchi/ai-console");
            key.SetValue("EstimatedSize", (int)((withLocal ? LocalAI.TotalSize : 0) / 1024 + 200), RegistryValueKind.DWord);
            key.SetValue("NoModify", 1, RegistryValueKind.DWord);
            key.SetValue("NoRepair", 1, RegistryValueKind.DWord);
        }
    }

    // WScript.Shell でショートカット (.lnk) を作る
    static void Shortcut(string link, string target)
    {
        Type t = Type.GetTypeFromProgID("WScript.Shell");
        object shell = Activator.CreateInstance(t);
        object lnk = t.InvokeMember("CreateShortcut", BindingFlags.InvokeMethod, null, shell, new object[] { link });
        Type lt = lnk.GetType();
        lt.InvokeMember("TargetPath", BindingFlags.SetProperty, null, lnk, new object[] { target });
        lt.InvokeMember("WorkingDirectory", BindingFlags.SetProperty, null, lnk, new object[] { Path.GetDirectoryName(target) });
        lt.InvokeMember("Description", BindingFlags.SetProperty, null, lnk, new object[] { "顔のあるコンソール風アシスタント" });
        lt.InvokeMember("Save", BindingFlags.InvokeMethod, null, lnk, null);
        Marshal.FinalReleaseComObject(lnk);
        Marshal.FinalReleaseComObject(shell);
    }

    void Launch()
    {
        try { Process.Start(new ProcessStartInfo(Path.Combine(InstallDir, "Lumi.exe")) { WorkingDirectory = InstallDir }); } catch { }
    }

    // ---- アンインストール ----

    public static void Uninstall()
    {
        if (MessageBox.Show("ルミをアンインストールしますか？\nダウンロードしたローカルAIと設定もすべて削除されます。",
                            "ルミ アンインストール", MessageBoxButtons.YesNo, MessageBoxIcon.Question) != DialogResult.Yes) return;

        StopAll(RunningFromInstallDir());

        foreach (string link in new[] { StartMenuLink, DesktopLink })
            try { if (File.Exists(link)) File.Delete(link); } catch { }
        try { Registry.CurrentUser.DeleteSubKeyTree(UninstallKey, false); } catch { }
        try { using (var run = Registry.CurrentUser.OpenSubKey(@"Software\Microsoft\Windows\CurrentVersion\Run", true)) if (run != null) run.DeleteValue("Lumi", false); } catch { }

        MessageBox.Show("ルミをアンインストールしました。", "ルミ アンインストール");
        // 実行中の uninstall.exe 自身もフォルダごと消すため、終了してから cmd で削除する
        Process.Start(new ProcessStartInfo("cmd.exe", "/c ping 127.0.0.1 -n 3 > nul & rmdir /s /q \"" + InstallDir + "\"")
        {
            CreateNoWindow = true,
            UseShellExecute = false,
            WorkingDirectory = Path.GetTempPath(),
        });
    }
}

static class Program
{
    public const string Version = "0.2.0-beta";

    [DllImport("user32.dll")] static extern bool SetProcessDPIAware();

    [STAThread]
    static void Main(string[] args)
    {
        try { SetProcessDPIAware(); } catch { }
        Application.EnableVisualStyles();
        Application.SetCompatibleTextRenderingDefault(false);
        if (Array.IndexOf(args, "/uninstall") >= 0) { SetupForm.Uninstall(); return; }
        Application.Run(new SetupForm());
    }
}
