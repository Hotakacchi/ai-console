"""lumi.ico を作る (要 Pillow: pip install pillow)。

背景の顔と同じデザインを、暗い角丸の四角に描く。小さいサイズほど線を太くして潰れないようにする。
"""

from pathlib import Path
from PIL import Image, ImageDraw

BG = (12, 12, 12, 255)
LINE = (97, 214, 214, 255)
CHEEK = (231, 72, 140, 255)
SIZES = [16, 20, 24, 32, 40, 48, 64, 128, 256]


def draw(size: int) -> Image.Image:
    s = 8  # 大きく描いて縮める (アンチエイリアス)
    n = size * s
    img = Image.new("RGBA", (n, n), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    small = size <= 24
    w = n * (0.075 if small else 0.045)  # 線の太さ

    m = n * 0.04
    d.rounded_rectangle((m, m, n - m, n - m), radius=n * 0.22, fill=BG, outline=LINE, width=int(w))

    # 目
    er = n * (0.085 if small else 0.075)
    for cx in (n * 0.33, n * 0.67):
        cy = n * 0.42
        d.ellipse((cx - er, cy - er, cx + er, cy + er), fill=LINE)

    # ほっぺ (小さいサイズでは省く)
    if size >= 32:
        for bx in (n * 0.17, n * 0.71):
            for i in range(3):
                x = bx + i * n * 0.04
                d.line((x, n * 0.62, x + n * 0.03, n * 0.56), fill=CHEEK, width=int(n * 0.018))

    # 口 (笑顔)
    mw = n * 0.16
    d.arc((n / 2 - mw, n * 0.50, n / 2 + mw, n * 0.72), start=25, end=155, fill=LINE, width=int(w))

    return img.resize((size, size), Image.LANCZOS)


def main():
    out = Path(__file__).resolve().parent.parent / "lumi.ico"
    images = [draw(sz) for sz in SIZES]
    images[-1].save(out, format="ICO", sizes=[(sz, sz) for sz in SIZES], append_images=images[:-1])
    images[-1].save(out.with_name("lumi.png"))
    print("wrote", out)


if __name__ == "__main__":
    main()
