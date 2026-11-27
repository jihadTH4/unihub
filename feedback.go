package main

// Technical feedback between the two users, stored as GitHub Issues in the project's repo.
// Notes are queued locally (outbox) and sent whenever the internet is available.

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const fbPrefix = "[ملاحظة"

type FBItem struct {
	LocalID  string `json:"localId"`
	Kind     string `json:"kind"`
	Priority string `json:"priority"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Diag     bool   `json:"diag"`
	Created  int64  `json:"created"`
	Status   string `json:"status"` // pending | error
	Error    string `json:"error"`
}

type FBThread struct {
	Number   int    `json:"number"`
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	Body     string `json:"body"`
	State    string `json:"state"`
	Author   string `json:"author"`
	Avatar   string `json:"avatar"`
	Created  string `json:"created"`
	Updated  string `json:"updated"`
	Comments int    `json:"comments"`
	URL      string `json:"url"`
}

type FBSeen struct {
	Comments int    `json:"comments"`
	State    string `json:"state"`
}

type FBComment struct {
	Author  string `json:"author"`
	Avatar  string `json:"avatar"`
	Body    string `json:"body"`
	Created string `json:"created"`
}

type FeedbackStore struct {
	Outbox    []FBItem       `json:"outbox"`
	Threads   []FBThread     `json:"threads"`
	Seen      map[int]FBSeen `json:"seen"`
	Unread    map[int]bool   `json:"unread"`
	LastFetch int64          `json:"lastFetch"`
	LastError string         `json:"lastError"`
	Fetched   bool           `json:"fetched"`
}

var (
	fbMu    sync.Mutex
	fb      FeedbackStore
	fbFlush = make(chan struct{}, 1)
)

func loadFeedback() {
	fbMu.Lock()
	defer fbMu.Unlock()
	loadJSON("feedback.json", &fb)
	if fb.Seen == nil {
		fb.Seen = map[int]FBSeen{}
	}
	if fb.Unread == nil {
		fb.Unread = map[int]bool{}
	}
}

func saveFeedback() {
	fbMu.Lock()
	defer fbMu.Unlock()
	saveJSON("feedback.json", &fb)
}

func feedbackSnapshot() FeedbackStore {
	fbMu.Lock()
	defer fbMu.Unlock()
	s := fb
	s.Outbox = append([]FBItem(nil), fb.Outbox...)
	s.Threads = append([]FBThread(nil), fb.Threads...)
	s.Unread = map[int]bool{}
	for k, v := range fb.Unread {
		s.Unread[k] = v
	}
	return s
}

func unreadFeedback() int {
	fbMu.Lock()
	defer fbMu.Unlock()
	return len(fb.Unread)
}

// ---------- GitHub calls ----------

func ghDo(method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, ghAPI()+path, rd)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "UniHub/"+Version)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if t := getConfig().GHToken; t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return errOffline
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		switch resp.StatusCode {
		case 401:
			return errors.New("مفتاح GitHub غير صالح أو انتهت صلاحيته")
		case 403:
			return errors.New("لا تملك صلاحية لهذا الإجراء على المستودع")
		case 404:
			return errors.New("المستودع غير موجود أو لا تملك صلاحية الوصول إليه")
		case 410:
			return errors.New("خاصية Issues معطلة في المستودع، فعّلها من إعدادات المستودع")
		}
		return fmt.Errorf("GitHub: HTTP %d", resp.StatusCode)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

var errOffline = errors.New("لا يوجد اتصال بالإنترنت")

type ghUser struct {
	Login  string `json:"login"`
	Avatar string `json:"avatar_url"`
}

type ghIssue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	HTMLURL   string    `json:"html_url"`
	Comments  int       `json:"comments"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
	User      ghUser    `json:"user"`
	PR        *struct{} `json:"pull_request"`
}

// verifyGitHub checks a token and the repository, returning the login and whether the user can manage issues.
func verifyGitHub(token, repo string) (string, bool, error) {
	old := getConfig().GHToken
	cfgMu.Lock()
	cfg.GHToken = token
	cfgMu.Unlock()
	defer func() {
		cfgMu.Lock()
		cfg.GHToken = old
		cfgMu.Unlock()
	}()
	var u ghUser
	if err := ghDo("GET", "/user", nil, &u); err != nil {
		return "", false, err
	}
	var r struct {
		HasIssues   bool `json:"has_issues"`
		Permissions struct {
			Push bool `json:"push"`
		} `json:"permissions"`
	}
	if err := ghDo("GET", "/repos/"+repo, nil, &r); err != nil {
		return "", false, err
	}
	if !r.HasIssues {
		return "", false, errors.New("خاصية Issues معطلة في المستودع، فعّلها من Settings ثم Features")
	}
	return u.Login, r.Permissions.Push, nil
}

var titleRe = regexp.MustCompile(`^\[ملاحظة(?::([^\]]+))?\]\s*(.*)$`)

func parseTitle(t string) (kind, title string, ok bool) {
	m := titleRe.FindStringSubmatch(strings.TrimSpace(t))
	if m == nil {
		return "", t, false
	}
	return m[1], m[2], true
}

