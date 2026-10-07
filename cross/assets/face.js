// ルミの顔 (Windows 版 Lumi.cs の Face を Canvas に移したもの)。
// expression: normal / happy / think / listen / sad / sleep
// running: コマンドの実行中 (顔の中をプログラムが流れる)、shell: シェルモード (顔が >_ になる)
// activity: search (Web を調べている) / download (ダウンロード中、progress 0〜1)
// effect(名前, 秒): しばらくだけの演出
//   boot (起動) / off (終了) / glitch (エラー) / eat (ファイルを食べる) / shutter (画面を撮る) /
//   bulb (覚えた) / bell (リマインダー) / sun・rain・snow・cloud (天気)
// lookAt(x, y): マウスのほうを見る。長く放っておくと、顔が窓の中を跳ね回る (スクリーンセーバー)

const VISEME_OPEN = [0, .6, 1, .8, .5, .6, .4, .5, .7, .8, .7, .8, .3, .4, .3, .2, .3, .3, .2, .3, .4, 0];
const SLEEP_AFTER = 90_000;
const SAVER_AFTER = 300_000;

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
    this.running = false; this.runCmd = ""; this.runStart = 0;
    this.shell = false;
    this.activity = null; this.progress = 0;
    this.fx = null;                // { name, start, until }
    this.mouseX = 0; this.mouseY = 0; this.mouseUntil = 0;
    this.saver = false;            // スクリーンセーバーで跳ね回っている
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

  // マウスのほうを見る (x, y は顔の中心から見た向き、-1〜1)
  lookAt(x, y) {
    this.mouseX = Math.max(-1, Math.min(1, x));
    this.mouseY = Math.max(-1, Math.min(1, y));
    this.mouseUntil = performance.now() + 1500;
    this.lastActivity = performance.now();
  }

  // しばらくだけ表情を変える
  flash(expr, seconds) { this.flashExpr = expr; this.flashUntil = performance.now() + seconds * 1000; }

  // しばらくだけの演出
  effect(name, seconds) {
    const now = performance.now();
    this.fx = { name, start: now, until: now + seconds * 1000 };
    if (name !== "off") this.lastActivity = now;
    this.lastState = "";
  }

  // 調べもの・ダウンロードなど、終わるまで続く様子 (null で終わり)
  setActivity(name, progress = 0) {
    this.activity = name;
    this.progress = progress;
    this.lastState = "";
  }

  // コマンドの実行を始めた / 終わった
  setRunning(on, cmd = "") {
    if (on && !this.running) this.runStart = performance.now();
    this.running = on;
    this.runCmd = cmd;
    this.lastState = "";
  }

  fxNow(now) {
    if (this.fx && now >= this.fx.until) this.fx = null;
    return this.fx;
  }

  current(now) {
    if (this.speaking || this.running || this.activity) this.lastActivity = now;
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
    const now = performance.now(), t = (now - this.born) / 1000, ex = this.current(now), fx = this.fxNow(now);
    this.saver = now - this.lastActivity > SAVER_AFTER;
    if (now >= this.nextBlink) {
      this.blinkStart = now;
      this.nextBlink = Math.random() < 0.15 ? now + 300 : now + 2500 + Math.random() * 3500;
    }
    if (this.activity === "search") { this.lookXT = Math.sin(t * 3) * 0.8; this.lookYT = 0.3; this.tiltT = 0; }
    else if (ex === "think") { this.lookXT = 0.6; this.lookYT = -0.7; this.tiltT = 5; }
    else if (now < this.typingUntil) { this.lookXT = -0.7; this.lookYT = 0.6; this.tiltT = -3; }
    else if (ex === "listen" || ex === "sleep" || this.speaking) { this.lookXT = 0; this.lookYT = 0; this.tiltT = 0; }
    else if (now < this.mouseUntil) { this.lookXT = this.mouseX; this.lookYT = this.mouseY; this.tiltT = this.mouseX * 4; }
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
    this.eyeScale += ((fx && fx.name === "surprised" ? 1.4 : ex === "listen" ? 1.25 : 1) - this.eyeScale) * 0.25;

    if (!this.speaking) this.target = 0;
    else if (now - this.lastViseme > 250 && now >= this.nextMouth) {
      this.target = 0.15 + Math.random() * 0.85;
      this.nextMouth = now + 80 + Math.random() * 70;
    }
    this.open += (this.target - this.open) * 0.45;
    if (this.open < 0.01) this.open = 0;

    const breathe = ex === "sleep" ? 0.8 : 1.6;
    const anim = fx ? fx.name + Math.floor(t * 30)
      : this.running ? "run" + Math.floor(t * 30)
      : this.activity ? this.activity + Math.floor(t * 20) + "," + Math.round(this.progress * 100)
      : this.saver ? "saver" + Math.floor(t * 15)
      : this.shell && !this.speaking ? "sh" + Math.floor(t * 2) : "";
    const state = [anim, ex, this.speaking, Math.round(Math.sin(t * breathe) * 4), Math.round(this.blink(now) * 6),
      Math.round(this.lookX * 12), Math.round(this.lookY * 12), Math.round(this.tilt), Math.round(this.eyeScale * 20),
      Math.round(this.open * 30), (ex === "think" || ex === "listen" || ex === "sleep") ? Math.floor(t * 12) : ""].join(",");
    const changed = state !== this.lastState;
    this.lastState = state;
    return changed;
  }

  draw(ctx, w, h) {
    const now = performance.now(), t = (now - this.born) / 1000, ex = this.current(now), c = this.c;
    const fx = this.fxNow(now), fxName = fx ? fx.name : "", fxT = fx ? (now - fx.start) / 1000 : 0;
    ctx.save();
    let s = Math.min(w / 640, h / 440) * this.sizeRatio;
    let cx = w / 2, cy = h / 2;
    // スクリーンセーバー: 小さくなって、窓の中をゆっくり跳ね回る
    if (this.saver && !fx) {
      s *= 0.55;
      const fw = 640 * s, fh = 440 * s;
      cx = fw / 2 + tri(t * 0.045) * Math.max(0, w - fw);
      cy = fh / 2 + tri(t * 0.06 + 0.3) * Math.max(0, h - fh);
    }
    // 起動: 古いパソコンのような文字が流れてから、顔が出てくる
    if (fxName === "boot") {
      drawBoot(ctx, w, h, fxT, c);
      if (fxT < 1.8) { ctx.restore(); return; }
      ctx.globalAlpha = Math.min(1, (fxT - 1.8) / 0.6);
    }
    const bob = Math.sin(t * (ex === "sleep" ? 0.8 : 1.6)) * 6 + (this.speaking ? this.open * 5 : 0);
    ctx.translate(cx, cy);
    ctx.scale(s, s);
    // 終了: ブラウン管の電源を切るように、横一本の線になって消える
    if (fxName === "off") {
      const p = fxT / 0.7;
      const sy = Math.max(0.015, 1 - p * 2.2), sx = p < 0.45 ? 1 + p * 0.3 : Math.max(0, 1.13 - (p - 0.45) * 2.4);
      ctx.scale(sx, sy);
      if (sy <= 0.02) {
        ctx.fillStyle = "#ffffff";
        ctx.fillRect(-320, -30, 640, 60);
        ctx.restore();
        return;
      }
    }
    // エラー: 顔が乱れる
    if (fxName === "glitch") ctx.translate((Math.random() - 0.5) * 40, (Math.random() - 0.5) * 10);
    ctx.translate(0, bob);
    // 驚いた: 最初にぴょこっと跳ねる
    if (fxName === "surprised" && fxT < 0.4) ctx.translate(0, -Math.sin(fxT / 0.4 * Math.PI) * 36);
    ctx.rotate(this.tilt * Math.PI / 180);
    ctx.lineCap = "round";
    ctx.lineJoin = "round";
    ctx.lineWidth = 7;
    ctx.strokeStyle = c.line;
    ctx.fillStyle = c.fill;

    roundRect(ctx, -300, -200, 600, 400, 90);
    ctx.stroke();

    // コマンドの実行中: 顔の中をプログラムが流れる
    if (this.running) {
      drawCode(ctx, (now - this.runStart) / 1000, c, this.runCmd);
      ctx.restore();
      return;
    }
    // シェルモード: 顔がシェルのマーク (>_) になる (喋っている間はいつもの顔)
    if (this.shell && !this.speaking && !fx) {
      drawShellMark(ctx, t, c);
      ctx.restore();
      return;
    }

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
      if (fxName === "glitch") { line(ctx, x - 26, y - 26, x + 26, y + 26); line(ctx, x - 26, y + 26, x + 26, y - 26); }
      else if (fxName === "bell") {   // 「！」の目
        ctx.save(); ctx.lineWidth = 14;
        line(ctx, x, y - 40, x, y + 8);
        ctx.beginPath(); ctx.arc(x, y + 34, 8, 0, Math.PI * 2); ctx.fill();
        ctx.restore();
      }
      else if (fxName !== "surprised" && (ex === "happy" || fxName === "eat" && fxT > 1.2)) ellipseArc(ctx, x, y + 10, 34, 30, 200, 140);
      else if (fxName !== "surprised" && ex === "sleep") ellipseArc(ctx, x, y - 8, 32, 22, 30, 120);
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
        if (fxName === "surprised") ellipseArc(ctx, x, y - 18, 40, 52, 235, 70);   // 高く上がった眉
        else if (ex === "sad") {   // 困り眉 (内側を上げる)
          if (ex0 < 0) line(ctx, x - 30, y - 48, x + 25, y - 62);
          else line(ctx, x - 25, y - 62, x + 30, y - 48);
        }
      }
    }
    // 晴れ: サングラス
    if (fxName === "sun") {
      ctx.save();
      ctx.fillStyle = "#3a3a3a";
      for (const gx of [-120, 120]) { roundRect(ctx, gx - 62, -90, 124, 80, 26); ctx.fill(); ctx.stroke(); }
      ctx.strokeStyle = "rgba(255,255,255,0.5)";   // レンズの光
      for (const gx of [-120, 120]) line(ctx, gx - 34, -70, gx - 10, -70);
      line(ctx, -58, -60, 58, -60);
      ctx.restore();
    }

    // ほっぺ
    if (ex === "happy" || this.speaking) {
      ctx.save(); ctx.strokeStyle = c.cheek; ctx.lineWidth = 6;
      for (const bx of [-215, 165]) for (let i = 0; i < 3; i++) line(ctx, bx + i * 18, 40, bx + i * 18 + 12, 18);
      ctx.restore();
    }

    // 口
    const my = 95;
    if (this.activity === "download") drawProgress(ctx, my, this.progress, c);
    else if (fxName === "eat") drawEating(ctx, my, fxT, c);
    else if (fxName === "surprised" && !this.speaking) {   // 「お」の口
      ctx.beginPath(); ctx.ellipse(0, my + 4, 20, 28, 0, 0, Math.PI * 2); ctx.stroke();
    }
    else if (this.speaking || this.open > 0) {
      const hh = 6 + this.open * 60;
      ctx.beginPath(); ctx.ellipse(0, my, 48, hh / 2, 0, 0, Math.PI * 2); ctx.stroke();
    } else if (fxName === "glitch" || fxName === "bell") {
      ctx.beginPath(); ctx.ellipse(0, my, 22, 18, 0, 0, Math.PI * 2); ctx.stroke();
    } else if (ex === "think") {
      ctx.beginPath(); ctx.moveTo(-24, my); ctx.bezierCurveTo(-8, my - 14, 8, my + 14, 24, my); ctx.stroke();
    } else if (ex === "sad") ellipseArc(ctx, 0, my + 21, 40, 25, 210, 120);
    else if (ex === "sleep") { ctx.beginPath(); ctx.ellipse(0, my + 2, 10, 8, 0, 0, Math.PI * 2); ctx.stroke(); }
    else if (ex === "happy") ellipseArc(ctx, 0, my - 17, 64, 45, 25, 130);
    else ellipseArc(ctx, 0, my - 14, 52, 36, 30, 120);

    // 考え中: 跳ねる 3 つの点
    if (ex === "think" && !this.activity) {
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

    // Web を調べている: 虫眼鏡が顔の前を動く
    if (this.activity === "search") drawMagnifier(ctx, Math.cos(t * 1.6) * 170, 30 + Math.sin(t * 3.2) * 40, c);
    if (fxName === "bulb") drawBulb(ctx, fxT, c);
    if (fxName === "bell") drawBell(ctx, fxT, c);
    if (fxName === "sun") drawSun(ctx, t, c);
    if (fxName === "rain") drawUmbrella(ctx, t, c);
    if (fxName === "snow") drawSnow(ctx, t, c);
    if (fxName === "cloud") drawCloud(ctx, t, c);
    if (fxName === "glitch") drawGlitchBars(ctx, c);
    ctx.restore();

    // 画面を撮った: シャッターのように一瞬白く光る
    if (fxName === "shutter") {
      ctx.save();
      ctx.fillStyle = `rgba(255,255,255,${Math.max(0, 0.85 - fxT * 2.4)})`;
      ctx.fillRect(0, 0, w, h);
      ctx.restore();
    }
  }
}

