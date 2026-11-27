package main

// A small, dependency-free PDF text extractor. It is not a full PDF renderer:
// it walks pages, decodes content streams and maps glyph codes to Unicode
// through the fonts' ToUnicode maps, which is enough for searching lectures.

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rc4"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

type pdfName string
type pdfStr []byte
type pdfRef struct{ N, G int }
type pdfDict map[string]any
type pdfArray []any
type pdfKeyword string
type pdfStream struct {
	Dict pdfDict
	Raw  []byte
}

// ---------------- lexer / parser ----------------

type pdfLexer struct {
	b   []byte
	pos int
}

func isPDFSpace(c byte) bool {
	return c == ' ' || c == '\n' || c == '\r' || c == '\t' || c == '\f' || c == 0
}

func isPDFDelim(c byte) bool {
	return strings.IndexByte("()<>[]{}/%", c) >= 0
}

func (l *pdfLexer) skipSpace() {
	for l.pos < len(l.b) {
		c := l.b[l.pos]
		if isPDFSpace(c) {
			l.pos++
		} else if c == '%' {
			for l.pos < len(l.b) && l.b[l.pos] != '\n' && l.b[l.pos] != '\r' {
				l.pos++
			}
		} else {
			return
		}
	}
}

// token returns the next raw token: an object value, or a structural marker
// ("<<", ">>", "[", "]") as pdfKeyword, or any other keyword as pdfKeyword. io.EOF at end.
func (l *pdfLexer) token() (any, error) {
	l.skipSpace()
	if l.pos >= len(l.b) {
		return nil, io.EOF
	}
	c := l.b[l.pos]
	switch {
	case c == '/':
		l.pos++
		start := l.pos
		for l.pos < len(l.b) && !isPDFSpace(l.b[l.pos]) && !isPDFDelim(l.b[l.pos]) {
			l.pos++
		}
		n := string(l.b[start:l.pos])
		if strings.IndexByte(n, '#') >= 0 {
			n = decodeNameHex(n)
		}
		return pdfName(n), nil
	case c == '(':
		return l.literalString(), nil
	case c == '<':
		if l.pos+1 < len(l.b) && l.b[l.pos+1] == '<' {
			l.pos += 2
			return pdfKeyword("<<"), nil
		}
		l.pos++
		start := l.pos
		for l.pos < len(l.b) && l.b[l.pos] != '>' {
			l.pos++
		}
		h := l.b[start:l.pos]
		l.pos++
		return pdfStr(decodeHexBytes(h)), nil
	case c == '>':
		if l.pos+1 < len(l.b) && l.b[l.pos+1] == '>' {
			l.pos += 2
			return pdfKeyword(">>"), nil
		}
		l.pos++
		return pdfKeyword(">"), nil
	case c == '[' || c == ']' || c == '{' || c == '}':
		l.pos++
		return pdfKeyword(string(c)), nil
	case c == ')':
		l.pos++
		return pdfKeyword(")"), nil
	}
	start := l.pos
	for l.pos < len(l.b) && !isPDFSpace(l.b[l.pos]) && !isPDFDelim(l.b[l.pos]) {
		l.pos++
	}
	w := string(l.b[start:l.pos])
	if (w[0] >= '0' && w[0] <= '9') || w[0] == '-' || w[0] == '+' || w[0] == '.' {
		if f, err := strconv.ParseFloat(w, 64); err == nil {
			return f, nil
		}
	}
	switch w {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}
	return pdfKeyword(w), nil
}

func decodeNameHex(n string) string {
	var sb strings.Builder
	for i := 0; i < len(n); i++ {
		if n[i] == '#' && i+2 < len(n) {
			if v, err := strconv.ParseUint(n[i+1:i+3], 16, 8); err == nil {
				sb.WriteByte(byte(v))
				i += 2
				continue
			}
		}
		sb.WriteByte(n[i])
	}
	return sb.String()
}

func decodeHexBytes(h []byte) []byte {
	clean := make([]byte, 0, len(h))
	for _, c := range h {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			clean = append(clean, c)
		}
	}
	if len(clean)%2 == 1 {
		clean = append(clean, '0')
	}
	out := make([]byte, len(clean)/2)
	hex.Decode(out, clean)
	return out
}

