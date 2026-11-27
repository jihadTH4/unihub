//go:build android

package main

// Android: UniHub runs as the engine inside the Android app (android/ folder), which
// starts it with UNIHUB_HOST_KEY, UNIHUB_DATA, UNIHUB_ROOT and UNIHUB_DNS. Everything that
// needs the phone itself (opening files, notifications, installing updates) goes through
// the host queue in host.go.

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var errNoHost = errors.New("هذه الميزة تعمل من داخل تطبيق منظّم الجامعة فقط")

func openPath(p string) error {
	if fi, err := os.Stat(p); err == nil && fi.IsDir() {
		return errors.New("فتح المجلدات غير متاح على الجوال، افتح الملفات من داخل التطبيق")
	}
	if !hostSend(hostCmd{Op: "open", Path: p}) {
		return errNoHost
	}
	return nil
}
func revealPath(p string) error { return openPath(p) }
func openURL(u string) error {
	if !hostSend(hostCmd{Op: "url", URL: u}) {
		return errNoHost
	}
	return nil
}
func notify(title, body string) { hostSend(hostCmd{Op: "notify", Title: title, Body: body}) }
func pickFolder() (string, error) {
	return "", errors.New("اختيار مجلد آخر غير متاح على الجوال")
}
func setAutostart(on bool) error                  { return nil }
func getAutostart() bool                          { return true } // the app restarts the engine after boot
func setAutostartPath(exe string) error           { return nil }
func protectSecret(s string) string               { return s } // the data folder is private to the app
func unprotectSecret(s string) string             { return s }
func installDir() string                          { return "" }
func canInstall() bool                            { return false }
func createShortcuts(target string) error         { return nil }
func startDetached(exe string, a ...string) error { return errors.New("not supported on Android") }
func osVersion() string                           { return "android/" + runtime.GOARCH }

// toRecycleBin moves the file into a hidden .trash folder next to its category.
func toRecycleBin(p string) error {
	dir := filepath.Join(filepath.Dir(filepath.Dir(p)), ".trash")
	os.MkdirAll(dir, 0o755)
	return os.Rename(p, filepath.Join(dir, strconv.FormatInt(time.Now().UnixNano(), 36)+"-"+filepath.Base(p)))
}

// Android has no /etc/resolv.conf, so without cgo Go would ask 127.0.0.1 for DNS.
// Use the phone's DNS servers (passed by the app) and public resolvers as a fallback.
func init() {
	var servers []string
	for _, s := range strings.FieldsFunc(os.Getenv("UNIHUB_DNS"), func(r rune) bool { return r == ',' || r == ' ' }) {
		if ip := net.ParseIP(strings.Trim(s, "[]")); ip != nil {
			servers = append(servers, net.JoinHostPort(ip.String(), "53"))
		}
	}
	servers = append(servers, "1.1.1.1:53", "8.8.8.8:53", "9.9.9.9:53")
	var next atomic.Uint32
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 4 * time.Second}
			var last error
			start := int(next.Add(1))
			for i := 0; i < len(servers); i++ {
				c, err := d.DialContext(ctx, network, servers[(start+i)%len(servers)])
				if err == nil {
					return c, nil
				}
				last = err
			}
			return nil, last
		},
	}
}
