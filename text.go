package main

import (
	"strings"
	"unicode"
)

// Arabic presentation forms B (U+FE80..U+FEF4): base letter and how many glyph forms it has.
var presBTable = []struct {
	base  rune
	count int
}{
	{'ء', 1}, {'آ', 2}, {'أ', 2}, {'ؤ', 2}, {'إ', 2}, {'ئ', 4}, {'ا', 2}, {'ب', 4}, {'ة', 2}, {'ت', 4},
	{'ث', 4}, {'ج', 4}, {'ح', 4}, {'خ', 4}, {'د', 2}, {'ذ', 2}, {'ر', 2}, {'ز', 2}, {'س', 4}, {'ش', 4},
	{'ص', 4}, {'ض', 4}, {'ط', 4}, {'ظ', 4}, {'ع', 4}, {'غ', 4}, {'ف', 4}, {'ق', 4}, {'ك', 4}, {'ل', 4},
	{'م', 4}, {'ن', 4}, {'ه', 4}, {'و', 2}, {'ى', 2}, {'ي', 4},
}

var presBMap = func() map[rune]rune {
	m := map[rune]rune{}
	r := rune(0xFE80)
	for _, e := range presBTable {
		for i := 0; i < e.count; i++ {
			m[r] = e.base
			r++
		}
	}
	return m
}()

// foldPresentation converts Arabic glyph forms (common in PDFs) back to normal letters.
func foldPresentation(s string) string {
	need := false
	for _, r := range s {
		if r >= 0xFB50 && r <= 0xFEFF {
			need = true
			break
		}
	}
	if !need {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 0xFEF5 && r <= 0xFEFC:
			b.WriteString("ل")
			switch {
			case r <= 0xFEF6:
				b.WriteRune('آ')
			case r <= 0xFEF8:
				b.WriteRune('أ')
			case r <= 0xFEFA:
				b.WriteRune('إ')
			default:
				b.WriteRune('ا')
			}
		case r >= 0xFE80 && r <= 0xFEF4:
			b.WriteRune(presBMap[r])
		case r >= 0xFE70 && r <= 0xFE7F:
			// harakat forms - drop
		case r == 0xFDF2:
			b.WriteString("الله")
		case r >= 0xFB8E && r <= 0xFB91:
			b.WriteRune('ك')
		case r >= 0xFBFC && r <= 0xFBFF:
			b.WriteRune('ي')
		case r == 0xFEFF:
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func normRune(r rune) (rune, bool) {
	switch {
	case r >= 0x064B && r <= 0x065F, r == 0x0670, r == 0x0640, r == 0x200C, r == 0x200D, r == 0x200F, r == 0x200E:
		return 0, false // harakat, tatweel, invisible marks
	case r == 'أ' || r == 'إ' || r == 'آ' || r == 'ٱ':
		return 'ا', true
	case r == 'ة':
		return 'ه', true
	case r == 'ى' || r == 'ئ' || r == 'ی':
		return 'ي', true
	case r == 'ؤ':
		return 'و', true
	case r == 'ک':
		return 'ك', true
	case r >= 0x0660 && r <= 0x0669:
		return '0' + (r - 0x0660), true
	case r >= 0x06F0 && r <= 0x06F9:
		return '0' + (r - 0x06F0), true
	}
	return unicode.ToLower(r), true
}

// normalizeText makes Arabic and English text comparable: removes harakat,
// unifies alef/ya/ta-marbuta forms and lowercases Latin.
func normalizeText(s string) string {
	s = foldPresentation(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if n, ok := normRune(r); ok {
			b.WriteRune(n)
		}
	}
	return b.String()
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// tokenize splits normalized text into words.
func tokenize(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !isWordRune(r) })
}

func isArabic(r rune) bool { return r >= 0x0600 && r <= 0x06FF }

// stripArticle removes the Arabic definite article and common attached prefixes.
func stripArticle(tok string) string {
	r := []rune(tok)
	if len(r) < 4 || !isArabic(r[0]) {
		return tok
	}
	for _, p := range []string{"وال", "بال", "فال", "كال", "ال", "لل"} {
		pr := []rune(p)
		if len(r)-len(pr) >= 2 && string(r[:len(pr)]) == p {
			return string(r[len(pr):])
		}
	}
	return tok
}

// splitAlnum returns sub-parts of tokens mixing letters and digits (lecture3 -> lecture, 3).
func splitAlnum(tok string) []string {
	var parts []string
	var cur []rune
	kind := 0
	for _, r := range tok {
		k := 1
		if unicode.IsDigit(r) {
			k = 2
		}
		if kind != 0 && k != kind {
			parts = append(parts, string(cur))
			cur = cur[:0]
		}
		kind = k
		cur = append(cur, r)
	}
	if len(cur) > 0 {
		parts = append(parts, string(cur))
	}
	if len(parts) < 2 {
		return nil
	}
	return parts
}

// fixArabicOrder detects PDFs that store Arabic in visual (reversed) order and fixes them.
func fixArabicOrder(text string) string {
	starts, ends := 0, 0
	for _, w := range strings.Fields(normalizeText(text)) {
		r := []rune(w)
		if len(r) < 3 || !isArabic(r[0]) {
			continue
		}
		if r[0] == 'ا' && r[1] == 'ل' {
			starts++
		}
		if r[len(r)-1] == 'ا' && r[len(r)-2] == 'ل' {
			ends++
		}
	}
	if ends < 5 || ends < starts*2 {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = reverseVisualLine(line)
	}
	return strings.Join(lines, "\n")
}

func reverseVisualLine(line string) string {
	r := []rune(line)
	hasAr := false
	for _, c := range r {
		if isArabic(c) {
			hasAr = true
			break
		}
	}
	if !hasAr {
		return line
	}
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	// put Latin words, numbers and symbols back in reading order
	for i := 0; i < len(r); {
		if !isArabic(r[i]) && !unicode.IsSpace(r[i]) {
			j := i
			for j < len(r) && !isArabic(r[j]) && !unicode.IsSpace(r[j]) {
				j++
			}
			for a, b := i, j-1; a < b; a, b = a+1, b-1 {
				r[a], r[b] = r[b], r[a]
			}
			i = j
		} else {
			i++
		}
	}
	return string(r)
}
