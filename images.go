package main

// "صوري": a gallery of university images the user keeps (exam timetable, study plan,
// payment notices, ID cards...). Images are ordinary files in
// <RootDir>/صور الجامعة/<category>/ so they stay visible in File Explorer.

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const imagesFolder = "صور الجامعة"

var defaultImageCats = []string{"جدول الامتحانات", "الخطة الدراسية", "الجدول الدراسي", "المالية والدفع", "البطاقات والوثائق", "أخرى"}

var imageExts = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".webp": "image/webp",
	".gif": "image/gif", ".bmp": "image/bmp", ".jfif": "image/jpeg",
}

type ImageItem struct {
	Rel      string `json:"rel"`
	Path     string `json:"path"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Size     int64  `json:"size"`
	Mtime    int64  `json:"mtime"`
	Pinned   bool   `json:"pinned"`
}

type ImageCat struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func imagesRoot() string {
	return filepath.Join(getConfig().RootDir, imagesFolder)
}

// imagePath turns a gallery-relative path into an absolute one, refusing anything
// that would leave the gallery folder.
func imagePath(rel string) (string, error) {
	rel = filepath.Clean(filepath.FromSlash(strings.TrimSpace(rel)))
	if rel == "." || filepath.IsAbs(rel) || strings.HasPrefix(rel, "..") || filepath.VolumeName(rel) != "" {
		return "", errors.New("مسار غير صالح")
	}
	root := imagesRoot()
	p := filepath.Join(root, rel)
	if !strings.HasPrefix(p, root+string(filepath.Separator)) {
		return "", errors.New("مسار غير صالح")
	}
	return p, nil
}

func relOf(p string) string {
	r, _ := filepath.Rel(imagesRoot(), p)
	return filepath.ToSlash(r)
}

func listImages() ([]ImageItem, []ImageCat) {
	root := imagesRoot()
	pins := map[string]bool{}
	for _, p := range getConfig().PinnedImages {
		pins[p] = true
	}
	counts := map[string]int{}
	var items []ImageItem
	ents, _ := os.ReadDir(root)
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		cat := e.Name()
		counts[cat] += 0
		files, _ := os.ReadDir(filepath.Join(root, cat))
		for _, f := range files {
			if f.IsDir() || imageExts[strings.ToLower(filepath.Ext(f.Name()))] == "" {
				continue
			}
			fi, err := f.Info()
			if err != nil {
				continue
			}
			p := filepath.Join(root, cat, f.Name())
			rel := relOf(p)
			items = append(items, ImageItem{Rel: rel, Path: p, Name: strings.TrimSuffix(f.Name(), filepath.Ext(f.Name())),
				Category: cat, Size: fi.Size(), Mtime: fi.ModTime().Unix(), Pinned: pins[rel]})
			counts[cat]++
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Mtime > items[j].Mtime })
	var cats []ImageCat
	seen := map[string]bool{}
	for _, c := range defaultImageCats {
		cats = append(cats, ImageCat{c, counts[c]})
		seen[c] = true
	}
	var extra []string
	for c := range counts {
		if !seen[c] {
			extra = append(extra, c)
		}
	}
	sort.Strings(extra)
	for _, c := range extra {
		cats = append(cats, ImageCat{c, counts[c]})
	}
	return items, cats
}

func cleanCategory(c string) string {
	c = strings.TrimSpace(c)
	if c == "" {
		return "أخرى"
	}
	c = sanitizeName(c)
	if strings.HasPrefix(c, ".") {
		c = "_" + c
	}
	return c
}

// saveUploadedImage stores one uploaded image; the name is kept, made safe and unique.
func saveUploadedImage(cat, name string, r io.Reader) (string, error) {
	ext := strings.ToLower(filepath.Ext(name))
	if imageExts[ext] == "" {
		return "", errors.New("الملف «" + name + "» ليس صورة مدعومة (PNG أو JPG أو WEBP أو GIF أو BMP)")
	}
	base := sanitizeName(strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)))
	dir := filepath.Join(imagesRoot(), cleanCategory(cat))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := uniquePath(filepath.Join(dir, base+ext))
	f, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	n, err := io.Copy(f, io.LimitReader(r, 40<<20+1))
	f.Close()
	if err == nil && n > 40<<20 {
		err = errors.New("الصورة أكبر من 40 ميغابايت")
	}
	if err != nil {
		os.Remove(dst)
		return "", err
	}
	return dst, nil
}

func renameImage(rel, newName, newCat string) (string, error) {
	src, err := imagePath(rel)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(src); err != nil {
		return "", errors.New("الصورة لم تعد موجودة")
	}
	ext := filepath.Ext(src)
	name := strings.TrimSpace(newName)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(src), ext)
	}
	cat := filepath.Base(filepath.Dir(src))
	if strings.TrimSpace(newCat) != "" {
		cat = cleanCategory(newCat)
	}
	dir := filepath.Join(imagesRoot(), cat)
	dst := filepath.Join(dir, sanitizeName(name)+ext)
	if dst == src {
		return rel, nil
	}
	if !strings.EqualFold(dst, src) {
		dst = uniquePath(dst)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(src, dst); err != nil {
		return "", err
	}
	newRel := relOf(dst)
	updateConfig(func(c *Config) {
		for i, p := range c.PinnedImages {
			if p == rel {
				c.PinnedImages[i] = newRel
			}
		}
	})
	return newRel, nil
}

func pinImage(rel string, on bool) {
	updateConfig(func(c *Config) {
		out := c.PinnedImages[:0:0]
		for _, p := range c.PinnedImages {
			if p != rel {
				out = append(out, p)
			}
		}
		if on {
			out = append(out, rel)
		}
		c.PinnedImages = out
	})
}

// removeImage sends the image to the Recycle Bin (or a hidden .trash folder elsewhere),
// so a mistaken delete can always be undone.
func removeImage(rel string) error {
	p, err := imagePath(rel)
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err != nil {
		return nil
	}
	if err := toRecycleBin(p); err != nil {
		return err
	}
	pinImage(rel, false)
	return nil
}

func serveImage(w http.ResponseWriter, r *http.Request) {
	p, err := imagePath(r.URL.Query().Get("p"))
	ct := imageExts[strings.ToLower(filepath.Ext(p))]
	if err != nil || ct == "" {
		http.Error(w, "forbidden", 403)
		return
	}
	f, err := os.Open(p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	fi, _ := f.Stat()
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	mt := time.Time{}
	if fi != nil {
		mt = fi.ModTime()
	}
	http.ServeContent(w, r, "", mt, f)
}
