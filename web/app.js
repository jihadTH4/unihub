"use strict";
/* ================= helpers ================= */
const $ = (s, el = document) => el.querySelector(s);
const $$ = (s, el = document) => [...el.querySelectorAll(s)];
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
const ic = (name, cls = "") => `<svg class="icon ${cls}"><use href="#i-${name}"/></svg>`;
const SEP = '<span class="sep">•</span>';

async function api(path, body) {
  const opt = { headers: { "X-UniHub-Token": window.API_TOKEN } };
  if (body !== undefined) {
    opt.method = "POST";
    opt.headers["Content-Type"] = "application/json";
    opt.body = JSON.stringify(body);
  }
  let r;
  try { r = await fetch(path, opt); } catch (e) { throw new Error("البرنامج غير متصل. يتم إعادة الاتصال…"); }
  const data = await r.json().catch(() => ({}));
  if (r.status === 403 && /الجلسة/.test(data.error || "")) { location.reload(); }
  if (!r.ok) throw new Error(data.error || "حدث خطأ غير متوقع");
  return data;
}

function toast(msg, icon = "check-circle", ms = 3800) {
  const t = $("#toast");
  t.innerHTML = ic(icon) + `<span>${esc(msg)}</span>`;
  t.classList.remove("hide");
  clearTimeout(toast._t);
  toast._t = setTimeout(() => t.classList.add("hide"), ms);
}

