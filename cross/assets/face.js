// ルミの顔 (Windows 版 Lumi.cs の Face を Canvas に移したもの)。
// expression: normal / happy / think / listen / sad / sleep

const VISEME_OPEN = [0, .6, 1, .8, .5, .6, .4, .5, .7, .8, .7, .8, .3, .4, .3, .2, .3, .3, .2, .3, .4, 0];
const SLEEP_AFTER = 90_000;

export class Face {
  constructor(colors, sizeRatio = 0.6) {
    this.c = colors;               // { line, fill, cheek, bg }
    this.sizeRatio = sizeRatio;
    this.expression = "normal";
    this.speaking = false;
    this.born = performance.now();
    this.open = 0; this.target = 0;
    this.lookX = 0; this.lookY = 0; this.lookXT = 0; this.lookYT = 0;
    this.tilt = 0; this.tiltT = 0; this.eyeScale = 1;
    const now = performance.now();
    this.nextMouth = 0; this.lastViseme = -1e9; this.blinkStart = -1e9; this.nextBlink = now + 2000;
    this.nextGlance = now + 4000; this.glanceUntil = 0;
    this.lastActivity = now; this.typingUntil = 0;
    this.flashUntil = 0; this.flashExpr = null;
    this.lastState = "";
  }

  viseme(v) {
    this.lastViseme = performance.now();
    if (v >= 0 && v < VISEME_OPEN.length) this.target = VISEME_OPEN[v];
  }

  // 口の開き具合を直接決める (VOICEVOX の音の時刻表から)
  mouth(open) {
    this.lastViseme = performance.now();
    this.target = open;
  }

  // キー入力などがあったとき (目線を入力行に向け、眠っていたら起きる)
  poke(typing) {
    this.lastActivity = performance.now();
    if (typing) this.typingUntil = this.lastActivity + 900;
  }

  // しばらくだけ表情を変える
  flash(expr, seconds) { this.flashExpr = expr; this.flashUntil = performance.now() + seconds * 1000; }

  current(now) {
    if (this.speaking) this.lastActivity = now;
    if (now < this.flashUntil) return this.flashExpr;
    if (this.expression === "normal" && now - this.lastActivity > SLEEP_AFTER) return "sleep";
    return this.expression;
  }

  blink(now) {
    const t = now - this.blinkStart;
    return t < 160 ? 1 - Math.abs(t - 80) / 80 : 0;
  }

  // 状態を進め、見た目が変わったら true
  tick() {
    const now = performance.now(), t = (now - this.born) / 1000, ex = this.current(now);
    if (now >= this.nextBlink) {
      this.blinkStart = now;
      this.nextBlink = Math.random() < 0.15 ? now + 300 : now + 2500 + Math.random() * 3500;
    }
    if (ex === "think") { this.lookXT = 0.6; this.lookYT = -0.7; this.tiltT = 5; }
    else if (now < this.typingUntil) { this.lookXT = -0.7; this.lookYT = 0.6; this.tiltT = -3; }
    else if (ex === "listen" || ex === "sleep" || this.speaking) { this.lookXT = 0; this.lookYT = 0; this.tiltT = 0; }
    else if (now >= this.nextGlance) {
      this.lookXT = Math.random() * 1.6 - 0.8;
      this.lookYT = Math.random() * 0.8 - 0.4;
      this.tiltT = this.lookXT * 5;
      this.glanceUntil = now + 800 + Math.random() * 1000;
      this.nextGlance = now + 4000 + Math.random() * 5000;
    } else if (now >= this.glanceUntil) { this.lookXT = 0; this.lookYT = 0; this.tiltT = 0; }
    this.lookX += (this.lookXT - this.lookX) * 0.2;
    this.lookY += (this.lookYT - this.lookY) * 0.2;
    this.tilt += (this.tiltT - this.tilt) * 0.12;
    this.eyeScale += ((ex === "listen" ? 1.25 : 1) - this.eyeScale) * 0.25;

    if (!this.speaking) this.target = 0;
    else if (now - this.lastViseme > 250 && now >= this.nextMouth) {
      this.target = 0.15 + Math.random() * 0.85;
      this.nextMouth = now + 80 + Math.random() * 70;
    }
    this.open += (this.target - this.open) * 0.45;
    if (this.open < 0.01) this.open = 0;

    const breathe = ex === "sleep" ? 0.8 : 1.6;
    const state = [ex, this.speaking, Math.round(Math.sin(t * breathe) * 4), Math.round(this.blink(now) * 6),
      Math.round(this.lookX * 12), Math.round(this.lookY * 12), Math.round(this.tilt), Math.round(this.eyeScale * 20),
      Math.round(this.open * 30), (ex === "think" || ex === "listen" || ex === "sleep") ? Math.floor(t * 12) : ""].join(",");
    const changed = state !== this.lastState;
    this.lastState = state;
    return changed;
  }

