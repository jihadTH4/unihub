package main

import (
	"encoding/gob"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type Doc struct {
	Path     string
	Name     string
	Ext      string
	Size     int64
	MTime    int64
	Root     string
	Course   string
	Category string
	Text     string
}

type posting struct {
	doc int32
	w   float32
}

type Index struct {
	mu       sync.RWMutex
	docs     []*Doc
	byPath   map[string]int
	post     map[string][]posting
	terms    []string
	dirty    bool // terms list needs rebuild
	stale    int
	scanMu   sync.Mutex
	statMu   sync.RWMutex
	scanned  int
	toScan   int
	running  bool
	lastScan time.Time
}

var indexer = &Index{byPath: map[string]int{}, post: map[string][]posting{}}

type IndexStatus struct {
	Running  bool   `json:"running"`
	Done     int    `json:"done"`
	Total    int    `json:"total"`
	Docs     int    `json:"docs"`
	LastScan string `json:"lastScan"`
}

func (ix *Index) Status() IndexStatus {
	ix.statMu.RLock()
	s := IndexStatus{Running: ix.running, Done: ix.scanned, Total: ix.toScan}
	if !ix.lastScan.IsZero() {
		s.LastScan = ix.lastScan.Format("15:04")
	}
	ix.statMu.RUnlock()
	ix.mu.RLock()
	s.Docs = len(ix.byPath)
	ix.mu.RUnlock()
	return s
}

// ---------- building ----------

func addTerm(m map[string]float32, t string, w float32) {
	if t == "" || len([]rune(t)) > 40 {
		return
	}
	if m[t] < w {
		m[t] = w
	}
}

func docTerms(d *Doc) map[string]float32 {
	m := map[string]float32{}
	add := func(text string, w float32, count bool) {
		tf := map[string]int{}
		for _, t := range tokenize(normalizeText(text)) {
			tf[t]++
			if s := stripArticle(t); s != t {
				tf[s]++
			}
			for _, p := range splitAlnum(t) {
				tf[p]++
			}
		}
		for t, n := range tf {
			ww := w
			if count {
				ww = w * float32(1+math.Log(float64(n)))
				if ww > 6 {
					ww = 6
				}
			}
			if cur, ok := m[t]; ok {
				m[t] = cur + ww
			} else {
				m[t] = ww
			}
		}
	}
	add(strings.TrimSuffix(d.Name, filepath.Ext(d.Name)), 8, false)
	add(d.Course+" "+d.Category, 2, false)
	add(d.Text, 1, true)
	return m
}

func (ix *Index) addLocked(d *Doc) {
	if old, ok := ix.byPath[d.Path]; ok {
		ix.docs[old] = nil
		ix.stale++
	}
	id := int32(len(ix.docs))
	ix.docs = append(ix.docs, d)
	ix.byPath[d.Path] = int(id)
	for t, w := range docTerms(d) {
		if _, ok := ix.post[t]; !ok {
			ix.dirty = true
		}
		ix.post[t] = append(ix.post[t], posting{id, w})
	}
}

func (ix *Index) removeLocked(path string) {
	if id, ok := ix.byPath[path]; ok {
		ix.docs[id] = nil
		delete(ix.byPath, path)
		ix.stale++
	}
}

// compactLocked rebuilds postings when many docs were removed.
func (ix *Index) compactLocked() {
	live := make([]*Doc, 0, len(ix.byPath))
	for _, d := range ix.docs {
		if d != nil {
			live = append(live, d)
		}
	}
	ix.docs = nil
	ix.byPath = map[string]int{}
	ix.post = map[string][]posting{}
	ix.stale = 0
	for _, d := range live {
		ix.addLocked(d)
	}
	ix.dirty = true
}

func (ix *Index) termsLocked() []string {
	if ix.dirty || ix.terms == nil {
		ix.terms = make([]string, 0, len(ix.post))
		for t := range ix.post {
			ix.terms = append(ix.terms, t)
		}
		sort.Strings(ix.terms)
		ix.dirty = false
	}
	return ix.terms
}

// ---------- persistence ----------

func (ix *Index) Load() {
	f, err := os.Open(filepath.Join(dataDir, "index.gob"))
	if err != nil {
		return
	}
	defer f.Close()
	var docs []*Doc
	if err := gob.NewDecoder(f).Decode(&docs); err != nil {
		log.Println("index load:", err)
		return
	}
	ix.mu.Lock()
	for _, d := range docs {
		ix.addLocked(d)
	}
	ix.mu.Unlock()
}

func (ix *Index) Save() {
	ix.mu.RLock()
	docs := make([]*Doc, 0, len(ix.byPath))
	for _, d := range ix.docs {
		if d != nil {
			docs = append(docs, d)
		}
	}
	ix.mu.RUnlock()
	tmp := filepath.Join(dataDir, "index.gob.tmp")
	f, err := os.Create(tmp)
	if err != nil {
		return
	}
	if err := gob.NewEncoder(f).Encode(docs); err != nil {
		f.Close()
		return
	}
	f.Close()
	os.Rename(tmp, filepath.Join(dataDir, "index.gob"))
}

// ---------- scanning ----------

type scanRoot struct {
	dir    string
	label  string
	moodle bool
}

func skipName(n string) bool {
	return strings.HasPrefix(n, ".") || strings.HasPrefix(n, "~$") || strings.HasSuffix(n, ".part") ||
		strings.HasSuffix(n, ".tmp") || n == "node_modules" || n == "$RECYCLE.BIN" || n == "desktop.ini" || n == "Thumbs.db"
}

func (ix *Index) Rescan() {
	if !ix.scanMu.TryLock() {
		return
	}
	defer ix.scanMu.Unlock()
	c := getConfig()
	roots := []scanRoot{{c.RootDir, "المودل", true}}
	for _, e := range c.ExtraDirs {
		roots = append(roots, scanRoot{e, filepath.Base(e), false})
	}
	type job struct {
		path string
		fi   os.FileInfo
		r    scanRoot
	}
	var jobs []job
	seen := map[string]bool{}
	for _, r := range roots {
		if r.dir == "" {
			continue
		}
		filepath.Walk(r.dir, func(p string, fi os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if p != r.dir && skipName(fi.Name()) {
				if fi.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if fi.IsDir() {
				return nil
			}
			if seen[p] {
				return nil
			}
			seen[p] = true
			ix.mu.RLock()
			id, ok := ix.byPath[p]
			same := ok && ix.docs[id].Size == fi.Size() && ix.docs[id].MTime == fi.ModTime().Unix()
			ix.mu.RUnlock()
			if !same {
				jobs = append(jobs, job{p, fi, r})
			}
			return nil
		})
	}
	// removed files
	ix.mu.Lock()
	removed := 0
	for p := range ix.byPath {
		if !seen[p] {
			ix.removeLocked(p)
			removed++
		}
	}
	ix.mu.Unlock()

	ix.statMu.Lock()
	ix.running, ix.scanned, ix.toScan = len(jobs) > 0, 0, len(jobs)
	ix.statMu.Unlock()

	ch := make(chan job)
	var wg sync.WaitGroup
	workers := runtime.NumCPU()
	if workers > 4 {
		workers = 4
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				d := &Doc{Path: j.path, Name: j.fi.Name(), Ext: strings.ToLower(filepath.Ext(j.path)),
					Size: j.fi.Size(), MTime: j.fi.ModTime().Unix(), Root: j.r.label}
				rel, _ := filepath.Rel(j.r.dir, j.path)
				parts := strings.Split(rel, string(filepath.Separator))
				if len(parts) > 1 {
					d.Course = parts[0]
					if j.r.moodle && len(parts) > 2 {
						d.Category = parts[1]
					}
				} else if !j.r.moodle {
					d.Course = j.r.label
				}
				d.Text = extractText(j.path)
				ix.mu.Lock()
				ix.addLocked(d)
				ix.mu.Unlock()
				ix.statMu.Lock()
				ix.scanned++
				ix.statMu.Unlock()
			}
		}()
	}
	for _, j := range jobs {
		ch <- j
	}
	close(ch)
	wg.Wait()

	ix.mu.Lock()
	if ix.stale > 200 && ix.stale > len(ix.byPath)/3 {
		ix.compactLocked()
	}
	ix.mu.Unlock()
	ix.statMu.Lock()
	ix.running = false
	ix.lastScan = time.Now()
	ix.statMu.Unlock()
	if len(jobs) > 0 || removed > 0 {
		ix.Save()
	}
}