const fmtDT = new Intl.DateTimeFormat("ar-u-nu-latn", { weekday: "long", day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" });
const fmtD = new Intl.DateTimeFormat("ar-u-nu-latn", { day: "numeric", month: "short", year: "numeric" });
const fmtDay = new Intl.DateTimeFormat("ar-u-nu-latn", { day: "numeric" });
const fmtMon = new Intl.DateTimeFormat("ar-u-nu-latn", { month: "short" });
const ts = (t) => (typeof t === "string" ? Date.parse(t) / 1000 : t);
const dateTime = (t) => (t ? fmtDT.format(new Date(ts(t) * 1000)) : "—");
const dateOnly = (t) => (t ? fmtD.format(new Date(ts(t) * 1000)) : "");

function ago(t) {
  t = ts(t);
  if (!t) return "لم تتم بعد";
  const s = Math.max(0, Date.now() / 1000 - t);
  if (s < 60) return "الآن";
  if (s < 3600) return `منذ ${Math.floor(s / 60)} د`;
  if (s < 86400) return `منذ ${Math.floor(s / 3600)} س`;
  if (s < 86400 * 30) return `منذ ${Math.floor(s / 86400)} يوم`;
  return dateOnly(t);
}

function until(t) {
  const s = t - Date.now() / 1000;
  if (s <= 0) return { text: "انتهى الموعد", cls: "gray" };
  const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
  let text;
  if (d >= 2) text = `بعد ${d} أيام`;
  else if (d === 1) text = `بعد يوم و${h} س`;
  else if (h >= 1) text = `بعد ${h} ساعة`;
  else text = `بعد ${m} دقيقة`;
  return { text, cls: s < 86400 ? "red" : s < 3 * 86400 ? "amber" : "blue" };
}

function size(b) {
  if (!b) return "";
  if (b < 1024) return b + " B";
  if (b < 1048576) return (b / 1024).toFixed(0) + " KB";
  return (b / 1048576).toFixed(1) + " MB";
}

const extBox = (ext) => {
  const e = (ext || "").replace(".", "").toLowerCase();
  return `<div class="ext ${esc(e)}">${esc(e.slice(0, 4)) || ic("file")}</div>`;
};

function statusPill(a) {
  if (a.status === "submitted") return `<span class="pill green">${ic("check")}تم التسليم</span>`;
  if (a.due && a.due < Date.now() / 1000) return `<span class="pill gray">لم يُسلّم</span>`;
  if (a.status === "draft") return `<span class="pill amber">مسودة</span>`;
  return `<span class="pill red">مطلوب</span>`;
}

// tiny, safe markdown for release notes and feedback
function md(src) {
  const lines = esc(src || "").replace(/\r/g, "").split("\n");
  let html = "", inList = false;
  const inline = (s) => s.replace(/`([^`]+)`/g, "<code>$1</code>").replace(/\*\*([^*]+)\*\*/g, "<b>$1</b>")
    .replace(/(https?:\/\/[^\s<]+)/g, '<a data-url="$1">$1</a>');
  for (const l of lines) {
    const li = l.match(/^\s*[-*]\s+(.*)/);
    if (li) { if (!inList) { html += "<ul>"; inList = true; } html += `<li>${inline(li[1])}</li>`; continue; }
    if (inList) { html += "</ul>"; inList = false; }
    const h = l.match(/^#{1,4}\s+(.*)/);
    if (h) html += `<h4>${inline(h[1])}</h4>`;
    else if (l.trim()) html += `<p>${inline(l)}</p>`;
  }
  if (inList) html += "</ul>";
  return `<div class="md">${html}</div>`;
}

const openFile = (p, reveal = false) => api("/api/open", { path: p, reveal }).catch((e) => toast(e.message, "alert"));
const openUrl = (u) => api("/api/openurl", { url: u }).catch((e) => toast(e.message, "alert"));
const sep = () => (S && S.isWindows ? "\\" : "/");

document.addEventListener("click", (e) => {
  const el = e.target.closest("[data-open],[data-reveal],[data-url],[data-course],[data-go]");
  if (!el) return;
  e.preventDefault();
  e.stopPropagation();
  if (el.dataset.open) openFile(el.dataset.open);
  else if (el.dataset.reveal) openFile(el.dataset.reveal, true);
  else if (el.dataset.url) openUrl(el.dataset.url);
  else if (el.dataset.course) go("course", { id: +el.dataset.course });
  else if (el.dataset.go) go(el.dataset.go);
});

function fileRow(f, o = {}) {
  const meta = [o.showCourse && f.course, f.category, dateOnly(f.mtime), size(f.size)].filter(Boolean).map(esc).join(SEP);
  return `<div class="item clickable hit" data-open="${esc(f.path)}" title="${esc(f.path)}">
    ${extBox(f.ext)}
    <div class="body"><div class="title" dir="auto">${o.nameHtml || esc(f.name)}</div>
      <div class="meta">${meta}</div>${o.extra || ""}</div>
    <div class="actions"><button class="btn sm ghost" data-reveal="${esc(f.path)}" title="إظهار في المجلد">${ic("folder")}</button></div>
  </div>`;
}

const empty = (icon, text) => `<div class="empty">${ic(icon)}<div>${text}</div></div>`;
const head = (title, sub = "", actions = "", eyebrow = "") =>
  `<div class="page-head"><div>${eyebrow ? `<div class="eyebrow">${eyebrow}</div>` : ""}<h1>${title}</h1>${sub ? `<p>${sub}</p>` : ""}</div><div class="row wrap">${actions}</div></div>`;
const cardHead = (icon, title, extra = "") => `<div class="card-head"><h2><span class="chip-ic">${ic(icon)}</span>${title}</h2>${extra}</div>`;

/* ================= theme ================= */
const mq = matchMedia("(prefers-color-scheme: dark)");
function applyTheme(t, save) {
  if (t === "light" || t === "dark") document.documentElement.dataset.theme = t;
  else delete document.documentElement.dataset.theme;
  try { localStorage.setItem("uh-theme", t); } catch (e) {}
  $$("[data-theme-set]").forEach((b) => b.classList.toggle("on", b.dataset.themeSet === t));
  if (save) api("/api/settings", { theme: t }).catch(() => {});
  syncBars();
}
// inside the Android app: colour the phone's status and navigation bars like the page
function syncBars() {
  if (!window.UniHubApp?.setBars) return;
  requestAnimationFrame(() => {
    const cs = getComputedStyle(document.documentElement);
    const dark = cs.colorScheme.trim() === "dark";
    try { window.UniHubApp.setBars(dark ? "#0a0d19" : "#f5f3ee", dark); } catch (e) {}
  });
}
$$("[data-theme-set]").forEach((b) => b.addEventListener("click", () => applyTheme(b.dataset.themeSet, true)));
mq.addEventListener?.("change", syncBars);

/* ================= state ================= */
let S = null;
let current = "home";
let params = {};
let bootId = null;
let offline = false;
let themeSynced = false;

async function refresh() {
  try {
    S = await api("/api/state");
  } catch (e) {
    offline = true;
    $("#sync-line").textContent = "البرنامج متوقف أو يُعاد تشغيله…";
    $("#sync-dot").className = "dot off";
    waitForServer();
    return;
  }
  if (bootId && S.bootId !== bootId) { location.reload(); return; }
  bootId = S.bootId;
  offline = false;
  if (!themeSynced) { themeSynced = true; applyTheme(S.theme || "auto", false); }
  document.documentElement.classList.toggle("on-app", S.platform === "android");

  $("#who").textContent = S.linked ? (S.fullName || S.siteName || "UniHub") : "غير مربوط بالمودل";
  $("#ver").textContent = "الإصدار " + S.version;
  const pending = (S.assignments || []).filter((a) => a.status !== "submitted" && a.due > S.now).length;
  setBadge("#badge-assign", pending);
  setBadge("#tb-assign", pending);
  setBadge("#badge-notif", S.newNotif);
  setBadge("#badge-vid", S.videos === "manual" ? S.videosWaiting || 0 : 0);
  updateQuickbar();
  setBadge("#badge-fb", S.fbUnread);
  setBadge("#tb-more", S.fbUnread);
  const sy = S.sync;
  const dot = $("#sync-dot");
  if (sy.running) {
    dot.className = "dot busy";
    $("#sync-line").textContent = (sy.phase || "مزامنة") + (sy.total ? ` (${sy.done}/${sy.total})` : "");
  } else {
    dot.className = "dot" + (!S.linked ? " off" : S.lastError ? " err" : "");
    $("#sync-line").textContent = S.linked ? "آخر مزامنة " + ago(S.lastSync) : "غير مربوط";
  }
  const ix = S.index;
  $("#index-line").textContent = ix.running ? `فهرسة ${ix.done} من ${ix.total}` : `${ix.docs.toLocaleString("en")} ملف جاهز للبحث`;
  $("#btn-sync").disabled = sy.running || !S.linked;
  $("#btn-sync").classList.toggle("spinning", sy.running);
  renderNavLinks();
  renderBanners();
  if (S.update.justUpdated && !$("#modal").dataset.shown) showWhatsNew();
  if (!S.linked && !["settings", "setup", "search", "import", "feedback", "links", "images"].includes(current)) go("setup");
  else if (["home", "assignments", "notifications", "courses"].includes(current) && !document.activeElement?.closest("#view")) render();
}

async function waitForServer() {
  if (waitForServer.busy) return;
  waitForServer.busy = true;
  for (;;) {
    await new Promise((r) => setTimeout(r, 1500));
    try {
      const r = await fetch("/api/ping");
      if (r.ok) { location.reload(); return; }
    } catch (e) {}
  }
}

function setBadge(sel, n) {
  const b = $(sel);
  b.textContent = n;
  b.classList.toggle("hide", !n);
}

function go(view, p = {}, fromHash = false) {
  current = view;
  params = p;
  if (!fromHash) {
    const h = "#" + view + (p.id ? "/" + p.id : "");
    if (location.hash !== h) history.pushState(null, "", h);
  }
  $$("#nav a").forEach((a) => a.classList.toggle("active", a.dataset.view === view || (view === "course" && a.dataset.view === "courses")));
  if (typeof markTabs === "function") { markTabs(); closeSide(); }
  const v = $("#view");
  v.onclick = null;
  v.style.animation = "none";
  void v.offsetWidth;
  v.style.animation = "";
  render(true);
  window.scrollTo({ top: 0 });
  if (view === "notifications" && S?.newNotif) api("/api/notifications/seen", {}).then(refresh);
}

$$("#nav a").forEach((a) => a.addEventListener("click", () => go(a.dataset.view)));
$("#btn-sync").addEventListener("click", async () => {
  await api("/api/sync", {});
  toast("بدأت المزامنة مع المودل", "refresh");
  setTimeout(refresh, 600);
});

const views = {};
let lastHTML = "";
function render(fresh) {
  if (!S) return;
  const v = $("#view");
  const fn = views[current] || views.home;
  if (fresh || !["search", "import", "settings", "setup", "course", "feedback", "links", "images", "videos"].includes(current)) {
    const html = fn(v, fresh);
    // the 5-second refresh rebuilds the page only when something changed (smoother on weak devices)
    if (typeof html === "string" && (fresh || html !== lastHTML)) v.innerHTML = html;
    lastHTML = typeof html === "string" ? html : "";
  }
}

/* ================= performance ================= */
// "auto" watches the first seconds: if the device can't keep a smooth frame rate,
// it switches to the light mode (fewer effects, shorter animations) and remembers it.
function perfPref() { try { return localStorage.getItem("uh-perf") || "auto"; } catch (e) { return "auto"; } }
function applyPerf(p) {
  try { localStorage.setItem("uh-perf", p); } catch (e) {}
  let detected = "";
  try { detected = localStorage.getItem("uh-perf-detected") || ""; } catch (e) {}
  const n = navigator, mem = n.deviceMemory || 8, cores = n.hardwareConcurrency || 8;
  const weak = mem <= 2 || cores <= 2 || n.connection?.saveData || (/Android/.test(n.userAgent) && (mem <= 4 || cores <= 4)) || detected === "lite";
  H.classList.toggle("perf-lite", p === "lite" || (p === "auto" && weak));
}
function measurePerf() {
  if (perfPref() !== "auto" || H.classList.contains("perf-lite") || document.hidden) return;
  let frames = 0, slow = 0, last = performance.now();
  const start = last;
  const step = (t) => {
    frames++;
    if (t - last > 34) slow++; // a frame that took longer than ~30 fps
    last = t;
    if (t - start < 2500) return requestAnimationFrame(step);
    if (frames > 20 && slow / frames > 0.3) {
      try { localStorage.setItem("uh-perf-detected", "lite"); } catch (e) {}
      H.classList.add("perf-lite");
    }
  };
  requestAnimationFrame(step);
}

/* ================= banners & modal ================= */
function renderBanners() {
  const b = [];
  const u = S.update;
  if (u.installing) {
    b.push(`<div class="banner"><div class="b-ic">${ic("download")}</div><div class="b-txt"><b>جارٍ تثبيت التحديث ${esc(u.available?.version || "")}</b>
      <span>${u.progress < 100 ? "تنزيل الملف…" : S.platform === "android" ? "التحقق من الملف…" : "التحقق والتثبيت… سيُعاد تشغيل البرنامج تلقائياً"}</span>
      <div class="progress"><i style="width:${u.progress || 2}%"></i></div></div></div>`);
  } else if (u.available && u.ready === u.available.version) {
    b.push(`<div class="banner"><div class="b-ic">${ic("sparkles")}</div><div class="b-txt"><b>التحديث ${esc(u.ready)} جاهز للتثبيت</b>
      <span>تم تنزيله والتحقق منه. اضغط «تثبيت» ثم أكّد في نافذة أندرويد.</span></div>
      <button class="btn sm ghost" id="bn-notes">ما الجديد؟</button><button class="btn sm gold" id="bn-upd">${ic("download")}تثبيت</button></div>`);
  } else if (u.available && u.error) {
    b.push(`<div class="banner warn"><div class="b-ic">${ic("alert")}</div><div class="b-txt"><b>لم يكتمل التحديث إلى ${esc(u.available.version)}</b>
      <span>${esc(u.error)}</span></div><button class="btn sm gold" id="bn-upd">${ic("refresh")}إعادة المحاولة</button></div>`);
  } else if (u.available) {
    b.push(`<div class="banner"><div class="b-ic">${ic("sparkles")}</div><div class="b-txt"><b>إصدار جديد ${esc(u.available.version)} متاح</b>
      <span>بياناتك وملفاتك محفوظة بالكامل أثناء التحديث.</span></div>
      <button class="btn sm ghost" id="bn-notes">ما الجديد؟</button><button class="btn sm gold" id="bn-upd">${ic("download")}تحديث الآن</button></div>`);
  }
  if (S.lastError && S.linked && !S.sync.running) {
    b.push(`<div class="banner warn"><div class="b-ic">${ic("alert")}</div><div class="b-txt"><b>تنبيه من المزامنة</b><span>${esc(S.lastError)}</span></div></div>`);
  }
  if (S.canInstall && !S.installed && !S.hideInstall) {
    b.push(`<div class="banner"><div class="b-ic">${ic("install")}</div><div class="b-txt"><b>ثبّت البرنامج على جهازك</b>
      <span>يضيف اختصاراً في قائمة ابدأ وسطح المكتب، ويجعل التحديثات التلقائية أسهل.</span></div>
      <button class="btn sm ghost" id="bn-later">لاحقاً</button><button class="btn sm primary" id="bn-install">تثبيت</button></div>`);
  }
  const html = b.join("");
  const box = $("#banners");
  if (box.dataset.html !== html) {
    box.dataset.html = html;
    box.innerHTML = html;
    $("#bn-upd")?.addEventListener("click", startUpdate);
    $("#bn-notes")?.addEventListener("click", () => showNotes(S.update.available));
    $("#bn-later")?.addEventListener("click", () => {
      S.hideInstall = true; renderBanners();
      api("/api/settings", { hideInstall: true }).catch(() => {});
      toast("لن تظهر هذه الرسالة مرة أخرى. يمكنك التثبيت لاحقاً من الإعدادات", "install", 5000);
    });
    $("#bn-install")?.addEventListener("click", async () => {
      try { await api("/api/install", {}); toast("تم التثبيت. يُعاد تشغيل البرنامج من مكانه الجديد…", "install", 6000); waitForServer(); }
      catch (e) { toast(e.message, "alert"); }
    });
  }
}

async function startUpdate() {
  await api("/api/update/apply", {});
  toast(S.update.ready ? "افتح نافذة التثبيت وأكّد" : "بدأ تنزيل التحديث", "download");
  setTimeout(refresh, 500);
}

function modal(html) {
  const m = $("#modal");
  m.innerHTML = `<div class="sheet">${html}</div>`;
  m.classList.remove("hide");
  m.onclick = (e) => { if (e.target === m || e.target.closest("[data-close]")) closeModal(); };
}
function closeModal() { $("#modal").classList.add("hide"); }

function showNotes(rel) {
  modal(`<div class="row"><span class="chip-ic" style="width:42px;height:42px;border-radius:13px;display:flex;align-items:center;justify-content:center;background:var(--gold-soft);color:var(--gold-ink)">${ic("sparkles", "lg")}</span>
    <div><div class="eyebrow">الإصدار ${esc(rel.version)}</div><h2 style="font-size:19px">ما الجديد</h2></div></div>
    <div style="margin:16px 0">${rel.notes ? md(rel.notes) : '<p class="muted">لا توجد ملاحظات لهذا الإصدار.</p>'}</div>
    <div class="row"><span class="spacer"></span><button class="btn ghost" data-close>إغلاق</button><button class="btn gold" id="m-upd">${ic("download")}تحديث الآن</button></div>`);
  $("#m-upd").addEventListener("click", () => { closeModal(); startUpdate(); });
}

function showWhatsNew() {
  $("#modal").dataset.shown = "1";
  modal(`<div style="text-align:center;margin-bottom:10px"><img src="icon.svg" style="width:64px;height:64px"></div>
    <div style="text-align:center"><div class="eyebrow">تم التحديث بنجاح</div><h2 style="justify-content:center;font-size:20px">أنت الآن على الإصدار ${esc(S.version)}</h2>
    <p class="muted small">من الإصدار ${esc(S.update.justUpdated)} · كل بياناتك وإعداداتك كما هي</p></div>
    <div style="margin:14px 0">${S.update.notes ? md(S.update.notes) : ""}</div>
    <div class="row"><span class="spacer"></span><button class="btn primary" data-close>رائع</button></div>`);
  api("/api/update/dismiss-notes", {});
}

/* ================= home ================= */
function upcoming() {
  const now = S.now, items = [];
  for (const a of S.assignments || []) {
    if (a.due && a.due > now - 3600 && a.status !== "submitted")
      items.push({ name: a.name, course: a.course, time: a.due, kind: "واجب", url: a.url, folder: a.folder });
  }
  for (const e of S.events || []) {
    if (e.time > now - 3600 && !(e.kind === "assign" && items.some((i) => i.time === e.time)))
      items.push({ name: e.name, course: e.course, time: e.time, kind: e.kind === "quiz" ? "اختبار" : "موعد", url: e.url });
  }
  return items.sort((x, y) => x.time - y.time);
}

function deadlineRow(i) {
  const u = until(i.time);
  const d = new Date(i.time * 1000);
  return `<div class="item">
    <div class="when ${u.cls}"><b>${fmtDay.format(d)}</b><span>${fmtMon.format(d)}</span></div>
    <div class="body"><div class="title">${esc(i.name)}</div>
      <div class="meta"><span class="pill ${u.cls}">${ic("clock")}${u.text}</span>${esc(i.kind)}${SEP}${esc(i.course)}</div></div>
    <div class="actions">
      ${i.folder ? `<button class="btn sm ghost" title="مجلد الواجب" data-reveal="${esc(i.folder + sep() + "تفاصيل الواجب.html")}">${ic("folder")}</button>` : ""}
      ${i.url ? `<button class="btn sm" data-url="${esc(i.url)}">${ic("external")}المودل</button>` : ""}
    </div></div>`;
}

function notifRow(n) {
  return `<div class="item">
    <div class="kind-ic" style="background:var(--blue-soft);color:var(--blue)">${ic("bell")}</div>
    <div class="body"><div class="title">${n.isNew ? '<span class="newdot"></span>' : ""}${esc(n.subject)}</div>
      ${n.text && n.text !== n.subject ? `<div class="snip">${esc(n.text.slice(0, 240))}</div>` : ""}
      <div class="meta">${esc(ago(n.time))}</div></div>
    ${n.url ? `<div class="actions"><button class="btn sm ghost" data-url="${esc(n.url)}">${ic("external")}</button></div>` : ""}
  </div>`;
}

// home summary cards: each one opens its own section
const statLink = (view, icon, tone, n, label) =>
  `<a class="stat link" data-go="${view}" role="button" tabindex="0" aria-label="${esc(label)}: ${esc(String(n))}"><div class="ic ${tone}">${ic(icon, "lg")}</div><div><b>${n}</b><span>${label}</span></div><span class="go">${ic("chevron")}</span></a>`;

document.addEventListener("keydown", (e) => {
  const el = e.target.closest?.(".stat.link");
  if (el && (e.key === "Enter" || e.key === " ")) { e.preventDefault(); go(el.dataset.go); }
});

views.home = () => {
  const up = upcoming();
  const pending = (S.assignments || []).filter((a) => a.status !== "submitted" && a.due > S.now).length;
  const hour = new Date().getHours();
  const greet = hour < 12 ? "صباح الخير" : hour < 18 ? "مساء الخير" : "مساء النور";
  const first = (S.fullName || "").split(" ")[0];
  return head(`${greet}${first ? "، " + esc(first) : ""}`, "هذه نظرة سريعة على دراستك اليوم", "", dateOnly(S.now)) + `
  <div class="stats">
    ${statLink("courses", "book", "", (S.courses || []).length, "مادة دراسية")}
    ${statLink("assignments", "tasks", "red", pending, "واجب بانتظار التسليم")}
    ${statLink("search", "search", "blue", S.index.docs.toLocaleString("en"), "ملف قابل للبحث")}
    ${statLink("notifications", "bell", "green", S.newNotif || 0, "إشعار جديد")}
  </div>
  ${pinnedCard()}
  ${(S.links || []).length ? `<div class="card" style="margin-bottom:20px">${cardHead("link", "روابطي", `<a data-go="links" class="small">تعديل</a>`)}
    <div class="quick-links">${S.links.map(qlink).join("")}</div></div>` : ""}
  <div class="grid g2">
    <div class="card">${cardHead("calendar", "المواعيد القادمة", up.length > 6 ? `<a data-go="assignments" class="small">عرض الكل</a>` : "")}
      <div class="list">${up.slice(0, 6).map(deadlineRow).join("") || empty("check-circle", "لا توجد مواعيد قادمة. استمتع بوقتك")}</div></div>
    <div class="card">${cardHead("bell", "آخر الإشعارات", (S.notifications || []).length > 5 ? `<a data-go="notifications" class="small">عرض الكل</a>` : "")}
      <div class="list">${(S.notifications || []).slice(0, 5).map(notifRow).join("") || empty("bell", "لا توجد إشعارات")}</div></div>
    <div class="card span-all">${cardHead("inbox", "وصل حديثاً من المودل")}
      <div class="list">${(S.recent || []).slice(0, 8).map((f) => fileRow({ path: f.localPath, name: f.name, ext: "." + f.name.split(".").pop(), course: f.course, category: f.category, mtime: f.downloaded, size: f.size }, { showCourse: true })).join("") || empty("inbox", "لم يتم تنزيل ملفات بعد")}</div></div>
  </div>`;
};

/* ================= courses ================= */
views.courses = () => {
  const hidden = new Set(S.hidden || []);
  const list = (S.courses || []).filter((c) => !hidden.has(c.id));
  const pend = {};
  for (const a of S.assignments || []) if (a.status !== "submitted" && a.due > S.now) pend[a.courseId] = (pend[a.courseId] || 0) + 1;
  return head("المواد", `${list.length} مادة في هذا الفصل`) + `
  <div class="grid g3">${list.map((c) => `
    <div class="card course-card" data-course="${c.id}">
      <div class="cic">${ic("book")}</div>
      <div class="cname">${esc(c.name)}</div>
      <div class="muted small" dir="ltr" style="text-align:right">${esc(c.shortName)}</div>
      <div style="margin-top:14px">${pend[c.id] ? `<span class="pill red">${ic("clock")}${pend[c.id]} واجب مطلوب</span>` : `<span class="pill green">${ic("check")}لا واجبات معلّقة</span>`}</div>
    </div>`).join("") || `<div class="card span-all">${empty("book", "لا توجد مواد بعد. اضغط زر المزامنة")}</div>`}</div>`;
};

views.course = (v) => {
  v.innerHTML = empty("refresh", "جارٍ التحميل…");
  api("/api/course?id=" + params.id).then((d) => {
    const c = d.course;
    const cats = ["الكل", "محاضرات", "فيديوهات المحاضرات", "واجبات", "امتحانات", "ملفات أخرى"].filter((t, i) => i !== 2 || d.files.some((f) => f.category === t));
    const tab = params.tab || "الكل";
    const catOf = (f) => f.category || "ملفات أخرى";
    const count = (t) => (t === "الكل" ? d.files.length : d.files.filter((f) => catOf(f) === t).length);
    const files = tab === "الكل" ? d.files : d.files.filter((f) => catOf(f) === tab);
    v.innerHTML = head(esc(c.name), esc(c.shortName),
      `<button class="btn" data-reveal="${esc(c.folder)}">${ic("folder")}مجلد المادة</button><button class="btn" data-url="${esc(c.url)}">${ic("external")}فتح في المودل</button>`,
      `<a data-go="courses">المواد</a>`) + `
      ${d.assignments?.length ? `<div class="card" style="margin-bottom:20px">${cardHead("tasks", "واجبات المادة")}<div class="list">
        ${d.assignments.map((a) => `<div class="item"><div class="body"><div class="title">${esc(a.name)}</div>
          <div class="meta">${ic("calendar")}${esc(dateTime(a.due))}</div></div>${statusPill(a)}
          <div class="actions"><button class="btn sm ghost" title="مجلد الحل" data-reveal="${esc(a.folder + sep() + "حلّي")}">${ic("folder")}</button>
          <button class="btn sm" data-url="${esc(a.url)}">${ic("external")}تسليم</button></div></div>`).join("")}</div></div>` : ""}
      <div class="row wrap" style="margin-bottom:14px"><div class="seg">${cats.map((t) => `<button class="${t === tab ? "on" : ""}" data-tab="${t}">${t} <span class="muted">${count(t)}</span></button>`).join("")}</div></div>
      <div class="card"><div class="list">${files.map((f) => fileRow(f)).join("") || empty("folder", "لا توجد ملفات هنا")}</div></div>`;
    $$("[data-tab]", v).forEach((b) => b.addEventListener("click", () => { params.tab = b.dataset.tab; views.course(v); }));
  }).catch((e) => (v.innerHTML = empty("alert", esc(e.message))));
};

/* ================= lecture videos ================= */
let VID = null, vidTimer = null, vidFilter = "all";
const vidModes = { auto: "تلقائي", manual: "يدوي", off: "إيقاف" };
const vidStatus = {
  new: ["gray", "download", "لم يُنزّل"], queued: ["blue", "clock", "في الانتظار"], downloading: ["gold", "download", "جارٍ التنزيل"],
  done: ["green", "check", "على جهازك"], error: ["red", "alert", "تعذّر"], hidden: ["gray", "eye-off", "متجاهل"], missing: ["amber", "alert", "نُقل أو حُذف"],
};
function speedTxt(b) { return b > 0 ? size(b) + "/ث" : ""; }
function etaTxt(p) {
  if (!(p.speed > 0) || !(p.total > p.done)) return "";
  const s = Math.round((p.total - p.done) / p.speed);
  return s < 60 ? "أقل من دقيقة" : s < 3600 ? `${Math.ceil(s / 60)} دقيقة متبقية` : `${(s / 3600).toFixed(1)} ساعة متبقية`;
}
async function loadVideos() { VID = await api("/api/videos"); return VID; }

views.videos = (v) => {
  clearInterval(vidTimer);
  if (!VID) v.innerHTML = empty("refresh", "جارٍ التحميل…");
  const draw = () => {
    const items = VID.items || [], p = VID.progress || {};
    const waiting = items.filter((i) => i.status === "new" || i.status === "error").length;
    const done = items.filter((i) => i.status === "done");
    const total = done.reduce((s, i) => s + (i.size || 0), 0);
    const shown = items.filter((i) => vidFilter === "all" ? i.status !== "hidden" : vidFilter === "todo" ? !["done", "hidden"].includes(i.status) : vidFilter === "done" ? i.status === "done" : i.status === "hidden");
    const groups = {};
    for (const it of shown) (groups[it.course] = groups[it.course] || []).push(it);
    const row = (it) => {
      const p = VID.progress || {}; // read fresh: the active row is redrawn every second
      const [cls, icn, txt] = vidStatus[it.status] || vidStatus.new;
      const live = p.running && p.key === it.key;
      const pct = live && p.total ? Math.min(100, Math.round((p.done / p.total) * 100)) : 0;
      const ext = "." + (it.name.split(".").pop() || "mp4");
      return `<div class="item vid-row ${it.status === "done" ? "clickable" : ""}" data-key="${esc(it.key)}" ${it.status === "done" ? `data-open="${esc(it.localPath)}"` : ""}>
        <div class="vid-ic ${it.status === "done" ? "ok" : ""}">${ic(it.status === "done" ? "play" : /\.(mp3|m4a|aac|wav|ogg|oga|opus)$/i.test(ext) ? "bell" : "video")}</div>
        <div class="body"><div class="title" dir="auto">${esc(it.name)}</div>
          <div class="meta"><span class="pill ${cls}">${ic(icn)}${live ? pct + "%" : txt}</span>${it.section ? esc(it.section) + SEP : ""}${it.size ? `<span dir="ltr">${esc(size(it.size))}</span>` : ""}${it.updated ? SEP + "نسخة محدثة" : ""}</div>
          ${live ? `<div class="progress"><i style="width:${pct}%"></i></div><div class="meta" style="margin-top:4px">${p.done ? `<span dir="ltr">${esc(size(p.done))}</span>${p.total ? ` من <span dir="ltr">${esc(size(p.total))}</span>` : ""}` : "جارٍ البدء…"}${p.speed ? SEP + `<span dir="ltr">${esc(speedTxt(p.speed))}</span>` : ""}${etaTxt(p) ? SEP + etaTxt(p) : ""}</div>` : ""}
          ${it.status === "error" && it.error ? `<div class="snip">${esc(it.error)}</div>` : ""}</div>
        <div class="actions">
          ${it.status === "done" ? `<button class="btn sm ghost" data-reveal="${esc(it.localPath)}" title="إظهار في المجلد">${ic("folder")}</button>` : ""}
          ${["new", "error", "missing"].includes(it.status) ? `<button class="btn sm" data-vget="${esc(it.key)}">${ic("download")}تنزيل</button>` : ""}
          ${it.status === "new" || it.status === "error" ? `<button class="btn sm ghost" data-vhide="${esc(it.key)}" title="تجاهل هذا الفيديو">${ic("eye-off")}</button>` : ""}
          ${it.status === "hidden" ? `<button class="btn sm ghost" data-vshow="${esc(it.key)}">إظهار</button>` : ""}
        </div></div>`;
    };
    drawRow = row;
    const mode = VID.mode;
    v.innerHTML = head("الفيديوهات", "فيديوهات المحاضرات والتسجيلات الصوتية من المودل، تُحفظ في مجلد «فيديوهات المحاضرات» داخل كل مادة",
      p.running || items.some((i) => i.status === "queued")
        ? `<button class="btn" id="v-stop">${ic("stop")}إيقاف التنزيل</button>`
        : waiting ? `<button class="btn primary" id="v-all">${ic("download")}تنزيل الكل (${waiting})</button>` : "") + `
    <div class="stats vid-stats">
      <div class="stat"><div class="ic">${ic("video", "lg")}</div><div><b>${items.filter((i) => i.status !== "hidden").length}</b><span>فيديو في موادك</span></div></div>
      <div class="stat"><div class="ic green">${ic("check", "lg")}</div><div><b>${done.length}</b><span>على جهازك${total ? ` · <span dir="ltr">${esc(size(total))}</span>` : ""}</span></div></div>
      <div class="stat"><div class="ic blue">${ic("download", "lg")}</div><div><b>${waiting}</b><span>لم يُنزّل بعد</span></div></div>
    </div>
    <div class="card vid-mode">
      <div><b>طريقة الجلب</b><p class="hint" style="margin:2px 0 0">${mode === "auto" ? "كل فيديو جديد ينزل تلقائياً بعد المحاضرات والملفات" : mode === "manual" ? "يظهر الفيديو الجديد هنا، وتنزّله أنت عند الحاجة" : "لا يبحث البرنامج عن الفيديوهات"}</p></div>
      <div class="seg" id="v-mode">${Object.entries(vidModes).map(([k, t]) => `<button data-m="${k}" class="${mode === k ? "on" : ""}">${t}</button>`).join("")}</div>
    </div>
    <div class="row wrap" style="margin:18px 0 14px"><div class="seg" id="v-filter">${[["all", "الكل"], ["todo", "غير منزّلة"], ["done", "على جهازي"], ["hidden", "متجاهلة"]].map(([k, t]) => `<button data-f="${k}" class="${vidFilter === k ? "on" : ""}">${t}</button>`).join("")}</div></div>
    ${Object.entries(groups).map(([course, list]) => `<div class="card" style="margin-bottom:16px">${cardHead("book", esc(course), `<span class="pill gray">${list.length}</span>`)}<div class="list">${list.map(row).join("")}</div></div>`).join("")
      || `<div class="card">${empty("video", mode === "off" ? "جلب الفيديوهات متوقف. اختر «تلقائي» أو «يدوي»" : items.length ? "لا شيء هنا" : "لم يُعثر على فيديوهات بعد. ستظهر هنا بعد المزامنة القادمة")}</div>`}`;
    $("#v-all")?.addEventListener("click", async () => { await api("/api/videos/get", {}); toast("بدأ تنزيل الفيديوهات", "download"); tick(); });
    $("#v-stop")?.addEventListener("click", async () => { await api("/api/videos/stop", {}); toast("أُوقف التنزيل. سيُستكمل من حيث توقف عند البدء من جديد", "stop", 5000); tick(); });
    $$("#v-mode button").forEach((b) => b.addEventListener("click", async () => {
      await api("/api/settings", { videos: b.dataset.m });
      if (b.dataset.m !== "off") api("/api/sync", {}).catch(() => {});
      await refresh(); tick();
    }));
    $$("#v-filter button").forEach((b) => b.addEventListener("click", () => { vidFilter = b.dataset.f; draw(); }));
    $$("[data-vget]", v).forEach((b) => b.addEventListener("click", async (e) => { e.stopPropagation(); await api("/api/videos/get", { keys: [b.dataset.vget] }); tick(); }));
    $$("[data-vhide]", v).forEach((b) => b.addEventListener("click", async (e) => { e.stopPropagation(); await api("/api/videos/hide", { keys: [b.dataset.vhide], hide: true }); tick(); }));
    $$("[data-vshow]", v).forEach((b) => b.addEventListener("click", async (e) => { e.stopPropagation(); await api("/api/videos/hide", { keys: [b.dataset.vshow], hide: false }); tick(); }));
  };
  // a full redraw only when the list changes; while downloading, just the active row
  let sig = "", drawRow = null;
  const sigOf = () => JSON.stringify([VID.mode, VID.items, !!VID.progress?.running]);
  const tick = async () => {
    if (current !== "videos") { clearInterval(vidTimer); return; }
    try { await loadVideos(); } catch (e) { return; }
    const s = sigOf();
    if (s !== sig) { sig = s; draw(); return; }
    const p = VID.progress || {};
    const el = p.running && [...$$(".vid-row", v)].find((r) => r.dataset.key === p.key);
    const it = el && VID.items.find((i) => i.key === p.key);
    if (it && drawRow) el.outerHTML = drawRow(it);
  };
  if (VID) { sig = sigOf(); draw(); }
  tick();
  vidTimer = setInterval(tick, 1000);
};

/* ================= assignments & notifications ================= */
views.assignments = () => {
  const list = [...(S.assignments || [])];
  const now = S.now;
  const open = list.filter((a) => a.status !== "submitted" && (a.due === 0 || a.due > now)).sort((a, b) => (a.due || 9e12) - (b.due || 9e12));
  const rest = list.filter((a) => !open.includes(a)).sort((a, b) => b.due - a.due);
  const row = (a) => {
    const u = a.due ? until(a.due) : { text: "بدون موعد", cls: "gray" };
    const d = new Date((a.due || now) * 1000);
    return `<div class="item">
      <div class="when ${a.status === "submitted" ? "" : u.cls}"><b>${a.due ? fmtDay.format(d) : "—"}</b><span>${a.due ? fmtMon.format(d) : ""}</span></div>
      <div class="body"><div class="title">${esc(a.name)}</div>
        <div class="meta">${a.status !== "submitted" && a.due > now ? `<span class="pill ${u.cls}">${ic("clock")}${u.text}</span>` : ""}${esc(a.course)}${SEP}${esc(dateTime(a.due))}</div>
        ${a.intro ? `<div class="snip">${esc(a.intro.slice(0, 200))}</div>` : ""}</div>
      ${statusPill(a)}
      <div class="actions"><button class="btn sm ghost" title="مجلد الحل — ضع حلّك هنا" data-reveal="${esc(a.folder + sep() + "حلّي")}">${ic("folder")}</button>
      <button class="btn sm" data-url="${esc(a.url)}">${ic("external")}تسليم</button></div></div>`;
  };
  return head("الواجبات", "لكل واجب مجلد خاص فيه ملفاته وتفاصيله، ومجلد «حلّي» لتضع فيه حلّك حتى لا يضيع") + `
    <div class="card" style="margin-bottom:20px">${cardHead("clock", `مطلوبة الآن <span class="pill red">${open.length}</span>`)}<div class="list">${open.map(row).join("") || empty("check-circle", "لا توجد واجبات مطلوبة")}</div></div>
    <div class="card">${cardHead("archive", `المُسلّمة والسابقة <span class="pill gray">${rest.length}</span>`)}<div class="list">${rest.map(row).join("") || empty("archive", "—")}</div></div>`;
};

views.notifications = () => head("الإشعارات", "إشعارات المودل تصل هنا تلقائياً") +
  `<div class="card"><div class="list">${(S.notifications || []).map(notifRow).join("") || empty("bell", "لا توجد إشعارات")}</div></div>`;

/* ================= search ================= */
let sFilter = { type: "", course: "" };
let sSel = 0, sTimer = null, sSeq = 0;

views.search = (v, fresh) => {
  if (!fresh && $("#results", v)) return;
  const types = [["", "الكل", "layers"], ["pdf", "PDF", "file"], ["slides", "عروض", "monitor"], ["word", "مستندات", "file"], ["sheets", "جداول", "layers"], ["code", "برمجة", "keyboard"]];
  v.innerHTML = head("البحث", "يبحث في أسماء الملفات وداخل محتواها، بالعربي والإنجليزي") + `
    <div class="filters"><div class="seg">${types.map(([k, t, i]) => `<button class="${sFilter.type === k ? "on" : ""}" data-type="${k}">${ic(i)}${t}</button>`).join("")}</div>
      <select id="s-course" class="in"><option value="">كل المواد</option></select><span class="spacer"></span><span id="s-info" class="res-info"></span></div>
    <div class="card"><div id="results" class="list">${empty("search", "اكتب في مربع البحث: اسم ملف، أو كلمة داخل المحاضرة<br><span class='small desk-only'>الأسهم للتنقل · Enter للفتح · Ctrl+Enter لإظهار المجلد</span>")}</div></div>`;
  $$("[data-type]", v).forEach((b) => b.addEventListener("click", () => {
    sFilter.type = b.dataset.type;
    $$("[data-type]", v).forEach((x) => x.classList.toggle("on", x === b));
    doSearch();
  }));
  api("/api/courses-index").then((cs) => {
    const sel = $("#s-course", v);
    if (!sel) return;
    sel.innerHTML += cs.map((c) => `<option ${c === sFilter.course ? "selected" : ""}>${esc(c)}</option>`).join("");
    sel.addEventListener("change", () => { sFilter.course = sel.value; doSearch(); });
  });
  if ($("#q").value.trim()) doSearch();
};

async function doSearch() {
  const q = $("#q").value.trim();
  const box = $("#results");
  if (!box) return;
  if (!q) { box.innerHTML = empty("search", "اكتب كلمة للبحث"); $("#s-info").textContent = ""; return; }
  const seq = ++sSeq;
  let d;
  try { d = await api(`/api/search?q=${encodeURIComponent(q)}&type=${sFilter.type}&course=${encodeURIComponent(sFilter.course)}`); }
  catch (e) { box.innerHTML = empty("alert", esc(e.message)); return; }
  if (seq !== sSeq) return;
  $("#s-info").textContent = `${d.total} نتيجة · ${d.ms} ملي ثانية`;
  sSel = 0;
  const words = q.split(/\s+/).filter(Boolean);
  box.innerHTML = (d.hits || []).map((h, i) => {
    const snip = h.snippet ? `<div class="snip">${esc(h.snippet).replace(/\x01/g, "<mark>").replace(/\x02/g, "</mark>")}</div>` : "";
    return fileRow(h, { showCourse: true, nameHtml: highlight(h.name, words), extra: snip }).replace('class="item', `class="item${i === 0 ? " sel" : ""}`);
  }).join("") || empty("search", `لا توجد نتائج لـ «${esc(q)}»<br><span class="small">جرّب كلمة أقصر، أو أضف مجلداتك القديمة من الإعدادات</span>`);
}

