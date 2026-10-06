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
| Windows 10 / 11 | `Lumi-Windows-Setup-<バージョン>.exe` | 「自分だけ (管理者権限なし)」か「すべてのユーザー」かを選べます。新しい版を実行すると上書き更新になります。 |
| macOS 11 以降 | `lumi-mac-<バージョン>.dmg` | Lumi.app を「アプリケーション」に入れます。署名していないので、初回は右クリック →「開く」で起動してください。 |
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
| `/install-voicevox` | より自然な声 (VOICEVOX、約330MB) を入れる |
| `/install-whisper` | より正確な音声認識 (Whisper、約110MB) を入れる |
| `/install-vision` | ローカルAIが画像を読めるようにする (約670MB) |
| `/attach [パス]` / `/detach` | ファイルを付ける / 外す (窓にドラッグ＆ドロップでも付けられる) |
| `/screen [質問]` | 画面を撮って見せる (例: `/screen このエラーは何？`) |
| `/commands` | よく使うコマンドの単語帳 (`commands.txt`) を開く。「やりたいこと → コマンド」と書くと AI が優先して使う |
| `/memory` | 覚えていることの一覧 (`/memory delete <番号>`、`/memory clear`) |
| `/reminders` | タイマー・リマインダーの一覧 (`/reminders cancel <番号>`、`/reminders clear`) |
| `/history [件数]` | これまでの会話を表示する |
| `/update` | 新しい版が出ているか確かめる |
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
管理者権限で実行している間は、顔の色が琥珀色に変わります。

### そのほかの機能

- **表情** — 返事の内容に合わせて、うれしい顔・考える顔・驚いた顔などをします。
- **タイマー・リマインダー** — 「5分後に教えて」「15時に会議って言って」と頼むと、時間になったら声で知らせます (トレイにいるときは右下から顔を出します)。
- **覚える** — 「私の名前はほたかだよ、覚えておいて」と言うと覚えて、次からの会話に活かします。忘れてほしいときは「忘れて」と頼むか `/memory`。
- **会話の続き** — 前回の会話を覚えていて、起動したときに続きから話せます (`/set keep_history off` で無効、`/cls` で消去)。
- **クリップボード** — 「コピーした文を要約して」のように頼むと、クリップボードの文章を読みます (`/set clipboard ask` で毎回確認、`on` / `off`)。
- **ショートカット** — どこからでも `Ctrl+Alt+L` (Mac は `Cmd+Option+L`) でウィンドウを出し入れできます (`/set hotkey`)。
- **見た目** — 顔の色・大きさ、文字の大きさ・フォントを変えられます (`/set face_color pink` など)。
- **自然な声** — `/install-voicevox` で [VOICEVOX](https://voicevox.hiroshiba.jp/) の声で話し、声に合わせて口が動きます。`/voices` で声を選べます。
- **ファイルを読ませる** — 窓にファイルをドラッグ＆ドロップして「要約して」「この表の合計は？」のように頼めます。テキスト・PDF・Word・Excel・PowerPoint・画像に対応しています (ローカルAIで長いファイルを読むときは先頭の一部だけになります)。
- **画面を見せる** — 「この画面のエラーは何？」と聞くか `/screen` で、画面を撮って AI に見せます (撮る前に毎回確認します。`/set screen on` / `off`)。ローカルAIで画像を読むには `/install-vision` が必要です。
- **正確な聞き取り** — `/install-whisper` で、話しかけた内容を [Whisper](https://github.com/openai/whisper) で書き起こします (呼びかけは Vosk のまま)。`/set whisper_model small` でさらに正確に (約280MB)。

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
| `tts` / `voicevox_voice` | 読み上げ (`system` / `voicevox`) / VOICEVOX の声の番号 |
| `stt` / `whisper_model` | 書き起こし (`vosk` / `whisper`) / Whisper のモデル (`base` / `small`) |
| `clipboard` / `hotkey` | クリップボードを読む / ウィンドウを出し入れするショートカット |
| `screen` | 画面を撮って AI に見せる (`ask` / `on` / `off`) |
| `keep_history` / `update_check` | 会話を保存して続きから話す / 起動時に新しい版を確かめる |
| `face_color` / `face_size` / `font_size` / `font` | 見た目 |
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

[MIT License](LICENSE)。ダウンロードして使う llama.cpp は MIT、Qwen3.5-4B は Apache 2.0、Vosk とその認識モデルは Apache 2.0、Whisper は MIT、Qwen3.5-4B の画像用の部品は Apache 2.0、transformers.js は Apache 2.0、ONNX Runtime は MIT です。
VOICEVOX はそれぞれのキャラクターの利用規約に従ってください (VOICEVOX の声を使うと、画面に「VOICEVOX:キャラクター名」のクレジットを出します)。

---

## English

Lumi is a console-style assistant with an animated face in the background that answers out loud. It runs on **Windows, macOS and Linux**, ships with a **local AI** (llama.cpp + Qwen3.5-4B, downloaded on first use), listens for its **wake word** ("Lumi") with on-device speech recognition (Vosk), can **search the web**, and can **run commands on your PC — always after asking you first**. The UI and the conversation are available in Japanese and English (`/set language en`).

It also changes its expression to match what it says, sets **timers and reminders** ("remind me in 5 minutes"), **remembers** things you tell it (`/memory`), keeps the **conversation history** across restarts, can read your **clipboard** when asked, reads **files you drop onto the window** (text, PDF, Word, Excel, PowerPoint, images), can **look at your screen** when you ask (`/screen`, always after asking), toggles with a **global hotkey** (`Ctrl+Alt+L`), lets you change its **colors, size and fonts**, and checks for **updates**. Optional downloads: `/install-voicevox` for natural Japanese voices with lip-sync ([VOICEVOX](https://voicevox.hiroshiba.jp/)) and `/install-whisper` for more accurate speech recognition ([Whisper](https://github.com/openai/whisper)).

Download `Lumi-Windows-Setup-<version>.exe` (Windows), `lumi-mac-<version>.dmg` (macOS) or the `.deb` / `.tar.gz` (Linux) from [Releases](https://github.com/Hotakacchi/ai-console/releases/latest), then type `/help`.