func (ix *Index) loop() {
	ix.Rescan()
	for range time.Tick(3 * time.Minute) {
		ix.Rescan()
	}
}

// ---------- searching ----------

type SearchHit struct {
	Path     string  `json:"path"`
	Name     string  `json:"name"`
	Ext      string  `json:"ext"`
	Size     int64   `json:"size"`
	MTime    int64   `json:"mtime"`
	Course   string  `json:"course"`
	Category string  `json:"category"`
	Root     string  `json:"root"`
	Score    float64 `json:"score"`
	Snippet  string  `json:"snippet"`
	InName   bool    `json:"inName"`
}

var typeGroups = map[string][]string{
	"pdf":    {".pdf"},
	"word":   {".doc", ".docx", ".odt", ".rtf", ".txt", ".md"},
	"slides": {".ppt", ".pptx", ".ppsx", ".odp"},
	"sheets": {".xls", ".xlsx", ".csv", ".ods"},
	"media":  {".mp4", ".mkv", ".mp3", ".m4a", ".wav", ".jpg", ".jpeg", ".png", ".gif"},
	"code":   {".py", ".java", ".c", ".cpp", ".h", ".cs", ".js", ".m", ".sql", ".ipynb", ".go"},
}

func (ix *Index) matchTerm(terms []string, q string, scores map[int32]float64, mult float64) {
	exactOnly := len([]rune(q)) < 2
	i := sort.SearchStrings(terms, q)
	n := 0
	for ; i < len(terms) && strings.HasPrefix(terms[i], q); i++ {
		t := terms[i]
		f := mult
		if t != q {
			if exactOnly {
				continue
			}
			// shorter extra suffix = better match
			f *= 0.75 - math.Min(0.4, float64(len(t)-len(q))*0.02)
		}
		for _, p := range ix.post[t] {
			if ix.docs[p.doc] == nil {
				continue
			}
			s := float64(p.w) * f
			if s > scores[p.doc] {
				scores[p.doc] = s
			}
		}
		if n++; n > 3000 {
			break
		}
	}
}