  draw(ctx, w, h) {
    const now = performance.now(), t = (now - this.born) / 1000, ex = this.current(now), c = this.c;
    ctx.save();
    const s = Math.min(w / 640, h / 440) * this.sizeRatio;
    const bob = Math.sin(t * (ex === "sleep" ? 0.8 : 1.6)) * 6 + (this.speaking ? this.open * 5 : 0);
    ctx.translate(w / 2, h / 2);
    ctx.scale(s, s);
    ctx.translate(0, bob);
    ctx.rotate(this.tilt * Math.PI / 180);
    ctx.lineCap = "round";
    ctx.lineJoin = "round";
    ctx.lineWidth = 7;
    ctx.strokeStyle = c.line;
    ctx.fillStyle = c.fill;

    roundRect(ctx, -300, -200, 600, 400, 90);
    ctx.stroke();

    // 呼ばれて聞いているとき: 両側に音の波
    if (ex === "listen") {
      for (let k = 0; k < 3; k++) {
        const ph = (t * 1.2 + k / 3) % 1, r = 330 + ph * 70;
        ctx.save();
        ctx.strokeStyle = lerp(c.fill, c.bg, ph);
        ctx.lineWidth = 6;
        ellipseArc(ctx, 0, 0, r, r * 0.7, 160, 40);
        ellipseArc(ctx, 0, 0, r, r * 0.7, -20, 40);
        ctx.restore();
      }
    }

    // 目
    const blink = this.blink(now);
    for (const ex0 of [-120, 120]) {
      const x = ex0 + this.lookX * 22, y = -45 + this.lookY * 16;
      if (ex === "happy") ellipseArc(ctx, x, y + 10, 34, 30, 200, 140);
      else if (ex === "sleep") ellipseArc(ctx, x, y - 8, 32, 22, 30, 120);
      else {
        const r = 30 * this.eyeScale, hh = r * (1 - blink);
        if (hh < 4) line(ctx, x - r, y, x + r, y);
        else {
          ctx.beginPath(); ctx.ellipse(x, y, r, hh, 0, 0, Math.PI * 2); ctx.fill();
          if (hh > r * 0.5) {   // 目のハイライト
            ctx.save(); ctx.fillStyle = c.bg;
            ctx.beginPath(); ctx.arc(x + r * 0.41, y - hh * 0.55 + r * 0.16, r * 0.16, 0, Math.PI * 2); ctx.fill();
            ctx.restore();
          }
        }
        if (ex === "sad") {   // 困り眉 (内側を上げる)
          if (ex0 < 0) line(ctx, x - 30, y - 48, x + 25, y - 62);
          else line(ctx, x - 25, y - 62, x + 30, y - 48);
        }
      }
    }

    // ほっぺ
    if (ex === "happy" || this.speaking) {
      ctx.save(); ctx.strokeStyle = c.cheek; ctx.lineWidth = 6;
      for (const bx of [-215, 165]) for (let i = 0; i < 3; i++) line(ctx, bx + i * 18, 40, bx + i * 18 + 12, 18);
      ctx.restore();
    }

    // 口
    const my = 95;
    if (this.speaking || this.open > 0) {
      const hh = 6 + this.open * 60;
      ctx.beginPath(); ctx.ellipse(0, my, 48, hh / 2, 0, 0, Math.PI * 2); ctx.stroke();
    } else if (ex === "think") {
      ctx.beginPath(); ctx.moveTo(-24, my); ctx.bezierCurveTo(-8, my - 14, 8, my + 14, 24, my); ctx.stroke();
    } else if (ex === "sad") ellipseArc(ctx, 0, my + 21, 40, 25, 210, 120);
    else if (ex === "sleep") { ctx.beginPath(); ctx.ellipse(0, my + 2, 10, 8, 0, 0, Math.PI * 2); ctx.stroke(); }
    else if (ex === "happy") ellipseArc(ctx, 0, my - 17, 64, 45, 25, 130);
    else ellipseArc(ctx, 0, my - 14, 52, 36, 30, 120);

    // 考え中: 跳ねる 3 つの点
    if (ex === "think") {
      for (let i = 0; i < 3; i++) {
        const jump = Math.max(0, Math.sin(t * 6 - i * 0.9)) * 18;
        ctx.beginPath(); ctx.arc(260 + i * 36, -170 - jump, 10, 0, Math.PI * 2); ctx.fill();
      }
    }

    // 眠っている: 浮かんでいく z
    if (ex === "sleep") {
      ctx.font = "bold 54px Consolas, Menlo, monospace";
      for (let i = 0; i < 2; i++) {
        const ph = (t * 0.35 + i * 0.5) % 1;
        ctx.fillStyle = lerp(c.fill, c.bg, ph);
        ctx.fillText("z", 240 + ph * 60, -110 - ph * 90);
      }
    }
    ctx.restore();
  }
}

function line(ctx, x1, y1, x2, y2) { ctx.beginPath(); ctx.moveTo(x1, y1); ctx.lineTo(x2, y2); ctx.stroke(); }

// 楕円の一部 (角度は度、GDI+ と同じく時計回り)
function ellipseArc(ctx, cx, cy, rx, ry, start, sweep) {
  ctx.beginPath();
  ctx.ellipse(cx, cy, rx, ry, 0, start * Math.PI / 180, (start + sweep) * Math.PI / 180);
  ctx.stroke();
}

function roundRect(ctx, x, y, w, h, r) {
  ctx.beginPath();
  ctx.moveTo(x + r, y);
  ctx.arcTo(x + w, y, x + w, y + h, r);
  ctx.arcTo(x + w, y + h, x, y + h, r);
  ctx.arcTo(x, y + h, x, y, r);
  ctx.arcTo(x, y, x + w, y, r);
  ctx.closePath();
}

function lerp(a, b, k) {
  const pa = hex(a), pb = hex(b);
  k = Math.max(0, Math.min(1, k));
  return `rgb(${pa.map((v, i) => Math.round(v + (pb[i] - v) * k)).join(",")})`;
}

function hex(s) { return [1, 3, 5].map(i => parseInt(s.slice(i, i + 2), 16)); }
