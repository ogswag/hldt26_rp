package exporters

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"
)

// NOTE: IBM Plex Sans (OFL, fonts/OFL.txt), the typeface of the web app; semibold stands in for "B".
var (
	//go:embed fonts/IBMPlexSans-Regular.ttf
	plexRegular []byte
	//go:embed fonts/IBMPlexSans-SemiBold.ttf
	plexSemiBold []byte
)

const (
	pdfFont   = "plex"
	pdfMargin = 14.0
	pdfBottom = 16.0
	ptToMM    = 0.3528
)

type pdfDoc struct {
	*fpdf.Fpdf
	size  float64
	style string
	// said keeps every text and table cell as written, before it is wrapped; tests read the document through it.
	said []string
}

// addFonts registers the report's typeface in both weights it uses.
func addFonts(f *fpdf.Fpdf) {
	f.AddUTF8FontFromBytes(pdfFont, "", plexRegular)
	f.AddUTF8FontFromBytes(pdfFont, "B", plexSemiBold)
}

// PDF renders the report: a short financial one for a calculation, an operational one for a simulation.
func PDF(r Report) ([]byte, error) {
	data, _, err := renderPDF(r)
	return data, err
}

func renderPDF(r Report) (data []byte, said []string, err error) {
	f := fpdf.New("P", "mm", "A4", "")
	p := &pdfDoc{Fpdf: f}
	p.SetTitle(title(r), true)
	p.SetAuthor("platform", true)
	p.SetCreator("platform", true)
	p.SetCreationDate(r.generatedAt())
	p.SetModificationDate(r.generatedAt())
	addFonts(p.Fpdf)
	p.SetMargins(pdfMargin, pdfMargin, pdfMargin)
	p.SetAutoPageBreak(true, pdfBottom)
	p.AliasNbPages("{nb}")
	p.SetFooterFunc(func() { p.footer(r) })
	p.AddPage()

	switch {
	case r.Econ != nil:
		p.userFinancial(r)
	case r.Sim != nil:
		p.userSimulation(r)
	default:
		return nil, nil, errors.New("exporters.pdf: report has no result")
	}

	if err := p.Error(); err != nil {
		return nil, nil, fmt.Errorf("exporters.pdf: %w", err)
	}
	var buf bytes.Buffer
	if err := p.Output(&buf); err != nil {
		return nil, nil, fmt.Errorf("exporters.pdf: %w", err)
	}
	return buf.Bytes(), p.said, nil
}

func (p *pdfDoc) font(size float64) {
	p.size = size
	p.style = ""
	p.SetFont(pdfFont, "", size)
}

func (p *pdfDoc) bold(size float64) {
	p.size = size
	p.style = "B"
	p.SetFont(pdfFont, "B", size)
}

// restore sets back the font the text was in, after a footer or a drawing changed it.
func (p *pdfDoc) restore() {
	p.SetFont(pdfFont, p.style, p.size)
}

func (p *pdfDoc) lineH() float64 {
	return p.size * ptToMM * 1.25
}

func (p *pdfDoc) contentWidth() float64 {
	w, _ := p.GetPageSize()
	return w - 2*pdfMargin
}

func (p *pdfDoc) ensure(h float64) {
	_, ph := p.GetPageSize()
	if p.GetY()+h > ph-pdfBottom {
		p.AddPage()
	}
}

func (p *pdfDoc) footer(r Report) {
	p.SetY(-10)
	p.SetFont(pdfFont, "", 7)
	p.SetTextColor(110, 110, 110)
	p.CellFormat(0, 4, fmt.Sprintf("Отчёт для принятия решения. Стр. %d из {nb}", p.PageNo()), "", 0, "L", false, 0, "")
	p.SetTextColor(0, 0, 0)
	p.restore()
}

func (p *pdfDoc) h2(text string) {
	p.ensure(30)
	p.Ln(2)
	p.bold(12)
	p.SetTextColor(20, 60, 120)
	p.text(6, text)
	p.SetTextColor(0, 0, 0)
	p.Ln(1)
}

func (p *pdfDoc) h3(text string) {
	p.ensure(22)
	p.bold(10)
	p.text(5, text)
}

func (p *pdfDoc) para(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	p.text(p.lineH(), text)
	p.Ln(0.8)
}

// list writes items as dashed lines under the current heading.
func (p *pdfDoc) list(items []string) {
	p.font(8.5)
	for _, s := range items {
		p.para("- " + s)
	}
}

// table draws wrapped rows and repeats the header after a page break. weights are relative column widths.
func (p *pdfDoc) table(weights []float64, header []string, rows [][]string, size float64) {
	total := 0.0
	for _, w := range weights {
		total += w
	}
	width := p.contentWidth()
	widths := make([]float64, len(weights))
	for i, w := range weights {
		widths[i] = w / total * width
	}
	measure := func(cells []string) ([][]string, float64) {
		p.font(size)
		split := make([][]string, len(widths))
		n := 1
		for i := range widths {
			txt := ""
			if i < len(cells) {
				txt = cells[i]
			}
			split[i] = p.wrap(txt, widths[i])
			if len(split[i]) > n {
				n = len(split[i])
			}
		}
		return split, float64(n)*p.lineH() + 1.6
	}
	var draw func(cells []string, head bool)
	draw = func(cells []string, head bool) {
		split, h := measure(cells)
		lh := p.lineH()
		_, ph := p.GetPageSize()
		if p.GetY()+h > ph-pdfBottom {
			p.AddPage()
			if !head && header != nil {
				draw(header, true)
				p.font(size)
			}
		}
		x, y := pdfMargin, p.GetY()
		if head {
			p.SetFillColor(236, 240, 246)
		}
		p.SetDrawColor(190, 196, 206)
		p.said = append(p.said, cells...)
		for i, w := range widths {
			style := "D"
			if head {
				style = "FD"
			}
			p.Rect(x, y, w, h, style)
			for k, ln := range split[i] {
				p.Text(x+1, y+0.8+float64(k)*lh+size*ptToMM*0.95, ln)
			}
			x += w
		}
		p.SetDrawColor(0, 0, 0)
		p.SetXY(pdfMargin, y+h)
	}
	if header != nil {
		_, need := measure(header)
		if len(rows) > 0 {
			_, first := measure(rows[0])
			need += first
		}
		p.ensure(need)
		draw(header, true)
	}
	for _, row := range rows {
		draw(row, false)
	}
	p.Ln(2)
}

func (p *pdfDoc) kv(rows [][2]string) {
	body := make([][]string, 0, len(rows))
	for _, r := range rows {
		body = append(body, []string{r[0], r[1]})
	}
	p.table([]float64{32, 68}, nil, body, 8.5)
}