func (l *pdfLexer) literalString() pdfStr {
	l.pos++ // (
	var out []byte
	depth := 1
	for l.pos < len(l.b) {
		c := l.b[l.pos]
		l.pos++
		switch c {
		case '(':
			depth++
			out = append(out, c)
		case ')':
			depth--
			if depth == 0 {
				return out
			}
			out = append(out, c)
		case '\\':
			if l.pos >= len(l.b) {
				return out
			}
			e := l.b[l.pos]
			l.pos++
			switch e {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '\r':
				if l.pos < len(l.b) && l.b[l.pos] == '\n' {
					l.pos++
				}
			case '\n':
			default:
				if e >= '0' && e <= '7' {
					v := int(e - '0')
					for k := 0; k < 2 && l.pos < len(l.b) && l.b[l.pos] >= '0' && l.b[l.pos] <= '7'; k++ {
						v = v*8 + int(l.b[l.pos]-'0')
						l.pos++
					}
					out = append(out, byte(v))
				} else {
					out = append(out, e)
				}
			}
		default:
			out = append(out, c)
		}
	}
	return out
}

// parseObject parses a full object starting at the lexer position (handles refs, arrays, dicts).
func (l *pdfLexer) parseObject() (any, error) {
	t, err := l.token()
	if err != nil {
		return nil, err
	}
	return l.finish(t)
}

func (l *pdfLexer) finish(t any) (any, error) {
	switch v := t.(type) {
	case float64:
		// maybe "N G R"
		save := l.pos
		t2, err2 := l.token()
		if g, ok := t2.(float64); ok && err2 == nil {
			t3, err3 := l.token()
			if k, ok := t3.(pdfKeyword); ok && err3 == nil && k == "R" {
				return pdfRef{int(v), int(g)}, nil
			}
		}
		l.pos = save
		return v, nil
	case pdfKeyword:
		switch v {
		case "[":
			var arr pdfArray
			for {
				t, err := l.token()
				if err != nil {
					return arr, err
				}
				if k, ok := t.(pdfKeyword); ok && k == "]" {
					return arr, nil
				}
				o, err := l.finish(t)
				if err != nil {
					return arr, err
				}
				arr = append(arr, o)
			}
		case "<<":
			d := pdfDict{}
			for {
				t, err := l.token()
				if err != nil {
					return d, err
				}
				if k, ok := t.(pdfKeyword); ok && k == ">>" {
					return d, nil
				}
				key, ok := t.(pdfName)
				if !ok {
					continue
				}
				val, err := l.parseObject()
				if err != nil {
					return d, err
				}
				if kw, ok := val.(pdfKeyword); ok && kw == ">>" {
					return d, nil
				}
				d[string(key)] = val
			}
		}
	}
	return t, nil
}

// ---------------- document ----------------

type pdfDoc struct {
	data    []byte
	objs    map[int]any
	gens    map[int]int
	trailer []pdfDict
	crypt   *pdfCrypt
}

var objHeaderRe = regexp.MustCompile(`(?:^|[^0-9])(\d{1,7})\s+(\d{1,5})\s+obj\b`)

func loadPDF(data []byte) *pdfDoc {
	d := &pdfDoc{data: data, objs: map[int]any{}, gens: map[int]int{}}
	for _, m := range objHeaderRe.FindAllSubmatchIndex(data, -1) {
		num, _ := strconv.Atoi(string(data[m[2]:m[3]]))
		gen, _ := strconv.Atoi(string(data[m[4]:m[5]]))
		l := &pdfLexer{b: data, pos: m[1]}
		o, err := l.parseObject()
		if err != nil && o == nil {
			continue
		}
		if dict, ok := o.(pdfDict); ok {
			// stream?
			p := l.pos
			for p < len(data) && isPDFSpace(data[p]) {
				p++
			}
			if bytes.HasPrefix(data[p:], []byte("stream")) {
				p += 6
				if p < len(data) && data[p] == '\r' {
					p++
				}
				if p < len(data) && data[p] == '\n' {
					p++
				}
				end := -1
				if ln, ok := dict["Length"].(float64); ok {
					e := p + int(ln)
					if e <= len(data) && e >= p {
						tail := data[e:min(len(data), e+20)]
						if bytes.Contains(tail, []byte("endstream")) {
							end = e
						}
					}
				}
				if end < 0 {
					if i := bytes.Index(data[p:], []byte("endstream")); i >= 0 {
						end = p + i
						for end > p && (data[end-1] == '\n' || data[end-1] == '\r') {
							end--
						}
					} else {
						end = len(data)
					}
				}
				o = &pdfStream{Dict: dict, Raw: data[p:end]}
			}
		}
		d.objs[num] = o
		d.gens[num] = gen
	}
	// trailers
	for i := 0; ; {
		j := bytes.Index(data[i:], []byte("trailer"))
		if j < 0 {
			break
		}
		l := &pdfLexer{b: data, pos: i + j + 7}
		if o, _ := l.parseObject(); o != nil {
			if td, ok := o.(pdfDict); ok {
				d.trailer = append(d.trailer, td)
			}
		}
		i += j + 7
	}
	for _, o := range d.objs {
		if s, ok := o.(*pdfStream); ok && s.Dict["Type"] == pdfName("XRef") {
			d.trailer = append(d.trailer, s.Dict)
		}
	}
	d.setupCrypt()
	d.expandObjStreams()
	return d
}

