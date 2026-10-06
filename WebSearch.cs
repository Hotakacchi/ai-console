// Web 検索とページの読み込み。どの AI プロバイダーでも使えるよう、アプリ側で行う。
//  - 検索: DuckDuckGo (API キー不要) か、自分で立てた SearXNG
//  - 読み込み: 公開サイトの HTML から本文の文字だけを取り出す (PC 内・家庭内ネットワークは読まない)

using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Net;
using System.Net.Sockets;
using System.Text;
using System.Text.RegularExpressions;
using System.Web.Script.Serialization;

static class WebSearch
{
    const int MaxResults = 5;
    const int MaxPageChars = 3000;
    const string UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36";

    class Hit { public string Title, Url, Snippet; }

    // searxngUrl が空なら DuckDuckGo を使う
    public static string Search(string query, string searxngUrl)
    {
        var hits = string.IsNullOrEmpty(searxngUrl) ? DuckDuckGo(query) : SearXNG(query, searxngUrl);
        if (hits.Count == 0) return "(検索結果がありませんでした)";
        var sb = new StringBuilder();
        for (int i = 0; i < hits.Count; i++)
            sb.Append((i + 1) + ". " + hits[i].Title + "\n   " + hits[i].Url + "\n   " + hits[i].Snippet + "\n");
        return sb.ToString().TrimEnd();
    }

    static List<Hit> DuckDuckGo(string query)
    {
        string html = Request("https://html.duckduckgo.com/html/", "q=" + Uri.EscapeDataString(query) + "&kl=jp-jp");
        var hits = new List<Hit>();
        var links = Regex.Matches(html, "<a[^>]*class=\"result__a\"[^>]*href=\"([^\"]+)\"[^>]*>([\\s\\S]*?)</a>");
        var snippets = Regex.Matches(html, "class=\"result__snippet\"[^>]*>([\\s\\S]*?)</a>");
        for (int i = 0; i < links.Count && hits.Count < MaxResults; i++)
        {
            string url = WebUtility.HtmlDecode(links[i].Groups[1].Value);
            // 広告と、DuckDuckGo の転送リンク (/l/?uddg=本当のURL) をほどく
            if (url.Contains("duckduckgo.com/y.js")) continue;
            var m = Regex.Match(url, "[?&]uddg=([^&]+)");
            if (m.Success) url = Uri.UnescapeDataString(m.Groups[1].Value);
            if (url.StartsWith("//")) url = "https:" + url;
            hits.Add(new Hit
            {
                Title = Text(links[i].Groups[2].Value),
                Url = url,
                Snippet = i < snippets.Count ? Text(snippets[i].Groups[1].Value) : "",
            });
        }
        return hits;
    }

    static List<Hit> SearXNG(string query, string baseUrl)
    {
        string json = Request(baseUrl.TrimEnd('/') + "/search?format=json&language=ja&q=" + Uri.EscapeDataString(query), null);
        var root = new JavaScriptSerializer().DeserializeObject(json) as Dictionary<string, object>;
        var results = root != null && root.ContainsKey("results") ? root["results"] as object[] : null;
        var hits = new List<Hit>();
        if (results == null) return hits;
        foreach (var o in results.Take(MaxResults))
        {
            var r = o as Dictionary<string, object>;
            if (r == null) continue;
            hits.Add(new Hit { Title = Str(r, "title"), Url = Str(r, "url"), Snippet = Str(r, "content") });
        }
        return hits;
    }

    // ページを読み、本文の文字だけを返す
    public static string Fetch(string url)
    {
        Uri uri;
        if (!Uri.TryCreate(url.Trim(), UriKind.Absolute, out uri) || (uri.Scheme != "http" && uri.Scheme != "https"))
            return "(http か https の URL だけ読めます)";
        if (IsPrivate(uri)) return "(PC 内や家庭内ネットワークのページは読めません)";
        string html = Request(uri.AbsoluteUri, null, true);
        string title = Regex.Match(html, "<title[^>]*>([\\s\\S]*?)</title>", RegexOptions.IgnoreCase).Groups[1].Value;
        string body = Regex.Replace(html, "<(script|style|noscript|svg|nav|header|footer)[\\s\\S]*?</\\1>", " ", RegexOptions.IgnoreCase);
        body = Regex.Replace(body, "<(br|p|div|li|h[1-6]|tr)[^>]*>", "\n", RegexOptions.IgnoreCase);
        body = Text(body);
        body = Regex.Replace(body, "\n\\s*\n+", "\n");
        if (body.Length > MaxPageChars) body = body.Substring(0, MaxPageChars) + "…(以下省略)";
        return (title.Length > 0 ? "タイトル: " + Text(title) + "\n" : "") + body;
    }

