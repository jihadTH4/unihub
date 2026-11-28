# UniHub — notes for Claude

Desktop app (Windows) + Android app that syncs Moodle courses/assignments/notifications into an organized folder tree and offers fast full-text search. UI is Arabic (RTL). The owner has no programming experience: do the whole job (code, test, build, release) and explain results in Arabic.

## Architecture
- Go 1.24, **standard library only** (no third-party modules). Single binary; UI embedded via `//go:embed web/*`, served on `127.0.0.1:47821` and opened in the browser.
- `main.go` flags: `--background`, `--port`, `--wait-port`, `--updated-from=<old version>`, `--version` (prints the version; used to identify other copies).
- `copies_windows.go` — 20 s after start, finds other `UniHub*.exe` copies (autostart target, install dir, shortcut targets on Desktop/Start/taskbar, loose copies in Desktop/Downloads), asks each `--version` (pre-1.5.2 copies answer with usage text) and replaces older ones with the running exe. Fixes "reopening brings back an old version" when the user has two copies.
- `server.go` — local HTTP API (Host check + per-boot token header `X-UniHub-Token`).
- `moodle.go` (REST client, mobile service token) · `sync.go` (download/organize, never deletes; changed files saved as "(نسخة محدثة …)") · `organize.go` (copy-only import of old folders).
- `video.go` — lecture videos/audio (`isMedia` extensions): found during sync in resources/folders, inside pages, and as Moodle-site `pluginfile.php` links in labels/intros/section summaries (`sameSite` check, so the token never leaves the Moodle host). Stored in `State.Videos`, downloaded by `videoWorker` one at a time into `<course>/فيديوهات المحاضرات/` with progress, Range resume (`.part`), 90 s stall cut, stop. `Config.Videos` = auto | manual | off ("" → auto on desktop, manual on Android). API `/api/videos`, `/api/videos/get|stop|hide`.
- `index.go` + `text.go` (inverted index, Arabic normalization) · `extract.go` + `pdf.go` (text extraction, custom PDF parser incl. encryption).
- `handoff.go` — starting a newer exe while an older copy runs: it reads the old page's token, calls `/api/quit`, and takes over (replacing the installed copy if needed).
- `update.go` — GitHub Releases auto-update: assets `UniHub.exe` + `checksums.txt` (SHA-256 verified), backup of data dir, rename running exe to `.old`, restart.
- `feedback.go` — "الملاحظات الفنية": GitHub Issues titled `[ملاحظة:<نوع>] <عنوان>`, label `ملاحظة فنية`, offline outbox, polling + toasts. Diagnostics after `\n---\n` in the body are hidden in the app.
- `platform_windows.go` — toasts, folder picker, autostart, DPAPI secret encryption, self-install to `%LOCALAPPDATA%\Programs\UniHub`.
- `links.go` — the user's own links (`Config.Links`), http(s) only, per-device; never hardcode one university's links.
- `images.go` — «صوري» gallery: files in `<RootDir>/صور الجامعة/<category>/`, served by `/api/images/raw?p=&t=<token>` (img tags can't send headers), all paths checked by `imagePath`; delete = Recycle Bin (`toRecycleBin`); pins in `Config.PinnedImages`.
- `web/` — `index.html` (SVG icon sprite), `style.css` (navy/gold design system, light/dark/auto), `app.js` (views).
- Responsive layout: JS sets `mode-desktop` (>1100px, full sidebar always visible, no menu button — the owner rejected collapsing it), `mode-tablet` (761–1100, rail + overlay drawer), `mode-phone` (≤760, off-canvas drawer + bottom `#tabbar`, sheets for popups/modals) plus `is-rail`/`side-open` on `<html>` (head script sets them before paint). Check new views at 390px width: no horizontal overflow (`.grid > *` has `min-width:0`).
- Android: the same Go engine runs as `libunihub.so` (child process of a foreground service) inside `android/` (Java, no AndroidX, minSdk 26). `host.go` = bridge to the Java host: long-poll `/api/host/poll` (header `X-UniHub-Host`, env `UNIHUB_HOST_KEY`) with commands open/url/notify/install/quit; on Android pages are served only to the host WebView (`?k=` then `uh_host` cookie). `platform_android.go` (custom DNS resolver, `.trash` instead of Recycle Bin, no folder picker). Env from Java: `UNIHUB_DATA`, `UNIHUB_ROOT`, `UNIHUB_DNS`. The web UI gets class `on-app` (hides `.desk-only`, shows `.app-only`). Updates: engine downloads `UniHub.apk`, verifies SHA-256, sends `install`; Java opens the system installer (one tap is unavoidable). Build: `tools/build-android.sh <ver> release|debug`; `.github/workflows/android.yml` builds a debug APK on changes. No device/emulator here — ask the owner to test on a phone.
- `site/` — download page (GitHub Pages, `.github/workflows/pages.yml`): live version + download counts from the Releases API. Screenshots in `site/img/` are real app screenshots (webp).
- Performance: `html.perf-lite` (no blur, small shadows, short transitions, `content-visibility` on list rows). `Config.Perf`/localStorage `uh-perf` = auto | full | lite; auto = weak hardware heuristics + a 2.5 s frame-time check at start (result cached in `uh-perf-detected`). `render()` skips identical HTML on the 5 s refresh; refresh pauses while the window is hidden. Settings is a bento grid (`.bento` 12 cols, `.tile.span-N`), every control saves immediately.
- User data: `%AppData%\UniHub` (config.json, state.json, feedback.json, index.gob, backups/). Updates must never break these formats — add fields, don't rename.

## Rules
- Never delete or move user files. Never log tokens (use `redact`).
- Keep the UI professional: SVG icons from the sprite (`ic("name")`), no emoji.
- Run `gofmt -l .`, `go vet ./...`, and `GOOS=windows go build` before committing.
- Test hooks: `UNIHUB_DATA` (data dir), `UNIHUB_GH_API` (GitHub API base), `UNIHUB_INSTALL_DIR`.

## Handling technical notes (feedback loop)
1. Read open issues with the label `ملاحظة فنية` (GitHub MCP `list_issues` / `issue_read`), or the text the owner pasted from "نسخ للإصلاح".
2. Fix, commit to `main`, comment on each issue what changed, and close it when released.
3. Release (Claude does it; tags/releases can't be created from Claude sessions, so CI does it): put the new version in `VERSION` (must be greater than the last), write the Arabic changelog (Markdown list) in `RELEASE_NOTES.md`, commit and push to `main`. `.github/workflows/release.yml` runs on any `main` push that changes `VERSION`: it builds `UniHub.exe` with `-X main.Version` and `-X main.UpdateRepo=${{ github.repository }}`, creates tag + release with the notes, and uploads `UniHub.exe`, `UniHub.apk` (only when the repo secrets `ANDROID_KEYSTORE` (base64 .jks) and `ANDROID_KEYSTORE_PASSWORD` exist; alias `unihub`; never commit the keystore — losing it breaks Android updates), `checksums.txt`, `UniHub-Windows.zip`. Pushing `VERSION` also redeploys the download page. Confirm with `gh run list -R jihadTH4/unihub` and `gh api repos/jihadTH4/unihub/releases/latest`. Installed copies update themselves 15 s after start, when the window opens, and hourly. The owner only updates from inside the app.
4. If the icon changes: edit `assets/icon.svg`, render PNGs (`rsvg-convert -w N`), the workflow runs `tools/winres.py`.