func (d *pdfDoc) resolve(o any) any {
	for i := 0; i < 10; i++ {
		r, ok := o.(pdfRef)
		if !ok {
			return o
		}
		o = d.objs[r.N]
	}
	return o
}

func (d *pdfDoc) dict(o any) pdfDict {
	switch v := d.resolve(o).(type) {
	case pdfDict:
		return v
	case *pdfStream:
		return v.Dict
	}
	return nil
}

func (d *pdfDoc) expandObjStreams() {
	var nums []int
	for n, o := range d.objs {
		if s, ok := o.(*pdfStream); ok && s.Dict["Type"] == pdfName("ObjStm") {
			nums = append(nums, n)
		}
	}
	sort.Ints(nums)
	for _, n := range nums {
		s := d.objs[n].(*pdfStream)
		data := d.decodeStream(s)
		if data == nil {
			continue
		}
		cnt, _ := s.Dict["N"].(float64)
		first, _ := s.Dict["First"].(float64)
		l := &pdfLexer{b: data}
		type pair struct{ num, off int }
		var pairs []pair
		for i := 0; i < int(cnt); i++ {
			a, e1 := l.token()
			b, e2 := l.token()
			an, ok1 := a.(float64)
			bn, ok2 := b.(float64)
			if e1 != nil || e2 != nil || !ok1 || !ok2 {
				break
			}
			pairs = append(pairs, pair{int(an), int(bn)})
		}
		for _, p := range pairs {
			if _, exists := d.objs[p.num]; exists {
				continue
			}
			pos := int(first) + p.off
			if pos < 0 || pos >= len(data) {
				continue
			}
			l2 := &pdfLexer{b: data, pos: pos}
			if o, err := l2.parseObject(); err == nil || o != nil {
				d.objs[p.num] = o
			}
		}
	}
}

// ---------------- filters ----------------

func (d *pdfDoc) decodeStream(s *pdfStream) []byte {
	data := s.Raw
	filters := []any{}
	switch f := d.resolve(s.Dict["Filter"]).(type) {
	case pdfName:
		filters = append(filters, f)
	case pdfArray:
		filters = f
	}
	parms := []any{}
	switch p := d.resolve(s.Dict["DecodeParms"]).(type) {
	case pdfDict:
		parms = append(parms, p)
	case pdfArray:
		parms = p
	}
	for i, f := range filters {
		name, _ := d.resolve(f).(pdfName)
		var parm pdfDict
		if i < len(parms) {
			parm = d.dict(parms[i])
		}
		switch name {
		case "FlateDecode", "Fl":
			data = inflate(data)
			if parm != nil {
				if pr, ok := parm["Predictor"].(float64); ok && pr >= 10 {
					cols := 1
					if c, ok := parm["Columns"].(float64); ok {
						cols = int(c)
					}
					data = pngUnpredict(data, cols)
				}
			}
		case "ASCIIHexDecode", "AHx":
			if i := bytes.IndexByte(data, '>'); i >= 0 {
				data = data[:i]
			}
			data = decodeHexBytes(data)
		case "ASCII85Decode", "A85":
			data = ascii85(data)
		default:
			return nil // images and unsupported filters
		}
		if data == nil {
			return nil
		}
	}
	return data
}