function highlight(name, words) {
  let out = esc(name);
  for (const w of words) {
    if (w.length < 2) continue;
    out = out.replace(new RegExp(esc(w).replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "gi"), (m) => `<mark>${m}</mark>`);
  }
  return out;
}

$("#q").addEventListener("input", () => {
  if (current !== "search") go("search");
  clearTimeout(sTimer);
  sTimer = setTimeout(doSearch, 110);
});
$("#q").addEventListener("keydown", (e) => {
  const hits = $$("#results .hit");
  if (e.key === "ArrowDown" || e.key === "ArrowUp") {
    e.preventDefault();
    if (!hits.length) return;
    sSel = (sSel + (e.key === "ArrowDown" ? 1 : -1) + hits.length) % hits.length;
    hits.forEach((h, i) => h.classList.toggle("sel", i === sSel));
    hits[sSel].scrollIntoView({ block: "nearest" });
  } else if (e.key === "Enter" && hits[sSel]) {
    openFile(hits[sSel].dataset.open, e.ctrlKey);
  } else if (e.key === "Escape") {
    $("#q").value = "";
    doSearch();
  }
});
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape" && !$("#modal").classList.contains("hide")) { closeModal(); e.stopImmediatePropagation(); return; }
  if ((e.key === "/" || (e.ctrlKey && e.key.toLowerCase() === "k")) && !["INPUT", "TEXTAREA", "SELECT"].includes(document.activeElement.tagName)) {
    e.preventDefault();
    $("#q").focus();
    $("#q").select();
  }
});

