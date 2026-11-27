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
}
$$("[data-theme-set]").forEach((b) => b.addEventListener("click", () => applyTheme(b.dataset.themeSet, true)));
mq.addEventListener?.("change", () => { /* auto mode follows the system through CSS */ });

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

  $("#who").textContent = S.linked ? (S.fullName || S.siteName || "UniHub") : "غير مربوط بالمودل";
  $("#ver").textContent = "الإصدار " + S.version;
  const pending = (S.assignments || []).filter((a) => a.status !== "submitted" && a.due > S.now).length;
  setBadge("#badge-assign", pending);
  setBadge("#badge-notif", S.newNotif);
  setBadge("#badge-fb", S.fbUnread);
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
  renderBanners();
  if (S.update.justUpdated && !$("#modal").dataset.shown) showWhatsNew();
  if (!S.linked && !["settings", "setup", "search", "import", "feedback"].includes(current)) go("setup");
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
  const v = $("#view");
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
function render(fresh) {
  if (!S) return;
  const v = $("#view");
  const fn = views[current] || views.home;
  if (fresh || !["search", "import", "settings", "setup", "course", "feedback"].includes(current)) {
    const html = fn(v, fresh);
    if (typeof html === "string") v.innerHTML = html;
  }
}

/* ================= banners & modal ================= */
function renderBanners() {
  const b = [];
  const u = S.update;
  if (u.installing) {
    b.push(`<div class="banner"><div class="b-ic">${ic("download")}</div><div class="b-txt"><b>جارٍ تثبيت التحديث ${esc(u.available?.version || "")}</b>
      <span>${u.progress < 100 ? "تنزيل الملف…" : "التحقق والتثبيت… سيُعاد تشغيل البرنامج تلقائياً"}</span>
      <div class="progress"><i style="width:${u.progress || 2}%"></i></div></div></div>`);
  } else if (u.available) {
    b.push(`<div class="banner"><div class="b-ic">${ic("sparkles")}</div><div class="b-txt"><b>إصدار جديد ${esc(u.available.version)} متاح</b>
      <span>بياناتك وملفاتك محفوظة بالكامل أثناء التحديث.</span></div>
      <button class="btn sm ghost" id="bn-notes">ما الجديد؟</button><button class="btn sm gold" id="bn-upd">${ic("download")}تحديث الآن</button></div>`);
  }
  if (S.lastError && S.linked && !S.sync.running) {
    b.push(`<div class="banner warn"><div class="b-ic">${ic("alert")}</div><div class="b-txt"><b>تنبيه من المزامنة</b><span>${esc(S.lastError)}</span></div></div>`);
  }
  if (S.canInstall && !S.installed && !sessionStorage.getItem("hide-install")) {
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
    $("#bn-later")?.addEventListener("click", () => { sessionStorage.setItem("hide-install", "1"); renderBanners(); });
    $("#bn-install")?.addEventListener("click", async () => {
      try { await api("/api/install", {}); toast("تم التثبيت. يُعاد تشغيل البرنامج من مكانه الجديد…", "install", 6000); waitForServer(); }
      catch (e) { toast(e.message, "alert"); }
    });
  }
}

