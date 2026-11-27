package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Config holds the user's settings. The Moodle password is never stored, only the token.
type Config struct {
	MoodleURL     string   `json:"moodleUrl"`
	Token         string   `json:"token"`
	Username      string   `json:"username"`
	FullName      string   `json:"fullName"`
	SiteName      string   `json:"siteName"`
	UserID        int      `json:"userId"`
	RootDir       string   `json:"rootDir"`     // where Moodle content is organized
	ExtraDirs     []string `json:"extraDirs"`   // the user's old folders, indexed for search only
	SyncMinutes   int      `json:"syncMinutes"` // how often to check Moodle
	Toasts        bool     `json:"toasts"`      // Windows notifications
	HiddenCourses []int    `json:"hiddenCourses"`
	Repo          string   `json:"repo"`    // GitHub owner/name used for updates and feedback
	GHToken       string   `json:"ghToken"` // GitHub token for sending feedback (encrypted on disk)
	GHLogin       string   `json:"ghLogin"`
	AutoUpdate    bool     `json:"autoUpdate"`
	Theme         string   `json:"theme"` // auto | light | dark
	Links         []Link   `json:"links"` // the user's own university links (portal, pages, sites)
}

// Link is a personal shortcut shown in the sidebar and on the home page.
type Link struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Kind  string `json:"kind"` // portal | social | site | mail | other
}

var (
	cfgMu   sync.RWMutex
	cfg     Config
	dataDir string
)

func initDataDir() error {
	base, err := os.UserConfigDir()
	if err != nil {
		base, _ = os.UserHomeDir()
	}
	dataDir = filepath.Join(base, "UniHub")
	if v := os.Getenv("UNIHUB_DATA"); v != "" {
		dataDir = v
	}
	return os.MkdirAll(dataDir, 0o755)
}

func defaultRootDir() string {
	home, _ := os.UserHomeDir()
	desk := filepath.Join(home, "Desktop")
	if _, err := os.Stat(desk); err != nil {
		desk = home
	}
	return filepath.Join(desk, "الجامعة - مرتب")
}

func loadConfig() {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	cfg = Config{SyncMinutes: 20, Toasts: true, AutoUpdate: true, Theme: "auto"}
	b, err := os.ReadFile(filepath.Join(dataDir, "config.json"))
	if err == nil {
		_ = json.Unmarshal(b, &cfg)
	}
	cfg.Token = unprotectSecret(cfg.Token)
	cfg.GHToken = unprotectSecret(cfg.GHToken)
	if cfg.RootDir == "" {
		cfg.RootDir = defaultRootDir()
	}
	if cfg.SyncMinutes < 5 {
		cfg.SyncMinutes = 20
	}
}

func saveConfig() error {
	cfgMu.RLock()
	c := cfg
	cfgMu.RUnlock()
	// secrets are encrypted with the Windows user's key (DPAPI) before touching the disk
	c.Token = protectSecret(c.Token)
	c.GHToken = protectSecret(c.GHToken)
	b, _ := json.MarshalIndent(c, "", "  ")
	return writeFileAtomic(filepath.Join(dataDir, "config.json"), b)
}

func getConfig() Config {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	c := cfg
	c.ExtraDirs = append([]string(nil), cfg.ExtraDirs...)
	c.HiddenCourses = append([]int(nil), cfg.HiddenCourses...)
	return c
}

func updateConfig(f func(c *Config)) error {
	cfgMu.Lock()
	f(&cfg)
	cfgMu.Unlock()
	return saveConfig()
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func loadJSON(name string, v any) {
	b, err := os.ReadFile(filepath.Join(dataDir, name))
	if err == nil {
		_ = json.Unmarshal(b, v)
	}
}

func saveJSON(name string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dataDir, name), b)
}
