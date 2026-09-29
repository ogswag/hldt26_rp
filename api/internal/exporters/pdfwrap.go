package exporters

import (
	"strings"

	"moscow_hackathon_2026/api/internal/rutext"
)

// liner fills lines with pieces of rutext.Layout. A piece too wide for a line of its own starts a new line and
// opens up: a keep and a join into their parts, a number into its digit groups, a word into letters.
type liner struct {
	fits  func(string) bool
	lines []string
	cur   string
}

func (l *liner) push() {
	l.lines = append(l.lines, l.cur)
	l.cur = ""
}

func (l *liner) add(pc rutext.Piece) {
	s := pc.Text()
	next := s
	if l.cur != "" {
		next = l.cur + " " + s
	}
	if l.fits(next) {
		l.cur = next
		return
	}
	if l.cur != "" {
		l.push()
	}
	if l.fits(s) {
		l.cur = s
		return
	}
	switch {
	case pc.Parts != nil:
		for _, q := range pc.Parts {
			l.add(q)
		}
	case strings.Contains(s, rutext.NBSP):
		for _, g := range strings.Split(s, rutext.NBSP) {
			l.add(rutext.Piece{Word: g})
		}
	default:
		for _, r := range s {
			if l.cur != "" && !l.fits(l.cur+string(r)) {
				l.push()
			}
			l.cur += string(r)
		}
	}
}

// wrapLines splits text into lines that pass fits, one or more per paragraph of text.
func wrapLines(text string, fits func(string) bool) []string {
	var out []string
	for _, para := range strings.Split(text, "\n") {
		l := liner{fits: fits}
		for _, pc := range rutext.Layout(para) {
			l.add(pc)
		}
		out = append(out, l.lines...)
		out = append(out, l.cur)
	}
	return out
}

// wrap splits text into lines that fit a cell of width w in the current font.
func (p *pdfDoc) wrap(text string, w float64) []string {
	avail := w - 2*p.GetCellMargin()
	return wrapLines(text, func(s string) bool { return p.GetStringWidth(s) <= avail })
}

// text writes wrapped lines of height h across the content width.
func (p *pdfDoc) text(h float64, s string) {
	p.said = append(p.said, s)
	w := p.contentWidth()
	for _, ln := range p.wrap(s, w) {
		p.CellFormat(w, h, ln, "", 1, "L", false, 0, "")
	}
}
