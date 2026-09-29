package rutext

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// NBSP joins digit groups and words that must stay on one line.
const NBSP = "\u00a0"

// The rule matches web/src/ui/breaks.ts; the table in breaks_test.go mirrors breaks.test.ts.

var shortWords = set("в", "на", "и", "к", "с", "о", "у", "не", "от", "до", "по", "из", "за", "а", "но", "со", "ко", "во", "об")

var units = set(
	"шт", "кг", "г", "т", "м", "мм", "см", "км", "м²", "м³", "л", "ч", "мин", "сут", "мес", "год", "года", "лет",
	"чел", "ед", "тыс", "млн", "млрд", "%", "₽",
)

var scales = set("тыс", "млн", "млрд")

type gap int

// short: a preposition or conjunction and the next word; count: a word and its number (ошибок 0); chain: a word
// and a number that has a unit (Флот 13 шт); unit: a number and its unit (13 шт, 1,2 млн ₽).
const (
	noGap gap = iota
	gapShort
	gapCount
	gapChain
	gapUnit
)

// NOTE: a keep too wide for the line gives way at its weakest gap first, a number and its unit last.
var strength = map[gap]int{gapShort: 1, gapCount: 2, gapChain: 2, gapUnit: 3}

// Piece is laid out on one line: a Word, words joined by no-break spaces (Join), or a keep of smaller Parts.
type Piece struct {
	Word  string
	Parts []Piece
	Join  bool
}

// Text writes p as it prints on one line: parts of a keep are split by plain spaces, of a join by no-break spaces.
func (p Piece) Text() string {
	if p.Parts == nil {
		return p.Word
	}
	sep := " "
	if p.Join {
		sep = NBSP
	}
	out := make([]string, len(p.Parts))
	for i, q := range p.Parts {
		out[i] = q.Text()
	}
	return strings.Join(out, sep)
}

var (
	edge     = regexp.MustCompile(`^[^\p{L}\p{N}%₽/]+|[^\p{L}\p{N}%₽/]+$`)
	number   = regexp.MustCompile(`^[-+−]?\d+(?:[,.]\d+)?$`)
	slashed  = regexp.MustCompile(`^[\p{L}₽%]+/[\p{L}()·.²³]+$`)
	wordEnd  = regexp.MustCompile(`^[(«]?\p{L}[\p{L}-]*$`)
	letters  = regexp.MustCompile(`^[(«]?\p{L}+$`)
	numSpace = strings.NewReplacer(NBSP, "", "\u202f", "")
)

func set(ws ...string) map[string]bool {
	m := make(map[string]bool, len(ws))
	for _, w := range ws {
		m[w] = true
	}
	return m
}

func core(w string) string {
	return edge.ReplaceAllString(w, "")
}

func isNumber(w string) bool {
	return number.MatchString(numSpace.Replace(core(w)))
}

func isUnit(w string) bool {
	c := core(w)
	return units[c] || slashed.MatchString(c)
}

func isShort(w string) bool {
	return letters.MatchString(w) && shortWords[strings.ToLower(core(w))]
}

func endsInDigit(w string) bool {
	r, _ := utf8.DecodeLastRuneInString(w)
	return r >= '0' && r <= '9'
}

// isGroup reports whether w starts with three digits and no letter or digit after them (500 or 500,5 but not 5000).
func isGroup(w string) bool {
	if len(w) < 3 || strings.IndexFunc(w[:3], func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return false
	}
	r, _ := utf8.DecodeRuneInString(w[3:])
	return r == utf8.RuneError || !(unicode.IsLetter(r) || unicode.IsNumber(r))
}

// Words splits text at plain spaces and puts back digit groups written with a plain space (1 500).
func Words(text string) []string {
	var raw []string
	for _, w := range strings.Split(text, " ") {
		if w != "" {
			raw = append(raw, w)
		}
	}
	var out []string
	for i := 0; i < len(raw); i++ {
		w := raw[i]
		for i+1 < len(raw) && endsInDigit(w) && isGroup(raw[i+1]) {
			i++
			w += NBSP + raw[i]
		}
		out = append(out, w)
	}
	return out
}

func gaps(ws []string) []gap {
	amount := make([]bool, len(ws))
	for i, w := range ws {
		amount[i] = isNumber(w)
	}
	for i := 1; i < len(ws); i++ {
		if amount[i-1] && scales[core(ws[i])] {
			amount[i] = true
		}
	}
	unitAfter := make([]bool, len(ws))
	for i := range ws {
		unitAfter[i] = i+1 < len(ws) && amount[i] && isUnit(ws[i+1])
	}
	out := make([]gap, len(ws)-1)
	for i := range out {
		switch w := ws[i]; {
		case unitAfter[i]:
			out[i] = gapUnit
		case isShort(w):
			out[i] = gapShort
		case wordEnd.MatchString(w) && amount[i+1]:
			if unitAfter[i+1] {
				out[i] = gapChain
			} else {
				out[i] = gapCount
			}
		}
	}
	return out
}

// NOTE: a run of short-word gaps is a join, not a keep, so a line too narrow for it breaks at a no-break space
// rather than leaving the short word alone.
func piece(ws []string, gs []gap) Piece {
	if len(ws) == 1 {
		return Piece{Word: ws[0]}
	}
	allShort := true
	weakest := strength[gapUnit]
	for _, g := range gs {
		allShort = allShort && g == gapShort
		weakest = min(weakest, strength[g])
	}
	if allShort {
		parts := make([]Piece, len(ws))
		for i, w := range ws {
			parts[i] = Piece{Word: w}
		}
		return Piece{Parts: parts, Join: true}
	}
	type group struct {
		ws []string
		gs []gap
	}
	groups := []group{{ws: []string{ws[0]}}}
	for i, g := range gs {
		if strength[g] == weakest {
			groups = append(groups, group{ws: []string{ws[i+1]}})
		} else {
			last := &groups[len(groups)-1]
			last.ws = append(last.ws, ws[i+1])
			last.gs = append(last.gs, g)
		}
	}
	parts := make([]Piece, len(groups))
	for i, x := range groups {
		parts[i] = piece(x.ws, x.gs)
	}
	return Piece{Parts: parts}
}

// Layout turns text into pieces separated by plain spaces, where a line may break.
func Layout(text string) []Piece {
	ws := Words(text)
	if len(ws) == 0 {
		return nil
	}
	gs := gaps(ws)
	var out []Piece
	runWs, runGs := []string{ws[0]}, []gap(nil)
	for i, g := range gs {
		if g != noGap {
			runWs = append(runWs, ws[i+1])
			runGs = append(runGs, g)
			continue
		}
		out = append(out, piece(runWs, runGs))
		runWs, runGs = []string{ws[i+1]}, nil
	}
	return append(out, piece(runWs, runGs))
}
