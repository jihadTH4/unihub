package main

// Self-update from GitHub Releases.
//
// Flow: check releases/latest -> download UniHub.exe + checksums.txt -> verify SHA-256 ->
// back up the user's data -> rename the running exe to .old -> put the new exe in place ->
// start it and exit. The user's data lives in %AppData%\UniHub and in their own folders,
// so an update never touches it.

import (
	"crypto/sha256"
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
	"strconv"
	"strings"
	"sync"
	"time"
)

// Set at build time: -ldflags "-X main.Version=1.2.0 -X main.UpdateRepo=owner/repo"
var (
	Version    = "1.0.0"
	UpdateRepo = "jihadTH4/unihub" // official repository; the release build sets it too
	bootID     = strconv.FormatInt(time.Now().UnixNano(), 36)
)

func ghAPI() string {
	if v := os.Getenv("UNIHUB_GH_API"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://api.github.com"
}

func currentRepo() string {
	if r := strings.TrimSpace(getConfig().Repo); r != "" {
		return r
	}
	return UpdateRepo
}

var repoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// normalizeRepo accepts "owner/repo" or a github.com URL.
func normalizeRepo(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(strings.TrimSuffix(s, "/"), ".git")
	for _, p := range []string{"https://github.com/", "http://github.com/", "github.com/"} {
		s = strings.TrimPrefix(s, p)
	}
	parts := strings.Split(s, "/")
	if len(parts) >= 2 {
		s = parts[0] + "/" + parts[1]
	}
	if !repoRe.MatchString(s) {
		return ""
	}
	return s
}

type ReleaseInfo struct {
	Version   string `json:"version"`
	Notes     string `json:"notes"`
	URL       string `json:"url"`
	Published string `json:"published"`
	exeURL    string
	sumURL    string
}

type UpdateState struct {
	Checking    bool         `json:"checking"`
	Installing  bool         `json:"installing"`
	Progress    int          `json:"progress"`
	Available   *ReleaseInfo `json:"available"`
	Error       string       `json:"error"`
	LastCheck   int64        `json:"lastCheck"`
	JustUpdated string       `json:"justUpdated"` // previous version, right after an update
	Notes       string       `json:"notes"`       // notes of the version we just updated to
	Ready       string       `json:"ready"`       // Android: version downloaded and waiting for the user to install
}

// updateAsset is the release file this build updates itself from.
func updateAsset() string {
	if runtime.GOOS == "android" {
		return "UniHub.apk"
	}
	return "UniHub.exe"
}

var (
	updMu sync.Mutex
	upd   UpdateState
)

func getUpdateState() UpdateState {
	updMu.Lock()
	defer updMu.Unlock()
	u := upd
	return u
}

func setUpd(f func(u *UpdateState)) {
	updMu.Lock()
	f(&upd)
	updMu.Unlock()
}

// parseVersion turns "v1.2.3" into [1,2,3].
func parseVersion(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v = strings.SplitN(v, "-", 2)[0]
	for i, p := range strings.SplitN(v, ".", 3) {
		n, _ := strconv.Atoi(p)
		out[i] = n
	}
	return out
}

func newerVersion(a, b string) bool {
	x, y := parseVersion(a), parseVersion(b)
	for i := 0; i < 3; i++ {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return false
}

func ghGet(path string, out any) error {
	req, _ := http.NewRequest("GET", ghAPI()+path, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "UniHub/"+Version)
	if t := getConfig().GHToken; t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return errors.New("لا يوجد اتصال بالإنترنت")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return errors.New("لم يتم العثور على إصدارات في المستودع بعد")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("GitHub: HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func checkUpdate() (*ReleaseInfo, error) {
	repo := currentRepo()
	if repo == "" {
		return nil, errors.New("لم يتم تحديد مستودع GitHub للتحديثات")
	}
	setUpd(func(u *UpdateState) { u.Checking = true })
	defer setUpd(func(u *UpdateState) { u.Checking = false; u.LastCheck = time.Now().Unix() })
	var rel struct {
		TagName     string `json:"tag_name"`
		Body        string `json:"body"`
		HTMLURL     string `json:"html_url"`
		PublishedAt string `json:"published_at"`
		Draft       bool   `json:"draft"`
		Assets      []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := ghGet("/repos/"+repo+"/releases/latest", &rel); err != nil {
		setUpd(func(u *UpdateState) { u.Error = err.Error() })
		return nil, err
	}
	info := &ReleaseInfo{Version: strings.TrimPrefix(rel.TagName, "v"), Notes: rel.Body, URL: rel.HTMLURL, Published: rel.PublishedAt}
	for _, a := range rel.Assets {
		switch strings.ToLower(a.Name) {
		case strings.ToLower(updateAsset()):
			info.exeURL = a.URL
		case "checksums.txt":
			info.sumURL = a.URL
		}
	}
	if !newerVersion(info.Version, Version) {
		setUpd(func(u *UpdateState) { u.Available = nil; u.Error = "" })
		return nil, nil
	}
	if info.exeURL == "" || info.sumURL == "" {
		err := errors.New("الإصدار الجديد ما زال قيد التجهيز على GitHub، سيتم المحاولة لاحقاً")
		setUpd(func(u *UpdateState) { u.Error = err.Error() })
		return nil, err
	}
	setUpd(func(u *UpdateState) { u.Available = info; u.Error = "" })
	return info, nil
}

type progressWriter struct {
	total, done int64
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	if p.total > 0 {
		pct := int(p.done * 100 / p.total)
		setUpd(func(u *UpdateState) { u.Progress = pct })
	}
	return len(b), nil
}

func downloadTo(url, dest string, pw *progressWriter) error {
	c := &http.Client{Timeout: 20 * time.Minute}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "UniHub/"+Version)
	resp, err := c.Do(req)
	if err != nil {
		return errors.New("انقطع الاتصال أثناء التنزيل")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("تعذر تنزيل التحديث (HTTP %d)", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	var w io.Writer = f
	if pw != nil {
		pw.total = resp.ContentLength
		w = io.MultiWriter(f, pw)
	}
	_, err = io.Copy(w, io.LimitReader(resp.Body, 300<<20))
	f.Close()
	return err
}

func fileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// backupData copies the small, important data files before an update.
func backupData(tag string) {
	dir := filepath.Join(dataDir, "backups", time.Now().Format("2006-01-02_15-04")+"_v"+tag)
	os.MkdirAll(dir, 0o755)
	for _, n := range []string{"config.json", "state.json", "feedback.json"} {
		if b, err := os.ReadFile(filepath.Join(dataDir, n)); err == nil {
			os.WriteFile(filepath.Join(dir, n), b, 0o600)
		}
	}
	// keep only the 5 newest backups
	ents, _ := os.ReadDir(filepath.Join(dataDir, "backups"))
	var names []string
	for _, e := range ents {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for len(names) > 5 {
		os.RemoveAll(filepath.Join(dataDir, "backups", names[0]))
		names = names[1:]
	}
}

func applyUpdate() error {
	info := getUpdateState().Available
	if info == nil {
		var err error
		if info, err = checkUpdate(); err != nil {
			return err
		}
		if info == nil {
			return errors.New("أنت تستخدم أحدث إصدار")
		}
	}
	updMu.Lock()
	if upd.Installing {
		updMu.Unlock()
		return errors.New("التحديث يعمل الآن")
	}
	if upd.Ready != "" && upd.Ready == info.Version {
		// Android: already downloaded and verified, just ask the phone to install it again
		updMu.Unlock()
		hostSend(hostCmd{Op: "install", Path: filepath.Join(dataDir, "update", updateAsset()), Version: info.Version})
		return nil
	}
	upd.Installing, upd.Progress, upd.Error = true, 0, ""
	updMu.Unlock()
	fail := func(err error) error {
		setUpd(func(u *UpdateState) { u.Installing = false; u.Error = err.Error() })
		return err
	}

	dir := filepath.Join(dataDir, "update")
	os.RemoveAll(dir)
	os.MkdirAll(dir, 0o755)
	newExe := filepath.Join(dir, updateAsset())
	sums := filepath.Join(dir, "checksums.txt")
	if err := downloadTo(info.sumURL, sums, nil); err != nil {
		return fail(err)
	}
	if err := downloadTo(info.exeURL, newExe, &progressWriter{}); err != nil {
		return fail(err)
	}
	want := ""
	b, _ := os.ReadFile(sums)
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.EqualFold(strings.TrimPrefix(f[1], "*"), updateAsset()) {
			want = strings.ToLower(f[0])
		}
	}
	got, _ := fileSHA256(newExe)
	if want == "" || got != want {
		return fail(errors.New("فشل التحقق من سلامة ملف التحديث، لم يتم التثبيت"))
	}

	backupData(Version)
	if runtime.GOOS == "android" {
		// Android installs apps itself: hand the verified package to the app, which opens
		// the system installer (the user confirms with one tap).
		os.Remove(sums)
		log.Println("update", info.Version, "downloaded, waiting for the user to install")
		setUpd(func(u *UpdateState) { u.Installing = false; u.Progress = 100; u.Ready = info.Version })
		if !hostSend(hostCmd{Op: "install", Path: newExe, Version: info.Version}) {
			return fail(errNoHostUpdate)
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	exe, _ = filepath.EvalSymlinks(exe)
	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return fail(fmt.Errorf("لا يمكن استبدال البرنامج: %v", err))
	}
	// antivirus software may hold the freshly downloaded file for a few seconds
	var cerr error
	for i := 0; i < 10; i++ {
		if cerr = copyFile(newExe, exe); cerr == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if cerr != nil {
		os.Remove(exe)
		os.Rename(old, exe) // roll back
		return fail(fmt.Errorf("تعذر وضع النسخة الجديدة مكان القديمة: %v", cerr))
	}
	os.Chmod(exe, 0o755)
	os.RemoveAll(dir)
	log.Println("updated", Version, "->", info.Version)
	setUpd(func(u *UpdateState) { u.Progress = 100 })
	go restartInto(exe, "--background", "--wait-port", "--updated-from="+Version)
	return nil
}

// restartInto stops this process cleanly and starts exe in its place.
func restartInto(exe string, args ...string) {
	time.Sleep(500 * time.Millisecond) // let the HTTP response reach the browser
	indexer.Save()
	saveState()
	saveFeedback()
	if httpSrv != nil {
		httpSrv.Close()
	}
	var err error
	for i := 0; i < 5; i++ {
		if err = startDetached(exe, args...); err == nil {
			os.Exit(0)
		}
		log.Println("restart failed:", err)
		time.Sleep(2 * time.Second)
	}
	// the new program could not start: put the previous one back and run it
	if old := exe + ".old"; fileExists(old) {
		os.Remove(exe)
		if os.Rename(old, exe) == nil {
			startDetached(exe, "--background", "--wait-port")
		}
	}
	os.Exit(1)
}

var errNoHostUpdate = errors.New("ثبّت التحديث من داخل تطبيق منظّم الجامعة")

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// afterUpdate runs at startup: cleans the old exe and remembers what's new.
func afterUpdate(from string) {
	exe, err := os.Executable()
	if err == nil {
		go func() {
			for i := 0; i < 20; i++ {
				if err := os.Remove(exe + ".old"); err == nil || os.IsNotExist(err) {
					return
				}
				time.Sleep(time.Second)
			}
		}()
	}
	if from == "" {
		return
	}
	setUpd(func(u *UpdateState) { u.JustUpdated = from })
	go func() {
		time.Sleep(2 * time.Second)
		if info, err := checkLatestNotes(); err == nil {
			setUpd(func(u *UpdateState) { u.Notes = info })
		}
		notify("تم تحديث منظّم الجامعة ✓", "أنت الآن على الإصدار "+Version)
	}()
}

func checkLatestNotes() (string, error) {
	repo := currentRepo()
	if repo == "" {
		return "", errors.New("no repo")
	}
	var rel struct {
		Body string `json:"body"`
	}
	err := ghGet("/repos/"+repo+"/releases/tags/v"+Version, &rel)
	return rel.Body, err
}

// installSelf copies the program to a fixed folder and adds Start-menu and desktop shortcuts.
func installSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dst := filepath.Join(installDir(), "UniHub.exe")
	if filepath.Clean(exe) == filepath.Clean(dst) {
		return createShortcuts(dst)
	}
	if err := os.MkdirAll(installDir(), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		os.Remove(dst + ".old")
		os.Rename(dst, dst+".old")
	}
	if err := copyFile(exe, dst); err != nil {
		return err
	}
	os.Chmod(dst, 0o755)
	if err := createShortcuts(dst); err != nil {
		log.Println("shortcuts:", err)
	}
	if getAutostart() {
		setAutostartPath(dst)
	}
	go restartInto(dst, "--background", "--wait-port")
	return nil
}

func isInstalled() bool {
	if runtime.GOOS == "android" {
		return true
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(filepath.Dir(exe)), filepath.Clean(installDir()))
}

// updateLoop checks for updates shortly after start and then every hour.
func updateLoop() {
	time.Sleep(15 * time.Second)
	for {
		if currentRepo() != "" {
			info, err := checkUpdate()
			if err == nil && info != nil {
				c := getConfig()
				if getUpdateState().Ready == info.Version {
					// Android: already downloaded, the app shows "install" until the user taps it
				} else if c.AutoUpdate {
					// wait for a quiet moment (not in the middle of a Moodle sync)
					for getStatus().Running {
						time.Sleep(30 * time.Second)
					}
					if err := applyUpdate(); err != nil {
						log.Println("auto update:", err)
					}
				} else if c.Toasts {
					notify("تحديث جديد متاح", "الإصدار "+info.Version+" من منظّم الجامعة جاهز للتثبيت")
				}
			}
		}
		time.Sleep(time.Hour)
	}
}
