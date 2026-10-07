# インストーラーのアイコン (アプリのアイコン + 右下にダウンロードのバッジ) を作る
# python build/windows/setupicon.py assets/icon.png build/windows/setup.ico preview.png
import sys
from PIL import Image, ImageDraw

src, out, preview = sys.argv[1], sys.argv[2], sys.argv[3]
base = Image.open(src).convert("RGBA")  # 256x256
S = 1024  # 4 倍で描いてから縮める (なめらかに)

PINK = (255, 79, 160, 255)   # ほっぺの色
WHITE = (255, 255, 255, 255)
DARK = (14, 14, 14, 255)


def badge(scale):
    """右下のバッジ (丸の中に、トレイへ下向きの矢印)。scale は丸の直径 / アイコンの幅"""
    img = Image.new("RGBA", (S, S))
    d = ImageDraw.Draw(img)
    r = S * scale / 2
    cx, cy = S - r - S * 0.01, S - r - S * 0.01
    ring = S * 0.035
    d.ellipse([cx - r, cy - r, cx + r, cy + r], fill=DARK)            # 縁 (アイコンと区切る)
    d.ellipse([cx - r + ring, cy - r + ring, cx + r - ring, cy + r - ring], fill=PINK)
    u = (r - ring) / 100  # 丸の中の単位
    w = 15 * u
    # 矢印の軸
    d.rounded_rectangle([cx - w / 2, cy - 52 * u, cx + w / 2, cy + 8 * u], radius=w / 2, fill=WHITE)
    # 矢印の頭
    d.polygon([(cx - 34 * u, cy - 6 * u), (cx + 34 * u, cy - 6 * u), (cx, cy + 30 * u)], fill=WHITE)
    # トレイ
    d.rounded_rectangle([cx - 46 * u, cy + 40 * u, cx + 46 * u, cy + 40 * u + w], radius=w / 2, fill=WHITE)
    d.rounded_rectangle([cx - 46 * u, cy + 18 * u, cx - 46 * u + w, cy + 40 * u + w], radius=w / 2, fill=WHITE)
    d.rounded_rectangle([cx + 46 * u - w, cy + 18 * u, cx + 46 * u, cy + 40 * u + w], radius=w / 2, fill=WHITE)
    return img


def make(size):
    # 小さいほどバッジを大きめに (16px でも矢印と分かるように)
    scale = 0.5 if size <= 24 else 0.46 if size <= 48 else 0.42
    img = base.resize((S, S), Image.LANCZOS)
    img.alpha_composite(badge(scale))
    return img.resize((size, size), Image.LANCZOS)


sizes = [16, 20, 24, 32, 40, 48, 64, 128, 256]
imgs = [make(s) for s in sizes]
imgs[-1].save(out, format="ICO", sizes=[(s, s) for s in sizes], append_images=imgs[:-1])

# 見本: いくつかの大きさを並べる
sheet = Image.new("RGBA", (256 + 128 + 64 + 48 + 32 + 16 + 70, 260), (240, 240, 240, 255))
x = 0
for s in (256, 128, 64, 48, 32, 16):
    sheet.alpha_composite(make(s), (x, 0))
    x += s + 10
sheet.save(preview)
