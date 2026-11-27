package main

import (
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"
)

const maxLinks = 24

var linkKinds = map[string]bool{"portal": true, "social": true, "site": true, "mail": true, "other": true}

// cleanLinks validates the user's links. Only http(s) addresses are accepted,
// so a link can never run a program or open a local file.
func cleanLinks(in []Link) ([]Link, error) {
	if len(in) > maxLinks {
		return nil, errors.New("الحد الأقصى 24 رابطاً")
	}
	out := make([]Link, 0, len(in))
	for _, l := range in {
		raw := strings.TrimSpace(l.URL)
		if raw == "" {
			continue
		}
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || strings.ContainsAny(u.Host, " \\") {
			return nil, errors.New("رابط غير صالح: " + l.URL)
		}
		title := strings.TrimSpace(l.Title)
		if title == "" {
			title = strings.TrimPrefix(u.Hostname(), "www.")
		}
		if utf8.RuneCountInString(title) > 60 {
			title = string([]rune(title)[:60])
		}
		kind := l.Kind
		if !linkKinds[kind] {
			kind = guessLinkKind(u)
		}
		out = append(out, Link{Title: title, URL: u.String(), Kind: kind})
	}
	return out, nil
}

func guessLinkKind(u *url.URL) string {
	h := strings.ToLower(u.Hostname())
	p := strings.ToLower(u.Path)
	switch {
	case strings.Contains(h, "facebook.") || strings.Contains(h, "fb.") || strings.Contains(h, "instagram.") ||
		strings.Contains(h, "twitter.") || h == "x.com" || strings.Contains(h, "t.me") || strings.Contains(h, "youtube.") ||
		strings.Contains(h, "linkedin.") || strings.Contains(h, "tiktok.") || strings.Contains(h, "telegram."):
		return "social"
	case strings.Contains(h, "mail.") || strings.Contains(h, "outlook.") || strings.Contains(p, "/mail"):
		return "mail"
	case strings.Contains(h, "portal") || strings.Contains(p, "portal") || strings.Contains(p, "student") ||
		strings.Contains(h, "sis.") || strings.Contains(h, "my.") || strings.Contains(p, "login"):
		return "portal"
	}
	return "site"
}
