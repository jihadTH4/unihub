//go:build windows

package main

import (
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const createNoWindow = 0x08000000

func hidden(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd
}

func openPath(p string) error {
	return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", p).Start()
}

func revealPath(p string) error {
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + p + `"`}
	return cmd.Start()
}

func openURL(u string) error {
	return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", u).Start()
}

func psEncode(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, len(u)*2)
	for i, v := range u {
		b[2*i] = byte(v)
		b[2*i+1] = byte(v >> 8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func runPS(script string) ([]byte, error) {
	cmd := hidden(exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-STA", "-ExecutionPolicy", "Bypass",
		"-EncodedCommand", psEncode(script)))
	return cmd.Output()
}

func notify(title, body string) {
	script := `
$ErrorActionPreference = 'SilentlyContinue'
$title = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + b64(title) + `'))
$body = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + b64(body) + `'))
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null
$xml = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$t = $xml.GetElementsByTagName('text')
$t.Item(0).AppendChild($xml.CreateTextNode($title)) > $null
$t.Item(1).AppendChild($xml.CreateTextNode($body)) > $null
$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe').Show($toast)
`
	go runPS(script)
}

func pickFolder() (string, error) {
	script := `
Add-Type -AssemblyName System.Windows.Forms
[Console]::OutputEncoding = [Text.Encoding]::UTF8
$f = New-Object System.Windows.Forms.Form -Property @{TopMost = $true; ShowInTaskbar = $false}
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = 'اختر المجلد'
$d.ShowNewFolderButton = $true
if ($d.ShowDialog($f) -eq [System.Windows.Forms.DialogResult]::OK) { Write-Output $d.SelectedPath }
`
	out, err := runPS(script)
	if err != nil {
		return "", err
	}
	p := strings.TrimSpace(strings.TrimPrefix(string(out), "\uFEFF"))
	if p == "" {
		return "", errors.New("cancelled")
	}
	return p, nil
}

const runKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

func setAutostart(on bool) error {
	if !on {
		hidden(exec.Command("reg", "delete", runKey, "/v", "UniHub", "/f")).Run()
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return setAutostartPath(exe)
}

func setAutostartPath(exe string) error {
	return hidden(exec.Command("reg", "add", runKey, "/v", "UniHub", "/t", "REG_SZ", "/d", `"`+exe+`" --background`, "/f")).Run()
}

func getAutostart() bool {
	return hidden(exec.Command("reg", "query", runKey, "/v", "UniHub")).Run() == nil
}

// ---------- DPAPI: encrypt secrets with the current Windows user's key ----------

var (
	crypt32       = syscall.NewLazyDLL("crypt32.dll")
	kernel32      = syscall.NewLazyDLL("kernel32.dll")
	procProtect   = crypt32.NewProc("CryptProtectData")
	procUnprotect = crypt32.NewProc("CryptUnprotectData")
	procLocalFree = kernel32.NewProc("LocalFree")
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

func newBlob(d []byte) *dataBlob {
	if len(d) == 0 {
		return &dataBlob{}
	}
	return &dataBlob{cbData: uint32(len(d)), pbData: &d[0]}
}

func dpapi(proc *syscall.LazyProc, in []byte) ([]byte, error) {
	var out dataBlob
	r, _, err := proc.Call(uintptr(unsafe.Pointer(newBlob(in))), 0, 0, 0, 0, 0x1, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return nil, err
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return append([]byte(nil), unsafe.Slice(out.pbData, out.cbData)...), nil
}

func protectSecret(s string) string {
	if s == "" || strings.HasPrefix(s, "dpapi:") {
		return s
	}
	b, err := dpapi(procProtect, []byte(s))
	if err != nil {
		return s
	}
	return "dpapi:" + base64.StdEncoding.EncodeToString(b)
}

func unprotectSecret(s string) string {
	if !strings.HasPrefix(s, "dpapi:") {
		return s
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, "dpapi:"))
	if err != nil {
		return ""
	}
	b, err := dpapi(procUnprotect, raw)
	if err != nil {
		return ""
	}
	return string(b)
}

// ---------- installation ----------

func installDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base, _ = os.UserConfigDir()
	}
	return filepath.Join(base, "Programs", "UniHub")
}

func canInstall() bool { return true }

func createShortcuts(target string) error {
	script := `
$ErrorActionPreference = 'Stop'
$name = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + b64("منظّم الجامعة") + `'))
$target = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + b64(target) + `'))
$ws = New-Object -ComObject WScript.Shell
foreach ($dir in @([Environment]::GetFolderPath('Programs'), [Environment]::GetFolderPath('Desktop'))) {
  $s = $ws.CreateShortcut((Join-Path $dir ($name + '.lnk')))
  $s.TargetPath = $target
  $s.IconLocation = "$target,0"
  $s.WorkingDirectory = Split-Path $target
  $s.Description = $name
  $s.Save()
}
`
	_, err := runPS(script)
	return err
}

func startDetached(exe string, args ...string) error {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008 | 0x00000200} // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
	return cmd.Start()
}

func osVersion() string {
	out, err := hidden(exec.Command("cmd", "/c", "ver")).Output()
	if err != nil {
		return "Windows"
	}
	return strings.TrimSpace(string(out))
}

// toRecycleBin moves a file to the Windows Recycle Bin.
func toRecycleBin(p string) error {
	script := `
$ErrorActionPreference = 'Stop'
$p = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + b64(p) + `'))
Add-Type -AssemblyName Microsoft.VisualBasic
[Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($p, 'OnlyErrorDialogs', 'SendToRecycleBin')
`
	if _, err := runPS(script); err != nil {
		return errors.New("تعذر نقل الصورة إلى سلة المحذوفات")
	}
	if _, err := os.Stat(p); err == nil {
		return errors.New("تعذر نقل الصورة إلى سلة المحذوفات")
	}
	return nil
}