// 0→1→0 と行き来する (スクリーンセーバーの動き)
function tri(x) { const f = x % 1; return f < 0.5 ? f * 2 : 2 - f * 2; }

// ---- 起動 ----
const BOOT = ["LUMI BIOS  v2026", "CPU ............ OK", "MEMORY ......... OK", "FACE.SYS ....... OK", "VOICE.SYS ...... OK", "Starting Lumi..."];

function drawBoot(ctx, w, h, t, c) {
  ctx.save();
  const size = Math.max(12, Math.min(22, w / 40));
  ctx.font = `${size}px Consolas, Menlo, monospace`;
  ctx.fillStyle = c.fill;
  ctx.globalAlpha = t < 1.8 ? 1 : Math.max(0, 1 - (t - 1.8) / 0.5);
  const x = Math.max(16, w / 2 - size * 10), y0 = h / 2 - BOOT.length * size * 0.75;
  for (let i = 0; i < BOOT.length; i++) {
    const shown = Math.floor((t - i * 0.25) * 60);
    if (shown <= 0) break;
    ctx.fillText(BOOT[i].slice(0, shown), x, y0 + i * size * 1.5);
  }
  if (Math.floor(t * 4) % 2 === 0) ctx.fillRect(x, y0 + BOOT.length * size * 1.5 - size * 0.8, size * 0.6, size);
  ctx.restore();
}