func inflate(b []byte) []byte {
	var out bytes.Buffer
	if r, err := zlib.NewReader(bytes.NewReader(b)); err == nil {
		io.Copy(&out, io.LimitReader(r, 200<<20)) // keep whatever decoded even on error
		if out.Len() > 0 {
			return out.Bytes()
		}
	}
	out.Reset()
	if len(b) > 2 {
		r := flate.NewReader(bytes.NewReader(b[2:]))
		io.Copy(&out, io.LimitReader(r, 200<<20))
	}
	if out.Len() == 0 {
		return nil
	}
	return out.Bytes()
}

func pngUnpredict(b []byte, cols int) []byte {
	row := cols + 1
	if row <= 1 || len(b)%row != 0 {
		return b
	}
	out := make([]byte, 0, len(b)/row*cols)
	prev := make([]byte, cols)
	for i := 0; i+row <= len(b); i += row {
		ft := b[i]
		cur := append([]byte(nil), b[i+1:i+row]...)
		for j := range cur {
			var left, up, ul byte
			if j > 0 {
				left = cur[j-1]
				ul = prev[j-1]
			}
			up = prev[j]
			switch ft {
			case 1:
				cur[j] += left
			case 2:
				cur[j] += up
			case 3:
				cur[j] += byte((int(left) + int(up)) / 2)
			case 4:
				p := int(left) + int(up) - int(ul)
				pa, pb, pc := abs(p-int(left)), abs(p-int(up)), abs(p-int(ul))
				if pa <= pb && pa <= pc {
					cur[j] += left
				} else if pb <= pc {
					cur[j] += up
				} else {
					cur[j] += ul
				}
			}
		}
		out = append(out, cur...)
		prev = cur
	}
	return out
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func ascii85(b []byte) []byte {
	var out []byte
	var tuple uint32
	n := 0
	for _, c := range b {
		if c == '~' {
			break
		}
		if isPDFSpace(c) {
			continue
		}
		if c == 'z' && n == 0 {
			out = append(out, 0, 0, 0, 0)
			continue
		}
		if c < '!' || c > 'u' {
			continue
		}
		tuple = tuple*85 + uint32(c-'!')
		n++
		if n == 5 {
			out = append(out, byte(tuple>>24), byte(tuple>>16), byte(tuple>>8), byte(tuple))
			tuple, n = 0, 0
		}
	}
	if n > 1 {
		for i := n; i < 5; i++ {
			tuple = tuple*85 + 84
		}
		for i := 0; i < n-1; i++ {
			out = append(out, byte(tuple>>(24-8*i)))
		}
	}
	return out
}

// ---------------- encryption (empty user password) ----------------

type pdfCrypt struct {
	key     []byte
	aes     bool
	v5      bool
	skipNum int
}

var pdfPad = []byte{0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41, 0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
	0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80, 0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A}

func (d *pdfDoc) setupCrypt() {
	var encRef any
	var id []byte
	for _, t := range d.trailer {
		if e, ok := t["Encrypt"]; ok && encRef == nil {
			encRef = e
		}
		if arr, ok := d.resolve(t["ID"]).(pdfArray); ok && len(arr) > 0 && id == nil {
			if s, ok := arr[0].(pdfStr); ok {
				id = s
			}
		}
	}
	if encRef == nil {
		return
	}
	enc := d.dict(encRef)
	if enc == nil || enc["Filter"] != pdfName("Standard") {
		return
	}
	skip := -1
	if r, ok := encRef.(pdfRef); ok {
		skip = r.N
	}
	num := func(k string) int { f, _ := d.resolve(enc[k]).(float64); return int(f) }
	str := func(k string) []byte { s, _ := d.resolve(enc[k]).(pdfStr); return s }
	V, R := num("V"), num("R")
	c := &pdfCrypt{skipNum: skip}
	if V == 4 || V == 5 {
		if cf := d.dict(enc["CF"]); cf != nil {
			if std := d.dict(cf["StdCF"]); std != nil {
				cfm, _ := std["CFM"].(pdfName)
				c.aes = cfm == "AESV2" || cfm == "AESV3"
			}
		}
	}
	if R >= 5 {
		u, ue := str("U"), str("UE")
		if len(u) < 48 || len(ue) < 32 {
			return
		}
		var h []byte
		if R == 5 {
			s := sha256.Sum256(u[40:48])
			h = s[:]
		} else {
			h = pdfHash2B(nil, u[40:48], nil)
		}
		blk, err := aes.NewCipher(h[:32])
		if err != nil {
			return
		}
		key := make([]byte, 32)
		cipher.NewCBCDecrypter(blk, make([]byte, 16)).CryptBlocks(key, ue[:32])
		c.key, c.aes, c.v5 = key, true, true
	} else {
		length := 40
		if l := num("Length"); l > 0 {
			length = l
		}
		n := length / 8
		if R == 2 {
			n = 5
		}
		h := md5.New()
		h.Write(pdfPad)
		h.Write(str("O"))
		p := make([]byte, 4)
		binary.LittleEndian.PutUint32(p, uint32(int32(num("P"))))
		h.Write(p)
		h.Write(id)
		if R >= 4 {
			if em, ok := enc["EncryptMetadata"].(bool); ok && !em {
				h.Write([]byte{0xff, 0xff, 0xff, 0xff})
			}
		}
		sum := h.Sum(nil)
		if R >= 3 {
			for i := 0; i < 50; i++ {
				s := md5.Sum(sum[:n])
				sum = s[:]
			}
		}
		c.key = sum[:n]
	}
	d.crypt = c
	for num, o := range d.objs {
		if num == skip {
			continue
		}
		if s, ok := o.(*pdfStream); ok && s.Dict["Type"] == pdfName("XRef") {
			continue
		}
		d.objs[num] = c.decryptObj(o, num, d.gens[num])
	}
}

func pdfHash2B(pwd, salt, udata []byte) []byte {
	h := sha256.Sum256(append(append(append([]byte{}, pwd...), salt...), udata...))
	k := h[:]
	for i := 0; ; i++ {
		var k1 []byte
		seq := append(append(append([]byte{}, pwd...), k...), udata...)
		for j := 0; j < 64; j++ {
			k1 = append(k1, seq...)
		}
		blk, _ := aes.NewCipher(k[:16])
		e := make([]byte, len(k1))
		cipher.NewCBCEncrypter(blk, k[16:32]).CryptBlocks(e, k1)
		sum := 0
		for _, b := range e[:16] {
			sum += int(b)
		}
		switch sum % 3 {
		case 0:
			s := sha256.Sum256(e)
			k = s[:]
		case 1:
			s := sha512.Sum384(e)
			k = s[:]
		case 2:
			s := sha512.Sum512(e)
			k = s[:]
		}
		if i >= 63 && int(e[len(e)-1]) <= i-31 {
			break
		}
	}
	return k[:32]
}

func (c *pdfCrypt) objKey(num, gen int) []byte {
	if c.v5 {
		return c.key
	}
	k := append([]byte{}, c.key...)
	k = append(k, byte(num), byte(num>>8), byte(num>>16), byte(gen), byte(gen>>8))
	if c.aes {
		k = append(k, 's', 'A', 'l', 'T')
	}
	s := md5.Sum(k)
	n := len(c.key) + 5
	if n > 16 {
		n = 16
	}
	return s[:n]
}

func (c *pdfCrypt) decrypt(b []byte, key []byte) []byte {
	if c.aes {
		if len(b) < 32 || len(b)%16 != 0 {
			return b
		}
		blk, err := aes.NewCipher(key)
		if err != nil {
			return b
		}
		out := make([]byte, len(b)-16)
		cipher.NewCBCDecrypter(blk, b[:16]).CryptBlocks(out, b[16:])
		if p := int(out[len(out)-1]); p > 0 && p <= 16 && p <= len(out) {
			out = out[:len(out)-p]
		}
		return out
	}
	r, err := rc4.NewCipher(key)
	if err != nil {
		return b
	}
	out := make([]byte, len(b))
	r.XORKeyStream(out, b)
	return out
}

func (c *pdfCrypt) decryptObj(o any, num, gen int) any {
	key := c.objKey(num, gen)
	var walk func(o any) any
	walk = func(o any) any {
		switch v := o.(type) {
		case pdfStr:
			return pdfStr(c.decrypt(v, key))
		case pdfArray:
			for i := range v {
				v[i] = walk(v[i])
			}
			return v
		case pdfDict:
			for k := range v {
				v[k] = walk(v[k])
			}
			return v
		case *pdfStream:
			walk(v.Dict)
			v.Raw = c.decrypt(v.Raw, key)
			return v
		}
		return o
	}
	return walk(o)
}

// ---------------- fonts ----------------

type cmapRange struct {
	lo, hi uint32
	n      int
}

type pdfFont struct {
	codespace []cmapRange
	toUni     map[uint32]string
	twoByte   bool
	simple    map[byte]rune
}

func (d *pdfDoc) loadFont(o any, cache map[any]*pdfFont) *pdfFont {
	if f, ok := cache[o]; ok {
		return f
	}
	f := &pdfFont{toUni: map[uint32]string{}}
	cache[o] = f
	fd := d.dict(o)
	if fd == nil {
		return f
	}
	if fd["Subtype"] == pdfName("Type0") {
		f.twoByte = true
	}
	if s, ok := d.resolve(fd["ToUnicode"]).(*pdfStream); ok {
		if data := d.decodeStream(s); data != nil {
			parseCMap(data, f)
		}
	}
	if !f.twoByte && len(f.toUni) == 0 {
		// simple font without ToUnicode: Latin-1 plus /Differences glyph names
		f.simple = map[byte]rune{}
		if ed := d.dict(fd["Encoding"]); ed != nil {
			if diff, ok := d.resolve(ed["Differences"]).(pdfArray); ok {
				code := 0
				for _, x := range diff {
					switch v := x.(type) {
					case float64:
						code = int(v)
					case pdfName:
						if r := glyphRune(string(v)); r != 0 && code < 256 {
							f.simple[byte(code)] = r
						}
						code++
					}
				}
			}
		}
	}
	return f
}

var glyphNames = map[string]rune{"space": ' ', "period": '.', "comma": ',', "hyphen": '-', "colon": ':', "semicolon": ';',
	"parenleft": '(', "parenright": ')', "zero": '0', "one": '1', "two": '2', "three": '3', "four": '4', "five": '5',
	"six": '6', "seven": '7', "eight": '8', "nine": '9', "quoteright": '\'', "slash": '/', "underscore": '_', "bullet": '•'}

func glyphRune(n string) rune {
	if len(n) == 1 {
		return rune(n[0])
	}
	if r, ok := glyphNames[n]; ok {
		return r
	}
	if strings.HasPrefix(n, "uni") && len(n) >= 7 {
		if v, err := strconv.ParseUint(n[3:7], 16, 32); err == nil {
			return rune(v)
		}
	}
	return 0
}

func beUint(b []byte) uint32 {
	var v uint32
	for _, c := range b {
		v = v<<8 | uint32(c)
	}
	return v
}

func utf16beString(b []byte) string {
	if len(b)%2 == 1 {
		b = append(b, 0)
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.BigEndian.Uint16(b[2*i:])
	}
	return string(utf16.Decode(u))
}

func parseCMap(data []byte, f *pdfFont) {
	l := &pdfLexer{b: data}
	var stack []any
	mode := ""
	for {
		t, err := l.token()
		if err != nil {
			break
		}
		if kw, ok := t.(pdfKeyword); ok {
			switch kw {
			case "begincodespacerange", "beginbfchar", "beginbfrange", "begincidrange", "begincidchar":
				mode = string(kw)
				stack = stack[:0]
				continue
			case "endcodespacerange":
				for i := 0; i+1 < len(stack); i += 2 {
					a, _ := stack[i].(pdfStr)
					b, _ := stack[i+1].(pdfStr)
					if len(a) > 0 {
						f.codespace = append(f.codespace, cmapRange{beUint(a), beUint(b), len(a)})
					}
				}
			case "endbfchar":
				for i := 0; i+1 < len(stack); i += 2 {
					a, _ := stack[i].(pdfStr)
					switch dst := stack[i+1].(type) {
					case pdfStr:
						f.toUni[beUint(a)] = utf16beString(dst)
					case pdfName:
						if r := glyphRune(string(dst)); r != 0 {
							f.toUni[beUint(a)] = string(r)
						}
					}
				}
			case "endbfrange":
				for i := 0; i+2 < len(stack); i += 3 {
					a, _ := stack[i].(pdfStr)
					b, _ := stack[i+1].(pdfStr)
					lo, hi := beUint(a), beUint(b)
					if hi < lo || hi-lo > 65536 {
						continue
					}
					switch dst := stack[i+2].(type) {
					case pdfStr:
						base := []rune(utf16beString(dst))
						if len(base) == 0 {
							continue
						}
						for c := lo; c <= hi; c++ {
							r := append([]rune{}, base...)
							r[len(r)-1] += rune(c - lo)
							f.toUni[c] = string(r)
						}
					case pdfArray:
						for k, x := range dst {
							if s, ok := x.(pdfStr); ok && lo+uint32(k) <= hi {
								f.toUni[lo+uint32(k)] = utf16beString(s)
							}
						}
					}
				}
			}
			if strings.HasPrefix(string(kw), "end") {
				mode = ""
				stack = stack[:0]
			} else if kw == "[" && mode != "" {
				o, _ := l.finish(kw)
				stack = append(stack, o)
			}
			continue
		}
		if mode != "" {
			stack = append(stack, t)
		}
	}
	if len(f.codespace) > 0 {
		f.twoByte = f.codespace[0].n == 2 && len(f.codespace) == 1
	}
}

func (f *pdfFont) decode(s []byte) string {
	var sb strings.Builder
	if f.simple != nil {
		for _, c := range s {
			if r, ok := f.simple[c]; ok {
				sb.WriteRune(r)
			} else if c >= 32 {
				sb.WriteRune(rune(c))
			}
		}
		return sb.String()
	}
	for i := 0; i < len(s); {
		n := 1
		if len(f.codespace) > 0 {
			n = 0
			for _, cr := range f.codespace {
				if i+cr.n <= len(s) {
					v := beUint(s[i : i+cr.n])
					if v >= cr.lo && v <= cr.hi {
						n = cr.n
						break
					}
				}
			}
			if n == 0 {
				n = 1
				if f.twoByte {
					n = 2
				}
			}
		} else if f.twoByte {
			n = 2
		}
		if i+n > len(s) {
			break
		}
		code := beUint(s[i : i+n])
		if u, ok := f.toUni[code]; ok {
			sb.WriteString(u)
		} else if n == 1 && code >= 32 && code < 127 && len(f.toUni) == 0 {
			sb.WriteByte(byte(code))
		}
		i += n
	}
	return sb.String()
}

// ---------------- pages & content ----------------

type textState struct {
	d     *pdfDoc
	sb    *strings.Builder
	fonts map[any]*pdfFont
	lastY float64
	haveY bool
	depth int
}

func (d *pdfDoc) pages() []pdfDict {
	var out []pdfDict
	seen := map[int]bool{}
	var walk func(o any, res any, depth int)
	walk = func(o any, res any, depth int) {
		if depth > 50 {
			return
		}
		if r, ok := o.(pdfRef); ok {
			if seen[r.N] {
				return
			}
			seen[r.N] = true
		}
		n := d.dict(o)
		if n == nil {
			return
		}
		if n["Resources"] != nil {
			res = n["Resources"]
		}
		if kids, ok := d.resolve(n["Kids"]).(pdfArray); ok {
			for _, k := range kids {
				walk(k, res, depth+1)
			}
			return
		}
		if n["Contents"] != nil {
			p := pdfDict{}
			for k, v := range n {
				p[k] = v
			}
			p["Resources"] = res
			out = append(out, p)
		}
	}
	for _, t := range d.trailer {
		if root := d.dict(t["Root"]); root != nil {
			walk(root["Pages"], nil, 0)
			if len(out) > 0 {
				return out
			}
		}
	}
	// fallback: any page objects in object order
	var nums []int
	for n, o := range d.objs {
		if dd, ok := o.(pdfDict); ok && dd["Type"] == pdfName("Page") {
			nums = append(nums, n)
		}
	}
	sort.Ints(nums)
	for _, n := range nums {
		out = append(out, d.objs[n].(pdfDict))
	}
	return out
}

func extractPDF(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || !bytes.HasPrefix(bytes.TrimLeft(data[:min(len(data), 1024)], "\x00\r\n\t "), []byte("%PDF")) {
		if err != nil || !bytes.Contains(data[:min(len(data), 1024)], []byte("%PDF")) {
			return ""
		}
	}
	d := loadPDF(data)
	var sb strings.Builder
	ts := &textState{d: d, sb: &sb, fonts: map[any]*pdfFont{}}
	for _, p := range d.pages() {
		res := d.dict(p["Resources"])
		var content []byte
		switch c := d.resolve(p["Contents"]).(type) {
		case *pdfStream:
			content = d.decodeStream(c)
		case pdfArray:
			for _, x := range c {
				if s, ok := d.resolve(x).(*pdfStream); ok {
					content = append(content, d.decodeStream(s)...)
					content = append(content, '\n')
				}
			}
		}
		ts.haveY = false
		ts.run(content, res)
		sb.WriteString("\n\n")
		if sb.Len() > maxTextBytes {
			break
		}
	}
	return fixArabicOrder(foldPresentation(sb.String()))
}

func (ts *textState) run(content []byte, res pdfDict) {
	if len(content) == 0 || ts.depth > 5 {
		return
	}
	d := ts.d
	fontsDict := d.dict(res["Font"])
	xobjs := d.dict(res["XObject"])
	var font *pdfFont
	l := &pdfLexer{b: content}
	var ops []any
	newline := func() {
		s := ts.sb.String()
		if len(s) > 0 && s[len(s)-1] != '\n' {
			ts.sb.WriteByte('\n')
		}
	}
	space := func() {
		s := ts.sb.String()
		if len(s) > 0 && s[len(s)-1] != ' ' && s[len(s)-1] != '\n' {
			ts.sb.WriteByte(' ')
		}
	}
	show := func(s pdfStr) {
		if font == nil {
			font = &pdfFont{simple: map[byte]rune{}}
		}
		ts.sb.WriteString(font.decode(s))
	}
	num := func(i int) float64 {
		if i < 0 || i >= len(ops) {
			return 0
		}
		f, _ := ops[i].(float64)
		return f
	}
	for {
		t, err := l.token()
		if err != nil {
			return
		}
		kw, isKw := t.(pdfKeyword)
		if !isKw || kw == "[" || kw == "<<" {
			o, _ := l.finish(t)
			ops = append(ops, o)
			if len(ops) > 1000 {
				ops = ops[len(ops)-100:]
			}
			continue
		}
		switch kw {
		case "Tf":
			if len(ops) >= 2 {
				if n, ok := ops[len(ops)-2].(pdfName); ok && fontsDict != nil {
					font = d.loadFont(fontsDict[string(n)], ts.fonts)
				}
			}
		case "Tj", "'", "\"":
			if kw != "Tj" {
				newline()
			}
			if len(ops) > 0 {
				if s, ok := ops[len(ops)-1].(pdfStr); ok {
					show(s)
				}
			}
		case "TJ":
			if len(ops) > 0 {
				if arr, ok := ops[len(ops)-1].(pdfArray); ok {
					for _, x := range arr {
						switch v := x.(type) {
						case pdfStr:
							show(v)
						case float64:
							if v < -180 {
								space()
							}
						}
					}
				}
			}
		case "Td", "TD":
			if len(ops) >= 2 {
				if num(len(ops)-1) != 0 {
					newline()
				} else if num(len(ops)-2) > 0 {
					space()
				}
			}
		case "Tm":
			if len(ops) >= 6 {
				y := num(len(ops) - 1)
				if ts.haveY && abs(int(y-ts.lastY)) > 1 {
					newline()
				} else {
					space()
				}
				ts.lastY, ts.haveY = y, true
			}
		case "T*":
			newline()
		case "ET":
			space()
		case "Do":
			if len(ops) > 0 && xobjs != nil {
				if n, ok := ops[len(ops)-1].(pdfName); ok {
					if s, ok := d.resolve(xobjs[string(n)]).(*pdfStream); ok && s.Dict["Subtype"] == pdfName("Form") {
						sub := d.dict(s.Dict["Resources"])
						if sub == nil {
							sub = res
						}
						ts.depth++
						ts.run(d.decodeStream(s), sub)
						ts.depth--
					}
				}
			}
		case "ID":
			// inline image data: skip to EI
			if i := bytes.Index(content[l.pos:], []byte("EI")); i >= 0 {
				for j := l.pos + i; j < len(content)-1; {
					if (j == 0 || isPDFSpace(content[j-1])) && (j+2 >= len(content) || isPDFSpace(content[j+2])) {
						l.pos = j + 2
						break
					}
					k := bytes.Index(content[j+2:], []byte("EI"))
					if k < 0 {
						l.pos = len(content)
						break
					}
					j = j + 2 + k
				}
			} else {
				return
			}
		}
		ops = ops[:0]
	}
}
