//go:build !windows && !android

package main

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

func opener() string {
	if runtime.GOOS == "darwin" {
		return "open"
	}
	return "xdg-open"
}

func openPath(p string) error   { return exec.Command(opener(), p).Start() }
func revealPath(p string) error { return exec.Command(opener(), p).Start() }
func openURL(u string) error    { return exec.Command(opener(), u).Start() }
func notify(title, body string) { log.Printf("[notify] %s: %s", title, body) }
func pickFolder() (string, error) {
	return "", errors.New("unsupported")
}
func setAutostart(on bool) error { return nil }
func getAutostart() bool         { return false }

func setAutostartPath(exe string) error { return nil }
func protectSecret(s string) string     { return s }
func unprotectSecret(s string) string   { return s }
func installDir() string {
	if d := os.Getenv("UNIHUB_INSTALL_DIR"); d != "" {
		return d
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".local", "share", "UniHub")
}
func canInstall() bool                    { return os.Getenv("UNIHUB_INSTALL_DIR") != "" }
func createShortcuts(target string) error { return nil }
func startDetached(exe string, args ...string) error {
	cmd := exec.Command(exe, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Start()
}
func osVersion() string { return runtime.GOOS + "/" + runtime.GOARCH }

// toRecycleBin moves the file into a hidden .trash folder next to it.
func toRecycleBin(p string) error {
	dir := filepath.Join(filepath.Dir(filepath.Dir(p)), ".trash")
	os.MkdirAll(dir, 0o755)
	return os.Rename(p, filepath.Join(dir, strconv.FormatInt(time.Now().UnixNano(), 36)+"-"+filepath.Base(p)))
}
