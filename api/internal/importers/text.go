package importers

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// NOTE: yo/safe.txt.gz is the "safe" dictionary of eyo-kernel (MIT, yo/LICENSE): words where е is always ё.
//
//go:embed yo/safe.txt.gz
var yoDictionary []byte

// Catalog text: ё where the word has it, «» instead of straight quotes.
type textNormalizer struct {
	yo map[string]string
}

func newTextNormalizer() (*textNormalizer, error) {
	zr, err := gzip.NewReader(bytes.NewReader(yoDictionary))
	if err != nil {
		return nil, fmt.Errorf("importers.yo: %w", err)
	}
	defer zr.Close()
	yo := make(map[string]string, 110000)
	sc := bufio.NewScanner(zr)
	for sc.Scan() {
		// A line is a stem and its endings: Аксён(|а|ам).
		stem, endings, ok := strings.Cut(sc.Text(), "(")
		if !ok {
			addYo(yo, stem)
			continue
		}
		for _, end := range strings.Split(strings.TrimSuffix(endings, ")"), "|") {
			addYo(yo, stem+end)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("importers.yo: %w", err)
	}
	return &textNormalizer{yo: yo}, nil
}

var (
	sharedOnce sync.Once
	sharedNorm *textNormalizer
	sharedErr  error
)

// sharedText builds the dictionary once per process; it is read-only afterwards.
func sharedText() (*textNormalizer, error) {
	sharedOnce.Do(func() { sharedNorm, sharedErr = newTextNormalizer() })
	return sharedNorm, sharedErr
}

// Text puts ё where the word has it and «» instead of straight double quotes.
func Text(s string) string {
	n, err := sharedText()
	if err != nil {
		return quotes(s)
	}
	return n.Text(s)
}

var yoToE = strings.NewReplacer("ё", "е", "Ё", "Е")

func addYo(yo map[string]string, word string) {
	if word == "" {
		return
	}
	yo[yoToE.Replace(word)] = word
}

// Text returns s with ё restored and straight double quotes turned into «» (and „“ inside them).
func (n *textNormalizer) Text(s string) string {
	return n.words(quotes(s))
}

func (n *textNormalizer) words(s string) string {
	var b strings.Builder
	start := -1
	flush := func(end int) {
		if start >= 0 {
			b.WriteString(n.word(s[start:end]))
			start = -1
		}
	}
	for i, r := range s {
		if isCyrillic(r) {
			if start < 0 {
				start = i
			}
			continue
		}
		flush(i)
		b.WriteRune(r)
	}
	flush(len(s))
	return b.String()
}

func (n *textNormalizer) word(w string) string {
	if v, ok := n.yo[w]; ok {
		return v
	}
	// A word that starts a sentence has a capital letter the dictionary does not.
	first, size := utf8.DecodeRuneInString(w)
	if !unicode.IsUpper(first) {
		return w
	}
	lower := string(unicode.ToLower(first)) + w[size:]
	if v, ok := n.yo[lower]; ok {
		r, sz := utf8.DecodeRuneInString(v)
		return string(unicode.ToUpper(r)) + v[sz:]
	}
	return w
}

func isCyrillic(r rune) bool {
	return (r >= 'а' && r <= 'я') || (r >= 'А' && r <= 'Я') || r == 'ё' || r == 'Ё'
}

// quotes pairs straight double quotes: one after a space, a bracket or the start of the text opens, any other
// closes. Quotes inside quotes are „“.
func quotes(s string) string {
	if !strings.Contains(s, `"`) {
		return s
	}
	var b strings.Builder
	depth := 0
	prev := ' '
	for _, r := range s {
		if r != '"' {
			b.WriteRune(r)
			prev = r
			continue
		}
		opens := unicode.IsSpace(prev) || strings.ContainsRune("([{«„-/", prev)
		switch {
		case opens && depth == 0:
			r = '«'
			depth++
		case opens:
			r = '„'
			depth++
		case depth >= 2:
			r = '“'
			depth--
		default:
			r = '»'
			if depth > 0 {
				depth--
			}
		}
		b.WriteRune(r)
		prev = r
	}
	return b.String()
}
