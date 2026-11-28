//go:build windows

package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// A user can end up with several copies of UniHub.exe: the one autostart runs, one in
// Downloads, one on the Desktop, the installed one. Each copy only updates itself when it
// runs, so opening an older copy looked like "the update was undone". On start, the
// running (newest) copy brings every other copy it can find up to its own version.

var copyName = regexp.MustCompile(`(?i)^unihub( ?\(\d+\))?\.exe$`)

// otherCopies lists UniHub.exe files the user may start: autostart target, installed
// copy, shortcut targets (Desktop, Start menu, taskbar) and loose copies on the Desktop
// and in Downloads.
func otherCopies() []string {
	var list []string
	if out, err := hidden(exec.Command("reg", "query", runKey, "/v", "UniHub")).Output(); err == nil {
		if m := regexp.MustCompile(`"([^"]+\.exe)"`).FindSubmatch(out); m != nil {
			list = append(list, string(m[1]))
		}
	}
	list = append(list, filepath.Join(installDir(), "UniHub.exe"))
	script := `
$ErrorActionPreference = 'SilentlyContinue'
[Console]::OutputEncoding = [Text.Encoding]::UTF8
$ws = New-Object -ComObject WScript.Shell
$dirs = @([Environment]::GetFolderPath('Desktop'), [Environment]::GetFolderPath('Programs'),
  (Join-Path $env:APPDATA 'Microsoft\Internet Explorer\Quick Launch\User Pinned\TaskBar'),
  (Join-Path $env:USERPROFILE 'Downloads'), (Join-Path $env:USERPROFILE 'Desktop'))
foreach ($d in $dirs) {
  if (-not (Test-Path $d)) { continue }
  Get-ChildItem -LiteralPath $d -File | ForEach-Object {
    if ($_.Extension -eq '.lnk') { $t = $ws.CreateShortcut($_.FullName).TargetPath; if ($t) { $t } }
    elseif ($_.Extension -eq '.exe') { $_.FullName }
  }
}
`
	if out, err := runPS(script); err == nil {
		for _, l := range strings.Split(string(out), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				list = append(list, l)
			}
		}
	}
	return list
}

// copyVersion runs an exe with --version. New copies print their version; copies from
// before this flag existed reject it and print the usage text (which names our flags).
// Anything else is not UniHub and is left alone.
func copyVersion(exe string) (version string, isUniHub bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, _ := hidden(exec.CommandContext(ctx, exe, "--version")).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(s) {
		return s, true
	}
	if strings.Contains(s, "updated-from") && strings.Contains(s, "wait-port") {
		return "", true // older than 1.5.2
	}
	return "", false
}

func syncCopies() {
	if Version == "dev" {
		return
	}
	self, err := os.Executable()
	if err != nil {
		return
	}
	self, _ = filepath.EvalSymlinks(self)
	seen := map[string]bool{strings.ToLower(filepath.Clean(self)): true}
	for _, p := range otherCopies() {
		p = filepath.Clean(p)
		key := strings.ToLower(p)
		if seen[key] || !copyName.MatchString(filepath.Base(p)) {
			continue
		}
		seen[key] = true
		if real, err := filepath.EvalSymlinks(p); err != nil || strings.EqualFold(real, self) {
			continue
		}
		v, ok := copyVersion(p)
		if !ok || (v != "" && !newerVersion(Version, v)) {
			continue
		}
		old := p + ".old"
		os.Remove(old)
		if err := os.Rename(p, old); err != nil {
			log.Println("copies: cannot update", p, err)
			continue
		}
		if err := copyFile(self, p); err != nil {
			os.Remove(p)
			os.Rename(old, p)
			log.Println("copies: cannot update", p, err)
			continue
		}
		os.Remove(old)
		from := v
		if from == "" {
			from = "قديم"
		}
		log.Println("copies: updated", p, "from", from, "to", Version)
	}
}