/* ================= setup ================= */
views.setup = (v) => {
  v.innerHTML = `<div class="setup"><div class="card" style="padding:30px 32px">
    <div class="hero"><img src="icon.svg" alt=""><div><div class="eyebrow">أهلاً بك</div><h1>لنربط حسابك في المودل</h1></div></div>
    <p class="muted">مرة واحدة فقط، وبعدها سيقوم البرنامج تلقائياً بتنزيل المحاضرات والواجبات وترتيبها، وعرض الإشعارات والمواعيد، والبحث داخل كل الملفات.</p>
    <label class="f">رابط موقع المودل في جامعتك</label>
    <input id="u-url" class="in" dir="ltr" placeholder="https://moodle.university.edu" value="${esc(S.moodleUrl || "")}">
    <div class="hint">انسخه من شريط العنوان في المتصفح وأنت داخل المودل (يكفي أول جزء منه).</div>
    <div id="pw-box">
      <label class="f">اسم المستخدم (الرقم الجامعي)</label>
      <input id="u-user" class="in" dir="ltr" autocomplete="username">
      <label class="f">كلمة المرور</label>
      <input id="u-pass" class="in" type="password" dir="ltr" autocomplete="current-password">
      <div class="hint row">${ic("lock")}<span>كلمة المرور لا تُحفظ أبداً. تُستخدم مرة واحدة للحصول على مفتاح دخول مشفّر على جهازك.</span></div>
    </div>
    <div id="tok-box" class="hide">
      <label class="f">مفتاح الدخول (Token)</label>
      <input id="u-token" class="in" dir="ltr">
      <ol class="steps"><li>افتح المودل في المتصفح وسجّل دخولك.</li><li>الملف الشخصي ← التفضيلات ← <b>مفاتيح الأمان</b> (Security keys).</li>
        <li>انسخ المفتاح بجانب <code>Moodle mobile web service</code> والصقه هنا.</li></ol>
    </div>
    <p><a id="toggle-tok" class="small">جامعتي تستخدم الدخول عبر Microsoft أو Google</a></p>
    <div class="desk-only"><label class="f">أين تُحفظ ملفات الجامعة المرتبة؟</label>
    <div class="row"><input id="u-root" class="in grow" dir="ltr" value="${esc(S.rootDir)}">${S.isWindows ? `<button class="btn" id="pick-root">${ic("folder")}اختيار</button>` : ""}</div></div>
    <div class="err" id="u-err" style="margin-top:12px"></div>
    <div class="row" style="margin-top:14px"><button class="btn primary" id="u-go">${ic("link")}ربط الحساب وبدء الترتيب</button>
      ${S.linked ? `<button class="btn ghost" data-go="home">إلغاء</button>` : ""}</div>
  </div></div>`;
  let tokMode = false;
  $("#toggle-tok").addEventListener("click", () => {
    tokMode = !tokMode;
    $("#pw-box").classList.toggle("hide", tokMode);
    $("#tok-box").classList.toggle("hide", !tokMode);
    $("#toggle-tok").textContent = tokMode ? "الدخول باسم المستخدم وكلمة المرور" : "جامعتي تستخدم الدخول عبر Microsoft أو Google";
  });
  $("#pick-root")?.addEventListener("click", async () => { const r = await api("/api/pickfolder", {}); if (r.path) $("#u-root").value = r.path; });
  $("#u-go").addEventListener("click", async () => {
    const btn = $("#u-go"), err = $("#u-err");
    err.textContent = "";
    btn.disabled = true;
    btn.innerHTML = ic("refresh", "spin") + "جارٍ الاتصال…";
    try {
      await api("/api/settings", { rootDir: $("#u-root").value });
      const r = await api("/api/login", {
        url: $("#u-url").value, username: tokMode ? "" : $("#u-user").value,
        password: tokMode ? "" : $("#u-pass").value, token: tokMode ? $("#u-token").value : "",
      });
      toast(`تم الربط بنجاح يا ${r.fullName}. بدأ تنزيل المحاضرات`);
      await refresh();
      go("home");
    } catch (e) {
      err.textContent = e.message;
    } finally {
      btn.disabled = false;
      btn.innerHTML = ic("link") + "ربط الحساب وبدء الترتيب";
    }
  });
};

/* ================= feedback ================= */
const KINDS = { "خطأ": ["bug", "red"], "اقتراح": ["bulb", "blue"], "تحسين": ["wand", "gold"], "سؤال": ["help", "violet"] };
const kindIc = (k) => { const [i, c] = KINDS[k] || ["message", "gray"]; return `<div class="kind-ic" style="background:var(--${c}-soft);color:var(--${c === "gray" ? "muted" : c === "gold" ? "gold-ink" : c})">${ic(i)}</div>`; };
let fbData = null, fbFilter = "open", fbOpen = null;

views.feedback = (v) => {
  v.innerHTML = head("الملاحظات الفنية", "قناة بينك وبين صديقك لإرسال الأخطاء والاقتراحات، تُحفظ في GitHub وتصل للطرف الآخر داخل البرنامج") + empty("refresh", "جارٍ التحميل…");
  loadFeedback(v);
};

async function loadFeedback(v) {
  try { fbData = await api("/api/feedback"); } catch (e) { v.innerHTML = empty("alert", esc(e.message)); return; }
  const d = fbData;
  d.outbox = d.outbox || []; d.threads = d.threads || []; d.unread = d.unread || {};
  if (!d.hasToken) { v.innerHTML = head("الملاحظات الفنية", "قناة بينك وبين صديقك لإرسال الأخطاء والاقتراحات") + fbConnectCard(d); bindConnect(v); return; }
  const threads = d.threads.filter((t) => fbFilter === "all" || t.state === fbFilter);
  const openCount = d.threads.filter((t) => t.state === "open").length;
  v.innerHTML = head("الملاحظات الفنية", `متصل كـ <b dir="ltr">@${esc(d.login)}</b> · المستودع <a data-url="https://github.com/${esc(d.repo)}" dir="ltr">${esc(d.repo)}</a>`,
    `<button class="btn" id="fb-copy" title="انسخ الملاحظات المفتوحة لإرسالها إلى Claude">${ic("copy")}نسخ للإصلاح</button><button class="btn" id="fb-refresh">${ic("refresh")}تحديث</button>`) + `
  <div class="grid split">
    <div class="card">${cardHead("send", "ملاحظة جديدة")}
      <div class="seg" id="fb-kind">${Object.keys(KINDS).map((k, i) => `<button class="${i === 0 ? "on" : ""}" data-k="${k}">${ic(KINDS[k][0])}${k}</button>`).join("")}</div>
      <label class="f">العنوان</label><input id="fb-title" class="in" maxlength="120" placeholder="مثال: البحث لا يجد ملفات PowerPoint">
      <label class="f">التفاصيل</label><textarea id="fb-body" class="in" placeholder="ماذا حدث؟ ماذا كنت تتوقع؟ وما الخطوات التي أدت لذلك؟"></textarea>
      <label class="f">الأولوية</label>
      <div class="seg" id="fb-pri"><button data-p="عادية" class="on">عادية</button><button data-p="مهمة">مهمة</button><button data-p="عاجلة">عاجلة</button></div>
      <label class="toggle"><span class="t"><b>إرفاق معلومات تقنية</b><span>الإصدار والنظام وآخر سطور السجل (بدون كلمات مرور)</span></span>
        <span class="switch"><input type="checkbox" id="fb-diag" checked><i></i></span></label>
      <div class="row" style="margin-top:8px"><button class="btn primary" id="fb-send">${ic("send")}إرسال</button><span class="err" id="fb-err"></span></div>
      <div class="hint">بدون إنترنت؟ لا مشكلة، تُحفظ الملاحظة وتُرسل تلقائياً عند الاتصال.</div>
    </div>
    <div>
      ${d.outbox.length ? `<div class="card" style="margin-bottom:16px">${cardHead("wifi-off", "بانتظار الإرسال", `<button class="btn sm" id="fb-retry">${ic("refresh")}إعادة المحاولة</button>`)}<div class="list">
        ${d.outbox.map((o) => `<div class="item">${kindIc(o.kind)}<div class="body"><div class="title">${esc(o.title)}</div>
          <div class="meta">${o.status === "error" ? `<span class="pill red">${esc(o.error)}</span>` : `<span class="pill amber">${ic("clock")}سيُرسل عند الاتصال</span>`}${SEP}${esc(ago(o.created))}</div></div>
          <div class="actions"><button class="btn sm ghost" data-discard="${esc(o.localId)}" title="حذف">${ic("trash")}</button></div></div>`).join("")}</div></div>` : ""}
      <div class="row" style="margin-bottom:12px"><div class="seg" id="fb-filter">
        <button data-f="open" class="${fbFilter === "open" ? "on" : ""}">قيد المتابعة <span class="muted">${openCount}</span></button>
        <button data-f="closed" class="${fbFilter === "closed" ? "on" : ""}">تم حلها <span class="muted">${d.threads.length - openCount}</span></button>
        <button data-f="all" class="${fbFilter === "all" ? "on" : ""}">الكل</button></div><span class="spacer"></span>
        <span class="res-info">${d.lastError ? esc(d.lastError) : d.lastFetch ? "آخر تحديث " + ago(d.lastFetch) : ""}</span></div>
      <div id="fb-threads">${threads.map((t) => threadCard(t, d)).join("") || `<div class="card">${empty("message", fbFilter === "open" ? "لا توجد ملاحظات مفتوحة" : "لا توجد ملاحظات هنا")}</div>`}</div>
    </div>
  </div>`;
  bindFeedback(v);
  if (fbOpen) openThread(fbOpen);
}

function avatar(login, url) {
  return `<div class="avatar">${url ? `<img src="${esc(url)}&s=68" onerror="this.remove()" alt="">` : ""}<span>${esc((login || "?").slice(0, 1).toUpperCase())}</span></div>`;
}

function threadCard(t, d) {
  const unread = d.unread && d.unread[t.number];
  return `<div class="thread ${unread ? "unread" : ""}" id="th-${t.number}">
    <div class="thread-head" data-th="${t.number}">${kindIc(t.kind)}
      <div class="body" style="flex:1;min-width:0"><div class="title">${unread ? '<span class="newdot"></span>' : ""}${esc(t.title)}</div>
        <div class="meta"><span dir="ltr">#${t.number}</span>${SEP}<span dir="ltr">@${esc(t.author)}</span>${SEP}${esc(ago(t.updated))}${t.comments ? SEP + ic("reply") + t.comments : ""}</div></div>
      ${t.state === "open" ? `<span class="pill blue">قيد المتابعة</span>` : `<span class="pill green">${ic("check")}تم الحل</span>`}
    </div><div class="convo hide" id="cv-${t.number}"></div></div>`;
}

async function openThread(n) {
  const box = $("#cv-" + n);
  if (!box) return;
  fbOpen = n;
  const t = fbData.threads.find((x) => x.number === n);
  box.classList.remove("hide");
  box.innerHTML = empty("refresh", "جارٍ تحميل المحادثة…");
  let cs = [];
  try { cs = (await api("/api/feedback/thread?n=" + n)).comments || []; fbData = await api("/api/feedback"); fbData.threads = fbData.threads || []; } catch (e) { box.innerHTML = empty("alert", esc(e.message)); return; }
  $("#th-" + n)?.classList.remove("unread");
  const me = fbData.login;
  const bubble = (who, av, when, body) => `<div class="bubble ${who === me ? "me" : ""}">${avatar(who, av)}<div class="bb"><div class="who" dir="auto">@${esc(who)} · ${esc(ago(when))}</div>${md(body)}</div></div>`;
  box.innerHTML = bubble(t.author, t.avatar, t.created, t.body) + cs.map((c) => bubble(c.author, c.avatar, c.created, c.body)).join("") + `
    <textarea class="in" id="rp-${n}" style="min-height:80px" placeholder="اكتب رداً…"></textarea>
    <div class="row wrap" style="margin-top:10px"><button class="btn primary sm" data-reply="${n}">${ic("reply")}إرسال الرد</button>
      ${t.state === "open" ? `<button class="btn sm" data-state="closed" data-n="${n}">${ic("check")}تم الحل</button>` : `<button class="btn sm" data-state="open" data-n="${n}">${ic("refresh")}إعادة فتح</button>`}
      <span class="spacer"></span><button class="btn sm ghost" data-url="${esc(t.url)}">${ic("github")}فتح في GitHub</button></div>`;
  refresh();
}