    // localhost・プライベート IP などには行かない (名前解決した先も確かめる)
    static bool IsPrivate(Uri uri)
    {
        if (uri.IsLoopback || uri.Host.EndsWith(".local") || uri.Host == "localhost") return true;
        IPAddress[] addrs;
        try { addrs = Dns.GetHostAddresses(uri.DnsSafeHost); } catch { return false; }
        foreach (var a in addrs)
        {
            if (IPAddress.IsLoopback(a) || a.IsIPv6LinkLocal || a.IsIPv6SiteLocal) return true;
            if (a.AddressFamily == AddressFamily.InterNetwork)
            {
                byte[] b = a.GetAddressBytes();
                if (b[0] == 10 || b[0] == 127 || b[0] == 0 || (b[0] == 169 && b[1] == 254) ||
                    (b[0] == 172 && b[1] >= 16 && b[1] <= 31) || (b[0] == 192 && b[1] == 168)) return true;
            }
            if (a.AddressFamily == AddressFamily.InterNetworkV6 && (a.GetAddressBytes()[0] & 0xFE) == 0xFC) return true;   // fc00::/7
        }
        return false;
    }

    // GET (postBody が null) か POST。文字コードはヘッダーか <meta> から判断する。
    // checkHosts なら、転送 (リダイレクト) を 1 回ずつたどり、転送先が PC 内・家庭内でないか確かめる
    static string Request(string url, string postBody, bool checkHosts = false)
    {
        HttpWebResponse res = null;
        for (int hop = 0; hop <= 5; hop++)
        {
            var req = (HttpWebRequest)WebRequest.Create(url);
            req.UserAgent = UserAgent;
            req.Accept = "text/html,application/json;q=0.9,*/*;q=0.8";
            req.Headers["Accept-Language"] = "ja,en;q=0.8";
            req.AutomaticDecompression = DecompressionMethods.GZip | DecompressionMethods.Deflate;
            req.Timeout = req.ReadWriteTimeout = 15000;
            req.AllowAutoRedirect = !checkHosts;
            if (postBody != null)
            {
                req.Method = "POST";
                req.ContentType = "application/x-www-form-urlencoded";
                byte[] data = Encoding.UTF8.GetBytes(postBody);
                using (var s = req.GetRequestStream()) s.Write(data, 0, data.Length);
            }
            res = (HttpWebResponse)req.GetResponse();
            int code = (int)res.StatusCode;
            if (!checkHosts || code < 300 || code >= 400) break;

            string location = res.Headers["Location"];
            res.Close();
            res = null;
            if (string.IsNullOrEmpty(location)) throw new Exception("転送先がありません");
            var next = new Uri(new Uri(url), location);
            if ((next.Scheme != "http" && next.Scheme != "https") || IsPrivate(next))
                throw new Exception("PC 内や家庭内ネットワークへの転送なので読みません");
            url = next.AbsoluteUri;
        }
        if (res == null) throw new Exception("転送が多すぎます");
        using (res)
        using (var stream = res.GetResponseStream())
        using (var mem = new MemoryStream())
        {
            var buf = new byte[65536];
            int n;
            while ((n = stream.Read(buf, 0, buf.Length)) > 0 && mem.Length < 2 * 1024 * 1024) mem.Write(buf, 0, n);
            byte[] bytes = mem.ToArray();

            string charset = res.CharacterSet;
            if (string.IsNullOrEmpty(charset) || res.ContentType.IndexOf("charset", StringComparison.OrdinalIgnoreCase) < 0)
            {
                string head = Encoding.ASCII.GetString(bytes, 0, Math.Min(bytes.Length, 4096));
                var m = Regex.Match(head, "charset=[\"']?([\\w-]+)", RegexOptions.IgnoreCase);
                charset = m.Success ? m.Groups[1].Value : "utf-8";
            }
            Encoding enc;
            try { enc = Encoding.GetEncoding(charset); } catch { enc = Encoding.UTF8; }
            return enc.GetString(bytes);
        }
    }

    // タグを取り、文字参照を戻し、空白をまとめる
    static string Text(string html)
    {
        string s = Regex.Replace(html, "<[^>]+>", "");
        s = WebUtility.HtmlDecode(s);
        s = Regex.Replace(s, "[ \\t\\r\\f\\v ]+", " ");
        return s.Trim();
    }

    static string Str(Dictionary<string, object> d, string k)
    {
        object v;
        return d.TryGetValue(k, out v) && v != null ? v.ToString() : "";
    }
}