func (ix *Index) Search(query, course, typ string, limit int) ([]SearchHit, int) {
	qn := normalizeText(query)
	qtoks := tokenize(qn)
	if len(qtoks) == 0 {
		return nil, 0
	}
	ix.mu.Lock()
	terms := ix.termsLocked()
	ix.mu.Unlock()
	ix.mu.RLock()
	defer ix.mu.RUnlock()

	var allowExt map[string]bool
	if g, ok := typeGroups[typ]; ok {
		allowExt = map[string]bool{}
		for _, e := range g {
			allowExt[e] = true
		}
	}
	total := map[int32]float64{}
	matched := map[int32]int{}
	for _, q := range qtoks {
		sc := map[int32]float64{}
		ix.matchTerm(terms, q, sc, 1)
		if s := stripArticle(q); s != q {
			ix.matchTerm(terms, s, sc, 0.9)
		}
		for d, s := range sc {
			total[d] += s
			matched[d]++
		}
	}
	type cand struct {
		id    int32
		score float64
	}
	var cands []cand
	need := len(qtoks)
	phraseText := len(total) < 400
	for d, s := range total {
		doc := ix.docs[d]
		if doc == nil {
			continue
		}
		if course != "" && doc.Course != course {
			continue
		}
		if allowExt != nil && !allowExt[doc.Ext] {
			continue
		}
		if matched[d] < need {
			continue // every word must match
		}
		nn := normalizeText(doc.Name)
		if len(qtoks) > 1 && strings.Contains(nn, strings.Join(qtoks, " ")) {
			s += 15
		}
		if phraseText && len(qtoks) > 1 && strings.Contains(normalizeText(doc.Text), strings.Join(qtoks, " ")) {
			s += 4
		}
		// slight preference for recent files
		age := time.Since(time.Unix(doc.MTime, 0)).Hours() / 24
		s += math.Max(0, 1-age/180)
		cands = append(cands, cand{d, s})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].score != cands[j].score {
			return cands[i].score > cands[j].score
		}
		return ix.docs[cands[i].id].MTime > ix.docs[cands[j].id].MTime
	})
	count := len(cands)
	if len(cands) > limit {
		cands = cands[:limit]
	}
	hits := make([]SearchHit, 0, len(cands))
	for _, c := range cands {
		d := ix.docs[c.id]
		nn := normalizeText(d.Name)
		inName := true
		for _, q := range qtoks {
			if !strings.Contains(nn, q) {
				inName = false
			}
		}
		hits = append(hits, SearchHit{Path: d.Path, Name: d.Name, Ext: d.Ext, Size: d.Size, MTime: d.MTime,
			Course: d.Course, Category: d.Category, Root: d.Root, Score: math.Round(c.score*10) / 10,
			Snippet: makeSnippet(d.Text, qtoks), InName: inName})
	}
	return hits, count
}

