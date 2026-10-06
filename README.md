<img src="lumi.png" width="96" align="right">

# ルミ — 顔のあるコンソール風アシスタント

コンソールそっくりのウィンドウの背景に顔が浮かび、返事を声で読み上げるアシスタントです。
喋っている間は口が音声に合わせて動き、まばたきやよそ見もします。「ルミ」と呼びかければ声で話しかけられ、頼めば PC の操作もします (実行前に必ず確認します)。

- Windows 10 / 11 標準の .NET Framework 4.8 だけで動く、約 120KB の単体 exe
- ローカルAIを標準搭載 (インターネットなしで会話できる。初回にダウンロード)
- 音声合成・音声認識は Windows 標準のもの (音声は外に送られない)
- Web 検索で最新の情報も調べられる (API キー不要)
- つなぐ AI は `settings.json` で自由に選べる (ローカル / Claude / OpenAI 互換 API / 自作スクリプト / AI なし)

## インストール

[Releases](https://github.com/Hotakacchi/ai-console/releases) から `LumiSetup.exe` をダウンロードして実行します。

- 管理者権限は不要で、`%LOCALAPPDATA%\Programs\Lumi` に入ります。スタートメニューに「ルミ」ができます。
- 「ローカルAIも入れる」にチェックすると、AIエンジン ([llama.cpp](https://github.com/ggml-org/llama.cpp)) と AIモデル ([Qwen3.5-4B](https://huggingface.co/Qwen/Qwen3.5-4B)、約 2.8GB) をダウンロードします。途中で止めても、次回は続きから再開します。
- 新しい `LumiSetup.exe` を実行すると、そのまま上書き更新できます (設定とローカルAIは残ります)。
- アンインストールは Windows の「設定 → アプリ」から行えます。

## ビルド

追加のインストールは不要です。`build.bat` を実行すると `Lumi.exe` と `LumiSetup.exe` ができます。

```bat
build.bat
```

## 使い方

ルミを起動して、プロンプトに打ち込むか「ルミ」と呼びかけて話しかけます。`/` で始まる入力はアプリのコマンドで、それ以外はルミへの話しかけです。`/` を打つと候補が薄く表示され、`Tab` で補完できます。

| コマンド | 内容 |
|---|---|
| `/help` | コマンド一覧 |
| `/settings` | 今の設定を一覧する |
| `/set <項目> <値>` | 設定を変えて、すぐ反映する (例: `/set voice_rate 3`、`/set provider local`) |
| `/voices` | 使える声の一覧 |
| `/config` | `settings.json` をメモ帳で開く |
| `/reload` | 設定ファイルを読み直す |
| `/mute` | 読み上げのオン/オフ |
| `/mic` | 音声入力のオン/オフ |
| `/install-local` | ローカルAIをダウンロードする |
| `/cls` | 画面と会話をリセット |
| `/exit` | 終了 |
| `Ctrl+C` / `Esc` | 返事を途中で止める |
| `↑` `↓` | 入力履歴 |

`Lumi.exe --mute` で読み上げオフ、`--no-mic` で音声入力オフのまま起動できます。

### バックグラウンド

ウィンドウの × で閉じても終了せず、タスクトレイ (画面右下) で動き続けます。「ルミ」と呼びかけるか、トレイのアイコンをダブルクリックするとウィンドウが出てきます。
終了するときは `/exit` か、トレイのアイコンの右クリックメニューから「終了」を選びます。

- `/set startup on` で、Windows の起動時にトレイで起動するようになります。
- `/set background off` で、× で閉じたときに終了するようになります。
- ルミがすでに動いているときにもう一度起動すると、新しく起動せずに今のウィンドウを出します。

### 音声入力

「ルミ」(「ねえルミ」「ルミちゃん」も可) と呼びかけると、目を大きく開いて聞く状態になるので、続けて話しかけます。
「ルミ、今何時？」のように一息で言っても大丈夫です。声で話しかけたときは、返事のあと少しのあいだ呼びかけなしで続けて話せます。
ルミが喋っている間は聞き取りません。「ミュート」「画面を消して」「マイクをオフ」「終了して」などアプリの操作も声でできます。

Windows 標準の日本語音声認識を使っています。入っていない場合は「設定 → 時刻と言語 → 音声認識」から追加してください。

### Web 検索

最新の情報や知らないことは、ルミが自分で Web を調べて答えます (どの AI でも使えます)。調べた検索語と結果のタイトルは画面に表示されます。

- 検索は [DuckDuckGo](https://duckduckgo.com) を使います (API キー不要)。自分で立てた [SearXNG](https://github.com/searxng/searxng) を使うなら `/set search_url <URL>`。
- 結果のページを詳しく読むこともあります。PC 内や家庭内ネットワーク (localhost・192.168.x.x など) のページは読みません。
- `/set web_search ask` で検索のたびに確認、`/set web_search off` で検索しないようになります。
- Web ページの内容に「このコマンドを実行して」などと書かれていても、PC の操作は必ず確認してから実行します。

### PC の操作

「メモリの使用量を調べて」のように頼むと、ルミが PowerShell のコマンドを提案します。

- 実行する前に、必ずコマンドを表示して確認します。`y` で実行、`a` で管理者として実行、それ以外 (Enter だけも) は実行しません。
- 声で答えるときは「実行して」「やめて」と言います。聞き間違いで実行されないよう、決まった言葉だけを受け付けます。
- 管理者権限が必要なコマンドは、さらに Windows の確認画面 (UAC) が出ます。そこで許可しない限り管理者としては実行されません。
- 実行結果はルミに渡され、ルミがそれを見て説明します。
- 使わないときは `/set pc_control off` で無効にできます。

AI が提案したコマンドは間違っていることもあります。内容を確かめてから許可してください。

## AI の設定 (`settings.json`)

| キー | 内容 |
|---|---|
| `provider` | `local` / `offline` / `anthropic` / `openai` / `command` |
| `model` | モデル名 (`local` では models フォルダ内の .gguf ファイル名か、フルパス) |
| `endpoint` | API の URL (省略時は各プロバイダーの公式 URL) |
| `api_key_env` | API キーが入っている**環境変数の名前** |
| `command` | `command` プロバイダーで実行するコマンド |
| `max_tokens` | 返答の最大トークン数 (0 で既定値) |
| `effort` | Anthropic の effort (`low` / `medium` / `high` など。空なら既定) |
| `local_gpu` | `auto` で GPU があれば使う、`off` で CPU だけ |
| `pc_control` | PC の操作 (`on` / `off`。実行前に毎回確認あり) |
| `web_search` | Web 検索 (`on` / `ask` で毎回確認 / `off`) |
| `search_url` | SearXNG の URL (空なら DuckDuckGo) |
| `background` | × で閉じてもトレイで動き続けるか (`on` / `off`) |
| `startup` | Windows の起動時にトレイで起動するか (`on` / `off`) |
| `voice_input` | `on` / `off` |
| `wake_word` | 呼びかけの言葉 (既定は `ルミ`) |
| `wake_confidence` | 呼びかけの聞き取りの厳しさ (0〜1。誤反応が多ければ上げる) |
| `system_prompt` | キャラクター設定 (空なら既定の「ルミ」) |
| `voice` | 音声名 (例: `Microsoft Haruka Desktop`。空なら日本語の女性音声) |
| `voice_rate` | 読み上げ速度 (-10 〜 10) |

API キーは設定ファイルに直接書かず、環境変数に入れて `api_key_env` でその名前を指定します。
(`settings.json` は `.gitignore` 済みですが、キーをファイルに残さないための仕組みです)

### ローカル (`local`) と オフライン (`offline`)

- **`local`**: 同梱のローカルAIで会話します。インターネットは不要です (初回のダウンロードだけ必要)。メモリ 16GB 程度を推奨。
- **`offline`**: AI を使わず、時刻・日付・あいさつなど決まった返事だけをします。とても軽く、ダウンロードも不要です。

```json
{
  "provider": "local",
  "local_gpu": "auto"
}
```

別の GGUF モデルを使いたいときは、`local\models` に置いて `model` にファイル名を書きます。

### Claude (Anthropic)

```json
{
  "provider": "anthropic",
  "model": "claude-opus-5-5",
  "api_key_env": "ANTHROPIC_API_KEY"
}
```

### OpenAI 互換 API

OpenAI の Chat Completions 形式 (`stream: true`) に対応したサービスならつながります。

```json
{
  "provider": "openai",
  "model": "<モデル名>",
  "api_key_env": "OPENAI_API_KEY"
}
```

Ollama なら API キーは不要です。

```json
{
  "provider": "openai",
  "endpoint": "http://localhost:11434/v1/chat/completions",
  "model": "<pull したモデル名>"
}
```

LM Studio (`http://localhost:1234/v1/chat/completions`) や OpenRouter なども `endpoint` を変えるだけで使えます。

### 自作スクリプト (`command`)

どんな言語・どんな AI でもつなげる方法です。発言のたびにコマンドが起動されます。

- **stdin**: `{"system": "...", "messages": [{"role": "user", "content": "..."}, ...]}` (UTF-8 の JSON、会話履歴つき)
- **stdout**: 書いたテキスト (UTF-8) がそのまま返答になります。少しずつ書けば、書いた分から喋り始めます。

```json
{
  "provider": "command",
  "command": "python examples\\echo_bot.py"
}
```

[examples/echo_bot.py](examples/echo_bot.py) が最小のサンプルです。

## コードの構成

| ファイル | 内容 |
|---|---|
| `Lumi.cs` | 画面 (文字グリッドの描画・背景の顔・入力)、音声合成 |
| `Listen.cs` | 呼びかけによる音声入力 |
| `PcControl.cs` | PC の操作 (コマンドの取り出しと実行) |
| `WebSearch.cs` | Web 検索とページの読み込み |
| `Peek.cs` | バックグラウンドで呼ばれたときに右下から出てくる顔 |
| `tools/make_icon.py` | アイコン (`lumi.ico`) の生成 |
| `Providers.cs` | `settings.json` の読み込み、AI プロバイダー、文の区切り |
| `LocalAI.cs` | ローカルAI (llama.cpp + モデル) のダウンロードと起動 |
| `Setup.cs` | インストーラー (`LumiSetup.exe`) |

新しいプロバイダーを足すときは、`Providers.cs` の `IProvider` を実装して `Providers.Create` に 1 行追加してください。

## ライセンス

[MIT License](LICENSE)

ローカルAIでダウンロードする llama.cpp は MIT License、Qwen3.5-4B は Apache License 2.0 です。
