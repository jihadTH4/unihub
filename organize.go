package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Organizing the user's old, messy folders: files are COPIED (never moved or deleted)
// into the same tidy structure used for Moodle content.

type ImportItem struct {
	Src      string `json:"src"`
	Dest     string `json:"dest"`
	Rel      string `json:"rel"`
	Category string `json:"category"`
	Size     int64  `json:"size"`
	Skip     bool   `json:"skip"`
	Reason   string `json:"reason"`
}

var categoryFolderWords = []string{"محاضرات", "محاضره", "lectures", "lecture", "slides", "واجبات", "assignments",
	"homework", "امتحانات", "exams", "سلايدات", "شرائح"}

var lectureExts = map[string]bool{".pdf": true, ".ppt": true, ".pptx": true, ".doc": true, ".docx": true, ".ppsx": true,
	".odp": true, ".odt": true, ".txt": true, ".md": true, ".mp4": true, ".mkv": true, ".mp3": true, ".m4a": true}

func classifyLocal(rel string) string {
	parts := strings.Split(rel, string(filepath.Separator))
	name := parts[len(parts)-1]
	ext := strings.ToLower(filepath.Ext(name))
	full := strings.Join(parts, " ")
	for _, e := range kwOther {
		if ext == e {
			return CatOther
		}
	}
	if hasKeyword(full, kwExams) {
		return CatExams
	}
	if hasKeyword(full, kwAssign) {
		return CatAssign
	}
	if lectureExts[ext] {
		return CatLectures
	}
	return CatOther
}

func isCategoryFolder(n string) bool {
	nn := stripArticle(normalizeText(n))
	for _, w := range categoryFolderWords {
		if nn == stripArticle(normalizeText(w)) {
			return true
		}
	}
	return false
}

func previewImport(src, course string) ([]ImportItem, error) {
	c := getConfig()
	courseDir := filepath.Join(c.RootDir, sanitizeName(course))
	var items []ImportItem
	err := filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if p != src && skipName(fi.Name()) {
			if fi.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// never import from inside the organized folder itself
		if fi.IsDir() && p == c.RootDir {
			return filepath.SkipDir
		}
		if fi.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		cat := classifyLocal(rel)
		var keep []string
		parts := strings.Split(rel, string(filepath.Separator))
		for _, d := range parts[:len(parts)-1] {
			if !isCategoryFolder(d) {
				keep = append(keep, sanitizeName(d))
			}
		}
		keep = append(keep, sanitizeName(fi.Name()))
		dest := filepath.Join(append([]string{courseDir, cat}, keep...)...)
		it := ImportItem{Src: p, Dest: dest, Rel: rel, Category: cat, Size: fi.Size()}
		if di, err := os.Stat(dest); err == nil {
			if di.Size() == fi.Size() {
				it.Skip, it.Reason = true, "موجود مسبقاً"
			}
		}
		items = append(items, it)
		if len(items) > 20000 {
			return io.EOF
		}
		return nil
	})
	if err == io.EOF {
		err = nil
	}
	return items, err
}

func runImport(items []ImportItem) (int, []string) {
	c := getConfig()
	root := filepath.Clean(c.RootDir) + string(filepath.Separator)
	copied := 0
	var errs []string
	for _, it := range items {
		if it.Skip {
			continue
		}
		dest := filepath.Clean(it.Dest)
		if !strings.HasPrefix(dest, root) {
			continue // only ever write inside the organized folder
		}
		if di, err := os.Stat(dest); err == nil {
			if si, err2 := os.Stat(it.Src); err2 == nil && si.Size() == di.Size() {
				continue
			}
			dest = uniquePath(dest)
		}
		if err := copyFile(it.Src, dest); err != nil {
			errs = append(errs, filepath.Base(it.Src)+": "+err.Error())
			continue
		}
		copied++
	}
	go indexer.Rescan()
	return copied, errs
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	out.Close()
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	return os.Chtimes(dst, fi.ModTime(), fi.ModTime())
}
