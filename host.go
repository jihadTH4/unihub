package main

// Bridge to a native "host" app that runs this program as its engine (the Android app).
//
// The host starts UniHub with UNIHUB_HOST_KEY set to a secret only it knows. UniHub then:
//   - hands work it cannot do itself (open a file, show a notification, install an update)
//     to the host through a queue the host long-polls at /api/host/poll;
//   - serves its page (which carries the API token) only to the host's WebView, which
//     proves itself with ?k=<key> once and a cookie afterwards. On a phone every app can
//     reach 127.0.0.1, so without this another app could read the token.
// Without UNIHUB_HOST_KEY (Windows, normal desktop use) none of this is active.

import (
	"crypto/subtle"
	"net/http"
	"os"
	"time"
)

var hostKey = os.Getenv("UNIHUB_HOST_KEY")

type hostCmd struct {
	Op      string `json:"op"` // open | url | notify | install | quit
	Path    string `json:"path,omitempty"`
	URL     string `json:"url,omitempty"`
	Title   string `json:"title,omitempty"`
	Body    string `json:"body,omitempty"`
	Version string `json:"version,omitempty"`
}

var hostQ = make(chan hostCmd, 64)

func hasHost() bool { return hostKey != "" }

// hostSend queues a command for the host app; false when there is no host.
func hostSend(c hostCmd) bool {
	if !hasHost() {
		return false
	}
	select {
	case hostQ <- c:
		return true
	default:
		return false
	}
}

func keyEqual(a, b string) bool {
	return a != "" && b != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// hostPoll waits up to 25 seconds for commands and returns everything queued.
func hostPoll(w http.ResponseWriter, r *http.Request) {
	var out []hostCmd
	select {
	case c := <-hostQ:
		out = append(out, c)
	case <-time.After(25 * time.Second):
	case <-r.Context().Done():
		return
	}
drain:
	for len(out) < 20 {
		select {
		case c := <-hostQ:
			out = append(out, c)
		default:
			break drain
		}
	}
	if out == nil {
		out = []hostCmd{}
	}
	writeJSON(w, out)
}

// hostPageAllowed reports whether r may receive the page. When it proves itself with
// ?k=, a cookie is set so reloads keep working.
func hostPageAllowed(w http.ResponseWriter, r *http.Request) bool {
	if !hasHost() {
		return true
	}
	if c, err := r.Cookie("uh_host"); err == nil && keyEqual(c.Value, hostKey) {
		return true
	}
	if keyEqual(r.URL.Query().Get("k"), hostKey) {
		http.SetCookie(w, &http.Cookie{Name: "uh_host", Value: hostKey, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 10 * 365 * 24 * 3600})
		return true
	}
	return false
}
