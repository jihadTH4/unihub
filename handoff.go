package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// When the user starts a newer UniHub.exe (for example one downloaded from GitHub)
// while an older copy is still running, the new copy asks the old one to quit and
// takes its place, so the user never ends up looking at the old version.

var tokenRe = regexp.MustCompile(`API_TOKEN = "([0-9a-f]+)"`)

func pingRunning(base string) (string, bool) {
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(base + "api/ping")
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	var p struct {
		App     string `json:"app"`
		Version string `json:"version"`
	}
	if json.NewDecoder(resp.Body).Decode(&p) != nil || p.App != "unihub" {
		return "", false
	}
	return p.Version, true
}

// stopRunning asks the running copy to quit, using the session token it gives its own page.
func stopRunning(base string) error {
	c := &http.Client{Timeout: 3 * time.Second}
	pageURL := base
	if hasHost() {
		pageURL += "?k=" + hostKey
	}
	resp, err := c.Get(pageURL)
	if err != nil {
		return err
	}
	page, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	m := tokenRe.FindSubmatch(page)
	if m == nil {
		return errors.New("no session token")
	}
	req, _ := http.NewRequest("POST", base+"api/quit", strings.NewReader("{}"))
	req.Header.Set("X-UniHub-Token", string(m[1]))
	req.Header.Set("Content-Type", "application/json")
	resp, err = c.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.New("quit refused: " + resp.Status)
	}
	// wait until it is gone
	for i := 0; i < 50; i++ {
		time.Sleep(200 * time.Millisecond)
		if _, alive := pingRunning(base); !alive {
			return nil
		}
	}
	return errors.New("the old copy did not stop")
}

// replaceInstalledCopy puts this (newer) program in the install folder when an
// installed copy exists and this one runs from somewhere else (e.g. Downloads).
func replaceInstalledCopy() (string, bool) {
	if !canInstall() || isInstalled() {
		return "", false
	}
	dst := filepath.Join(installDir(), "UniHub.exe")
	if !fileExists(dst) {
		return "", false
	}
	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	os.Remove(dst + ".old")
	if os.Rename(dst, dst+".old") != nil {
		return "", false
	}
	for i := 0; i < 10; i++ {
		if copyFile(exe, dst) == nil {
			os.Chmod(dst, 0o755)
			return dst, true
		}
		time.Sleep(time.Second)
	}
	os.Rename(dst+".old", dst)
	return "", false
}
