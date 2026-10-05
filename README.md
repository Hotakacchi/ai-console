# ルミ — 顔のあるコンソール風アシスタント

コンソールそっくりのウィンドウの背景に顔が浮かび、返事を声で読み上げるアシスタントです。
喋っている間は口が音声に合わせて動き、まばたきもします。

- Windows 10 / 11 標準の .NET Framework 4.8 だけで動く、約 40KB の単体 exe
- 音声は Windows 標準の音声合成 (日本語音声 Haruka など)
- つなぐ AI は `settings.json` で自由に選べる (Claude / OpenAI 互換 API / 自作スクリプト / AI なし)

## ビルド

追加のインストールは不要です。`build.bat` を実行すると `Lumi.exe` ができます。

```bat
build.bat
```

## 使い方

`Lumi.exe` を起動して、プロンプトに話しかけるだけです。初回起動時に exe と同じフォルダへ `settings.json` が作られます (最初は AI なしの `offline`)。

| コマンド | 内容 |
|---|---|
| `help` | コマンド一覧 |
| `config` | `settings.json` をメモ帳で開く |
| `reload` | 設定を読み直す (再起動不要) |
| `mute` | 音声のオン/オフ |
| `cls` | 画面と会話をリセット |
| `exit` | 終了 |
| `Ctrl+C` / `Esc` | 返事を途中で止める |
| `↑` `↓` | 入力履歴 |

`Lumi.exe --mute` で音声オフのまま起動できます。

## AI の設定 (`settings.json`)

| キー | 内容 |
|---|---|
| `provider` | `offline` / `anthropic` / `openai` / `command` |
| `model` | モデル名 |
| `endpoint` | API の URL (省略時は各プロバイダーの公式 URL) |
| `api_key_env` | API キーが入っている**環境変数の名前** |
| `command` | `command` プロバイダーで実行するコマンド |
| `max_tokens` | 返答の最大トークン数 (0 で既定値) |
| `effort` | Anthropic の effort (`low` / `medium` / `high` など。空なら既定) |
| `system_prompt` | キャラクター設定 (空なら既定の「ルミ」) |
| `voice` | 音声名 (例: `Microsoft Haruka Desktop`。空なら日本語の女性音声) |
| `voice_rate` | 読み上げ速度 (-10 〜 10) |

API キーは設定ファイルに直接書かず、環境変数に入れて `api_key_env` でその名前を指定します。
(`settings.json` は `.gitignore` 済みですが、キーをファイルに残さないための仕組みです)

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

ローカルの Ollama なら API キーは不要です。

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
| `Providers.cs` | `settings.json` の読み込み、AI プロバイダー、文の区切り |

新しいプロバイダーを足すときは、`Providers.cs` の `IProvider` を実装して `Providers.Create` に 1 行追加してください。

## ライセンス

[MIT License](LICENSE)
