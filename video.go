package main

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Lecture videos (and audio recordings) are big, so they are not downloaded with the
// documents. Every sync records the ones it finds (as files, inside pages, or embedded in
// course text); a separate queue downloads them one at a time, with progress, resume
// after a broken connection, and a stop button. Mode "auto" queues new videos after each
// sync, "manual" waits for the user, "off" ignores them.

const CatVideos = "فيديوهات المحاضرات"

var mediaExts = map[string]bool{
	".mp4": true, ".m4v": true, ".mov": true, ".mkv": true, ".webm": true, ".avi": true, ".wmv": true,
	".flv": true, ".3gp": true, ".mpg": true, ".mpeg": true,
	".mp3": true, ".m4a": true, ".aac": true, ".wav": true, ".ogg": true, ".oga": true, ".opus": true,
}

func isMedia(name string) bool { return mediaExts[strings.ToLower(filepath.Ext(name))] }

type VideoRec struct {
	Key          string `json:"key"`
	URL          string `json:"url"`
	CourseID     int    `json:"courseId"`
	Course       string `json:"course"`
	Section      string `json:"section"`
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	TimeModified int64  `json:"timeModified"`
	Found        int64  `json:"found"`
	Status       string `json:"status"` // new | queued | downloading | done | error | hidden
	LocalPath    string `json:"localPath,omitempty"`
	Error        string `json:"error,omitempty"`
	Tries        int    `json:"tries,omitempty"`
	Updated      bool   `json:"updated,omitempty"` // the teacher replaced a video we already had
}

// videoMode: auto | manual | off. Phones default to manual (data plans, storage).
func videoMode(c Config) string {
	switch c.Videos {
	case "auto", "manual", "off":
		return c.Videos
	}
	if runtime.GOOS == "android" {
		return "manual"
	}
	return "auto"
}

var mediaLinkRe = regexp.MustCompile(`(?i)https?://[^"'\s<>]+?\.(?:mp4|m4v|mov|mkv|webm|avi|wmv|flv|3gp|mpe?g|mp3|m4a|aac|wav|ogg|oga|opus)(?:\?[^"'\s<>]*)?`)

// videoFinder collects the videos of one course during a sync.
type videoFinder struct {
	m  *Moodle
	cr CourseRec
}

// fromHTML picks video links out of course text (labels, intros, section summaries).
// Only files on the Moodle site itself are taken, so the token never leaves it.
func (v videoFinder) fromHTML(sec Section, title, h string) {
	if h == "" || !strings.Contains(h, "pluginfile.php") {
		return
	}
	for _, raw := range mediaLinkRe.FindAllString(h, -1) {
		u := html.UnescapeString(raw)
		if !strings.Contains(u, "pluginfile.php") || !v.m.sameSite(u) {
			continue
		}
		pu, err := url.Parse(u)
		if err != nil {
			continue
		}
		name, _ := url.PathUnescape(path.Base(pu.Path))
		v.add(sec, title, name, u, 0, 0)
	}
}

func (v videoFinder) add(sec Section, title, fileName, fileURL string, size, tmod int64) {
	if fileName == "" || !isMedia(fileName) {
		return
	}
	name := fileName
	if t := strings.TrimSpace(html.UnescapeString(title)); t != "" && !strings.Contains(fileName, t) {
		name = t + " - " + fileName
	}
	key := fileKey(fileURL)
	stMu.Lock()
	defer stMu.Unlock()
	if f := st.Files[key]; f != nil && (tmod == 0 || f.TimeModified == tmod) {
		return // downloaded earlier together with the documents
	}
	r := st.Videos[key]
	if r == nil {
		st.Videos[key] = &VideoRec{Key: key, URL: fileURL, CourseID: v.cr.ID, Course: v.cr.Name, Section: html.UnescapeString(sec.Name),
			Name: sanitizeName(name), Size: size, TimeModified: tmod, Found: time.Now().Unix(), Status: "new"}
		return
	}
	r.URL, r.Course = fileURL, v.cr.Name
	if size > 0 {
		r.Size = size
	}
	if tmod != 0 && r.TimeModified != 0 && tmod != r.TimeModified && r.Status == "done" {
		r.Status, r.Updated, r.Tries, r.Error = "new", true, 0, ""
	}
	r.TimeModified = tmod
}

// ---------- download queue ----------

