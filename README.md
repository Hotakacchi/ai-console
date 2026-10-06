<img src="lumi.png" width="96" align="right">

# ルミ (Lumi) — 顔のあるコンソール風アシスタント

[English](#english)

コンソールそっくりのウィンドウの背景に顔が浮かび、返事を声で読み上げるアシスタントです。
まばたきやよそ見をし、喋るときは口が動きます。「ルミ」と呼びかければ声で話しかけられ、頼めば Web を調べたり PC を操作したりもします (PC の操作は実行前に必ず確認します)。

- **Windows / macOS / Linux** に対応 (Go + [Wails](https://wails.io))
- **ローカルAIを標準搭載** — インターネットなしで会話できる ([llama.cpp](https://github.com/ggml-org/llama.cpp) + [Qwen3.5-4B](https://huggingface.co/Qwen/Qwen3.5-4B)、初回にダウンロード)
- **声で話しかけられる** — 呼びかけ・音声認識もPCの中だけで処理 ([Vosk](https://alphacephei.com/vosk/))
- **日本語・英語** に対応 (翻訳ファイルを足せば言語を増やせる)
- つなぐ AI は自由に選べる: ローカル / Claude / OpenAI 互換 API (Ollama・LM Studio など) / 自作スクリプト / AI なし

## インストール

[Releases](https://github.com/Hotakacchi/ai-console/releases/latest) からダウンロードします。

| OS | ファイル | |
|---|---|---|
| Windows 10 / 11 | `LumiSetup.exe` | 「自分だけ (管理者権限なし)」か「すべてのユーザー」かを選べます。新しい版を実行すると上書き更新になります。 |
| macOS 11 以降 | `Lumi.dmg` | Lumi.app を「アプリケーション」に入れます。署名していないので、初回は右クリック →「開く」で起動してください。 |
| Linux (x64 / arm64) | `lumi_*.deb` / `lumi-linux-*.tar.gz` | GTK4・WebKitGTK 6.0 が必要です (Ubuntu 24.04 以降など)。 |

インストーラーで「ローカルAIもダウンロードする」を選ぶと、初回起動時にローカルAI (約2.8GB) をダウンロードします。あとから `/install-local` でも入れられます。

## 使い方

プロンプトに打ち込むか、「ルミ」と呼びかけて話しかけます。`/` で始まる入力はアプリのコマンドで、`Tab` で補完できます。

| コマンド | 内容 |
|---|---|
| `/help` | コマンド一覧 |
| `/settings` | 今の設定を一覧する |
| `/set <項目> <値>` | 設定を変えて、すぐ反映する (例: `/set language en`、`/set provider local`) |
| `/voices` | 読み上げに使える声の一覧 |
| `/config` | `settings.json` を開く |
| `/reload` | 設定ファイルを読み直す |
| `/mute` | 読み上げのオン/オフ |
| `/mic` | 音声入力のオン/オフ (初回は認識モデル約50MBをダウンロード) |
| `/install-local` | ローカルAIをダウンロードする |
| `/install-voice` | 音声認識モデルをダウンロードする |
| `/peek` | バックグラウンドで呼ばれたときの動きを試す |
| `/cls` | 画面と会話をリセット |
| `/exit` | 終了 |
| `Ctrl+C` / `Esc` | 返事を途中で止める |

### 声で話しかける

「ルミ」と呼ぶと聞く顔になるので、続けて話しかけます。「ルミ、今何時？」と一息で言っても大丈夫です。
ルミが返事をしたあとは、しばらく呼びかけなしで続けて話せます。ウィンドウを閉じてトレイにいるときに呼ぶと、画面の右下から顔が出てきます。
うまく反応しないときは `/set voice_debug on` で、何と聞こえたかを確かめられます。

### Web 検索と PC の操作

- 最新の情報や知らないことは、ルミが自分で Web を調べて答えます (DuckDuckGo、API キー不要)。`/set web_search ask` で毎回確認、`off` で無効。
- 「メモリの使用量を調べて」のように頼むと、ルミが OS のコマンド (Windows は PowerShell、Mac は zsh、Linux は bash) を提案します。**実行前に必ずコマンドを表示して確認**し、`y` (声なら「実行して」) と答えたときだけ実行します。管理者権限が必要なときは、さらに OS の確認画面が出ます。`/set pc_control off` で無効。

AI の提案は間違っていることもあります。内容を確かめてから許可してください。

## 設定 (`settings.json`)

`/settings` で一覧、`/set` で変更できます。ファイルの場所は Windows が `%LOCALAPPDATA%\Lumi`、macOS が `~/Library/Application Support/Lumi`、Linux が `~/.local/share/lumi` です。

| 項目 | 内容 |
|---|---|
| `language` | `auto` (OS に合わせる) / `ja` / `en` |
| `provider` | `local` / `offline` / `anthropic` / `openai` / `command` |
| `model` / `endpoint` / `api_key_env` | モデル名・API の URL・API キーが入っている**環境変数の名前** |
| `pc_control` / `web_search` | PC の操作 / Web 検索 |
| `voice_input` / `wake_word` | 音声入力 / 呼びかけの言葉 (空なら言語ごとの既定) |
| `voice` / `voice_rate` | 読み上げの声 / 速さ |
| `background` / `startup` | 閉じてもトレイで動き続ける / ログイン時に起動する |
| `system_prompt` | キャラクター設定 (空なら既定のルミ) |

API キーは設定ファイルに書かず、環境変数に入れて `api_key_env` でその名前を指定します。

```json
{ "provider": "anthropic", "model": "claude-opus-5-5", "api_key_env": "ANTHROPIC_API_KEY" }
```

```json
{ "provider": "openai", "endpoint": "http://localhost:11434/v1/chat/completions", "model": "<モデル名>" }
```

自作スクリプト (`command`) は、発言のたびに起動され、stdin に `{"system": ..., "messages": [...]}` の JSON を受け取り、stdout に書いた文字が返事になります。[examples/echo_bot.py](examples/echo_bot.py) を見てください。

### 言語を増やす

[cross/locales](cross/locales) の `ja.json` / `en.json` と同じ形のファイル (例: `fr.json`) を作り、データフォルダの `locales` に置くと使えるようになります。プルリクエストも歓迎です。

## ビルド

```bash
cd cross
go build -tags production -o lumi .
```

Windows では `CGO_ENABLED=0` で、Linux では `libgtk-4-dev` と `libwebkitgtk-6.0-dev` が必要です。配布物は GitHub Actions ([.github/workflows/build.yml](.github/workflows/build.yml)) で作っています。

## 軽量版 (Windows のみ)

リポジトリ直下の C# 版 (`Lumi.cs` など、`build.bat` でビルド) は、Windows 標準の .NET Framework だけで動く約130KBの最初の版です。新しい機能はクロスプラットフォーム版 (`cross/`) に入れています。

## ライセンス

[MIT License](LICENSE)。ダウンロードして使う llama.cpp は MIT、Qwen3.5-4B は Apache 2.0、Vosk とその認識モデルは Apache 2.0 です。

---

## English

Lumi is a console-style assistant with an animated face in the background that answers out loud. It runs on **Windows, macOS and Linux**, ships with a **local AI** (llama.cpp + Qwen3.5-4B, downloaded on first use), listens for its **wake word** ("Lumi") with on-device speech recognition (Vosk), can **search the web**, and can **run commands on your PC — always after asking you first**. The UI and the conversation are available in Japanese and English (`/set language en`).

Download `LumiSetup.exe` (Windows), `Lumi.dmg` (macOS) or the `.deb` / `.tar.gz` (Linux) from [Releases](https://github.com/Hotakacchi/ai-console/releases/latest), then type `/help`.
