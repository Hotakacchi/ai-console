// ローカルAI (llama.cpp の llama-server + GGUF モデル) のダウンロードと起動。
// Lumi.exe とインストーラー (LumiSetup.exe) の両方から使う。

using System;
using System.Diagnostics;
using System.IO;
using System.IO.Compression;
using System.Linq;
using System.Net;
using System.Net.Sockets;
using System.Runtime.InteropServices;
using System.Security.Cryptography;
using System.Text;
using System.Threading;

static class LocalAI
{
    // 取得元はバージョンを固定し、SHA-256 で中身を確かめる
    public const string LlamaUrl = "https://github.com/ggml-org/llama.cpp/releases/download/b11433/llama-b11433-bin-win-vulkan-x64.zip";
    public const long LlamaSize = 33337778;
    public const string LlamaSha256 = "021001e2b7a60aab1d23b301edb2f076e8a5136d8d29686d0d95341342088714";

    public const string ModelName = "Qwen3.5-4B";
    public const string ModelFile = "Qwen3.5-4B-Q4_K_M.gguf";
    public const string ModelUrl = "https://huggingface.co/unsloth/Qwen3.5-4B-GGUF/resolve/e87f176479d0855a907a41277aca2f8ee7a09523/Qwen3.5-4B-Q4_K_M.gguf";
    public const long ModelSize = 2740937888;
    public const string ModelSha256 = "00fe7986ff5f6b463e62455821146049db6f9313603938a70800d1fb69ef11a4";

    public const long TotalSize = LlamaSize + ModelSize;

    public static string Dir(string baseDir) { return Path.Combine(baseDir, "local"); }
    public static string LlamaDir(string baseDir) { return Path.Combine(Dir(baseDir), "llama"); }
    public static string ModelsDir(string baseDir) { return Path.Combine(Dir(baseDir), "models"); }

    public static string ServerExe(string baseDir)
    {
        string dir = LlamaDir(baseDir);
        if (!Directory.Exists(dir)) return null;
        return Directory.GetFiles(dir, "llama-server.exe", SearchOption.AllDirectories).FirstOrDefault();
    }

    // model が空なら標準モデル、ファイル名だけなら models フォルダ内として扱う
    public static string ModelPath(string baseDir, string model)
    {
        if (string.IsNullOrEmpty(model)) model = ModelFile;
        return Path.IsPathRooted(model) ? model : Path.Combine(ModelsDir(baseDir), model);
    }

    public static bool IsInstalled(string baseDir)
    {
        return ServerExe(baseDir) != null && File.Exists(ModelPath(baseDir, null));
    }

    // llama.cpp と標準モデルをダウンロードする。progress(説明, 0〜1)
    public static void Install(string baseDir, Action<string, double> progress, Func<bool> cancelled)
    {
        ServicePointManager.SecurityProtocol = SecurityProtocolType.Tls12;
        Directory.CreateDirectory(ModelsDir(baseDir));

        if (ServerExe(baseDir) == null)
        {
            string zip = Path.Combine(Dir(baseDir), "llama.zip");
            Download(LlamaUrl, zip, LlamaSize, LlamaSha256,
                     (done, total) => progress("AIエンジン (llama.cpp) をダウンロード中", (double)done / TotalSize), cancelled);
            progress("AIエンジンを展開中", (double)LlamaSize / TotalSize);
            string dir = LlamaDir(baseDir);
            if (Directory.Exists(dir)) Directory.Delete(dir, true);
            ZipFile.ExtractToDirectory(zip, dir);
            File.Delete(zip);
        }

        string model = ModelPath(baseDir, null);
        if (!File.Exists(model))
            Download(ModelUrl, model, ModelSize, ModelSha256,
                     (done, total) => progress("AIモデル (" + ModelName + ") をダウンロード中", (double)(LlamaSize + done) / TotalSize), cancelled,
                     () => progress("AIモデルを検証中", 1.0));
        progress("完了", 1.0);
    }

    // 途中で止まっても .part から再開できるダウンロード。最後にサイズと SHA-256 を確かめる
    static void Download(string url, string dest, long size, string sha256,
                         Action<long, long> progress, Func<bool> cancelled, Action verifying = null)
    {
        string part = dest + ".part";
        long have = File.Exists(part) ? new FileInfo(part).Length : 0;
        if (have > size) { File.Delete(part); have = 0; }

        if (have < size)
        {
            var req = (HttpWebRequest)WebRequest.Create(url);
            req.UserAgent = "Lumi";
            req.Timeout = req.ReadWriteTimeout = 60000;
            if (have > 0) req.AddRange(have);
            using (var res = (HttpWebResponse)req.GetResponse())
            {
                if (have > 0 && res.StatusCode != HttpStatusCode.PartialContent) have = 0;   // 再開できないサーバーなら最初から
                using (var input = res.GetResponseStream())
                using (var output = new FileStream(part, have > 0 ? FileMode.Append : FileMode.Create, FileAccess.Write))
                {
                    var buf = new byte[1 << 20];
                    int n;
                    DateTime last = DateTime.MinValue;
                    while ((n = input.Read(buf, 0, buf.Length)) > 0)
                    {
                        if (cancelled()) throw new OperationCanceledException();
                        output.Write(buf, 0, n);
                        have += n;
                        if ((DateTime.Now - last).TotalMilliseconds > 200) { progress(have, size); last = DateTime.Now; }
                    }
                }
            }
        }

        if (verifying != null) verifying();
        if (new FileInfo(part).Length != size || Sha256(part) != sha256)
        {
            File.Delete(part);
            throw new Exception(Path.GetFileName(dest) + " の中身が正しくありません。もう一度試してください。");
        }
        if (File.Exists(dest)) File.Delete(dest);
        File.Move(part, dest);
    }

