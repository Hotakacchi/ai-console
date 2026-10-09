// 設定画面 (/settings window)。URL が #settings のとき app.js から読み込まれる。
// 項目を Go から受け取って並べ、変えたらその場で Go に送る (確かめ方と保存は /set と同じ)。

export function settingsMain(emit, on) {
  document.body.classList.add("settings");
  const root = document.createElement("div");
  root.id = "settings";
  document.body.appendChild(root);
  let labels = {};
  const status = {};   // 項目ごとの「保存しました」などの表示

  on("settingsData", d => {
    labels = d.labels || {};
    document.title = labels["settings.title"] || "Settings";
    render(d.groups || []);
  });
  on("settingsSaved", d => {
    const el = status[d.key];
    if (!el) return;
    el.textContent = d.ok ? "✓ " + (labels["settings.saved"] || "") : d.msg;
    el.className = "status " + (d.ok ? "ok" : "bad");
    if (d.ok) setTimeout(() => { if (el.textContent.startsWith("✓")) el.textContent = ""; }, 2500);
  });

  function render(groups) {
    const scroll = root.querySelector(".body")?.scrollTop || 0;
    const active = root.querySelector(".tabs .active")?.dataset.id || groups[0]?.id;
    root.textContent = "";
    const head = document.createElement("div");
    head.className = "head";
    head.textContent = labels["settings.title"] || "";
    const hint = document.createElement("div");
    hint.className = "hint";
    hint.textContent = labels["settings.hint"] || "";
    const tabs = document.createElement("div");
    tabs.className = "tabs";
    const body = document.createElement("div");
    body.className = "body";
    root.append(head, hint, tabs, body);

    const show = id => {
      for (const t of tabs.children) t.classList.toggle("active", t.dataset.id === id);
      for (const s of body.children) s.hidden = s.dataset.id !== id;
    };
    for (const g of groups) {
      const tab = document.createElement("button");
      tab.dataset.id = g.id;
      tab.textContent = g.title;
      tab.onclick = () => show(g.id);
      tabs.appendChild(tab);

      const sec = document.createElement("section");
      sec.dataset.id = g.id;
      for (const it of g.items) sec.appendChild(row(it));
      body.appendChild(sec);
    }
    show(active);
    body.scrollTop = scroll;
  }

  function row(it) {
    const r = document.createElement("div");
    r.className = "row";
    const name = document.createElement("div");
    name.className = "name";
    name.textContent = it.key;
    const help = document.createElement("div");
    help.className = "help";
    help.textContent = it.help;
    const st = document.createElement("div");
    st.className = "status";
    status[it.key] = st;
    const send = v => { st.textContent = "…"; st.className = "status"; emit("settingsSet", { key: it.key, value: v }); };

    let input;
    if (it.kind === "choice") {
      input = document.createElement("select");
      for (const c of it.choices) {
        const o = document.createElement("option");
        o.value = c;
        o.textContent = c === "" ? (labels["settings.default"] || "-") : c;
        input.appendChild(o);
      }
      input.value = it.choices.includes(String(it.value).toLowerCase()) ? String(it.value).toLowerCase() : it.choices[0];
      input.onchange = () => send(input.value);
    } else {
      input = document.createElement("input");
      input.type = it.kind === "number" ? "number" : "text";
      if (it.kind === "number") {
        if (it.min !== undefined) input.min = it.min;
        if (it.max !== undefined) input.max = it.max;
        input.step = it.int ? "1" : "any";
      }
      input.value = it.value ?? "";
      input.placeholder = labels["settings.default"] || "";
      input.spellcheck = false;
      // 打ち終わったら (Enter か、ほかを押したとき) 送る
      input.onchange = () => send(input.value === "" ? '""' : input.value);
      input.onkeydown = e => { if (e.key === "Enter") input.blur(); };
    }
    input.className = "value";
    const top = document.createElement("div");
    top.className = "top";
    top.append(name, input);
    r.append(top, help, st);
    return r;
  }

  emit("settingsGet");
}
