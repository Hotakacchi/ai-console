// ルミに PC を操作させる仕組み。
// AI は返事の中に <run>PowerShell のコマンド</run> (管理者権限が必要なら <run admin>…</run>) を書く。
// 実行前に必ずユーザーに確認し、許可されたときだけ実行して、結果を AI に返す。

using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Diagnostics;
using System.IO;
using System.Text;

class RunRequest
{
    public string Kind = "run";   // run (コマンド) / search (Web 検索) / fetch (ページを読む)
    public string Command;        // コマンド・検索語・URL
    public bool Admin;
}

// ストリーミング中の返事から <run>…</run> <search>…</search> <fetch>…</fetch> を取り出し、
// 残りの (読み上げる) テキストを返す
class RunExtractor
{
    static readonly string[] Tags = { "run", "search", "fetch" };
    readonly StringBuilder buf = new StringBuilder();
    string inside;   // 中にいるタグの名前 (外なら null)
    bool admin;
    public readonly List<RunRequest> Requests = new List<RunRequest>();

    public string Push(string chunk)
    {
        buf.Append(chunk);
        var visible = new StringBuilder();
        while (true)
        {
            string s = buf.ToString();
            if (inside == null)
            {
                // いちばん手前にある開きタグを探す
                int i = -1;
                string tag = null;
                foreach (string t in Tags)
                {
                    int at = s.IndexOf("<" + t, StringComparison.Ordinal);
                    if (at >= 0 && (i < 0 || at < i)) { i = at; tag = t; }
                }
                if (i < 0)
                {
                    // "<sea" のようにタグの途中で切れているかもしれない末尾は持ち越す
                    int lt = s.LastIndexOf('<');
                    int keep = 0;
                    if (lt >= 0 && s.IndexOf('>', lt) < 0)
                    {
                        string tail = s.Substring(lt);
                        foreach (string t in Tags)
                            if (("<" + t).StartsWith(tail, StringComparison.Ordinal)) { keep = tail.Length; break; }
                    }
                    visible.Append(s.Substring(0, s.Length - keep));
                    buf.Remove(0, s.Length - keep);
                    return visible.ToString();
                }
                int close = s.IndexOf('>', i);
                visible.Append(s.Substring(0, i));
                if (close < 0) { buf.Remove(0, i); return visible.ToString(); }
                admin = s.Substring(i, close - i).Contains("admin");
                inside = tag;
                buf.Remove(0, close + 1);
            }
            else
            {
                string endTag = "</" + inside + ">";
                int end = s.IndexOf(endTag, StringComparison.Ordinal);
                if (end < 0) return visible.ToString();
                string body = s.Substring(0, end).Trim();
                if (body.Length > 0) Requests.Add(new RunRequest { Kind = inside, Command = body, Admin = admin && inside == "run" });
                inside = null;
                buf.Remove(0, end + endTag.Length);
            }
        }
    }

    public string Flush()
    {
        string s = inside != null ? "" : buf.ToString();
        buf.Clear();
        return s;
    }
}

static class CommandRunner
{
    const int TimeoutMs = 120000;
    const int MaxOutput = 4000;

    // PowerShell でコマンドを実行し、出力 (標準出力と標準エラー) を返す。admin なら UAC で昇格する
    public static string Run(string command, bool admin, Func<bool> cancelled)
    {
        string tmp = Path.Combine(Path.GetTempPath(), "lumi_run_" + Guid.NewGuid().ToString("N") + ".txt");
        // 出力は UTF-8 で一時ファイルに書く (昇格した PowerShell の出力は直接受け取れないため、通常時も同じ方法にする)
        string script =
            "$ErrorActionPreference = 'Continue'\n" +
            "$o = & { " + command + "\n} 2>&1 | Out-String -Width 200\n" +
            "[IO.File]::WriteAllText('" + tmp.Replace("'", "''") + "', $o, (New-Object Text.UTF8Encoding $false))\n";
        string encoded = Convert.ToBase64String(Encoding.Unicode.GetBytes(script));
        var psi = new ProcessStartInfo("powershell.exe", "-NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand " + encoded);
        if (admin)
        {
            psi.UseShellExecute = true;
            psi.Verb = "runas";   // Windows の管理者確認 (UAC) が出る
            psi.WindowStyle = ProcessWindowStyle.Hidden;
        }
        else
        {
            psi.UseShellExecute = false;
            psi.CreateNoWindow = true;
        }

        try
        {
            using (var p = Process.Start(psi))
            {
                var limit = DateTime.Now.AddMilliseconds(TimeoutMs);
                while (!p.WaitForExit(200))
                {
                    if (cancelled() || DateTime.Now > limit)
                    {
                        try { p.Kill(); } catch { }
                        return cancelled() ? "(中断しました)" : "(" + TimeoutMs / 1000 + " 秒たっても終わらなかったので止めました)";
                    }
                }
                string output = File.Exists(tmp) ? File.ReadAllText(tmp, Encoding.UTF8).Trim() : "";
                if (output.Length > MaxOutput) output = output.Substring(0, MaxOutput) + "\n…(以下省略)";
                return "終了コード " + p.ExitCode + (output.Length > 0 ? "\n" + output : "\n(出力なし)");
            }
        }
        catch (Win32Exception e)
        {
            if (e.NativeErrorCode == 1223) return "(管理者権限の確認画面で許可されませんでした)";
            throw;
        }
        finally
        {
            try { if (File.Exists(tmp)) File.Delete(tmp); } catch { }
        }
    }
}