// ---- 口がプログレスバー ----
function drawProgress(ctx, my, p, c) {
  ctx.save();
  ctx.lineWidth = 6;
  roundRect(ctx, -130, my - 22, 260, 44, 12);
  ctx.stroke();
  const fw = Math.max(0, Math.min(1, p)) * 244;
  if (fw > 2) { roundRect(ctx, -122, my - 14, fw, 28, 6); ctx.fill(); }
  ctx.font = "bold 30px Consolas, Menlo, monospace";
  ctx.textAlign = "center";
  ctx.fillText(Math.round(p * 100) + "%", 0, my + 66);
  ctx.restore();
}

// ---- ファイルを食べる ----
function drawEating(ctx, my, t, c) {
  ctx.save();
  if (t < 0.7) {
    // 紙が口に入っていく
    const open = Math.min(1, t * 4);
    ctx.beginPath(); ctx.ellipse(0, my, 60, 10 + open * 40, 0, 0, Math.PI * 2); ctx.stroke();
    const py = -330 + (t / 0.7) * (my + 330);
    ctx.save();
    ctx.beginPath(); ctx.rect(-300, -400, 600, my + 400); ctx.clip();
    ctx.fillStyle = "#f2f2f2";
    ctx.fillRect(-36, py - 60, 72, 92);
    ctx.strokeStyle = "#888";
    ctx.lineWidth = 4;
    for (let i = 0; i < 4; i++) line(ctx, -24, py - 44 + i * 18, 24, py - 44 + i * 18);
    ctx.restore();
  } else if (t < 1.4) {
    // もぐもぐ
    const chew = Math.abs(Math.sin((t - 0.7) * 18));
    ctx.beginPath(); ctx.ellipse(0, my, 44, 6 + chew * 20, 0, 0, Math.PI * 2); ctx.stroke();
  } else ellipseArc(ctx, 0, my - 17, 64, 45, 25, 130);   // 満足
  ctx.restore();
}

