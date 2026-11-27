package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed web/*
var webFS embed.FS

var apiToken = func() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}()

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func readBody(r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 20<<20)).Decode(v)
}

// allowedPath makes sure we only open files inside the user's folders.
func allowedPath(p string) bool {
	p = filepath.Clean(p)
	c := getConfig()
	for _, root := range append([]string{c.RootDir}, c.ExtraDirs...) {
		if root == "" {
			continue
		}
		root = filepath.Clean(root)
		if p == root || strings.HasPrefix(p, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func newServer(port int) http.Handler {
	mux := http.NewServeMux()

	page := func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		b, err := webFS.ReadFile("web/" + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		switch filepath.Ext(name) {
		case ".html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			b = []byte(strings.Replace(string(b), "__API_TOKEN__", apiToken, 1))
		case ".js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		case ".css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case ".svg":
			w.Header().Set("Content-Type", "image/svg+xml")
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Write(b)
	}
	mux.HandleFunc("/", page)

	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		c := getConfig()
		stMu.RLock()
		var recent []*FileRec
		for _, f := range st.Files {
			recent = append(recent, f)
		}
		sort.Slice(recent, func(i, j int) bool { return recent[i].Downloaded > recent[j].Downloaded })
		if len(recent) > 25 {
			recent = recent[:25]
		}
		newNotif := 0
		for _, n := range st.Notifications {
			if n.IsNew {
				newNotif++
			}
		}
		resp := map[string]any{
			"linked":        c.Token != "",
			"moodleUrl":     c.MoodleURL,
			"fullName":      c.FullName,
			"siteName":      c.SiteName,
			"username":      c.Username,
			"rootDir":       c.RootDir,
			"extraDirs":     c.ExtraDirs,
			"syncMinutes":   c.SyncMinutes,
			"toasts":        c.Toasts,
			"hidden":        c.HiddenCourses,
			"autostart":     getAutostart(),
			"courses":       st.Courses,
			"assignments":   st.Assignments,
			"notifications": st.Notifications,
			"newNotif":      newNotif,
			"events":        st.Events,
			"lastSync":      st.LastSync,
			"lastError":     st.LastError,
			"recent":        recent,
			"sync":          getStatus(),
			"index":         indexer.Status(),
			"now":           time.Now().Unix(),
			"isWindows":     isWindows,
			"version":       Version,
			"bootId":        bootID,
			"update":        getUpdateState(),
			"installed":     isInstalled(),
			"canInstall":    canInstall(),
			"theme":         c.Theme,
			"autoUpdate":    c.AutoUpdate,
			"repo":          currentRepo(),
			"repoBuiltIn":   UpdateRepo != "",
			"ghLogin":       c.GHLogin,
			"hasGH":         c.GHToken != "",
			"fbUnread":      unreadFeedback(),
			"links":         c.Links,
		}
		stMu.RUnlock()
		writeJSON(w, resp)
	})

	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ URL, Username, Password, Token string }
		if readBody(r, &req) != nil {
			writeErr(w, 400, "طلب غير صالح")
			return
		}
		base := normalizeBaseURL(req.URL)
		if base == "" {
			writeErr(w, 400, "أدخل رابط موقع المودل")
			return
		}
		token := strings.TrimSpace(req.Token)
		if token == "" {
			t, err := GetToken(base, strings.TrimSpace(req.Username), req.Password)
			if err != nil {
				writeErr(w, 400, err.Error())
				return
			}
			token = t
		}
		info, err := NewMoodle(base, token).SiteInfo()
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		updateConfig(func(c *Config) {
			if c.MoodleURL != base || c.UserID != info.UserID {
				// different account: start fresh
				stMu.Lock()
				st = State{Files: map[string]*FileRec{}, SeenNotif: map[int]bool{}, Reminded: map[int]bool{}}
				stMu.Unlock()
			}
			c.MoodleURL, c.Token, c.UserID, c.FullName, c.SiteName = base, token, info.UserID, info.FullName, info.SiteName
			c.Username = info.Username
		})
		saveState()
		go runSync()
		writeJSON(w, map[string]any{"ok": true, "fullName": info.FullName, "siteName": info.SiteName})
	})

	mux.HandleFunc("/api/logout", func(w http.ResponseWriter, r *http.Request) {
		updateConfig(func(c *Config) { c.Token = "" })
		writeJSON(w, map[string]any{"ok": true})
	})

	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RootDir     *string  `json:"rootDir"`
			ExtraDirs   []string `json:"extraDirs"`
			SyncMinutes *int     `json:"syncMinutes"`
			Toasts      *bool    `json:"toasts"`
			Hidden      []int    `json:"hidden"`
			Autostart   *bool    `json:"autostart"`
			Theme       *string  `json:"theme"`
			AutoUpdate  *bool    `json:"autoUpdate"`
			Repo        *string  `json:"repo"`
			Links       *[]Link  `json:"links"`
		}
		if readBody(r, &req) != nil {
			writeErr(w, 400, "طلب غير صالح")
			return
		}
		if req.RootDir != nil {
			p := strings.TrimSpace(*req.RootDir)
			if p == "" || !filepath.IsAbs(p) {
				writeErr(w, 400, "مسار مجلد غير صالح")
				return
			}
			if err := os.MkdirAll(p, 0o755); err != nil {
				writeErr(w, 400, "لا يمكن إنشاء المجلد: "+err.Error())
				return
			}
		}
		for _, d := range req.ExtraDirs {
			if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
				writeErr(w, 400, "المجلد غير موجود: "+d)
				return
			}
		}
		if req.Autostart != nil {
			setAutostart(*req.Autostart)
		}
		var links []Link
		if req.Links != nil {
			var err error
			if links, err = cleanLinks(*req.Links); err != nil {
				writeErr(w, 400, err.Error())
				return
			}
		}
		var repo string
		if req.Repo != nil && strings.TrimSpace(*req.Repo) != "" {
			if repo = normalizeRepo(*req.Repo); repo == "" {
				writeErr(w, 400, "اكتب المستودع بالشكل: اسم-المستخدم/اسم-المستودع")
				return
			}
		}
		updateConfig(func(c *Config) {
			if req.RootDir != nil {
				c.RootDir = filepath.Clean(strings.TrimSpace(*req.RootDir))
			}
			if req.ExtraDirs != nil {
				c.ExtraDirs = nil
				for _, d := range req.ExtraDirs {
					c.ExtraDirs = append(c.ExtraDirs, filepath.Clean(d))
				}
			}
			if req.SyncMinutes != nil && *req.SyncMinutes >= 5 {
				c.SyncMinutes = *req.SyncMinutes
			}
			if req.Toasts != nil {
				c.Toasts = *req.Toasts
			}
			if req.Hidden != nil {
				c.HiddenCourses = req.Hidden
			}
			if req.Theme != nil && (*req.Theme == "auto" || *req.Theme == "light" || *req.Theme == "dark") {
				c.Theme = *req.Theme
			}
			if req.AutoUpdate != nil {
				c.AutoUpdate = *req.AutoUpdate
			}
			if req.Repo != nil {
				c.Repo = repo
			}
			if req.Links != nil {
				c.Links = links
			}
		})
		if req.RootDir != nil || req.ExtraDirs != nil {
			go indexer.Rescan()
		}
		writeJSON(w, map[string]any{"ok": true})
	})

	mux.HandleFunc("/api/sync", func(w http.ResponseWriter, r *http.Request) {
		go runSync()
		writeJSON(w, map[string]any{"ok": true})
	})

	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		t0 := time.Now()
		hits, total := indexer.Search(q.Get("q"), q.Get("course"), q.Get("type"), 60)
		writeJSON(w, map[string]any{"hits": hits, "total": total, "ms": float64(time.Since(t0).Microseconds()) / 1000})
	})

	mux.HandleFunc("/api/courses-index", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, indexer.Courses())
	})

	mux.HandleFunc("/api/course", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(r.URL.Query().Get("id"))
		stMu.RLock()
		var course *CourseRec
		for i := range st.Courses {
			if st.Courses[i].ID == id {
				c := st.Courses[i]
				course = &c
			}
		}
		var assigns []AssignRec
		for _, a := range st.Assignments {
			if a.CourseID == id {
				assigns = append(assigns, a)
			}
		}
		stMu.RUnlock()
		if course == nil {
			writeErr(w, 404, "المادة غير موجودة")
			return
		}
		writeJSON(w, map[string]any{"course": course, "files": indexer.FilesUnder(course.Folder), "assignments": assigns})
	})

	mux.HandleFunc("/api/folder-files", func(w http.ResponseWriter, r *http.Request) {
		dir := r.URL.Query().Get("dir")
		if !allowedPath(dir) {
			writeErr(w, 403, "غير مسموح")
			return
		}
		writeJSON(w, indexer.FilesUnder(dir))
	})

	mux.HandleFunc("/api/open", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path   string `json:"path"`
			Reveal bool   `json:"reveal"`
		}
		if readBody(r, &req) != nil || !allowedPath(req.Path) {
			writeErr(w, 403, "لا يمكن فتح هذا المسار")
			return
		}
		if _, err := os.Stat(req.Path); err != nil {
			writeErr(w, 404, "الملف لم يعد موجوداً (ربما نُقل أو حُذف)")
			return
		}
		var err error
		if req.Reveal {
			err = revealPath(req.Path)
		} else {
			err = openPath(req.Path)
		}
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})

	mux.HandleFunc("/api/openurl", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ URL string }
		if readBody(r, &req) != nil || !(strings.HasPrefix(req.URL, "https://") || strings.HasPrefix(req.URL, "http://")) {
			writeErr(w, 400, "رابط غير صالح")
			return
		}
		openURL(req.URL)
		writeJSON(w, map[string]any{"ok": true})
	})

	mux.HandleFunc("/api/pickfolder", func(w http.ResponseWriter, r *http.Request) {
		p, err := pickFolder()
		if err != nil {
			writeJSON(w, map[string]any{"path": ""})
			return
		}
		writeJSON(w, map[string]any{"path": p})
	})

	mux.HandleFunc("/api/import/preview", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Src, Course string }
		if readBody(r, &req) != nil || req.Src == "" || strings.TrimSpace(req.Course) == "" {
			writeErr(w, 400, "اختر المجلد واكتب اسم المادة")
			return
		}
		if fi, err := os.Stat(req.Src); err != nil || !fi.IsDir() {
			writeErr(w, 400, "المجلد غير موجود")
			return
		}
		items, err := previewImport(req.Src, req.Course)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"items": items})
	})

	mux.HandleFunc("/api/import/run", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Items []ImportItem }
		if readBody(r, &req) != nil {
			writeErr(w, 400, "طلب غير صالح")
			return
		}
		n, errs := runImport(req.Items)
		writeJSON(w, map[string]any{"copied": n, "errors": errs})
	})

	mux.HandleFunc("/api/notifications/seen", func(w http.ResponseWriter, r *http.Request) {
		stMu.Lock()
		for i := range st.Notifications {
			st.Notifications[i].IsNew = false
		}
		stMu.Unlock()
		saveState()
		writeJSON(w, map[string]any{"ok": true})
	})

	mux.HandleFunc("/api/rescan", func(w http.ResponseWriter, r *http.Request) {
		go indexer.Rescan()
		writeJSON(w, map[string]any{"ok": true})
	})

	mux.HandleFunc("/api/quit", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true})
		go func() {
			time.Sleep(300 * time.Millisecond)
			indexer.Save()
			saveState()
			os.Exit(0)
		}()
	})

	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"app": "unihub", "version": Version, "bootId": bootID})
	})

	// ----- updates & install -----
	mux.HandleFunc("/api/update/check", func(w http.ResponseWriter, r *http.Request) {
		info, err := checkUpdate()
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, map[string]any{"available": info})
	})
	mux.HandleFunc("/api/update/apply", func(w http.ResponseWriter, r *http.Request) {
		go func() {
			if err := applyUpdate(); err != nil {
				log.Println("update:", err)
			}
		}()
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/update/dismiss-notes", func(w http.ResponseWriter, r *http.Request) {
		setUpd(func(u *UpdateState) { u.JustUpdated = ""; u.Notes = "" })
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/install", func(w http.ResponseWriter, r *http.Request) {
		if err := installSelf(); err != nil {
			writeErr(w, 500, "تعذر التثبيت: "+err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/backup", func(w http.ResponseWriter, r *http.Request) {
		backupData(Version)
		dir := filepath.Join(dataDir, "backups")
		revealPath(dir)
		writeJSON(w, map[string]any{"ok": true, "dir": dir})
	})

	// ----- technical feedback -----
	mux.HandleFunc("/api/feedback", func(w http.ResponseWriter, r *http.Request) {
		snap := feedbackSnapshot()
		c := getConfig()
		writeJSON(w, map[string]any{"outbox": snap.Outbox, "threads": snap.Threads, "unread": snap.Unread,
			"lastFetch": snap.LastFetch, "lastError": snap.LastError, "repo": currentRepo(),
			"login": c.GHLogin, "hasToken": c.GHToken != ""})
	})
	mux.HandleFunc("/api/feedback/connect", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Token, Repo string }
		if readBody(r, &req) != nil {
			writeErr(w, 400, "طلب غير صالح")
			return
		}
		repo := normalizeRepo(req.Repo)
		if repo == "" {
			repo = currentRepo()
		}
		if repo == "" {
			writeErr(w, 400, "اكتب اسم المستودع بالشكل: اسم-المستخدم/اسم-المستودع")
			return
		}
		token := strings.TrimSpace(req.Token)
		if token == "" {
			writeErr(w, 400, "الصق مفتاح GitHub")
			return
		}
		login, canManage, err := verifyGitHub(token, repo)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		updateConfig(func(c *Config) {
			c.GHToken, c.GHLogin = token, login
			if repo != UpdateRepo || c.Repo != "" {
				c.Repo = repo
			}
		})
		go func() { refreshThreads(); flushOutbox() }()
		writeJSON(w, map[string]any{"ok": true, "login": login, "canManage": canManage})
	})
	mux.HandleFunc("/api/feedback/disconnect", func(w http.ResponseWriter, r *http.Request) {
		updateConfig(func(c *Config) { c.GHToken, c.GHLogin = "", "" })
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/feedback/send", func(w http.ResponseWriter, r *http.Request) {
		var it FBItem
		if readBody(r, &it) != nil || strings.TrimSpace(it.Title) == "" || strings.TrimSpace(it.Body) == "" {
			writeErr(w, 400, "اكتب عنواناً ووصفاً للملاحظة")
			return
		}
		it.Title = truncate(strings.TrimSpace(it.Title), 120)
		if it.Kind == "" {
			it.Kind = "اقتراح"
		}
		if it.Priority == "" {
			it.Priority = "عادية"
		}
		queueFeedback(it)
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/feedback/discard", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ LocalID string }
		readBody(r, &req)
		fbMu.Lock()
		for i, it := range fb.Outbox {
			if it.LocalID == req.LocalID {
				fb.Outbox = append(fb.Outbox[:i], fb.Outbox[i+1:]...)
				break
			}
		}
		fbMu.Unlock()
		saveFeedback()
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/feedback/retry", func(w http.ResponseWriter, r *http.Request) {
		fbMu.Lock()
		for i := range fb.Outbox {
			fb.Outbox[i].Status, fb.Outbox[i].Error = "pending", ""
		}
		fbMu.Unlock()
		n := flushOutbox()
		writeJSON(w, map[string]any{"sent": n})
	})
	mux.HandleFunc("/api/feedback/refresh", func(w http.ResponseWriter, r *http.Request) {
		flushOutbox()
		if err := refreshThreads(); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/feedback/thread", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.URL.Query().Get("n"))
		cs, err := threadComments(n)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, map[string]any{"comments": cs})
	})
	mux.HandleFunc("/api/feedback/reply", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			N    int
			Body string
		}
		if readBody(r, &req) != nil || strings.TrimSpace(req.Body) == "" {
			writeErr(w, 400, "اكتب الرد")
			return
		}
		if err := replyThread(req.N, strings.TrimSpace(req.Body)); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/feedback/state", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			N     int
			State string
		}
		if readBody(r, &req) != nil || (req.State != "open" && req.State != "closed") {
			writeErr(w, 400, "طلب غير صالح")
			return
		}
		if err := setThreadState(req.N, req.State); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/feedback/export", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"text": exportForClaude()})
	})

	// security: only accept requests addressed to localhost, and API calls carrying our token
	hostOK := map[string]bool{
		"127.0.0.1:" + strconv.Itoa(port): true,
		"localhost:" + strconv.Itoa(port): true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hostOK[r.Host] {
			http.Error(w, "forbidden", 403)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/ping" {
			if r.Header.Get("X-UniHub-Token") != apiToken {
				writeErr(w, 403, "انتهت الجلسة، أعد تحميل الصفحة")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func listen(port int) (net.Listener, error) {
	return net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
}