// makeSnippet returns text around the first match; matches are wrapped in \x01 ... \x02.
func makeSnippet(text string, qtoks []string) string {
	if text == "" {
		return ""
	}
	words := strings.Fields(text)
	if len(words) > 400000 {
		words = words[:400000]
	}
	isMatch := func(w string) bool {
		for _, t := range tokenize(normalizeText(w)) {
			st := stripArticle(t)
			for _, q := range qtoks {
				sq := stripArticle(q)
				if strings.HasPrefix(t, q) || strings.HasPrefix(st, q) || strings.HasPrefix(st, sq) {
					return true
				}
			}
		}
		return false
	}
	best, bestCount := -1, 0
	for i, w := range words {
		if isMatch(w) {
			// prefer windows containing several query words
			cnt := 0
			for j := i; j < len(words) && j < i+12; j++ {
				if isMatch(words[j]) {
					cnt++
				}
			}
			if cnt > bestCount {
				best, bestCount = i, cnt
			}
			if cnt >= len(qtoks) {
				break
			}
		}
	}
	if best < 0 {
		return ""
	}
	start := best - 10
	if start < 0 {
		start = 0
	}
	end := best + 22
	if end > len(words) {
		end = len(words)
	}
	var sb strings.Builder
	if start > 0 {
		sb.WriteString("… ")
	}
	for i := start; i < end; i++ {
		w := words[i]
		if isMatch(w) {
			sb.WriteString("\x01" + w + "\x02")
		} else {
			sb.WriteString(w)
		}
		sb.WriteByte(' ')
	}
	if end < len(words) {
		sb.WriteString("…")
	}
	return sb.String()
}

func (ix *Index) Courses() []string {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	set := map[string]bool{}
	for _, d := range ix.docs {
		if d != nil && d.Course != "" {
			set[d.Course] = true
		}
	}
	var out []string
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// Files returns docs under a folder (used by the course page).
func (ix *Index) FilesUnder(dir string) []SearchHit {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	prefix := strings.TrimRight(dir, string(filepath.Separator)) + string(filepath.Separator)
	var out []SearchHit
	for _, d := range ix.docs {
		if d != nil && strings.HasPrefix(d.Path, prefix) {
			out = append(out, SearchHit{Path: d.Path, Name: d.Name, Ext: d.Ext, Size: d.Size, MTime: d.MTime,
				Course: d.Course, Category: d.Category, Root: d.Root})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MTime > out[j].MTime })
	return out
}

func (ix *Index) Recent(n int) []SearchHit {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	var out []SearchHit
	for _, d := range ix.docs {
		if d != nil {
			out = append(out, SearchHit{Path: d.Path, Name: d.Name, Ext: d.Ext, Size: d.Size, MTime: d.MTime,
				Course: d.Course, Category: d.Category, Root: d.Root})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MTime > out[j].MTime })
	if len(out) > n {
		out = out[:n]
	}
	return out
}