// ---- 虫眼鏡 ----
function drawMagnifier(ctx, x, y, c) {
  ctx.save();
  ctx.lineWidth = 10;
  ctx.fillStyle = lerp(c.fill, c.bg, 0.8);
  ctx.beginPath(); ctx.arc(x, y, 50, 0, Math.PI * 2); ctx.fill(); ctx.stroke();
  ctx.lineWidth = 16;
  line(ctx, x + 36, y + 36, x + 80, y + 80);
  ctx.restore();
}

// ---- 電球 (覚えた) ----
function drawBulb(ctx, t, c) {
  ctx.save();
  const glow = Math.min(1, t * 4);
  ctx.translate(0, -290);
  ctx.fillStyle = glow > 0.5 ? "#ffe066" : c.fill;
  ctx.strokeStyle = c.line;
  ctx.lineWidth = 6;
  ctx.beginPath(); ctx.arc(0, 0, 40, 0, Math.PI * 2); ctx.fill(); ctx.stroke();
  ctx.fillStyle = c.line;
  ctx.fillRect(-18, 38, 36, 22);
  if (glow > 0.5) {
    ctx.strokeStyle = "#ffe066";
    for (let i = 0; i < 8; i++) {
      const a = i * Math.PI / 4, r1 = 56, r2 = 56 + 26 * glow;
      line(ctx, Math.cos(a) * r1, Math.sin(a) * r1, Math.cos(a) * r2, Math.sin(a) * r2);
    }
  }
  ctx.restore();
}