function bindFeedback(v) {
  let kind = "خطأ", pri = "عادية";
  $$("#fb-kind button", v).forEach((b) => b.addEventListener("click", () => { kind = b.dataset.k; $$("#fb-kind button", v).forEach((x) => x.classList.toggle("on", x === b)); }));
  $$("#fb-pri button", v).forEach((b) => b.addEventListener("click", () => { pri = b.dataset.p; $$("#fb-pri button", v).forEach((x) => x.classList.toggle("on", x === b)); }));
  $$("#fb-filter button", v).forEach((b) => b.addEventListener("click", () => { fbFilter = b.dataset.f; loadFeedback(v); }));
  $("#fb-send").addEventListener("click", async () => {
    $("#fb-err").textContent = "";
    try {
      await api("/api/feedback/send", { kind, priority: pri, title: $("#fb-title").value, body: $("#fb-body").value, diag: $("#fb-diag").checked });
      toast("تم حفظ الملاحظة وجارٍ إرسالها", "send");
      setTimeout(() => loadFeedback(v), 2500);
      $("#fb-title").value = ""; $("#fb-body").value = "";
    } catch (e) { $("#fb-err").textContent = e.message; }
  });
  $("#fb-refresh").addEventListener("click", async () => {
    try { await api("/api/feedback/refresh", {}); toast("تم التحديث"); } catch (e) { toast(e.message, "alert"); }
    loadFeedback(v);
  });
  $("#fb-retry")?.addEventListener("click", async () => { await api("/api/feedback/retry", {}); loadFeedback(v); });
  $("#fb-copy").addEventListener("click", async () => {
    const r = await api("/api/feedback/export");
    try { await navigator.clipboard.writeText(r.text); toast("تم النسخ. الصقه في محادثة Claude ليقوم بالإصلاحات", "copy", 5000); }
    catch (e) { modal(`<h2>انسخ النص التالي</h2><textarea class="in" style="min-height:300px;margin-top:12px">${esc(r.text)}</textarea><div class="row" style="margin-top:12px"><span class="spacer"></span><button class="btn" data-close>إغلاق</button></div>`); }
  });
  v.onclick = async (e) => {
    const th = e.target.closest("[data-th]");
    if (th) {
      const n = +th.dataset.th, box = $("#cv-" + n);
      if (!box.classList.contains("hide")) { box.classList.add("hide"); fbOpen = null; } else openThread(n);
      return;
    }
    const rp = e.target.closest("[data-reply]");
    if (rp) {
      const n = +rp.dataset.reply;
      try { await api("/api/feedback/reply", { n, body: $("#rp-" + n).value }); toast("تم إرسال الرد", "reply"); fbData = await api("/api/feedback"); openThread(n); }
      catch (err) { toast(err.message, "alert"); }
      return;
    }
    const stb = e.target.closest("[data-state]");
    if (stb) {
      try { await api("/api/feedback/state", { n: +stb.dataset.n, state: stb.dataset.state }); toast(stb.dataset.state === "closed" ? "تم وضع علامة «تم الحل»" : "أُعيد فتح الملاحظة"); loadFeedback(v); }
      catch (err) { toast(err.message, "alert"); }
      return;
    }
    const ds = e.target.closest("[data-discard]");
    if (ds) { await api("/api/feedback/discard", { localId: ds.dataset.discard }); loadFeedback(v); }
  };
}

function fbConnectCard(d) {
  return `<div class="grid g2" style="align-items:start"><div class="card">${cardHead("github", "ربط حساب GitHub")}
    <p class="muted small">يتم تخزين الملاحظات في قسم Issues في مستودع المشروع، فتصل إليك وإلى صديقك أينما كنتما. هذا الإعداد يتم مرة واحدة.</p>
    <ol class="steps">
      <li>سجّل الدخول إلى GitHub (أو أنشئ حساباً مجانياً).</li>
      <li><a data-url="https://github.com/settings/tokens/new?scopes=public_repo&description=UniHub%20feedback">اضغط هنا لإنشاء مفتاح</a>، ثم انزل للأسفل واضغط <b>Generate token</b>.</li>
      <li>انسخ المفتاح (يبدأ بـ <code dir="ltr">ghp_</code>) والصقه هنا.</li>
    </ol>
    <label class="f">مفتاح GitHub</label><input id="gh-token" class="in" dir="ltr" placeholder="ghp_…">
    <label class="f">المستودع</label><input id="gh-repo" class="in" dir="ltr" placeholder="username/unihub" value="${esc(d.repo || "")}">
    <div class="hint row">${ic("shield")}<span>المفتاح يُشفَّر على جهازك ولا يُرسل إلا إلى GitHub.</span></div>
    <div class="row" style="margin-top:14px"><button class="btn primary" id="gh-go">${ic("link")}ربط</button><span class="err" id="gh-err"></span></div>
  </div>
  <div class="card">${cardHead("info", "كيف تعمل؟")}
    <ul class="steps">
      <li>صديقك يكتب ملاحظة (خطأ، اقتراح، تحسين) من هذه الصفحة.</li>
      <li>تصلك داخل البرنامج مع تنبيه ويندوز، ويمكنكما التحاور حولها.</li>
      <li>اضغط «نسخ للإصلاح» وأرسلها إلى Claude، أو اطلب منه قراءتها مباشرة من GitHub.</li>
      <li>بعد نشر الإصلاح كإصدار جديد، يتحدث البرنامج عند كليكما تلقائياً.</li>
    </ul></div></div>`;
}

function bindConnect(v) {
  $("#gh-go").addEventListener("click", async () => {
    $("#gh-err").textContent = "";
    $("#gh-go").disabled = true;
    try {
      const r = await api("/api/feedback/connect", { token: $("#gh-token").value, repo: $("#gh-repo").value });
      toast(`تم الربط كـ @${r.login}`, "github");
      views.feedback(v);
    } catch (e) { $("#gh-err").textContent = e.message; }
    $("#gh-go").disabled = false;
  });
}


/* ================= links ================= */
const LINK_KINDS = {
  portal: ["idcard", "gold", "بوابة الطالب"],
  site: ["globe", "green", "موقع"],
  social: ["users", "blue", "صفحة تواصل"],
  mail: ["mail", "violet", "بريد"],
  other: ["link", "gray", "رابط آخر"],
};
function linkIc(kind, size = "") {
  const [i, c] = LINK_KINDS[kind] || LINK_KINDS.other;
  const fg = c === "gray" ? "var(--muted)" : c === "gold" ? "var(--gold-ink)" : `var(--${c})`;
  const bg = c === "gray" ? "var(--surface-2)" : `var(--${c}-soft)`;
  return `<div class="kind-ic" style="background:${bg};color:${fg}">${ic(i, size)}</div>`;
}
const hostOf = (u) => { try { return new URL(u).hostname.replace(/^www\./, ""); } catch (e) { return u; } };
const qlink = (l) => `<a class="qlink" data-url="${esc(l.url)}" title="${esc(l.url)}">${linkIc(l.kind)}<div><b dir="auto">${esc(l.title)}</b><small>${esc(hostOf(l.url))}</small></div></a>`;