    static string Sha256(string path)
    {
        using (var sha = SHA256.Create())
        using (var f = File.OpenRead(path))
            return BitConverter.ToString(sha.ComputeHash(f)).Replace("-", "").ToLowerInvariant();
    }
}

// llama-server をバックグラウンドで 1 つだけ動かす。Lumi が終了すると一緒に終了する
static class LocalServer
{
    static readonly object gate = new object();
    static Process proc;
    static string runningArgs;
    static IntPtr job;
    public static string Endpoint { get; private set; }
    // 同じ PC のほかのアプリから勝手に使われないよう、起動ごとにランダムな鍵をかける
    public static string ApiKey { get; private set; }

    public static void Start(string baseDir, string modelPath, bool gpu, int context)
    {
        lock (gate)
        {
            string exe = LocalAI.ServerExe(baseDir);
            if (exe == null || !File.Exists(modelPath))
                throw new Exception("ローカルAIが入っていません。/install-local と入力するか、LumiSetup.exe で入れてください");

            int port;
            string key = modelPath + "|" + gpu + "|" + context;
            if (proc != null && !proc.HasExited && runningArgs == key) return;
            Stop();

            var l = new TcpListener(IPAddress.Loopback, 0);
            l.Start();
            port = ((IPEndPoint)l.LocalEndpoint).Port;
            l.Stop();

            string apiKey = Guid.NewGuid().ToString("N");
            // -np 1: 会話は 1 つだけなので、コンテキストを丸ごと 1 つの会話に使う
            var psi = new ProcessStartInfo(exe,
                "-m \"" + modelPath + "\" --host 127.0.0.1 --port " + port + " -c " + context + " -np 1 -ngl " + (gpu ? "99" : "0") +
                " --api-key " + apiKey)
            {
                UseShellExecute = false,
                CreateNoWindow = true,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                WorkingDirectory = Path.GetDirectoryName(exe),
            };
            string logPath = Path.Combine(LocalAI.Dir(baseDir), "server.log");
            var log = new StreamWriter(logPath, false, new UTF8Encoding(false)) { AutoFlush = true };
            var p = new Process { StartInfo = psi };
            DataReceivedEventHandler write = (s, e) => { if (e.Data != null) lock (log) log.WriteLine(e.Data); };
            p.OutputDataReceived += write;
            p.ErrorDataReceived += write;
            p.Start();
            p.BeginOutputReadLine();
            p.BeginErrorReadLine();
            KillWithParent(p);
            proc = p;
            runningArgs = key;
            Endpoint = "http://127.0.0.1:" + port;
            ApiKey = apiKey;

            // モデルの読み込みが終わると /health が 200 を返す
            DateTime limit = DateTime.Now.AddMinutes(5);
            while (true)
            {
                if (p.HasExited)
                {
                    proc = null;
                    throw new Exception("ローカルAIの起動に失敗しました。詳しくは local\\server.log を見てください");
                }
                try
                {
                    var req = (HttpWebRequest)WebRequest.Create(Endpoint + "/health");
                    req.Timeout = 2000;
                    using (var res = (HttpWebResponse)req.GetResponse())
                        if (res.StatusCode == HttpStatusCode.OK) return;
                }
                catch (WebException) { }
                if (DateTime.Now > limit) { Stop(); throw new Exception("ローカルAIの起動がタイムアウトしました"); }
                Thread.Sleep(500);
            }
        }
    }

    public static void Stop()
    {
        lock (gate)
        {
            try { if (proc != null && !proc.HasExited) proc.Kill(); } catch { }
            proc = null;
            runningArgs = null;
        }
    }

    // Job オブジェクトに入れておき、Lumi が落ちても llama-server が残らないようにする
    [StructLayout(LayoutKind.Sequential)]
    struct BasicLimit
    {
        public long PerProcessUserTimeLimit, PerJobUserTimeLimit;
        public uint LimitFlags;
        public UIntPtr MinimumWorkingSetSize, MaximumWorkingSetSize;
        public uint ActiveProcessLimit;
        public UIntPtr Affinity;
        public uint PriorityClass, SchedulingClass;
    }
    [StructLayout(LayoutKind.Sequential)]
    struct IoCounters { public ulong R, W, O, RB, WB, OB; }
    [StructLayout(LayoutKind.Sequential)]
    struct ExtendedLimit
    {
        public BasicLimit Basic;
        public IoCounters Io;
        public UIntPtr ProcessMemoryLimit, JobMemoryLimit, PeakProcessMemoryUsed, PeakJobMemoryUsed;
    }
    [DllImport("kernel32.dll")] static extern IntPtr CreateJobObject(IntPtr attrs, string name);
    [DllImport("kernel32.dll")] static extern bool SetInformationJobObject(IntPtr job, int cls, ref ExtendedLimit info, int size);
    [DllImport("kernel32.dll")] static extern bool AssignProcessToJobObject(IntPtr job, IntPtr process);

    static void KillWithParent(Process p)
    {
        try
        {
            if (job == IntPtr.Zero)
            {
                job = CreateJobObject(IntPtr.Zero, null);
                var info = new ExtendedLimit();
                info.Basic.LimitFlags = 0x2000;   // JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
                SetInformationJobObject(job, 9, ref info, Marshal.SizeOf(typeof(ExtendedLimit)));
            }
            AssignProcessToJobObject(job, p.Handle);
        }
        catch { }
    }
}
