package main

import (
	"fmt"
	"html"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	CatLectures = "محاضرات"
	CatAssign   = "واجبات"
	CatExams    = "امتحانات"
	CatOther    = "ملفات أخرى"
)

var allCategories = []string{CatLectures, CatAssign, CatExams, CatOther}

type FileRec struct {
	Key          string `json:"key"`
	LocalPath    string `json:"localPath"`
	CourseID     int    `json:"courseId"`
	Course       string `json:"course"`
	Category     string `json:"category"`
	Name         string `json:"name"`
	Section      string `json:"section"`
	TimeModified int64  `json:"timeModified"`
	Size         int64  `json:"size"`
	Downloaded   int64  `json:"downloaded"`
}

type CourseRec struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"shortName"`
	Folder    string `json:"folder"`
	URL       string `json:"url"`
}

type AssignRec struct {
	ID       int    `json:"id"`
	CmID     int    `json:"cmid"`
	CourseID int    `json:"courseId"`
	Course   string `json:"course"`
	Name     string `json:"name"`
	Due      int64  `json:"due"`
	Cutoff   int64  `json:"cutoff"`
	Status   string `json:"status"`
	URL      string `json:"url"`
	Folder   string `json:"folder"`
	Intro    string `json:"intro"`
}

type NotifRec struct {
	ID      int    `json:"id"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	URL     string `json:"url"`
	Time    int64  `json:"time"`
	Read    bool   `json:"read"`
	IsNew   bool   `json:"isNew"`
}

type EventRec struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Time   int64  `json:"time"`
	Course string `json:"course"`
	Kind   string `json:"kind"`
	URL    string `json:"url"`
}

type State struct {
	Files         map[string]*FileRec `json:"files"`
	Courses       []CourseRec         `json:"courses"`
	Assignments   []AssignRec         `json:"assignments"`
	Notifications []NotifRec          `json:"notifications"`
	Events        []EventRec          `json:"events"`
	SeenNotif     map[int]bool        `json:"seenNotif"`
	Reminded      map[int]bool        `json:"reminded"`
	LastSync      int64               `json:"lastSync"`
	LastError     string              `json:"lastError"`
	FirstSyncDone bool                `json:"firstSyncDone"`
}

type SyncStatus struct {
	Running  bool   `json:"running"`
	Phase    string `json:"phase"`
	Done     int    `json:"done"`
	Total    int    `json:"total"`
	NewFiles int    `json:"newFiles"`
}

var (
	stMu     sync.RWMutex
	st       State
	syncMu   sync.Mutex // only one sync at a time
	statMu   sync.RWMutex
	syncStat SyncStatus
)

func loadState() {
	stMu.Lock()
	defer stMu.Unlock()
	st = State{}
	loadJSON("state.json", &st)
	if st.Files == nil {
		st.Files = map[string]*FileRec{}
	}
	if st.SeenNotif == nil {
		st.SeenNotif = map[int]bool{}
	}
	if st.Reminded == nil {
		st.Reminded = map[int]bool{}
	}
}

func saveState() {
	stMu.RLock()
	defer stMu.RUnlock()
	if err := saveJSON("state.json", &st); err != nil {
		log.Println("save state:", err)
	}
}

func setStatus(f func(s *SyncStatus)) {
	statMu.Lock()
	f(&syncStat)
	statMu.Unlock()
}

func getStatus() SyncStatus {
	statMu.RLock()
	defer statMu.RUnlock()
	return syncStat
}

// ---------- naming & classification ----------

var badChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

func sanitizeName(s string) string {
	s = html.UnescapeString(s)
	s = badChars.ReplaceAllString(s, "-")
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimRight(s, ". ")
	r := []rune(s)
	if len(r) > 80 {
		s = strings.TrimRight(string(r[:80]), ". ")
	}
	if s == "" {
		s = "بدون اسم"
	}
	up := strings.ToUpper(s)
	for _, res := range []string{"CON", "PRN", "AUX", "NUL", "COM1", "LPT1"} {
		if up == res {
			s = "_" + s
		}
	}
	return s
}

