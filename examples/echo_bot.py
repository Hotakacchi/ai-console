"""command プロバイダーのサンプル。

settings.json:
  "provider": "command",
  "command": "python examples\\echo_bot.py"

stdin に {"system": "...", "messages": [{"role": "user", "content": "..."}, ...]} が UTF-8 の JSON で届く。
stdout に書いたテキストがそのまま返答になり、少しずつ書けば少しずつ喋る。
"""

import json
import sys
import time

sys.stdout.reconfigure(encoding="utf-8")
data = json.loads(sys.stdin.buffer.read().decode("utf-8-sig"))
last = data["messages"][-1]["content"]
turns = sum(1 for m in data["messages"] if m["role"] == "user")

for piece in [f"「{last}」ですね。", f"これで{turns}回目の発言です。", "ここを好きなAIの呼び出しに書き換えてください。"]:
    print(piece, end="", flush=True)
    time.sleep(0.2)