type VideoProgress struct {
	Running bool   `json:"running"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	Done    int64  `json:"done"`
	Total   int64  `json:"total"`
	Speed   int64  `json:"speed"` // bytes per second
}

var (
	vpMu       sync.Mutex
	vidProg    VideoProgress
	vidCancel  context.CancelFunc
	vidWake    = make(chan struct{}, 1)
	vidStopped atomic.Bool // set by the stop button, cleared when a new download starts
)

func getVideoProgress() VideoProgress {
	vpMu.Lock()
	defer vpMu.Unlock()
	return vidProg
}

func wakeVideos() {
	select {
	case vidWake <- struct{}{}:
	default:
	}
}

// queueVideos marks videos for download (all new ones when keys is empty).
func queueVideos(keys []string, retry bool) int {
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	n := 0
	stMu.Lock()
	for _, r := range st.Videos {
		if len(keys) > 0 && !want[r.Key] {
			continue
		}
		ok := r.Status == "new" || (r.Status == "error" && (retry || r.Tries < 3)) || (len(keys) > 0 && r.Status == "hidden")
		if ok {
			r.Status, r.Error = "queued", ""
			n++
		}
	}
	stMu.Unlock()
	if n > 0 {
		saveState()
		wakeVideos()
	}
	return n
}

// stopVideos cancels the current download and empties the queue. Partial files are kept,
// so the next download resumes where it stopped.
func stopVideos() {
	stMu.Lock()
	for _, r := range st.Videos {
		if r.Status == "queued" {
			r.Status = "new"
		}
	}
	stMu.Unlock()
	vpMu.Lock()
	vidStopped.Store(true)
	if vidCancel != nil {
		vidCancel()
	}
	vpMu.Unlock()
	saveState()
}

func setVideosHidden(keys []string, hide bool) {
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	stMu.Lock()
	for _, r := range st.Videos {
		if !want[r.Key] {
			continue
		}
		if hide && r.Status != "done" && r.Status != "downloading" {
			r.Status = "hidden"
		} else if !hide && r.Status == "hidden" {
			r.Status = "new"
		}
	}
	stMu.Unlock()
	saveState()
}

// afterSyncVideos runs at the end of each sync.
func afterSyncVideos(c Config) {
	if videoMode(c) == "auto" {
		queueVideos(nil, false)
	}
}

func nextVideo() *VideoRec {
	stMu.Lock()
	defer stMu.Unlock()
	var list []*VideoRec
	for _, r := range st.Videos {
		if r.Status == "queued" {
			list = append(list, r)
		}
	}
	if len(list) == 0 {
		return nil
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Found < list[j].Found || (list[i].Found == list[j].Found && list[i].Name < list[j].Name)
	})
	list[0].Status = "downloading"
	cp := *list[0]
	return &cp
}

func videoWorker() {
	stMu.Lock()
	for _, r := range st.Videos {
		if r.Status == "downloading" {
			r.Status = "queued" // the program was closed in the middle of a download
		}
	}
	stMu.Unlock()
	wakeVideos()
	for range vidWake {
		var got []string
		for {
			r := nextVideo()
			if r == nil {
				break
			}
			if err := downloadVideo(r); err != nil {
				stopped := errors.Is(err, context.Canceled)
				stMu.Lock()
				if x := st.Videos[r.Key]; x != nil {
					if stopped {
						x.Status = "new"
					} else {
						x.Status, x.Error = "error", err.Error()
						x.Tries++
					}
				}
				stMu.Unlock()
				if !stopped {
					log.Println("video:", r.Name, err)
				}
			} else {
				got = append(got, r.Name)
			}
			saveState()
		}
		vpMu.Lock()
		vidProg = VideoProgress{}
		vpMu.Unlock()
		if len(got) > 0 {
			go indexer.Rescan()
			if getConfig().Toasts {
				body := strings.Join(got[:min(3, len(got))], "\n")
				if len(got) > 3 {
					body += fmt.Sprintf("\nو %d غيرها", len(got)-3)
				}
				notify(fmt.Sprintf("تم تنزيل %d من فيديوهات المحاضرات", len(got)), body)
			}
		}
	}
}

func downloadVideo(r *VideoRec) error {
	c := getConfig()
	if c.Token == "" {
		return errors.New("لم يتم ربط حساب المودل")
	}
	m := NewMoodle(c.MoodleURL, c.Token)
	if !m.sameSite(r.URL) {
		return errors.New("الرابط ليس على موقع المودل")
	}
	folder := ""
	stMu.RLock()
	for _, cr := range st.Courses {
		if cr.ID == r.CourseID {
			folder = cr.Folder
		}
	}
	stMu.RUnlock()
	if folder == "" {
		folder = filepath.Join(c.RootDir, sanitizeName(r.Course))
	}
	t := r.TimeModified
	if t == 0 {
		t = r.Found
	}
	dest := filepath.Join(folder, CatVideos, datePrefix(t)+r.Name)
	if r.Updated {
		ext := filepath.Ext(dest)
		dest = strings.TrimSuffix(dest, ext) + " (نسخة محدثة " + time.Unix(t, 0).Format("2006-01-02") + ")" + ext
	}
	part := dest + ".part"
	if _, err := os.Stat(part); err != nil {
		dest = uniquePath(dest) // keep the same name while a .part exists, so we can resume it
		part = dest + ".part"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	vpMu.Lock()
	vidStopped.Store(false)
	vidCancel = cancel
	vidProg = VideoProgress{Running: true, Key: r.Key, Name: r.Name, Total: r.Size}
	vpMu.Unlock()
	defer func() { vpMu.Lock(); vidCancel = nil; vpMu.Unlock() }()

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	var have int64
	if fi, err := os.Stat(part); err == nil {
		have = fi.Size()
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", m.authURL(r.URL), nil)
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		if ctx.Err() != nil && vidStopped.Load() {
			return context.Canceled
		}
		return errors.New(redact(err.Error()))
	}
	defer resp.Body.Close()
	flags := os.O_CREATE | os.O_WRONLY
	switch resp.StatusCode {
	case http.StatusPartialContent:
		flags |= os.O_APPEND
	case http.StatusOK:
		have = 0
		flags |= os.O_TRUNC
	case http.StatusRequestedRangeNotSatisfiable:
		have = 0
		os.Remove(part)
		return errors.New("تعذر استكمال التنزيل، سيبدأ من جديد في المحاولة القادمة")
	default:
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4000))
		if strings.Contains(string(b), "\"error\"") || strings.Contains(string(b), "\"exception\"") {
			return errors.New("رفض المودل تنزيل الملف")
		}
		return errors.New("المودل أرسل صفحة بدل الفيديو")
	}
	total := r.Size
	if resp.ContentLength > 0 {
		total = have + resp.ContentLength
	}
	f, err := os.OpenFile(part, flags, 0o644)
	if err != nil {
		return err
	}

	// a connection that stops sending is cut after 90 s; the next try resumes
	stall := time.AfterFunc(90*time.Second, cancel)
	defer stall.Stop()
	done, last, lastAt := have, have, time.Now()
	var speed int64
	buf := make([]byte, 256<<10)
	var werr error
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			stall.Reset(90 * time.Second)
			if _, werr = f.Write(buf[:n]); werr != nil {
				break
			}
			done += int64(n)
			if dt := time.Since(lastAt); dt >= 500*time.Millisecond {
				s := int64(float64(done-last) / dt.Seconds())
				if speed == 0 {
					speed = s
				} else {
					speed = (speed*7 + s*3) / 10
				}
				last, lastAt = done, time.Now()
				vpMu.Lock()
				vidProg.Done, vidProg.Total, vidProg.Speed = done, total, speed
				vpMu.Unlock()
			}
		}
		if rerr != nil {
			if rerr != io.EOF {
				werr = rerr
			}
			break
		}
	}
	f.Close()
	if werr != nil {
		if ctx.Err() != nil {
			if vidStopped.Load() {
				return context.Canceled // the user pressed stop
			}
			return errors.New("انقطع الاتصال أثناء التنزيل، سيُستكمل لاحقاً")
		}
		return errors.New(redact(werr.Error()))
	}
	if total > 0 && done < total {
		return errors.New("انقطع الاتصال أثناء التنزيل، سيُستكمل لاحقاً")
	}
	if err := os.Rename(part, dest); err != nil {
		return err
	}
	if r.TimeModified > 0 {
		os.Chtimes(dest, time.Now(), time.Unix(r.TimeModified, 0))
	}
	stMu.Lock()
	if x := st.Videos[r.Key]; x != nil {
		x.Status, x.LocalPath, x.Error, x.Updated = "done", dest, "", false
		if x.Size == 0 {
			x.Size = done
		}
	}
	st.Files[r.Key] = &FileRec{Key: r.Key, LocalPath: dest, CourseID: r.CourseID, Course: r.Course, Category: CatVideos,
		Name: filepath.Base(dest), Section: r.Section, TimeModified: r.TimeModified, Size: done, Downloaded: time.Now().Unix()}
	stMu.Unlock()
	return nil
}

// videoCounts feeds the sidebar badge: videos found but not downloaded yet.
func videoCounts() (waiting, active int) {
	stMu.RLock()
	defer stMu.RUnlock()
	for _, r := range st.Videos {
		switch r.Status {
		case "new", "error":
			waiting++
		case "queued", "downloading":
			active++
		}
	}
	return
}

// videoList is what the «الفيديوهات» page shows.
func videoList() []VideoRec {
	stMu.RLock()
	defer stMu.RUnlock()
	out := make([]VideoRec, 0, len(st.Videos))
	for _, r := range st.Videos {
		x := *r
		x.URL = "" // the page does not need it
		if x.Status == "done" {
			if _, err := os.Stat(x.LocalPath); err != nil {
				x.Status = "missing" // the user moved or deleted it: never download again by itself
			}
		}
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Course != out[j].Course {
			return out[i].Course < out[j].Course
		}
		if out[i].TimeModified != out[j].TimeModified {
			return out[i].TimeModified > out[j].TimeModified
		}
		return out[i].Name < out[j].Name
	})
	return out
}