var (
	kwExams  = []string{"امتحان", "امتحانات", "اختبار", "اختبارات", "كويز", "quiz", "exam", "midterm", "mid", "final", "نهائي", "نصفي", "فاينل", "ميد", "نموذج", "نماذج", "سنوات"}
	kwAssign = []string{"واجب", "واجبات", "تكليف", "تكاليف", "assignment", "assign", "homework", "hw", "مشروع", "project", "sheet", "شيت", "تمرين", "تمارين", "exercise"}
	kwOther  = []string{".zip", ".rar", ".7z", ".exe", ".msi", ".iso"}
)

func hasKeyword(text string, kws []string) bool {
	for _, tok := range tokenize(normalizeText(text)) {
		t := stripArticle(tok)
		for _, k := range kws {
			k = normalizeText(k)
			if strings.HasPrefix(tok, k) || strings.HasPrefix(t, k) {
				return true
			}
		}
	}
	return false
}

func classify(modname, moduleName, fileName, sectionName string) string {
	low := strings.ToLower(fileName)
	for _, ext := range kwOther {
		if strings.HasSuffix(low, ext) {
			return CatOther
		}
	}
	if modname == "assign" {
		return CatAssign
	}
	if modname == "quiz" {
		return CatExams
	}
	name := moduleName + " " + strings.TrimSuffix(fileName, filepath.Ext(fileName))
	if hasKeyword(name, kwExams) {
		return CatExams
	}
	if hasKeyword(name, kwAssign) {
		return CatAssign
	}
	if hasKeyword(sectionName, kwExams) {
		return CatExams
	}
	if hasKeyword(sectionName, kwAssign) {
		return CatAssign
	}
	return CatLectures
}

func fileKey(u string) string {
	if p, err := url.Parse(u); err == nil {
		p.RawQuery = ""
		return p.String()
	}
	return u
}

// uniquePath returns a free path next to p by adding (2), (3)...
func uniquePath(p string) string {
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return p
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(c); os.IsNotExist(err) {
			return c
		}
	}
}

func datePrefix(t int64) string {
	if t <= 0 {
		return ""
	}
	return time.Unix(t, 0).Format("2006-01-02") + " - "
}

// ---------- the sync ----------

func runSync() error {
	if !syncMu.TryLock() {
		return fmt.Errorf("المزامنة تعمل حالياً")
	}
	defer syncMu.Unlock()
	c := getConfig()
	if c.Token == "" || c.MoodleURL == "" {
		return fmt.Errorf("لم يتم ربط حساب المودل بعد")
	}
	setStatus(func(s *SyncStatus) { *s = SyncStatus{Running: true, Phase: "الاتصال بالمودل"} })
	defer setStatus(func(s *SyncStatus) { s.Running = false; s.Phase = "" })

	err := doSync(c)
	stMu.Lock()
	st.LastSync = time.Now().Unix()
	if err != nil {
		st.LastError = err.Error()
	} else {
		st.LastError = ""
	}
	stMu.Unlock()
	saveState()
	go indexer.Rescan()
	return err
}