// ---- ベル (リマインダー) ----
function drawBell(ctx, t, c) {
  ctx.save();
  ctx.translate(0, -320);
  ctx.rotate(Math.sin(t * 14) * 0.45 * Math.max(0, 1 - t / 3));
  ctx.fillStyle = "#ffd43b";
  ctx.strokeStyle = c.line;
  ctx.lineWidth = 6;
  ctx.beginPath();
  ctx.moveTo(-44, 70); ctx.quadraticCurveTo(-44, 0, 0, 0); ctx.quadraticCurveTo(44, 0, 44, 70);
  ctx.lineTo(56, 84); ctx.lineTo(-56, 84); ctx.closePath();
  ctx.fill(); ctx.stroke();
  ctx.beginPath(); ctx.arc(0, 94, 12, 0, Math.PI * 2); ctx.fill(); ctx.stroke();
  ctx.restore();
}

// ---- 天気 ----
function drawSun(ctx, t, c) {
  ctx.save();
  ctx.translate(250, -230);
  ctx.rotate(t * 0.8);
  ctx.strokeStyle = "#ffd43b";
  ctx.fillStyle = "#ffd43b";
  ctx.lineWidth = 8;
  ctx.beginPath(); ctx.arc(0, 0, 34, 0, Math.PI * 2); ctx.fill();
  for (let i = 0; i < 8; i++) { const a = i * Math.PI / 4; line(ctx, Math.cos(a) * 48, Math.sin(a) * 48, Math.cos(a) * 66, Math.sin(a) * 66); }
  ctx.restore();
}

function drawUmbrella(ctx, t, c) {
  ctx.save();
  ctx.strokeStyle = c.line;
  ctx.fillStyle = lerp("#74c0fc", c.bg, 0.2);
  ctx.lineWidth = 7;
  ctx.beginPath();
  ctx.moveTo(-200, -250); ctx.quadraticCurveTo(0, -420, 200, -250);
  for (let i = 0; i < 4; i++) ctx.quadraticCurveTo(150 - i * 100, -280, 100 - i * 100, -250);
  ctx.closePath(); ctx.fill(); ctx.stroke();
  line(ctx, 0, -330, 0, -215);
  ctx.strokeStyle = "#74c0fc";
  ctx.lineWidth = 5;
  for (let i = 0; i < 14; i++) {   // 傘の外に落ちる雨
    const x = -330 + (i * 53) % 660, y = -400 + ((t * 420 + i * 97) % 640);
    if (Math.abs(x) < 210 && y < -240) continue;
    line(ctx, x, y, x - 6, y + 26);
  }
  ctx.restore();
}

function drawSnow(ctx, t, c) {
  ctx.save();
  ctx.fillStyle = "#e9f5ff";
  for (let i = 0; i < 18; i++) {
    const x = -330 + (i * 41) % 660 + Math.sin(t * 2 + i) * 14, y = -360 + ((t * 70 + i * 61) % 600);
    ctx.beginPath(); ctx.arc(x, y, 7 + (i % 3) * 2, 0, Math.PI * 2); ctx.fill();
  }
  ctx.restore();
}

function drawCloud(ctx, t, c) {
  ctx.save();
  ctx.translate(Math.sin(t * 0.6) * 60, -280);
  ctx.fillStyle = lerp("#dee2e6", c.bg, 0.15);
  for (const [x, y, r] of [[-70, 10, 40], [-20, -16, 56], [44, 0, 46], [90, 16, 32]]) {
    ctx.beginPath(); ctx.arc(x, y, r, 0, Math.PI * 2); ctx.fill();
  }
  ctx.fillRect(-110, 10, 230, 46);
  ctx.restore();
}