function guessKind(raw) {
  let u;
  try { u = new URL(/:\/\//.test(raw) ? raw : "https://" + raw); } catch (e) { return "other"; }
  const h = u.hostname.toLowerCase(), p = u.pathname.toLowerCase();
  if (/(facebook|fb|instagram|twitter|youtube|linkedin|tiktok|telegram)\.|^x\.com$|^t\.me$/.test(h)) return "social";
  if (/^(mail|outlook)\./.test(h) || p.includes("/mail")) return "mail";
  if (h.includes("portal") || /portal|student|login/.test(p) || /^(sis|my)\./.test(h)) return "portal";
  return "site";
}

let navLinksKey = null;
function renderNavLinks() {
  const links = S.links || [];
  const key = JSON.stringify(links);
  if (key === navLinksKey) return;
  navLinksKey = key;
  $("#nav-links").innerHTML = links.map((l) => {
    const [i] = LINK_KINDS[l.kind] || LINK_KINDS.other;
    return `<a class="ext-link" data-url="${esc(l.url)}" title="${esc(l.url)}">${ic(i)}<span dir="auto">${esc(l.title)}</span><svg class="icon go"><use href="#i-external"/></svg></a>`;
  }).join("") || `<a class="muted-link" data-go="links">${ic("plus")}<span>أضف روابط جامعتك</span></a>`;
}

// Suggests the university's main website from the Moodle address (moodle.uni.edu → uni.edu).
function linkSuggestions(have) {
  const out = [];
  const known = new Set(have.map((l) => hostOf(l.url) + new URL(l.url).pathname.replace(/\/$/, "")));
  const add = (title, url, kind) => { try { const u = new URL(url); if (!known.has(hostOf(url) + u.pathname.replace(/\/$/, ""))) out.push({ title, url, kind }); } catch (e) {} };
  if (S.moodleUrl) {
    try {
      const h = new URL(S.moodleUrl).hostname;
      const parts = h.split(".");
      if (parts.length > 2 && /^(moodle|lms|elearning|e-learning|learn|el|online|courses|vle|cms|eclass|portal)\d*$/i.test(parts[0])) {
        add("موقع الجامعة", "https://" + parts.slice(1).join("."), "site");
      }
    } catch (e) {}
  }
  return out;
}

views.links = (v) => {
  let links = (S.links || []).map((l) => ({ ...l }));
  let editing = -1;
  const draw = () => {
    const sugg = linkSuggestions(links);
    v.innerHTML = head("روابطي", "روابطك الخاصة بجامعتك: بوابة الطالب، صفحات الجامعة، والمواقع التي تزورها كثيراً. تظهر في الشريط الجانبي وفي الرئيسية، وهي خاصة بجهازك فقط.") + `
    <div class="grid split">
      <div class="card">${cardHead(editing >= 0 ? "pencil" : "plus", editing >= 0 ? "تعديل الرابط" : "إضافة رابط")}
        <label class="f" style="margin-top:0">الرابط</label>
        <input id="l-url" class="in" dir="ltr" placeholder="https://..." value="${editing >= 0 ? esc(links[editing].url) : ""}">
        <label class="f">الاسم الظاهر</label>
        <input id="l-title" class="in" maxlength="60" placeholder="مثال: بوابة الطالب" value="${editing >= 0 ? esc(links[editing].title) : ""}">
        <label class="f">النوع</label>
        <div class="seg wrap" id="l-kind">${Object.entries(LINK_KINDS).map(([k, [i, , t]]) => `<button data-k="${k}">${ic(i)}${t}</button>`).join("")}</div>
        <div class="row" style="margin-top:16px"><button class="btn primary" id="l-save">${ic(editing >= 0 ? "check" : "plus")}${editing >= 0 ? "حفظ التعديل" : "إضافة"}</button>
          ${editing >= 0 ? `<button class="btn ghost" id="l-cancel">إلغاء</button>` : ""}<span class="err" id="l-err"></span></div>
        <div class="hint row" style="margin-top:14px;align-items:flex-start">${ic("lock")}<span>الصفحات التي تحتاج تسجيل دخول (مثل بوابة الطالب) تُفتح في متصفحك، ويمكنك جعل المتصفح يحفظ كلمة المرور. البرنامج لا يطلب كلمات المرور هذه ولا يخزنها.</span></div>
      </div>
      <div>
        <div class="card">${cardHead("link", `روابطك <span class="pill gray">${links.length}</span>`)}
          <div class="list">${links.map((l, i) => `<div class="item">${linkIc(l.kind)}
            <div class="body"><div class="title" dir="auto">${esc(l.title)}</div><div class="meta"><span dir="ltr">${esc(l.url)}</span></div></div>
            <div class="actions">
              <button class="btn sm ghost sq" data-mv="${i}" data-d="-1" title="لأعلى" ${i === 0 ? "disabled" : ""}>${ic("arrow-up")}</button>
              <button class="btn sm ghost sq" data-mv="${i}" data-d="1" title="لأسفل" ${i === links.length - 1 ? "disabled" : ""}>${ic("arrow-down")}</button>
              <button class="btn sm ghost sq" data-ed="${i}" title="تعديل">${ic("pencil")}</button>
              <button class="btn sm ghost sq" data-del="${i}" title="حذف">${ic("trash")}</button>
              <button class="btn sm" data-url="${esc(l.url)}">${ic("external")}فتح</button>
            </div></div>`).join("") || empty("link", "لا توجد روابط بعد. أضف أول رابط من النموذج")}</div></div>
        ${sugg.length ? `<div class="card" style="margin-top:16px">${cardHead("sparkles", "اقتراحات من رابط المودل")}<div class="list">
          ${sugg.map((l, i) => `<div class="item">${linkIc(l.kind)}<div class="body"><div class="title">${esc(l.title)}</div><div class="meta"><span dir="ltr">${esc(l.url)}</span></div></div>
            <div class="actions"><button class="btn sm" data-sg="${i}">${ic("plus")}إضافة</button></div></div>`).join("")}</div>
          <p class="hint">تأكد أن الرابط المقترح يفتح موقع جامعتك قبل إضافته.</p></div>` : ""}
      </div>
    </div>`;

    let kind = editing >= 0 ? links[editing].kind : "site";
    let kindTouched = editing >= 0;
    const setKind = (k) => { kind = k; $$("#l-kind button", v).forEach((b) => b.classList.toggle("on", b.dataset.k === k)); };
    setKind(kind);
    $$("#l-kind button", v).forEach((b) => b.addEventListener("click", () => { kindTouched = true; setKind(b.dataset.k); }));
    $("#l-url", v).addEventListener("input", (e) => { if (!kindTouched) setKind(guessKind(e.target.value.trim())); });
    $("#l-cancel", v)?.addEventListener("click", () => { editing = -1; draw(); });
    $("#l-save", v).addEventListener("click", async () => {
      const url = $("#l-url", v).value.trim();
      if (!url) { $("#l-err", v).textContent = "اكتب الرابط"; return; }
      const item = { url, title: $("#l-title", v).value.trim(), kind };
      const next = links.map((l) => ({ ...l }));
      if (editing >= 0) next[editing] = item; else next.push(item);
      if (await save(next)) { editing = -1; draw(); toast("تم حفظ الرابط", "link"); }
      else $("#l-err", v).textContent = save.err;
    });
    v.onclick = async (e) => {
      const b = e.target.closest("[data-mv],[data-ed],[data-del],[data-sg]");
      if (!b || b.disabled) return;
      const next = links.map((l) => ({ ...l }));
      if (b.dataset.mv !== undefined) {
        const i = +b.dataset.mv, j = i + +b.dataset.d;
        [next[i], next[j]] = [next[j], next[i]];
      } else if (b.dataset.ed !== undefined) { editing = +b.dataset.ed; draw(); return; }
      else if (b.dataset.del !== undefined) {
        if (!confirm(`حذف «${links[+b.dataset.del].title}»؟`)) return;
        next.splice(+b.dataset.del, 1);
        if (editing === +b.dataset.del) editing = -1;
      } else if (b.dataset.sg !== undefined) next.push(sugg[+b.dataset.sg]);
      if (await save(next)) draw(); else toast(save.err, "alert");
    };
  };
  const save = async (next) => {
    try {
      await api("/api/settings", { links: next });
      await refresh();
      links = (S.links || []).map((l) => ({ ...l }));
      return true;
    } catch (e) { save.err = e.message; return false; }
  };
  draw();
};

/* ================= settings ================= */
const toggle = (id, title, sub, on) => `<label class="toggle"><span class="t"><b>${title}</b><span>${sub}</span></span><span class="switch"><input type="checkbox" id="${id}" ${on ? "checked" : ""}><i></i></span></label>`;

views.settings = (v) => {
  const extra = [...(S.extraDirs || [])];
  const hidden = new Set(S.hidden || []);
  const u = S.update;
  const theme = localStorage.getItem("uh-theme") || S.theme || "auto";
  const perf = perfPref();
  const lite = H.classList.contains("perf-lite");
  const initials = (S.fullName || "؟").split(/\s+/).filter(Boolean).slice(0, 2).map((w) => w[0]).join(" ");
  const seg = (id, cur, opts) => `<div class="seg full" id="${id}">${opts.map(([k, t, i]) => `<button data-k="${k}" class="${cur === k ? "on" : ""}">${i ? ic(i) : ""}${t}</button>`).join("")}</div>`;
  const updState = u.installing ? `<span class="pill blue">${ic("download")}جارٍ التحديث ${u.progress || 0}%</span>`
    : u.available ? `<span class="pill gold">${ic("sparkles")}يتوفر ${esc(u.available.version)}</span>`
    : u.error ? `<span class="pill red" title="${esc(u.error)}">${ic("alert")}تعذّر التحديث</span>`
    : u.lastCheck ? `<span class="pill green">${ic("check")}أحدث إصدار</span>` : `<span class="pill gray">لم يُفحص بعد</span>`;
  const shownCourses = (S.courses || []).length - (S.courses || []).filter((c) => hidden.has(c.id)).length;

  v.innerHTML = head("الإعدادات", "كل تغيير يُحفظ مباشرة") + `
  <div class="bento">
    <section class="tile t-account span-7">
      <div class="acc">
        <div class="avatar">${esc(initials)}</div>
        <div class="grow"><div class="eyebrow">${S.linked ? "حساب المودل" : "غير مربوط"}</div>
          <h3>${S.linked ? esc(S.fullName) : "اربط حسابك لتبدأ المزامنة"}</h3>
          ${S.linked ? `<div class="site" dir="ltr">${esc(S.moodleUrl.replace(/^https?:\/\//, ""))}</div>` : ""}</div>
      </div>
      <div class="acc-stats">
        <div><b>${(S.courses || []).length}</b><span>مادة</span></div>
        <div><b>${S.index.docs.toLocaleString("en")}</b><span>ملف مفهرس</span></div>
        <div><b>${S.lastSync ? ago(S.lastSync) : "—"}</b><span>آخر مزامنة</span></div>
      </div>
      <div class="row wrap"><button class="btn light" id="relink">${ic("link")}${S.linked ? "تغيير الحساب" : "ربط الحساب"}</button>
        ${S.linked ? `<button class="btn on-dark-ghost" id="logout">فصل الحساب</button>` : ""}</div>
    </section>

    <section class="tile span-5">
      ${cardHead("sparkles", "التحديثات", updState)}
      <div class="ver"><span>الإصدار</span><b dir="ltr">${esc(S.version)}</b><small>${u.lastCheck ? "آخر فحص " + ago(u.lastCheck) : ""}</small></div>
      ${toggle("s-autoupd", "التحديث التلقائي", S.platform === "android" ? "ينزّل التحديث ويتحقق منه، ثم تثبّته بضغطة" : "يثبّت التحديثات بهدوء ويحفظ بياناتك قبلها", S.autoUpdate)}
      <div class="row wrap" style="margin-top:12px"><button class="btn" id="s-check">${ic("refresh")}البحث عن تحديث</button>
        ${u.available ? `<button class="btn gold" id="s-upd">${ic("download")}تحديث إلى ${esc(u.available.version)}</button>` : ""}</div>
    </section>

    <section class="tile span-4">
      ${cardHead("sun", "المظهر")}
      ${seg("s-theme", theme, [["light", "فاتح", "sun"], ["auto", "تلقائي", "monitor"], ["dark", "داكن", "moon"]])}
      <p class="hint">الوضع التلقائي يتبع إعدادات الجهاز.</p>
    </section>

    <section class="tile span-4">
      ${cardHead("gauge", "الأداء والحركة", lite ? `<span class="pill gold">الوضع الخفيف يعمل</span>` : "")}
      ${seg("s-perf", perf, [["auto", "تلقائي"], ["full", "كامل"], ["lite", "خفيف"]])}
      <p class="hint">${perf === "auto" ? "يكتشف البرنامج الأجهزة الضعيفة ويخفف المؤثرات وحده." : perf === "lite" ? "مؤثرات أقل وحركة أسرع، مناسب للأجهزة والهواتف الضعيفة." : "كل المؤثرات والحركات، للأجهزة القوية."}</p>
    </section>

    <section class="tile span-4">
      ${cardHead("video", "فيديوهات المحاضرات")}
      ${seg("s-videos", S.videos, [["auto", "تلقائي"], ["manual", "يدوي"], ["off", "إيقاف"]])}
      <p class="hint">${S.videos === "auto" ? "تنزل الفيديوهات الجديدة وحدها بعد الملفات." : S.videos === "manual" ? "تظهر في صفحة الفيديوهات وتنزّلها متى شئت." : "لا يبحث البرنامج عن الفيديوهات."}
        <a data-go="videos">صفحة الفيديوهات</a></p>
    </section>

    <section class="tile span-6">
      ${cardHead("refresh", "المزامنة والتنبيهات")}
      <div class="row"><label class="f grow" style="margin:0">تحقق من المودل كل</label>
        <select id="s-min" class="in" style="width:auto">${[10, 15, 20, 30, 60, 120].map((m) => `<option value="${m}" ${m === S.syncMinutes ? "selected" : ""}>${m} دقيقة</option>`).join("")}</select></div>
      <div style="margin-top:8px">
      ${toggle("s-toast", S.isWindows ? "تنبيهات ويندوز" : "التنبيهات", "ملفات جديدة، إشعارات، مواعيد قريبة، ملاحظات", S.toasts)}
      ${S.isWindows ? toggle("s-auto", "التشغيل مع ويندوز", "يعمل في الخلفية لتصلك التنبيهات دائماً", S.autostart) : ""}</div>
    </section>

    <section class="tile span-6">
      ${cardHead("book", "المواد المعروضة", `<span class="pill gray">${shownCourses} من ${(S.courses || []).length}</span>`)}
      <p class="hint" style="margin-top:0">أوقف المواد القديمة التي لا تريد تنزيل ملفاتها.</p>
      <div class="course-toggles">${(S.courses || []).map((c) => `<label class="toggle"><span class="t"><b>${esc(c.name)}</b></span><span class="switch"><input type="checkbox" class="s-course" value="${c.id}" ${hidden.has(c.id) ? "" : "checked"}><i></i></span></label>`).join("") || '<div class="muted">لا توجد مواد بعد</div>'}</div>
    </section>

    <section class="tile span-6 desk-only">
      ${cardHead("folder", "مجلد الجامعة المرتب")}
      <p class="hint" style="margin-top:0">هنا تنزل ملفات المودل: مجلد لكل مادة، وبداخله محاضرات وفيديوهات وواجبات وامتحانات.</p>
      <div class="row"><input id="s-root" class="in grow" dir="ltr" value="${esc(S.rootDir)}">${S.isWindows ? `<button class="btn sq" id="s-pick" title="اختيار">${ic("folder")}</button>` : ""}</div>
      <div class="row" style="margin-top:10px"><button class="btn ghost" data-reveal="${esc(S.rootDir)}">${ic("external")}فتح المجلد</button></div>
    </section>

    <section class="tile span-6 desk-only">
      ${cardHead("search", "مجلدات إضافية للبحث")}
      <p class="hint" style="margin-top:0">أضف مجلداتك القديمة ليشملها البحث. لن يُعدَّل أي شيء فيها.</p>
      <div id="extra" class="list"></div>
      <div class="row" style="margin-top:10px"><input id="s-add" class="in grow" dir="ltr" placeholder="مسار المجلد">${S.isWindows ? `<button class="btn sq" id="s-addpick" title="اختيار">${ic("folder")}</button>` : ""}<button class="btn" id="s-addbtn">${ic("plus")}إضافة</button></div>
    </section>

    <section class="tile span-8">
      ${cardHead("github", "الملاحظات الفنية والتحديثات من GitHub")}
      <label class="f" style="margin-top:0">المستودع</label>
      <div class="row"><input id="s-repo" class="in grow" dir="ltr" placeholder="username/unihub" value="${esc(S.repo || "")}"><button class="btn" id="s-repo-save">${ic("check")}حفظ</button></div>
      <p class="hint">${S.repoBuiltIn ? "المستودع الأصلي مضمّن في البرنامج، ولا حاجة لتغييره عادةً." : "المستودع الذي تصل منه التحديثات وتُرسل إليه الملاحظات."}</p>
    </section>

    <section class="tile span-4">
      ${cardHead("power", "البرنامج")}
      <dl class="kv"><dt>مثبّت</dt><dd>${S.installed ? "نعم" : "لا"}</dd><dt>الجهاز</dt><dd>${S.platform === "android" ? "أندرويد" : S.isWindows ? "ويندوز" : esc(S.platform || "")}</dd></dl>
      <div class="stack">
        ${S.canInstall && !S.installed ? `<button class="btn primary desk-only" id="s-install">${ic("install")}تثبيت البرنامج</button>` : ""}
        <button class="btn" id="s-rescan">${ic("refresh")}إعادة فحص الملفات</button>
        <button class="btn ghost" id="s-backup">${ic("archive")}نسخة احتياطية الآن</button>
        <button class="btn danger" id="s-quit">${ic("power")}إيقاف البرنامج</button></div>
    </section>
  </div>
  <div class="err" id="s-err" style="margin-top:14px"></div>`;

  // every change is saved right away
  const save = async (patch, msg = "تم الحفظ") => {
    $("#s-err").textContent = "";
    try { await api("/api/settings", patch); toast(msg, "check", 1600); await refresh(); }
    catch (e) { $("#s-err").textContent = e.message; toast(e.message, "alert"); }
  };
  const segPick = (id, fn) => $$(`#${id} button`).forEach((b) => b.addEventListener("click", () => {
    $$(`#${id} button`).forEach((x) => x.classList.toggle("on", x === b)); fn(b.dataset.k);
  }));
  const drawExtra = () => {
    $("#extra").innerHTML = extra.map((d, i) => `<div class="item"><div class="kind-ic" style="background:var(--surface-2)">${ic("folder")}</div><div class="body small" dir="ltr" style="text-align:right">${esc(d)}</div>
      <div class="actions"><button class="btn sm ghost danger" data-rm="${i}" title="إزالة من البحث">${ic("x")}</button></div></div>`).join("") || '<div class="muted small">لا توجد مجلدات إضافية</div>';
    $$("[data-rm]").forEach((b) => b.addEventListener("click", () => { extra.splice(+b.dataset.rm, 1); drawExtra(); save({ extraDirs: extra }, "أُزيل المجلد من البحث"); }));
  };
  drawExtra();
  const addExtra = (p) => {
    p = p.trim();
    if (!p || extra.includes(p)) return;
    extra.push(p); $("#s-add").value = ""; drawExtra();
    save({ extraDirs: extra }, "أُضيف المجلد، وبدأ فحصه").catch(() => {});
  };
  segPick("s-theme", (t) => applyTheme(t, true));
  segPick("s-perf", (p) => { applyPerf(p); save({ perf: p }); setTimeout(() => views.settings(v), 50); });
  segPick("s-videos", async (m) => { await save({ videos: m }); if (m !== "off") api("/api/sync", {}).catch(() => {}); views.settings(v); });
  $("#s-min").addEventListener("change", (e) => save({ syncMinutes: +e.target.value }));
  $("#s-toast").addEventListener("change", (e) => save({ toasts: e.target.checked }));
  $("#s-auto")?.addEventListener("change", (e) => save({ autostart: e.target.checked }));
  $("#s-autoupd").addEventListener("change", (e) => save({ autoUpdate: e.target.checked }));
  $$(".s-course").forEach((c) => c.addEventListener("change", () => save({ hidden: $$(".s-course").filter((x) => !x.checked).map((x) => +x.value) })));
  $("#s-root").addEventListener("change", (e) => save({ rootDir: e.target.value }, "تم تغيير المجلد"));
  $("#s-repo-save").addEventListener("click", () => save({ repo: $("#s-repo").value }));
  $("#s-add").addEventListener("keydown", (e) => { if (e.key === "Enter") addExtra($("#s-add").value); });
  $("#s-addbtn").addEventListener("click", () => addExtra($("#s-add").value));
  $("#s-addpick")?.addEventListener("click", async () => { const r = await api("/api/pickfolder", {}); if (r.path) addExtra(r.path); });
  $("#s-pick")?.addEventListener("click", async () => { const r = await api("/api/pickfolder", {}); if (r.path) { $("#s-root").value = r.path; save({ rootDir: r.path }, "تم تغيير المجلد"); } });
  $("#s-install")?.addEventListener("click", async () => {
    try { await api("/api/install", {}); toast("تم التثبيت. يُعاد تشغيل البرنامج من مكانه الجديد…", "install", 6000); waitForServer(); }
    catch (e) { toast(e.message, "alert"); }
  });
  $("#relink").addEventListener("click", () => go("setup"));
  $("#logout")?.addEventListener("click", async () => {
    if (!confirm("فصل حساب المودل؟ ملفاتك المنزّلة ستبقى كما هي.")) return;
    await api("/api/logout", {});
    await refresh();
    go("setup");
  });
  $("#s-check").addEventListener("click", async () => {
    const btn = $("#s-check");
    btn.disabled = true;
    btn.innerHTML = ic("refresh", "spin") + "جارٍ البحث…";
    try { const r = await api("/api/update/check", {}); toast(r.available ? `يتوفر الإصدار ${r.available.version}` : "أنت تستخدم أحدث إصدار", r.available ? "sparkles" : "check-circle"); }
    catch (e) { toast(e.message, "alert"); }
    await refresh(); views.settings(v);
  });
  $("#s-upd")?.addEventListener("click", startUpdate);
  $("#s-backup").addEventListener("click", async () => { await api("/api/backup", {}); toast("تم حفظ نسخة احتياطية من بياناتك", "archive"); });
  $("#s-rescan").addEventListener("click", async () => { await api("/api/rescan", {}); toast("بدأ فحص الملفات", "refresh"); });
  $("#s-quit").addEventListener("click", quitApp);
};

/* ================= import ================= */
views.import = (v) => {
  const names = (S.courses || []).map((c) => c.name);
  v.innerHTML = head("ترتيب مجلد قديم", "انسخ ملفات مادة قديمة إلى الترتيب الجديد. الملفات الأصلية تبقى في مكانها كما هي") + `
  <div class="card" style="max-width:760px">
    <label class="f" style="margin-top:0">المجلد القديم</label>
    <div class="row"><input id="i-src" class="in grow" dir="ltr" placeholder="C:\\Users\\...\\Desktop\\الجامعة\\برمجة 1">${S.isWindows ? `<button class="btn" id="i-pick">${ic("folder")}اختيار</button>` : ""}</div>
    <label class="f">اسم المادة</label>
    <input id="i-course" class="in" list="i-courses" placeholder="اختر من القائمة أو اكتب اسماً جديداً">
    <datalist id="i-courses">${names.map((n) => `<option value="${esc(n)}">`).join("")}</datalist>
    <div class="row" style="margin-top:16px"><button class="btn primary" id="i-prev">${ic("search")}معاينة قبل النسخ</button><span class="err" id="i-err"></span></div>
  </div><div id="i-out"></div>`;
  $("#i-pick")?.addEventListener("click", async () => {
    const r = await api("/api/pickfolder", {});
    if (!r.path) return;
    $("#i-src").value = r.path;
    if (!$("#i-course").value) {
      const base = r.path.split(/[\\/]/).pop();
      $("#i-course").value = names.find((n) => n.includes(base) || base.includes(n)) || base;
    }
  });
  $("#i-prev").addEventListener("click", async () => {
    $("#i-err").textContent = "";
    let d;
    try { d = await api("/api/import/preview", { src: $("#i-src").value.trim(), course: $("#i-course").value.trim() }); }
    catch (e) { $("#i-err").textContent = e.message; return; }
    const items = d.items || [], todo = items.filter((i) => !i.skip), groups = {};
    for (const it of items) (groups[it.category] = groups[it.category] || []).push(it);
    $("#i-out").innerHTML = `<div class="card" style="margin-top:20px;max-width:760px">${cardHead("layers", `المعاينة: ${todo.length} ملف سيتم نسخه`, items.length - todo.length ? `<span class="pill gray">${items.length - todo.length} موجود مسبقاً</span>` : "")}
      ${Object.entries(groups).map(([cat, list]) => `<h3>${esc(cat)} <span class="muted">(${list.length})</span></h3>
        <table class="prev">${list.slice(0, 200).map((i) => `<tr><td dir="auto">${esc(i.rel)}</td><td class="muted" dir="auto">${i.skip ? esc(i.reason) : esc(i.dest.slice(S.rootDir.length + 1))}</td></tr>`).join("")}</table>
        ${list.length > 200 ? `<p class="muted">و ${list.length - 200} ملف آخر…</p>` : ""}`).join("")}
      <div class="row" style="margin-top:16px"><button class="btn primary" id="i-run" ${todo.length ? "" : "disabled"}>${ic("copy")}نسخ ${todo.length} ملف</button></div></div>`;
    $("#i-run")?.addEventListener("click", async () => {
      $("#i-run").disabled = true;
      const r = await api("/api/import/run", { items });
      toast(`تم نسخ ${r.copied} ملف`);
      $("#i-out").innerHTML = `<div class="card" style="margin-top:20px;max-width:760px">${cardHead("check-circle", `تم نسخ ${r.copied} ملف`)}
        ${r.errors?.length ? `<p class="err">${r.errors.map(esc).join("<br>")}</p>` : ""}
        <p class="muted">الملفات الأصلية بقيت في مكانها. بعد التأكد يمكنك حذف المجلد القديم بنفسك.</p></div>`;
    });
  });
};

/* ================= responsive layout & sidebar ================= */
const H = document.documentElement;
const mqPhone = matchMedia("(max-width: 760px)"), mqTablet = matchMedia("(max-width: 1100px)");
let sideOpen = false, layoutNow = "";
try { localStorage.removeItem("uh-side"); } catch (e) {}
const layoutMode = () => (mqPhone.matches ? "phone" : mqTablet.matches ? "tablet" : "desktop");
function applyLayout() {
  const m = layoutMode();
  if (m === "desktop") sideOpen = false;
  if (layoutNow && layoutNow !== m) {
    H.classList.add("no-anim");
    requestAnimationFrame(() => requestAnimationFrame(() => H.classList.remove("no-anim")));
  }
  layoutNow = m;
  H.classList.remove("mode-phone", "mode-tablet", "mode-desktop");
  H.classList.add("mode-" + m);
  H.classList.toggle("side-open", sideOpen);
  H.classList.toggle("is-rail", m === "tablet" && !sideOpen);
  $("#menu-btn").setAttribute("aria-expanded", String(sideOpen));
  $("#q").placeholder = m === "phone" ? "ابحث في ملفاتك…" : "ابحث في كل المحاضرات والواجبات والملفات…";
  document.body.style.overflow = sideOpen && m === "phone" ? "hidden" : "";
}
function toggleSide() {
  if (layoutMode() === "desktop") return; // the full sidebar always shows on large screens
  sideOpen = !sideOpen;
  applyLayout();
}
function closeSide() { if (sideOpen) { sideOpen = false; applyLayout(); } }
$("#menu-btn").addEventListener("click", toggleSide);
$("#drawer-close").addEventListener("click", closeSide);
$("#scrim").addEventListener("click", closeSide);
mqPhone.addEventListener("change", applyLayout);
mqTablet.addEventListener("change", applyLayout);
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape" && sideOpen) closeSide();
});
// links and buttons inside the sliding sidebar close it after use
$("#side").addEventListener("click", (e) => { if (layoutMode() !== "desktop" && e.target.closest("nav a, [data-go]")) setTimeout(closeSide, 60); });
// swipe: from the right edge to open the drawer, to the right to close it (phone)
(() => {
  let x0 = 0, y0 = 0, from = "";
  document.addEventListener("touchstart", (e) => {
    if (layoutMode() !== "phone" || e.touches.length !== 1) { from = ""; return; }
    const t = e.touches[0];
    x0 = t.clientX; y0 = t.clientY;
    from = sideOpen ? "open" : x0 > window.innerWidth - 24 && !LB ? "edge" : "";
  }, { passive: true });
  document.addEventListener("touchend", (e) => {
    if (!from) return;
    const t = e.changedTouches[0], dx = t.clientX - x0, dy = Math.abs(t.clientY - y0);
    if (from === "open" && dx > 60 && dy < 60) closeSide();
    if (from === "edge" && dx < -50 && dy < 60) { sideOpen = true; applyLayout(); }
    from = "";
  }, { passive: true });
})();
$$("#nav a[data-view]").forEach((a) => { if (!a.title) a.title = a.querySelector("span")?.textContent || ""; });

