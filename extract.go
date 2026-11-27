package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

const maxTextBytes = 3 << 20
const maxExtractFile = 150 << 20

var plainExts = map[string]bool{
	".txt": true, ".md": true, ".csv": true, ".json": true, ".xml": true, ".py": true, ".java": true,
	".c": true, ".cpp": true, ".h": true, ".cs": true, ".js": true, ".ts": true, ".sql": true, ".m": true,
	".r": true, ".tex": true, ".log": true, ".ini": true, ".yaml": true, ".yml": true, ".go": true, ".kt": true,
	".php": true, ".css": true, ".rtf": true, ".asm": true, ".vhd": true, ".v": true,
}

// extractText returns searchable text for a file (empty if the type is not supported).
func extractText(path string) string {
	fi, err := os.Stat(path)
	if err != nil || fi.Size() > maxExtractFile {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(path))
	var text string
	func() {
		defer func() { recover() }() // a broken file must never crash the program
		switch {
		case ext == ".pdf":
			text = extractPDF(path)
		case ext == ".docx" || ext == ".docm" || ext == ".dotx":
			text = extractZipXML(path, func(n string) bool {
				return strings.HasPrefix(n, "word/") && strings.HasSuffix(n, ".xml") &&
					(strings.Contains(n, "document") || strings.Contains(n, "footnotes") || strings.Contains(n, "endnotes"))
			})
		case ext == ".pptx" || ext == ".ppsx" || ext == ".pptm":
			text = extractZipXML(path, func(n string) bool {
				return (strings.HasPrefix(n, "ppt/slides/slide") || strings.HasPrefix(n, "ppt/notesSlides/")) && strings.HasSuffix(n, ".xml")
			})
		case ext == ".xlsx" || ext == ".xlsm":
			text = extractZipXML(path, func(n string) bool { return n == "xl/sharedStrings.xml" })
		case ext == ".odt" || ext == ".odp" || ext == ".ods":
			text = extractZipXML(path, func(n string) bool { return n == "content.xml" })
		case ext == ".doc" || ext == ".ppt" || ext == ".xls":
			text = extractLegacyOffice(path)
		case ext == ".html" || ext == ".htm":
			b, _ := os.ReadFile(path)
			text = stripHTML(string(b))
		case plainExts[ext]:
			b, _ := os.ReadFile(path)
			if len(b) > maxTextBytes {
				b = b[:maxTextBytes]
			}
			text = decodeTextBytes(b)
		}
	}()
	if len(text) > maxTextBytes {
		text = text[:maxTextBytes]
		for !utf8.ValidString(text) && len(text) > 0 {
			text = text[:len(text)-1]
		}
	}
	return text
}

func decodeTextBytes(b []byte) string {
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		return decodeUTF16LE(b[2:])
	}
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	if utf8.Valid(b) {
		return string(b)
	}
	// Windows-1256 (Arabic) fallback
	return decodeCP1256(b)
}

func decodeUTF16LE(b []byte) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return string(utf16.Decode(u))
}

var cp1256High = []rune("€پ‚ƒ„…†‡ˆ‰ٹ‹Œچژڈگ‘’“”•–—ک™ڑ›œ‌‍ں ،¢£¤¥¦§¨©ھ«¬­®¯°±²³´µ¶·¸¹؛»¼½¾؟ہءآأؤإئابةتثجحخدذرزسشصض×طظعغـفقكàلâمنهوçèéêëىيîïًٌٍَôُِ÷ّùْûü‎‏ے")

func decodeCP1256(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		if c < 0x80 {
			sb.WriteByte(c)
		} else if int(c-0x80) < len(cp1256High) {
			sb.WriteRune(cp1256High[c-0x80])
		}
	}
	return sb.String()
}

func numInName(n string) int {
	m := regexp.MustCompile(`(\d+)\.xml$`).FindStringSubmatch(n)
	if m == nil {
		return 0
	}
	v, _ := strconv.Atoi(m[1])
	return v
}

func extractZipXML(path string, want func(string) bool) string {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return ""
	}
	defer zr.Close()
	var files []*zip.File
	for _, f := range zr.File {
		if want(f.Name) {
			files = append(files, f)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		di, dj := filepath.Dir(files[i].Name), filepath.Dir(files[j].Name)
		if di != dj {
			return di > dj // slides before notes
		}
		return numInName(files[i].Name) < numInName(files[j].Name)
	})
	var sb strings.Builder
	for _, f := range files {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		xmlText(io.LimitReader(rc, 64<<20), &sb)
		rc.Close()
		sb.WriteString("\n")
		if sb.Len() > maxTextBytes {
			break
		}
	}
	return sb.String()
}

// xmlText collects the text of <w:t>/<a:t> (Office) and <text:p> (OpenDocument) elements.
func xmlText(r io.Reader, sb *strings.Builder) {
	const odf = "urn:oasis:names:tc:opendocument:xmlns:text:1.0"
	d := xml.NewDecoder(r)
	d.Strict = false
	tDepth, pDepth := 0, 0
	for {
		tok, err := d.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case t.Name.Local == "t" && t.Name.Space != odf:
				tDepth++
			case (t.Name.Local == "p" || t.Name.Local == "h") && t.Name.Space == odf:
				pDepth++
			case t.Name.Local == "tab" || t.Name.Local == "s":
				sb.WriteString(" ")
			case t.Name.Local == "br" || t.Name.Local == "line-break":
				sb.WriteString("\n")
			}
		case xml.EndElement:
			switch {
			case t.Name.Local == "t" && t.Name.Space != odf:
				if tDepth > 0 {
					tDepth--
				}
			case (t.Name.Local == "p" || t.Name.Local == "h") && t.Name.Space == odf:
				if pDepth > 0 {
					pDepth--
				}
				sb.WriteString("\n")
			case t.Name.Local == "p" || t.Name.Local == "si" || t.Name.Local == "tr":
				sb.WriteString("\n")
			case t.Name.Local == "tc":
				sb.WriteString(" ")
			}
		case xml.CharData:
			if tDepth > 0 || pDepth > 0 {
				sb.Write(t)
			}
		}
	}
}

// extractLegacyOffice pulls readable UTF-16 text runs out of old .doc/.ppt/.xls files.
func extractLegacyOffice(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var sb strings.Builder
	var run []rune
	flush := func() {
		letters := 0
		for _, r := range run {
			if unicode.IsLetter(r) {
				letters++
			}
		}
		if len(run) >= 4 && letters*2 >= len(run) {
			sb.WriteString(string(run))
			sb.WriteString("\n")
		}
		run = run[:0]
	}
	for i := 0; i+1 < len(b); i += 2 {
		r := rune(binary.LittleEndian.Uint16(b[i:]))
		ok := (r >= 0x20 && r < 0x7F) || (r >= 0x0600 && r <= 0x06FF) || (r >= 0xA0 && r < 0x250) || r == '\r'
		if ok {
			if r == '\r' {
				r = '\n'
			}
			run = append(run, r)
		} else {
			flush()
		}
		if sb.Len() > maxTextBytes {
			break
		}
	}
	flush()
	return sb.String()
}