func doSync(c Config) error {
	m := NewMoodle(c.MoodleURL, c.Token)
	info, err := m.SiteInfo()
	if err != nil {
		return err
	}
	if info.UserID != c.UserID || info.FullName != c.FullName {
		updateConfig(func(x *Config) { x.UserID = info.UserID; x.FullName = info.FullName; x.SiteName = info.SiteName })
		c.UserID = info.UserID
	}
	courses, err := m.Courses(c.UserID)
	if err != nil {
		return err
	}
	hidden := map[int]bool{}
	for _, id := range c.HiddenCourses {
		hidden[id] = true
	}
	stMu.RLock()
	firstSync := !st.FirstSyncDone
	stMu.RUnlock()

	var courseRecs []CourseRec
	var activeIDs []int
	for _, co := range courses {
		folder := filepath.Join(c.RootDir, sanitizeName(co.FullName))
		courseRecs = append(courseRecs, CourseRec{ID: co.ID, Name: html.UnescapeString(co.FullName), ShortName: co.ShortName,
			Folder: folder, URL: fmt.Sprintf("%s/course/view.php?id=%d", c.MoodleURL, co.ID)})
		if !hidden[co.ID] {
			activeIDs = append(activeIDs, co.ID)
		}
	}
	stMu.Lock()
	st.Courses = courseRecs
	stMu.Unlock()

	var newFiles []string
	var errs []string
	setStatus(func(s *SyncStatus) {
		s.Phase = "تنزيل المحاضرات والملفات"
		s.Total = len(activeIDs)
	})
	for i, cr := range courseRecs {
		if hidden[cr.ID] {
			continue
		}
		setStatus(func(s *SyncStatus) { s.Phase = "مادة: " + cr.Name })
		for _, cat := range allCategories {
			os.MkdirAll(filepath.Join(cr.Folder, cat), 0o755)
		}
		secs, err := m.CourseContents(cr.ID)
		if err != nil {
			errs = append(errs, cr.Name+": "+err.Error())
			continue
		}
		for _, sec := range secs {
			for _, mod := range sec.Modules {
				switch mod.ModName {
				case "assign", "url", "label", "forum", "quiz", "page", "lti", "choice", "feedback", "attendance":
					continue
				}
				for _, f := range mod.Contents {
					if f.Type != "file" || f.FileURL == "" {
						continue
					}
					name, err := syncFile(m, cr, sec, mod, f)
					if err != nil {
						errs = append(errs, f.FileName+": "+err.Error())
					} else if name != "" {
						newFiles = append(newFiles, name)
						setStatus(func(s *SyncStatus) { s.NewFiles++ })
					}
				}
			}
		}
		setStatus(func(s *SyncStatus) { s.Done = i + 1 })
		saveState()
	}

	// assignments
	setStatus(func(s *SyncStatus) { s.Phase = "الواجبات" })
	if len(activeIDs) > 0 {
		if am, err := m.Assignments(activeIDs); err != nil {
			errs = append(errs, "الواجبات: "+err.Error())
		} else {
			newFiles = append(newFiles, syncAssignments(m, c, courseRecs, am)...)
		}
	}

	// calendar
	if evs, err := m.UpcomingEvents(); err == nil {
		var recs []EventRec
		for _, e := range evs {
			recs = append(recs, EventRec{ID: e.ID, Name: html.UnescapeString(e.Name), Time: e.TimeSort,
				Course: html.UnescapeString(e.Course.FullName), Kind: e.ModuleName, URL: e.URL})
		}
		stMu.Lock()
		st.Events = recs
		stMu.Unlock()
	}

	// notifications
	setStatus(func(s *SyncStatus) { s.Phase = "الإشعارات" })
	if ns, err := m.Notifications(c.UserID); err == nil {
		var recs []NotifRec
		var fresh []NotifRec
		stMu.Lock()
		for _, n := range ns {
			text := n.Small
			if text == "" {
				text = n.Full
			}
			if text == "" {
				text = stripHTML(n.FullHTML)
			}
			r := NotifRec{ID: n.ID, Subject: html.UnescapeString(n.Subject), Text: strings.TrimSpace(stripHTML(text)),
				URL: n.ContextURL, Time: n.TimeCreated, Read: n.Read}
			if !st.SeenNotif[n.ID] {
				r.IsNew = true
				st.SeenNotif[n.ID] = true
				if !firstSync {
					fresh = append(fresh, r)
				}
			} else {
				for _, old := range st.Notifications {
					if old.ID == n.ID {
						r.IsNew = old.IsNew
					}
				}
			}
			recs = append(recs, r)
		}
		st.Notifications = recs
		stMu.Unlock()
		if c.Toasts {
			for i, n := range fresh {
				if i >= 3 {
					notify("المودل", fmt.Sprintf("و %d إشعارات أخرى", len(fresh)-3))
					break
				}
				notify(n.Subject, truncate(n.Text, 120))
			}
		}
	}

	// reminders for deadlines within 24 hours
	if c.Toasts {
		now := time.Now().Unix()
		stMu.Lock()
		var due []AssignRec
		for _, a := range st.Assignments {
			if a.Due > now && a.Due-now < 24*3600 && a.Status != "submitted" && !st.Reminded[a.ID] {
				st.Reminded[a.ID] = true
				due = append(due, a)
			}
		}
		stMu.Unlock()
		for _, a := range due {
			notify("⏰ موعد تسليم قريب", a.Name+" - "+a.Course+"\nالتسليم: "+time.Unix(a.Due, 0).Format("2006-01-02 15:04"))
		}
	}

	if len(newFiles) > 0 && !firstSync && c.Toasts {
		body := strings.Join(newFiles[:min(3, len(newFiles))], "\n")
		notify(fmt.Sprintf("📥 %d ملفات جديدة من المودل", len(newFiles)), body)
	}

	stMu.Lock()
	st.FirstSyncDone = true
	stMu.Unlock()
	if len(errs) > 0 {
		log.Println("sync errors:", errs)
		if len(errs) > 3 {
			errs = append(errs[:3], fmt.Sprintf("و %d أخطاء أخرى", len(errs)-3))
		}
		return fmt.Errorf("تمت المزامنة مع بعض الأخطاء: %s", strings.Join(errs, " | "))
	}
	return nil
}