// bottom tab bar (phone)
const TABS = ["home", "courses", "assignments", "images"];
function markTabs() {
  const v = current === "course" ? "courses" : current;
  $$("#tabbar a").forEach((a) => a.classList.toggle("active", a.dataset.tab === v || (a.dataset.tab === "more" && !TABS.includes(v))));
}
$("#tabbar").addEventListener("click", (e) => {
  const a = e.target.closest("a[data-tab]");
  if (!a) return;
  if (a.dataset.tab === "more") { sideOpen = true; applyLayout(); } else go(a.dataset.tab);
});
applyLayout();
requestAnimationFrame(() => requestAnimationFrame(() => H.classList.remove("no-anim")));

/* ================= quick access bar ================= */
async function quitApp() {
  if (!confirm("إيقاف البرنامج؟ لن تصلك تنبيهات حتى تشغّله مرة أخرى.")) return;
  if (S?.platform === "android") { try { await api("/api/quit", {}); } catch (e) {} return; }
  try { await api("/api/quit", {}); } catch (e) {}
  document.body.innerHTML = `<div class="setup" style="margin-top:12vh"><div class="card" style="text-align:center;padding:40px"><img src="icon.svg" style="width:64px"><h2 style="justify-content:center;margin-top:12px">تم إيقاف البرنامج</h2><p class="muted">يمكنك إغلاق هذه الصفحة. لتشغيله مرة أخرى افتح «منظّم الجامعة» من قائمة ابدأ.</p></div></div>`;
}
const initial = (name) => { const w = String(name || "").trim().split(/\s+/)[0] || ""; return [...w][0] || ""; };

function updateQuickbar() {
  const name = S.linked ? (S.fullName || S.username || "حسابي") : "غير مربوط";
  $("#qb-name").textContent = name;
  const ini = S.linked ? initial(S.fullName) : "";
  const av = $("#qb-av");
  if (av.dataset.ini !== ini) { av.dataset.ini = ini; av.innerHTML = ini ? esc(ini) : ic("user"); }
  const b = $("#qb-badge");
  b.textContent = S.newNotif > 9 ? "9+" : S.newNotif || "";
  b.classList.toggle("hide", !S.newNotif);
  $("#qb-account").title = name;
  if (popKind === "notif" && !$("#pop").classList.contains("hide")) drawNotifPop(false);
}

let popKind = null;
function closePop() {
  popKind = null;
  $("#pop").classList.add("hide");
  $$("#quickbar .on").forEach((b) => b.classList.remove("on"));
}
function openPop(kind, btn) {
  if (popKind === kind) { closePop(); return; }
  closePop();
  popKind = kind;
  btn.classList.add("on");
  $("#pop").classList.remove("hide");
  if (kind === "notif") drawNotifPop(true);
  else drawAccountPop();
}
function drawNotifPop(first) {
  const list = (S.notifications || []).slice(0, 7);
  $("#pop").innerHTML = `<div class="pop-head"><span>الإشعارات</span><a class="small" data-pop-go="notifications">عرض الكل</a></div>
    ${list.map((n) => `<div class="pop-notif" ${n.url ? `data-url="${esc(n.url)}" style="cursor:pointer"` : ""}>
      <b>${n.isNew ? '<span class="newdot"></span>' : ""}${esc(n.subject)}</b>
      ${n.text && n.text !== n.subject ? `<span>${esc(n.text.slice(0, 160))}</span>` : ""}<small>${esc(ago(n.time))}</small></div>`).join("")
      || `<div class="empty" style="padding:28px 10px">${ic("bell")}<div>لا توجد إشعارات</div></div>`}`;
  if (first && S.newNotif) api("/api/notifications/seen", {}).then(refresh).catch(() => {});
}
function drawAccountPop() {
  const item = (icon, label, attr, cls = "") => `<button class="pop-item ${cls}" ${attr}>${ic(icon)}<span>${label}</span></button>`;
  $("#pop").innerHTML = (S.linked
    ? `<div class="pop-user"><span class="qb-av">${esc(initial(S.fullName)) || ic("user")}</span><div style="min-width:0"><b dir="auto">${esc(S.fullName)}</b>
        <small dir="auto">${esc(S.siteName || "")}</small><small dir="ltr" style="text-align:right">${esc(S.username || "")}</small></div></div>`
    : `<div class="pop-user"><span class="qb-av">${ic("user")}</span><div><b>غير مربوط بالمودل</b><small>اربط حسابك لتبدأ المزامنة</small></div></div>`) +
    `<div class="pop-sep"></div>
    ${item("gear", "الإعدادات", 'data-pop-go="settings"')}
    ${item("cap", S.linked ? "تغيير حساب المودل" : "ربط حساب المودل", 'data-pop-go="setup"')}
    ${item("link", "روابطي", 'data-pop-go="links"')}
    ${item("images", "صوري", 'data-pop-go="images"')}
    ${item("message", "الملاحظات الفنية", 'data-pop-go="feedback"')}
    <div class="pop-sep"></div>
    ${item("power", "إيقاف البرنامج", 'data-pop-quit', "danger")}`;
}
$("#qb-notif").addEventListener("click", (e) => { e.stopPropagation(); openPop("notif", e.currentTarget); });
$("#qb-account").addEventListener("click", (e) => { e.stopPropagation(); openPop("account", e.currentTarget); });
$("#qb-settings").addEventListener("click", () => { closePop(); go("settings"); });
$("#qb-power").addEventListener("click", () => { closePop(); quitApp(); });
$("#pop").addEventListener("click", (e) => {
  const g = e.target.closest("[data-pop-go]");
  if (g) { e.preventDefault(); e.stopPropagation(); closePop(); go(g.dataset.popGo); return; }
  if (e.target.closest("[data-pop-quit]")) { e.stopPropagation(); closePop(); quitApp(); return; }
  if (e.target.closest("[data-url]")) setTimeout(closePop, 50);
});
document.addEventListener("click", (e) => { if (popKind && !e.target.closest("#quickbar")) closePop(); });
document.addEventListener("keydown", (e) => { if (e.key === "Escape" && popKind) closePop(); });

/* ================= images ================= */
let IMG = null;
let imgFilter = "all", imgQuery = "", imgUploadCat = "";
const imgUrl = (it) => `/api/images/raw?p=${encodeURIComponent(it.rel)}&t=${window.API_TOKEN}&v=${it.mtime}`;
async function loadImages() {
  try { IMG = await api("/api/images"); } catch (e) { IMG = IMG || { items: [], categories: [] }; }
  IMG.items = IMG.items || [];
  IMG.categories = IMG.categories || [];
  return IMG;
}

function pinnedCard() {
  const pins = (IMG?.items || []).filter((i) => i.pinned);
  if (!pins.length) return "";
  return `<div class="card" style="margin-bottom:20px">${cardHead("pin", "صور مثبّتة", `<a data-go="images" class="small">كل الصور</a>`)}
    <div class="pinned-strip">${pins.map((it, i) => `<div class="shot" data-pin-open="${i}" title="${esc(it.name)}">
      <img class="thumb" loading="lazy" src="${imgUrl(it)}" alt=""><div class="cap"><b dir="auto">${esc(it.name)}</b><small>${esc(it.category)}</small></div></div>`).join("")}</div></div>`;
}
document.addEventListener("click", (e) => {
  const p = e.target.closest("[data-pin-open]");
  if (p) openLightbox(IMG.items.filter((i) => i.pinned), +p.dataset.pinOpen);
});

