package exporters

import (
	"testing"

	"golang.org/x/image/font/sfnt"
)

// TestReportFontHasEveryGlyph checks the embedded faces against what the report prints: Russian text with ё,
// numbers with no-break spaces, the ruble sign and quotes. A missing glyph prints as a blank.
func TestReportFontHasEveryGlyph(t *testing.T) {
	var text []rune
	for r := 'А'; r <= 'я'; r++ {
		text = append(text, r)
	}
	for r := ' '; r <= '~'; r++ {
		text = append(text, r)
	}
	text = append(text, []rune("Ёё₽²³«»№  ")...)
	faces := []struct {
		name string
		data []byte
	}{
		{"regular", plexRegular},
		{"semibold", plexSemiBold},
	}
	for _, face := range faces {
		t.Run(face.name, func(t *testing.T) {
			f, err := sfnt.Parse(face.data)
			if err != nil {
				t.Fatal(err)
			}
			var buf sfnt.Buffer
			for _, r := range text {
				i, err := f.GlyphIndex(&buf, r)
				if err != nil {
					t.Fatal(err)
				}
				if i == 0 {
					t.Errorf("no glyph for %q (U+%04X)", r, r)
				}
			}
		})
	}
}