async function startUpdate() {
  await api("/api/update/apply", {});
  toast("بدأ تنزيل التحديث", "download");
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

views.home = () => {
  const up = upcoming();
  const pending = (S.assignments || []).filter((a) => a.status !== "submitted" && a.due > S.now).length;
  const hour = new Date().getHours();
  const greet = hour < 12 ? "صباح الخير" : hour < 18 ? "مساء الخير" : "مساء النور";
  const first = (S.fullName || "").split(" ")[0];
  return head(`${greet}${first ? "، " + esc(first) : ""}`, "هذه نظرة سريعة على دراستك اليوم", "", dateOnly(S.now)) + `
  <div class="stats">
    <div class="stat"><div class="ic">${ic("book", "lg")}</div><div><b>${(S.courses || []).length}</b><span>مادة دراسية</span></div></div>
    <div class="stat"><div class="ic red">${ic("tasks", "lg")}</div><div><b>${pending}</b><span>واجب بانتظار التسليم</span></div></div>
    <div class="stat"><div class="ic blue">${ic("search", "lg")}</div><div><b>${S.index.docs.toLocaleString("en")}</b><span>ملف قابل للبحث</span></div></div>
    <div class="stat"><div class="ic green">${ic("bell", "lg")}</div><div><b>${S.newNotif || 0}</b><span>إشعار جديد</span></div></div>
  </div>
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
    const cats = ["الكل", "محاضرات", "واجبات", "امتحانات", "ملفات أخرى"];
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
    <div class="card"><div id="results" class="list">${empty("search", "اكتب في مربع البحث: اسم ملف، أو كلمة داخل المحاضرة<br><span class='small'>الأسهم للتنقل · Enter للفتح · Ctrl+Enter لإظهار المجلد</span>")}</div></div>`;
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
  if (e.key === "Escape" && !$("#modal").classList.contains("hide")) closeModal();
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
    <label class="f">أين تُحفظ ملفات الجامعة المرتبة؟</label>
    <div class="row"><input id="u-root" class="in grow" dir="ltr" value="${esc(S.rootDir)}">${S.isWindows ? `<button class="btn" id="pick-root">${ic("folder")}اختيار</button>` : ""}</div>
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
  <div class="grid" style="grid-template-columns:minmax(320px,400px) 1fr;align-items:start">
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

/* ================= settings ================= */
const toggle = (id, title, sub, on) => `<label class="toggle"><span class="t"><b>${title}</b><span>${sub}</span></span><span class="switch"><input type="checkbox" id="${id}" ${on ? "checked" : ""}><i></i></span></label>`;

views.settings = (v) => {
  const extra = [...(S.extraDirs || [])];
  const hidden = new Set(S.hidden || []);
  const u = S.update;
  const theme = localStorage.getItem("uh-theme") || S.theme || "auto";
  v.innerHTML = head("الإعدادات", "", `<button class="btn primary" id="s-save">${ic("check")}حفظ الإعدادات</button>`) + `
  <div class="grid g2" style="align-items:start">
    <div class="card">${cardHead("cap", "حساب المودل")}
      ${S.linked ? `<dl class="kv"><dt>الاسم</dt><dd>${esc(S.fullName)}</dd><dt>الموقع</dt><dd dir="ltr" style="text-align:right">${esc(S.moodleUrl)}</dd></dl>` : `<p class="muted">غير مربوط</p>`}
      <div class="row" style="margin-top:14px"><button class="btn" id="relink">${ic("link")}${S.linked ? "تغيير الحساب" : "ربط الحساب"}</button>
      ${S.linked ? `<button class="btn danger" id="logout">فصل الحساب</button>` : ""}</div></div>

    <div class="card">${cardHead("sun", "المظهر")}
      <div class="seg" id="s-theme">
        <button data-t="light" class="${theme === "light" ? "on" : ""}">${ic("sun")}فاتح</button>
        <button data-t="auto" class="${theme === "auto" ? "on" : ""}">${ic("monitor")}تلقائي</button>
        <button data-t="dark" class="${theme === "dark" ? "on" : ""}">${ic("moon")}داكن</button></div>
      <p class="hint">الوضع التلقائي يتبع إعدادات ويندوز ويتغير معها مباشرة.</p></div>

    <div class="card">${cardHead("refresh", "المزامنة والتنبيهات")}
      <label class="f" style="margin-top:0">تحقق من المودل كل</label>
      <select id="s-min" class="in">${[10, 15, 20, 30, 60, 120].map((m) => `<option value="${m}" ${m === S.syncMinutes ? "selected" : ""}>${m} دقيقة</option>`).join("")}</select>
      <div style="margin-top:8px">
      ${toggle("s-toast", "تنبيهات ويندوز", "ملفات جديدة، إشعارات، مواعيد قريبة، ملاحظات", S.toasts)}
      ${S.isWindows ? toggle("s-auto", "التشغيل مع ويندوز", "يعمل في الخلفية لتصلك التنبيهات دائماً", S.autostart) : ""}</div></div>

    <div class="card">${cardHead("sparkles", "التحديثات")}
      <dl class="kv"><dt>الإصدار الحالي</dt><dd><b>${esc(S.version)}</b></dd>
        <dt>آخر فحص</dt><dd>${u.lastCheck ? ago(u.lastCheck) : "—"}</dd>
        <dt>الحالة</dt><dd>${u.available ? `<span class="pill gold">${ic("sparkles")}يتوفر ${esc(u.available.version)}</span>` : u.error ? `<span class="muted small">${esc(u.error)}</span>` : `<span class="pill green">${ic("check")}محدّث</span>`}</dd></dl>
      <div style="margin-top:8px">${toggle("s-autoupd", "التحديث التلقائي", "يثبّت التحديثات بهدوء ويحفظ بياناتك قبل كل تحديث", S.autoUpdate)}</div>
      <label class="f">مستودع GitHub للتحديثات والملاحظات</label>
      <input id="s-repo" class="in" dir="ltr" placeholder="username/unihub" value="${esc(S.repo || "")}">
      <div class="row wrap" style="margin-top:12px"><button class="btn" id="s-check">${ic("refresh")}البحث عن تحديث</button>
        ${u.available ? `<button class="btn gold" id="s-upd">${ic("download")}تحديث إلى ${esc(u.available.version)}</button>` : ""}
        <button class="btn ghost" id="s-backup">${ic("archive")}نسخة احتياطية الآن</button></div></div>

    <div class="card">${cardHead("folder", "مجلد الجامعة المرتب")}
      <p class="hint" style="margin-top:0">هنا تنزل ملفات المودل: مجلد لكل مادة، وبداخله محاضرات / واجبات / امتحانات.</p>
      <div class="row"><input id="s-root" class="in grow" dir="ltr" value="${esc(S.rootDir)}">${S.isWindows ? `<button class="btn sq" id="s-pick" title="اختيار">${ic("folder")}</button>` : ""}</div>
      <div class="row" style="margin-top:10px"><button class="btn ghost" data-reveal="${esc(S.rootDir)}">${ic("external")}فتح المجلد</button></div></div>

    <div class="card">${cardHead("search", "مجلدات إضافية للبحث")}
      <p class="hint" style="margin-top:0">أضف مجلداتك القديمة ليشملها البحث. لن يتم تعديل أي شيء فيها.</p>
      <div id="extra" class="list"></div>
      <div class="row" style="margin-top:10px"><input id="s-add" class="in grow" dir="ltr" placeholder="مسار المجلد">${S.isWindows ? `<button class="btn sq" id="s-addpick" title="اختيار">${ic("folder")}</button>` : ""}<button class="btn" id="s-addbtn">${ic("plus")}إضافة</button></div></div>

    <div class="card">${cardHead("book", "المواد المعروضة")}
      <p class="hint" style="margin-top:0">أوقف المواد القديمة التي لا تريد تنزيل ملفاتها.</p>
      ${(S.courses || []).map((c) => `<label class="toggle"><span class="t"><b>${esc(c.name)}</b></span><span class="switch"><input type="checkbox" class="s-course" value="${c.id}" ${hidden.has(c.id) ? "" : "checked"}><i></i></span></label>`).join("") || '<div class="muted">لا توجد مواد بعد</div>'}</div>

    <div class="card">${cardHead("power", "البرنامج")}
      <dl class="kv"><dt>ملفات مفهرسة</dt><dd>${S.index.docs.toLocaleString("en")}</dd><dt>مثبّت</dt><dd>${S.installed ? "نعم" : "لا"}</dd></dl>
      <div class="row wrap" style="margin-top:14px"><button class="btn" id="s-rescan">${ic("refresh")}إعادة فحص الملفات</button>
      <button class="btn danger" id="s-quit">${ic("power")}إيقاف البرنامج</button></div></div>
  </div>
  <div class="err" id="s-err" style="margin-top:14px"></div>`;

  const drawExtra = () => {
    $("#extra").innerHTML = extra.map((d, i) => `<div class="item"><div class="kind-ic" style="background:var(--surface-2)">${ic("folder")}</div><div class="body small" dir="ltr" style="text-align:right">${esc(d)}</div>
      <div class="actions"><button class="btn sm ghost danger" data-rm="${i}">${ic("x")}</button></div></div>`).join("") || '<div class="muted small">لا توجد مجلدات إضافية</div>';
    $$("[data-rm]").forEach((b) => b.addEventListener("click", () => { extra.splice(+b.dataset.rm, 1); drawExtra(); }));
  };
  drawExtra();
  const addExtra = (p) => { p = p.trim(); if (p && !extra.includes(p)) extra.push(p); $("#s-add").value = ""; drawExtra(); };
  $$("#s-theme button").forEach((b) => b.addEventListener("click", () => { applyTheme(b.dataset.t, true); $$("#s-theme button").forEach((x) => x.classList.toggle("on", x === b)); }));
  $("#s-addbtn").addEventListener("click", () => addExtra($("#s-add").value));
  $("#s-addpick")?.addEventListener("click", async () => { const r = await api("/api/pickfolder", {}); if (r.path) addExtra(r.path); });
  $("#s-pick")?.addEventListener("click", async () => { const r = await api("/api/pickfolder", {}); if (r.path) $("#s-root").value = r.path; });
  $("#relink").addEventListener("click", () => go("setup"));
  $("#logout")?.addEventListener("click", async () => {
    if (!confirm("فصل حساب المودل؟ ملفاتك المنزّلة ستبقى كما هي.")) return;
    await api("/api/logout", {});
    await refresh();
    go("setup");
  });
  $("#s-check").addEventListener("click", async () => {
    try { const r = await api("/api/update/check", {}); toast(r.available ? `يتوفر الإصدار ${r.available.version}` : "أنت تستخدم أحدث إصدار", r.available ? "sparkles" : "check-circle"); }
    catch (e) { toast(e.message, "alert"); }
    await refresh(); views.settings(v);
  });
  $("#s-upd")?.addEventListener("click", startUpdate);
  $("#s-backup").addEventListener("click", async () => { await api("/api/backup", {}); toast("تم حفظ نسخة احتياطية من بياناتك", "archive"); });
  $("#s-rescan").addEventListener("click", async () => { await api("/api/rescan", {}); toast("بدأ فحص الملفات", "refresh"); });
  $("#s-quit").addEventListener("click", async () => {
    if (!confirm("إيقاف البرنامج؟ لن تصلك تنبيهات حتى تشغّله مرة أخرى.")) return;
    await api("/api/quit", {});
    document.body.innerHTML = `<div class="setup" style="margin-top:12vh"><div class="card" style="text-align:center;padding:40px"><img src="icon.svg" style="width:64px"><h2 style="justify-content:center;margin-top:12px">تم إيقاف البرنامج</h2><p class="muted">يمكنك إغلاق هذه الصفحة. لتشغيله مرة أخرى افتح «منظّم الجامعة» من قائمة ابدأ.</p></div></div>`;
  });
  $("#s-save").addEventListener("click", async () => {
    $("#s-err").textContent = "";
    if ($("#s-add").value.trim()) addExtra($("#s-add").value);
    try {
      await api("/api/settings", {
        rootDir: $("#s-root").value, extraDirs: extra, syncMinutes: +$("#s-min").value, toasts: $("#s-toast").checked,
        hidden: $$(".s-course").filter((c) => !c.checked).map((c) => +c.value),
        autostart: $("#s-auto") ? $("#s-auto").checked : undefined, autoUpdate: $("#s-autoupd").checked, repo: $("#s-repo").value,
      });
      toast("تم حفظ الإعدادات");
      await refresh();
      views.settings(v);
    } catch (e) { $("#s-err").textContent = e.message; toast(e.message, "alert"); }
  });
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
  if (!S.linked && !location.hash.match(/feedback|settings|search|import/)) go("setup");
  else if (!fromHash()) go("home", {}, true);
  setInterval(() => { if (!offline) refresh(); }, 5000);
})();