// publicBody hides the technical details block when showing a note in the app.
func publicBody(b string) string {
	if i := strings.Index(b, "\n---\n"); i >= 0 {
		b = b[:i]
	}
	if i := strings.Index(b, "<!--"); i >= 0 {
		b = b[:i]
	}
	return strings.TrimSpace(b)
}

var secretRe = regexp.MustCompile(`(token=|wstoken=|Bearer )[^&\s"']+`)

func redact(s string) string { return secretRe.ReplaceAllString(s, "${1}***") }

func logTail(n int) string {
	b, err := os.ReadFile(filepath.Join(dataDir, "unihub.log"))
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return redact(strings.Join(lines, "\n"))
}

func composeBody(it FBItem) string {
	var sb strings.Builder
	sb.WriteString(strings.TrimSpace(it.Body))
	sb.WriteString("\n\n---\n")
	sb.WriteString(fmt.Sprintf("**النوع:** %s · **الأولوية:** %s · **الإصدار:** %s\n", it.Kind, it.Priority, Version))
	if it.Diag {
		stMu.RLock()
		lastErr, courses, lastSync := st.LastError, len(st.Courses), st.LastSync
		stMu.RUnlock()
		sb.WriteString("\n<details><summary>معلومات تقنية</summary>\n\n")
		sb.WriteString(fmt.Sprintf("- الإصدار: `%s`\n- النظام: `%s` (%s)\n- عدد المواد: %d\n- الملفات المفهرسة: %d\n",
			Version, osVersion(), runtime.GOARCH, courses, indexer.Status().Docs))
		if lastSync > 0 {
			sb.WriteString("- آخر مزامنة: " + time.Unix(lastSync, 0).Format("2006-01-02 15:04") + "\n")
		}
		if lastErr != "" {
			sb.WriteString("- آخر خطأ مزامنة: " + redact(lastErr) + "\n")
		}
		if u := getUpdateState(); u.Error != "" {
			sb.WriteString("- آخر خطأ تحديث: " + u.Error + "\n")
		}
		if t := logTail(30); t != "" {
			sb.WriteString("\nآخر سطور السجل:\n```text\n" + t + "\n```\n")
		}
		sb.WriteString("</details>\n")
	}
	sb.WriteString("\n<!-- sent-by: UniHub -->\n")
	return sb.String()
}

func newLocalID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func queueFeedback(it FBItem) {
	it.LocalID = newLocalID()
	it.Created = time.Now().Unix()
	it.Status = "pending"
	fbMu.Lock()
	fb.Outbox = append(fb.Outbox, it)
	fbMu.Unlock()
	saveFeedback()
	select {
	case fbFlush <- struct{}{}:
	default:
	}
}

// flushOutbox sends queued notes. Returns how many were sent.
func flushOutbox() int {
	c := getConfig()
	repo := currentRepo()
	if c.GHToken == "" || repo == "" {
		return 0
	}
	fbMu.Lock()
	items := append([]FBItem(nil), fb.Outbox...)
	fbMu.Unlock()
	sent := 0
	for _, it := range items {
		var iss ghIssue
		err := ghDo("POST", "/repos/"+repo+"/issues", map[string]any{
			"title":  fmt.Sprintf("[ملاحظة:%s] %s", it.Kind, it.Title),
			"body":   composeBody(it),
			"labels": []string{"ملاحظة فنية"},
		}, &iss)
		fbMu.Lock()
		for i := range fb.Outbox {
			if fb.Outbox[i].LocalID != it.LocalID {
				continue
			}
			if err == nil {
				fb.Outbox = append(fb.Outbox[:i], fb.Outbox[i+1:]...)
				fb.Seen[iss.Number] = FBSeen{Comments: 0, State: "open"}
				sent++
			} else if err != errOffline {
				fb.Outbox[i].Status, fb.Outbox[i].Error = "error", err.Error()
			}
			break
		}
		fbMu.Unlock()
		if err == errOffline {
			break
		}
	}
	if sent > 0 {
		saveFeedback()
		refreshThreads()
	}
	return sent
}