// ---- エラーのノイズ ----
function drawGlitchBars(ctx, c) {
  ctx.save();
  for (let i = 0; i < 6; i++) {
    const y = -220 + Math.random() * 440, hh = 6 + Math.random() * 22, x = -330 + Math.random() * 80;
    ctx.fillStyle = Math.random() < 0.5 ? c.bg : lerp(c.fill, "#ff0040", Math.random());
    ctx.fillRect(x, y, 600 + Math.random() * 60, hh);
  }
  ctx.restore();
}

// ---- コマンドの実行中: 顔の中を下から上へ流れるプログラム ----
const CODE = [
  "for (i = 0; i < n; i++) {", "  buf[i] ^= key[i % 16];", "}", "if err != nil {", "  return err", "mov rax, [rbp-8]",
  "call 0x7ff6a1c4", "SELECT * FROM tasks;", "Get-ChildItem -Recurse", "ls -la /usr/bin", "grep -rn TODO .",
  "while (ok) step();", "fn main() {", "  let x = run()?;", "def hello():", "  print('lumi')", "push rbp",
  "jmp short loop", "await fetch(url)", "git status", "make -j8", "echo $PATH", "ping 127.0.0.1", "return 0;",
];

function codeLine(i) {
  // 行ごとに決まった内容 (同じ行はいつも同じ見た目)
  const r = Math.abs(Math.sin(i * 12.9898) * 43758.5453) % 1;
  if (r < 0.3) {
    let hex = "";
    for (let k = 0; k < 6; k++) hex += Math.floor(Math.abs(Math.sin((i + 1) * (k + 3) * 78.233) * 9999) % 256).toString(16).padStart(2, "0") + " ";
    return (0x7ff0 + i * 16 % 0xffff).toString(16) + ": " + hex;
  }
  return CODE[Math.floor(r * 997) % CODE.length];
}

function drawCode(ctx, t, c, cmd) {
  ctx.save();
  roundRect(ctx, -282, -182, 564, 364, 76);
  ctx.clip();
  ctx.font = "28px Consolas, Menlo, monospace";
  ctx.textBaseline = "alphabetic";
  const lh = 38, speed = 95;           // 行の高さ、1 秒に流れる量
  const pos = t * speed, first = Math.floor(pos / lh), off = pos % lh;
  const top = cmd ? -128 : -170;       // 実行中のコマンドを上に出すときは、その下から
  for (let k = 0; k < 12; k++) {
    const y = 175 - k * lh - off;      // 下ほど新しい行
    if (y < top) break;
    const fade = Math.min(1, (175 - y) / (175 - top));
    ctx.fillStyle = lerp(c.fill, c.bg, 0.15 + fade * 0.75);
    const text = codeLine(first + k);
    // いちばん下の行は打ち込んでいる途中のように少しずつ出す
    const shown = k === 0 ? text.slice(0, Math.floor((off / lh) * text.length) + 1) : text;
    ctx.fillText(shown, -250, y);
  }
  if (cmd) {
    ctx.fillStyle = c.bg;
    ctx.fillRect(-300, -200, 600, 92);
    ctx.fillStyle = c.fill;
    ctx.font = "bold 30px Consolas, Menlo, monospace";
    const s = "$ " + cmd.replace(/\s+/g, " ");
    ctx.fillText(s.length > 30 ? s.slice(0, 29) + "…" : s, -250, -138);
    ctx.fillRect(-250, -122, 500, 3);
  }
  ctx.restore();
}

// ---- シェルモード: >_ ----
function drawShellMark(ctx, t, c) {
  ctx.save();
  ctx.lineWidth = 22;
  ctx.lineCap = "round";
  ctx.lineJoin = "round";
  ctx.beginPath();
  ctx.moveTo(-170, -85); ctx.lineTo(-60, 0); ctx.lineTo(-170, 85);
  ctx.stroke();
  if (Math.floor(t * 2) % 2 === 0) {   // 点滅するカーソル
    ctx.beginPath(); ctx.moveTo(10, 85); ctx.lineTo(170, 85); ctx.stroke();
  }
  ctx.restore();
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
