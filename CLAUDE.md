# UniHub — notes for Claude

Desktop app (Windows) that syncs Moodle courses/assignments/notifications into an organized folder tree and offers fast full-text search. UI is Arabic (RTL). The owner has no programming experience: do the whole job (code, test, build, release) and explain results in Arabic.

## Architecture
- Go 1.24, **standard library only** (no third-party modules). Single binary; UI embedded via `//go:embed web/*`, served on `127.0.0.1:47821` and opened in the browser.
- `main.go` flags: `--background`, `--port`, `--wait-port`, `--updated-from=<old version>`.
- `server.go` — local HTTP API (Host check + per-boot token header `X-UniHub-Token`).
- `moodle.go` (REST client, mobile service token) · `sync.go` (download/organize, never deletes; changed files saved as "(نسخة محدثة …)") · `organize.go` (copy-only import of old folders).
- `index.go` + `text.go` (inverted index, Arabic normalization) · `extract.go` + `pdf.go` (text extraction, custom PDF parser incl. encryption).
- `update.go` — GitHub Releases auto-update: assets `UniHub.exe` + `checksums.txt` (SHA-256 verified), backup of data dir, rename running exe to `.old`, restart.
- `feedback.go` — "الملاحظات الفنية": GitHub Issues titled `[ملاحظة:<نوع>] <عنوان>`, label `ملاحظة فنية`, offline outbox, polling + toasts. Diagnostics after `\n---\n` in the body are hidden in the app.
- `platform_windows.go` — toasts, folder picker, autostart, DPAPI secret encryption, self-install to `%LOCALAPPDATA%\Programs\UniHub`.
- `web/` — `index.html` (SVG icon sprite), `style.css` (navy/gold design system, light/dark/auto), `app.js` (views).
- User data: `%AppData%\UniHub` (config.json, state.json, feedback.json, index.gob, backups/). Updates must never break these formats — add fields, don't rename.

## Rules
- Never delete or move user files. Never log tokens (use `redact`).
- Keep the UI professional: SVG icons from the sprite (`ic("name")`), no emoji.
- Run `gofmt -l .`, `go vet ./...`, and `GOOS=windows go build` before committing.
- Test hooks: `UNIHUB_DATA` (data dir), `UNIHUB_GH_API` (GitHub API base), `UNIHUB_INSTALL_DIR`.

## Handling technical notes (feedback loop)
1. Read open issues with the label `ملاحظة فنية` (GitHub MCP `list_issues` / `issue_read`), or the text the owner pasted from "نسخ للإصلاح".
2. Fix, commit to `main`, comment on each issue what changed, and close it when released.
3. Release: bump the version — create an annotated tag `vX.Y.Z` whose message is the Arabic changelog (Markdown list) and push it. `.github/workflows/release.yml` builds `UniHub.exe` with `-X main.Version` and `-X main.UpdateRepo=${{ github.repository }}`, then publishes it. Every installed copy updates itself within ~6 hours (or immediately via Settings → البحث عن تحديث).
4. If the icon changes: edit `assets/icon.svg`, render PNGs (`rsvg-convert -w N`), the workflow runs `tools/winres.py`.