// refreshThreads downloads the list of notes and raises notifications for new activity.
func refreshThreads() error {
	repo := currentRepo()
	if repo == "" {
		return errors.New("لم يتم تحديد المستودع")
	}
	var issues []ghIssue
	if err := ghDo("GET", "/repos/"+repo+"/issues?state=all&per_page=100&sort=updated&direction=desc", nil, &issues); err != nil {
		fbMu.Lock()
		fb.LastError = err.Error()
		fbMu.Unlock()
		return err
	}
	me := getConfig().GHLogin
	var threads []FBThread
	type event struct{ title, body string }
	var events []event
	fbMu.Lock()
	first := !fb.Fetched
	for _, is := range issues {
		if is.PR != nil || !strings.HasPrefix(strings.TrimSpace(is.Title), fbPrefix) {
			continue
		}
		kind, title, _ := parseTitle(is.Title)
		t := FBThread{Number: is.Number, Title: title, Kind: kind, Body: publicBody(is.Body), State: is.State,
			Author: is.User.Login, Avatar: is.User.Avatar, Created: is.CreatedAt, Updated: is.UpdatedAt,
			Comments: is.Comments, URL: is.HTMLURL}
		threads = append(threads, t)
		prev, known := fb.Seen[is.Number]
		if !first {
			switch {
			case !known && is.User.Login != me:
				fb.Unread[is.Number] = true
				events = append(events, event{"ملاحظة فنية جديدة من " + is.User.Login, title})
			case known && is.Comments > prev.Comments:
				fb.Unread[is.Number] = true
				events = append(events, event{"رد جديد على ملاحظة", title})
			case known && prev.State != is.State:
				fb.Unread[is.Number] = true
				if is.State == "closed" {
					events = append(events, event{"✓ تم حل ملاحظة", title})
				} else {
					events = append(events, event{"أُعيد فتح ملاحظة", title})
				}
			}
		}
		fb.Seen[is.Number] = FBSeen{Comments: is.Comments, State: is.State}
	}
	fb.Threads = threads
	fb.Fetched = true
	fb.LastFetch = time.Now().Unix()
	fb.LastError = ""
	fbMu.Unlock()
	saveFeedback()
	if getConfig().Toasts {
		for i, e := range events {
			if i >= 3 {
				break
			}
			notify(e.title, e.body)
		}
	}
	return nil
}

func threadComments(n int) ([]FBComment, error) {
	repo := currentRepo()
	var raw []struct {
		Body      string `json:"body"`
		CreatedAt string `json:"created_at"`
		User      ghUser `json:"user"`
	}
	if err := ghDo("GET", fmt.Sprintf("/repos/%s/issues/%d/comments?per_page=100", repo, n), nil, &raw); err != nil {
		return nil, err
	}
	var out []FBComment
	for _, c := range raw {
		out = append(out, FBComment{Author: c.User.Login, Avatar: c.User.Avatar, Body: c.Body, Created: c.CreatedAt})
	}
	fbMu.Lock()
	delete(fb.Unread, n)
	fbMu.Unlock()
	saveFeedback()
	return out, nil
}

func replyThread(n int, body string) error {
	err := ghDo("POST", fmt.Sprintf("/repos/%s/issues/%d/comments", currentRepo(), n), map[string]string{"body": body}, nil)
	if err == nil {
		fbMu.Lock()
		s := fb.Seen[n]
		s.Comments++
		fb.Seen[n] = s
		fbMu.Unlock()
		refreshThreads()
	}
	return err
}

func setThreadState(n int, state string) error {
	err := ghDo("PATCH", fmt.Sprintf("/repos/%s/issues/%d", currentRepo(), n), map[string]string{"state": state}, nil)
	if err == nil {
		fbMu.Lock()
		s := fb.Seen[n]
		s.State = state
		fb.Seen[n] = s
		fbMu.Unlock()
		refreshThreads()
	}
	return err
}

// exportForClaude builds a plain-text summary of open notes, ready to paste into a chat.
func exportForClaude() string {
	snap := feedbackSnapshot()
	var open []FBThread
	for _, t := range snap.Threads {
		if t.State == "open" {
			open = append(open, t)
		}
	}
	sort.Slice(open, func(i, j int) bool { return open[i].Number < open[j].Number })
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("ملاحظات فنية مفتوحة على برنامج منظّم الجامعة (UniHub) - الإصدار الحالي %s\n", Version))
	if r := currentRepo(); r != "" {
		sb.WriteString("المستودع: https://github.com/" + r + "\n")
	}
	sb.WriteString("أرجو تنفيذ هذه الملاحظات وإصلاحها:\n\n")
	for _, t := range open {
		sb.WriteString(fmt.Sprintf("### #%d [%s] %s (من %s)\n%s\n", t.Number, t.Kind, t.Title, t.Author, t.Body))
		if t.Comments > 0 {
			if cs, err := threadComments(t.Number); err == nil {
				for _, c := range cs {
					sb.WriteString(fmt.Sprintf("  - رد %s: %s\n", c.Author, strings.ReplaceAll(strings.TrimSpace(c.Body), "\n", " ")))
				}
			}
		}
		sb.WriteString("\n")
	}
	if len(open) == 0 {
		sb.WriteString("(لا توجد ملاحظات مفتوحة)\n")
	}
	return sb.String()
}

func feedbackLoop() {
	time.Sleep(10 * time.Second)
	tick := time.NewTicker(2 * time.Minute)
	defer tick.Stop()
	lastRefresh := time.Time{}
	for {
		if currentRepo() != "" {
			flushOutbox()
			if time.Since(lastRefresh) > 10*time.Minute {
				if err := refreshThreads(); err != nil && err != errOffline {
					log.Println("feedback:", err)
				}
				lastRefresh = time.Now()
			}
		}
		select {
		case <-tick.C:
		case <-fbFlush:
		}
	}
}