async function uploadImages(files, cat) {
  files = [...files].filter((f) => /^image\//.test(f.type) || /\.(png|jpe?g|jfif|webp|gif|bmp)$/i.test(f.name));
  if (!files.length) { toast("اختر صوراً (PNG أو JPG أو WEBP…)", "alert"); return; }
  const fd = new FormData();
  fd.append("category", cat || "أخرى");
  for (const f of files) fd.append("file", f, f.name);
  toast(`جارٍ حفظ ${files.length} صورة…`, "upload");
  let r;
  try {
    const res = await fetch("/api/images/upload", { method: "POST", headers: { "X-UniHub-Token": window.API_TOKEN }, body: fd });
    r = await res.json();
    if (!res.ok) throw new Error(r.error || "تعذر الحفظ");
  } catch (e) { toast(e.message, "alert"); return; }
  if (r.errors?.length) toast(r.errors[0], "alert", 6000);
  else toast(`تم حفظ ${r.saved.length} صورة في «${cat || "أخرى"}»`, "check-circle");
  await loadImages();
  if (current === "images") drawImages($("#view"));
}

function stamp() {
  const d = new Date(), z = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${z(d.getMonth() + 1)}-${z(d.getDate())} ${z(d.getHours())}-${z(d.getMinutes())}-${z(d.getSeconds())}`;
}
document.addEventListener("paste", (e) => {
  if (current !== "images" || ["INPUT", "TEXTAREA"].includes(document.activeElement?.tagName)) return;
  const files = [...(e.clipboardData?.items || [])].filter((i) => i.kind === "file" && i.type.startsWith("image/")).map((i) => i.getAsFile()).filter(Boolean);
  if (!files.length) return;
  e.preventDefault();
  const named = files.map((f, i) => new File([f], `لقطة ${stamp()}${files.length > 1 ? "-" + (i + 1) : ""}.${(f.type.split("/")[1] || "png").replace("jpeg", "jpg")}`, { type: f.type }));
  uploadImages(named, imgUploadCat);
});

views.images = (v) => {
  v.innerHTML = head("صوري", "كل صور الجامعة في مكان واحد: جدول الامتحانات، الخطة الدراسية، إشعارات الدفع، البطاقات…") + empty("refresh", "جارٍ التحميل…");
  loadImages().then(() => { if (current === "images") drawImages(v); });
};

function drawImages(v) {
  const cats = IMG.categories;
  if (!imgUploadCat) imgUploadCat = imgFilter !== "all" ? imgFilter : cats[0]?.name || "أخرى";
  const q = imgQuery.trim().toLowerCase();
  const list = IMG.items.filter((i) => (imgFilter === "all" || i.category === imgFilter) && (!q || i.name.toLowerCase().includes(q)));
  v.innerHTML = head("صوري", "كل صور الجامعة في مكان واحد: جدول الامتحانات، الخطة الدراسية، إشعارات الدفع، البطاقات…",
    `<button class="btn" data-reveal="${esc(IMG.root)}">${ic("folder")}فتح المجلد</button>`) + `
  <div class="dropzone" id="dz">
    <div class="dz-ic">${ic("upload", "lg")}</div>
    <div class="dz-txt"><b>أضف صورة</b><span><span class="desk-only">اسحبها وأفلتها هنا، أو اختر من جهازك، أو الصق لقطة شاشة بـ <kbd>Ctrl</kbd>+<kbd>V</kbd></span><span class="app-only">اختر صوراً من معرض الهاتف أو التقط صورة جديدة</span></span></div>
    <select class="in" id="up-cat" style="width:auto;min-width:170px">${cats.map((c) => `<option ${c.name === imgUploadCat ? "selected" : ""}>${esc(c.name)}</option>`).join("")}<option value="__new">+ تصنيف جديد…</option></select>
    <button class="btn primary" id="up-btn">${ic("plus")}اختيار صور</button>
    <input type="file" id="up-file" accept="image/*" multiple hidden>
  </div>
  <div class="filters">
    <div class="seg wrap"><button data-cat="all" class="${imgFilter === "all" ? "on" : ""}">الكل <span class="muted">${IMG.items.length}</span></button>
      ${cats.map((c) => `<button data-cat="${esc(c.name)}" class="${imgFilter === c.name ? "on" : ""}">${esc(c.name)} <span class="muted">${c.count}</span></button>`).join("")}</div>
    <span class="spacer"></span>
    <input class="in" id="img-q" placeholder="ابحث في أسماء الصور" value="${esc(imgQuery)}" style="width:220px">
  </div>
  ${list.length ? `<div class="gallery">${list.map((it, i) => `<div class="shot" data-open-img="${i}" title="${esc(it.name)}">
      <img class="thumb" loading="lazy" src="${imgUrl(it)}" alt="">
      ${it.pinned ? `<span class="pin-badge" title="مثبّتة في الرئيسية">${ic("pin")}</span>` : ""}
      <div class="cap"><b dir="auto">${esc(it.name)}</b><small>${esc(it.category)}${SEP}${esc(dateOnly(it.mtime))}</small></div></div>`).join("")}</div>`
    : `<div class="card">${empty("images", IMG.items.length ? "لا توجد صور تطابق البحث" : "لا توجد صور بعد. أضف صورة جدول الامتحانات أو خطتك الدراسية لتجدها هنا دائماً")}</div>`}`;

  const dz = $("#dz", v), file = $("#up-file", v), sel = $("#up-cat", v);
  sel.addEventListener("change", async () => {
    if (sel.value !== "__new") { imgUploadCat = sel.value; return; }
    const name = prompt("اسم التصنيف الجديد (مثال: شهادات، نتائج):");
    if (!name || !name.trim()) { sel.value = imgUploadCat; return; }
    try { const r = await api("/api/images/category", { name }); imgUploadCat = r.name; await loadImages(); drawImages(v); }
    catch (e) { toast(e.message, "alert"); sel.value = imgUploadCat; }
  });
  $("#up-btn", v).addEventListener("click", () => file.click());
  file.addEventListener("change", () => { if (file.files.length) uploadImages(file.files, imgUploadCat); });
  ["dragenter", "dragover"].forEach((t) => v.addEventListener(t, (e) => { if (e.dataTransfer?.types?.includes("Files")) { e.preventDefault(); dz.classList.add("over"); } }));
  ["dragleave", "drop"].forEach((t) => v.addEventListener(t, (e) => { if (t === "dragleave" && v.contains(e.relatedTarget)) return; dz.classList.remove("over"); }));
  v.addEventListener("drop", (e) => { if (e.dataTransfer?.files?.length) { e.preventDefault(); uploadImages(e.dataTransfer.files, imgUploadCat); } });
  $$("[data-cat]", v).forEach((b) => b.addEventListener("click", () => {
    imgFilter = b.dataset.cat;
    if (imgFilter !== "all") imgUploadCat = imgFilter;
    drawImages(v);
  }));
  const qi = $("#img-q", v);
  qi.addEventListener("input", () => { imgQuery = qi.value; clearTimeout(qi._t); qi._t = setTimeout(() => { drawImages(v); const n = $("#img-q", v); n.focus(); n.setSelectionRange(n.value.length, n.value.length); }, 180); });
  $$("[data-open-img]", v).forEach((el) => el.addEventListener("click", () => openLightbox(list, +el.dataset.openImg)));
}

/* lightbox */
let LB = null;
function openLightbox(list, i) {
  if (!list.length) return;
  closePop();
  LB = { list, i, zoom: false };
  let el = $("#lightbox");
  if (!el) {
    el = document.createElement("div");
    el.id = "lightbox";
    el.className = "lightbox";
    document.body.appendChild(el);
    el.addEventListener("click", lbClick);
    // swipe between images on touch screens (RTL: swipe toward the right = next)
    let sx = 0, sy = 0;
    el.addEventListener("touchstart", (e) => { if (e.touches.length === 1) { sx = e.touches[0].clientX; sy = e.touches[0].clientY; } else sx = NaN; }, { passive: true });
    el.addEventListener("touchend", (e) => {
      if (!LB || LB.zoom || isNaN(sx)) return;
      const t = e.changedTouches[0], dx = t.clientX - sx, dy = t.clientY - sy;
      if (Math.abs(dx) > 60 && Math.abs(dy) < 80) $(`#lightbox [data-lb=${dx > 0 ? "next" : "prev"}]`)?.click();
      else if (dy > 110 && Math.abs(dx) < 60) closeLightbox();
    }, { passive: true });
  }
  el.classList.remove("hide");
  drawLightbox();
}
function closeLightbox() { $("#lightbox")?.classList.add("hide"); LB = null; }
function drawLightbox() {
  const it = LB.list[LB.i], n = LB.list.length;
  $("#lightbox").innerHTML = `<div class="lb-bar">
      <button class="lb-btn" data-lb="close" title="إغلاق (Esc)">${ic("x")}</button>
      <div class="lb-title"><b dir="auto">${esc(it.name)}</b><small>${esc(it.category)} · ${esc(dateOnly(it.mtime))} · <span dir="ltr">${esc(size(it.size))}</span>${n > 1 ? ` · ${LB.i + 1} من ${n}` : ""}</small></div>
      <button class="lb-btn ${LB.zoom ? "on" : ""}" data-lb="zoom" title="تكبير">${ic("zoom")}</button>
      <button class="lb-btn ${it.pinned ? "on" : ""}" data-lb="pin" title="${it.pinned ? "إلغاء التثبيت من الرئيسية" : "تثبيت في الرئيسية"}">${ic("pin")}</button>
      <button class="lb-btn" data-lb="edit" title="إعادة التسمية أو تغيير التصنيف">${ic("pencil")}</button>
      <button class="lb-btn" data-lb="open" title="فتح في عارض الصور">${ic("external")}</button>
      <button class="lb-btn" data-lb="reveal" title="إظهار في المجلد">${ic("folder")}</button>
      <button class="lb-btn danger" data-lb="del" title="حذف (إلى سلة المحذوفات)">${ic("trash")}</button>
    </div>
    <div class="lb-stage ${LB.zoom ? "zoomed" : ""}"><img src="${imgUrl(it)}" alt="" data-lb="zoom"></div>
    ${n > 1 ? `<button class="lb-btn lb-nav prev" data-lb="prev" title="السابقة">${ic("prev")}</button><button class="lb-btn lb-nav next" data-lb="next" title="التالية">${ic("next")}</button>` : ""}`;
}
async function lbClick(e) {
  const b = e.target.closest("[data-lb]");
  if (!b) { if (e.target.classList.contains("lb-stage")) closeLightbox(); return; }
  const it = LB.list[LB.i], act = b.dataset.lb;
  if (act === "close") closeLightbox();
  else if (act === "prev" || act === "next") { LB.i = (LB.i + (act === "next" ? 1 : -1) + LB.list.length) % LB.list.length; LB.zoom = false; drawLightbox(); }
  else if (act === "zoom") { LB.zoom = !LB.zoom; drawLightbox(); }
  else if (act === "open") openFile(it.path);
  else if (act === "reveal") openFile(it.path, true);
  else if (act === "pin") {
    await api("/api/images/pin", { rel: it.rel, pinned: !it.pinned });
    it.pinned = !it.pinned;
    toast(it.pinned ? "ثُبّتت الصورة في الصفحة الرئيسية" : "أُلغي التثبيت", "pin");
    drawLightbox(); afterImageChange();
  } else if (act === "del") {
    if (!confirm(`حذف «${it.name}»؟ ستنتقل إلى سلة المحذوفات ويمكنك استرجاعها منها.`)) return;
    try { await api("/api/images/delete", { rel: it.rel }); } catch (err) { toast(err.message, "alert"); return; }
    LB.list.splice(LB.i, 1);
    toast("نُقلت الصورة إلى سلة المحذوفات", "trash");
    if (!LB.list.length) closeLightbox(); else { LB.i %= LB.list.length; drawLightbox(); }
    afterImageChange();
  } else if (act === "edit") editImage(it);
}
function editImage(it) {
  const cats = IMG.categories.map((c) => c.name);
  modal(`<h2>${ic("pencil")}تعديل الصورة</h2>
    <label class="f">الاسم</label><input id="ed-name" class="in" maxlength="80" value="${esc(it.name)}">
    <label class="f">التصنيف</label><select id="ed-cat" class="in">${cats.map((c) => `<option ${c === it.category ? "selected" : ""}>${esc(c)}</option>`).join("")}</select>
    <div class="err" id="ed-err" style="margin-top:10px"></div>
    <div class="row" style="margin-top:16px"><span class="spacer"></span><button class="btn ghost" data-close>إلغاء</button><button class="btn primary" id="ed-save">${ic("check")}حفظ</button></div>`);
  $("#ed-name").focus();
  $("#ed-save").addEventListener("click", async () => {
    try {
      const r = await api("/api/images/edit", { rel: it.rel, name: $("#ed-name").value, category: $("#ed-cat").value });
      closeModal();
      await loadImages();
      const fresh = IMG.items.find((x) => x.rel === r.rel);
      if (fresh && LB) { LB.list[LB.i] = fresh; drawLightbox(); }
      toast("تم الحفظ");
      afterImageChange(false);
    } catch (e) { $("#ed-err").textContent = e.message; }
  });
}
async function afterImageChange(reload = true) {
  if (reload) await loadImages();
  if (current === "images") drawImages($("#view"));
  else if (current === "home") render();
}
document.addEventListener("keydown", (e) => {
  if (!LB || !$("#modal").classList.contains("hide")) return;
  if (e.key === "Escape") closeLightbox();
  else if (e.key === "ArrowLeft") $("#lightbox [data-lb=next]")?.click();
  else if (e.key === "ArrowRight") $("#lightbox [data-lb=prev]")?.click();
});

/* ================= start ================= */
function fromHash() {
  const h = decodeURIComponent(location.hash.slice(1));
  if (!h) return false;
  const [view, arg] = h.split("/");
  if (!views[view]) return false;
  if (view === "search" && arg) $("#q").value = arg;
  go(view, view === "course" ? { id: +arg } : {}, true);
  return true;
}
window.addEventListener("popstate", () => { if (!fromHash()) go("home", {}, true); });

(async function start() {
  applyTheme(localStorage.getItem("uh-theme") || "auto", false);
  await refresh();
  if (!S) return;
  loadImages().then(() => { if (current === "home") render(); });
  // check for a new version whenever the window opens (at most every 15 minutes)
  if (S.repo && !S.update.installing && Date.now() / 1000 - (S.update.lastCheck || 0) > 900) {
    api("/api/update/check", {}).then(refresh).catch(() => {});
  }
  if (!S.linked && !location.hash.match(/feedback|settings|search|import|links|images/)) go("setup");
  else if (!fromHash()) go("home", {}, true);
  setInterval(() => { if (!offline && !document.hidden) refresh(); }, 5000);
  document.addEventListener("visibilitychange", () => { if (!document.hidden && !offline) refresh(); });
  if (S.perf && S.perf !== perfPref()) applyPerf(S.perf);
  setTimeout(measurePerf, 1200);
})();