// syncFile downloads one file if it is new or changed. Returns the file name if downloaded.
func syncFile(m *Moodle, cr CourseRec, sec Section, mod Module, f ContentFile) (string, error) {
	key := fileKey(f.FileURL)
	stMu.RLock()
	rec := st.Files[key]
	stMu.RUnlock()
	if rec != nil && rec.TimeModified == f.TimeModified {
		return "", nil // already have it (never re-download files the user deleted or moved)
	}
	cat := classify(mod.ModName, mod.Name, f.FileName, sec.Name)
	t := f.TimeCreated
	if t <= 0 {
		t = f.TimeModified
	}
	dir := filepath.Join(cr.Folder, cat)
	if mod.ModName == "folder" {
		dir = filepath.Join(dir, sanitizeName(mod.Name))
		if sub := strings.Trim(f.FilePath, "/"); sub != "" {
			for _, p := range strings.Split(sub, "/") {
				dir = filepath.Join(dir, sanitizeName(p))
			}
		}
	}
	fname := sanitizeName(f.FileName)
	if mod.ModName != "folder" {
		fname = datePrefix(t) + fname
	}
	dest := filepath.Join(dir, fname)
	if rec != nil {
		// teacher updated the file: keep the old one, save the new version next to it
		ext := filepath.Ext(dest)
		dest = strings.TrimSuffix(dest, ext) + " (نسخة محدثة " + time.Unix(f.TimeModified, 0).Format("2006-01-02") + ")" + ext
	}
	if fi, err := os.Stat(dest); err == nil && fi.Size() == f.FileSize {
		// identical file already there (e.g. state was reset) - adopt it
	} else {
		dest = uniquePath(dest)
		if err := m.Download(f.FileURL, dest); err != nil {
			return "", err
		}
		os.Chtimes(dest, time.Now(), time.Unix(max64(f.TimeModified, 1), 0))
	}
	stMu.Lock()
	st.Files[key] = &FileRec{Key: key, LocalPath: dest, CourseID: cr.ID, Course: cr.Name, Category: cat,
		Name: filepath.Base(dest), Section: html.UnescapeString(sec.Name), TimeModified: f.TimeModified, Size: f.FileSize,
		Downloaded: time.Now().Unix()}
	stMu.Unlock()
	return filepath.Base(dest), nil
}

func syncAssignments(m *Moodle, c Config, courses []CourseRec, am map[int][]Assignment) []string {
	byID := map[int]CourseRec{}
	for _, cr := range courses {
		byID[cr.ID] = cr
	}
	stMu.RLock()
	oldStatus := map[int]string{}
	for _, a := range st.Assignments {
		oldStatus[a.ID] = a.Status
	}
	stMu.RUnlock()

	var newFiles []string
	var recs []AssignRec
	now := time.Now().Unix()
	for cid, list := range am {
		cr, ok := byID[cid]
		if !ok {
			continue
		}
		for _, a := range list {
			folder := filepath.Join(cr.Folder, CatAssign, sanitizeName(datePrefix(a.DueDate)+a.Name))
			// keep an existing folder name even if the due date changed
			stMu.RLock()
			for _, old := range st.Assignments {
				if old.ID == a.ID && old.Folder != "" {
					folder = old.Folder
				}
			}
			stMu.RUnlock()
			os.MkdirAll(filepath.Join(folder, "حلّي"), 0o755)
			intro := strings.TrimSpace(stripHTML(a.Intro))
			writeAssignInfo(folder, cr, a, c.MoodleURL)
			atts := append(append([]ContentFile{}, a.IntroAttachments...), a.IntroFiles...)
			for _, f := range atts {
				if f.FileURL == "" {
					continue
				}
				key := fileKey(f.FileURL)
				stMu.RLock()
				rec := st.Files[key]
				stMu.RUnlock()
				if rec != nil && rec.TimeModified == f.TimeModified {
					continue
				}
				dest := filepath.Join(folder, "ملفات الواجب", sanitizeName(f.FileName))
				if fi, err := os.Stat(dest); !(err == nil && fi.Size() == f.FileSize) {
					dest = uniquePath(dest)
					if err := m.Download(f.FileURL, dest); err != nil {
						log.Println("assign file:", err)
						continue
					}
				}
				newFiles = append(newFiles, filepath.Base(dest))
				stMu.Lock()
				st.Files[key] = &FileRec{Key: key, LocalPath: dest, CourseID: cr.ID, Course: cr.Name, Category: CatAssign,
					Name: filepath.Base(dest), Section: a.Name, TimeModified: f.TimeModified, Size: f.FileSize, Downloaded: time.Now().Unix()}
				stMu.Unlock()
			}
			status := oldStatus[a.ID]
			// only ask about assignments that still matter, to keep the sync fast
			if status != "submitted" && (a.DueDate == 0 || a.DueDate > now-14*24*3600) {
				if s, err := m.SubmissionStatus(a.ID); err == nil {
					status = s
				}
			}
			recs = append(recs, AssignRec{ID: a.ID, CmID: a.CmID, CourseID: cr.ID, Course: cr.Name,
				Name: html.UnescapeString(a.Name), Due: a.DueDate, Cutoff: a.CutoffDate, Status: status,
				URL: fmt.Sprintf("%s/mod/assign/view.php?id=%d", c.MoodleURL, a.CmID), Folder: folder, Intro: truncate(intro, 600)})
		}
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].Due < recs[j].Due })
	stMu.Lock()
	st.Assignments = recs
	stMu.Unlock()
	return newFiles
}

func writeAssignInfo(folder string, cr CourseRec, a Assignment, base string) {
	due := "بدون موعد محدد"
	if a.DueDate > 0 {
		due = time.Unix(a.DueDate, 0).Format("2006-01-02 الساعة 15:04")
	}
	page := fmt.Sprintf(`<!DOCTYPE html><html dir="rtl" lang="ar"><head><meta charset="utf-8"><title>%s</title>
<style>body{font-family:Segoe UI,Tahoma,sans-serif;max-width:800px;margin:40px auto;line-height:1.8;padding:0 20px}
.due{background:#fff3cd;padding:12px 16px;border-radius:8px;font-weight:bold}a{color:#2b59c3}</style></head><body>
<h1>%s</h1><p>المادة: %s</p><p class="due">موعد التسليم: %s</p><div>%s</div>
<p><a href="%s/mod/assign/view.php?id=%d">فتح الواجب في المودل للتسليم</a></p>
<p style="color:#888">ضع حلّك في مجلد (حلّي) الموجود بجانب هذا الملف حتى لا يضيع.</p></body></html>`,
		html.EscapeString(a.Name), html.EscapeString(a.Name), html.EscapeString(cr.Name), due, a.Intro, base, a.CmID)
	os.WriteFile(filepath.Join(folder, "تفاصيل الواجب.html"), []byte(page), 0o644)
}

var (
	tagRe      = regexp.MustCompile(`(?s)<[^>]*>`)
	styleRe    = regexp.MustCompile(`(?is)<(style|script)[^>]*>.*?</(style|script)>`)
	blockEndRe = regexp.MustCompile(`(?i)<(br|/p|/div|/h[1-6]|/li|/tr|/title)[^>]*>`)
)

func stripHTML(s string) string {
	s = styleRe.ReplaceAllString(s, " ")
	s = blockEndRe.ReplaceAllStringFunc(s, func(m string) string { return m + "\n" })
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	lines := strings.Split(s, "\n")
	var out []string
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// scheduler runs the sync periodically.
func scheduler() {
	time.Sleep(3 * time.Second)
	for {
		c := getConfig()
		if c.Token != "" {
			if err := runSync(); err != nil {
				log.Println("sync:", err)
			}
		}
		time.Sleep(time.Duration(getConfig().SyncMinutes) * time.Minute)
	}
}
