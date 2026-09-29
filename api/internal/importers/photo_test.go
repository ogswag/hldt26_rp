package importers

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func pngBytes(t *testing.T, w, h int, alpha uint8) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 90, A: alpha})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// pngHeader is a PNG that stops after its IHDR chunk: enough to read the size, too little to decode.
func pngHeader(w, h uint32) []byte {
	var ihdr bytes.Buffer
	ihdr.WriteString("IHDR")
	_ = binary.Write(&ihdr, binary.BigEndian, w)
	_ = binary.Write(&ihdr, binary.BigEndian, h)
	ihdr.Write([]byte{8, 6, 0, 0, 0})
	var out bytes.Buffer
	out.WriteString("\x89PNG\r\n\x1a\n")
	_ = binary.Write(&out, binary.BigEndian, uint32(13))
	out.Write(ihdr.Bytes())
	_ = binary.Write(&out, binary.BigEndian, crc32.ChecksumIEEE(ihdr.Bytes()))
	return out.Bytes()
}

func TestNormalizePhoto(t *testing.T) {
	opaque, err := NormalizePhoto(pngBytes(t, 2400, 600, 255))
	if err != nil {
		t.Fatal(err)
	}
	if opaque.ContentType != "image/jpeg" || len(opaque.SHA256) != 64 {
		t.Fatalf("opaque photo: %s %s", opaque.ContentType, opaque.SHA256)
	}
	img, err := jpeg.Decode(bytes.NewReader(opaque.Bytes))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 1200 || b.Dy() != 300 {
		t.Fatalf("scaled to %v, want 1200x300", b)
	}

	clear, err := NormalizePhoto(pngBytes(t, 300, 900, 128))
	if err != nil {
		t.Fatal(err)
	}
	if clear.ContentType != "image/png" {
		t.Fatalf("transparent photo stored as %s", clear.ContentType)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(clear.Bytes))
	if err != nil || cfg.Width != 300 || cfg.Height != 900 {
		t.Fatalf("small photo resized: %+v %v", cfg, err)
	}

	again, err := NormalizePhoto(pngBytes(t, 2400, 600, 255))
	if err != nil || again.SHA256 != opaque.SHA256 {
		t.Fatal("the same photo must give the same bytes")
	}

	if _, err := NormalizePhoto([]byte("<html>not an image</html>")); !errors.Is(err, ErrPhotoFormat) {
		t.Fatalf("html: %v", err)
	}
	if _, err := NormalizePhoto(pngHeader(10000, 5000)); !errors.Is(err, ErrPhotoPixels) {
		t.Fatalf("50 megapixels: %v", err)
	}
	if _, err := NormalizePhoto(pngHeader(100, 100)); !errors.Is(err, ErrPhotoFormat) {
		t.Fatalf("truncated png: %v", err)
	}
}

func TestPublicAddress(t *testing.T) {
	tests := map[string]bool{
		"8.8.8.8":          true,
		"77.88.8.8":        true,
		"2a00:1450::1":     true,
		"127.0.0.1":        false,
		"10.1.2.3":         false,
		"172.16.0.1":       false,
		"192.168.1.10":     false,
		"169.254.169.254":  false,
		"100.64.0.1":       false,
		"0.0.0.0":          false,
		"224.0.0.1":        false,
		"::1":              false,
		"fd00::1":          false,
		"fe80::1":          false,
		"::ffff:127.0.0.1": false,
		"::ffff:10.0.0.1":  false,
		"64:ff9b::a00:1":   false,
	}
	for addr, want := range tests {
		if got := PublicAddress(netip.MustParseAddr(addr)); got != want {
			t.Errorf("PublicAddress(%s) = %v, want %v", addr, got, want)
		}
	}
}

func TestPhotoFetcher(t *testing.T) {
	photo := pngBytes(t, 40, 30, 255)
	mux := http.NewServeMux()
	mux.HandleFunc("/photo.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(photo)
	})
	mux.HandleFunc("/page", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html></html>"))
	})
	mux.HandleFunc("/huge", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(bytes.Repeat([]byte{0xff}, MaxPhotoBytes+10))
	})
	mux.HandleFunc("/gone", func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	})
	mux.HandleFunc("/file", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ctx := context.Background()

	if _, err := NewPhotoFetcher(false).Fetch(ctx, srv.URL+"/photo.png"); !errors.Is(err, ErrPhotoAddress) {
		t.Fatalf("a local address must be refused: %v", err)
	}
	for _, link := range []string{"file:///etc/passwd", "ftp://example.com/a.png", "/photo.png", "http://"} {
		if _, err := NewPhotoFetcher(true).Fetch(ctx, link); !errors.Is(err, ErrPhotoAddress) {
			t.Errorf("%s: %v", link, err)
		}
	}

	local := NewPhotoFetcher(true)
	got, err := local.Fetch(ctx, srv.URL+"/photo.png")
	if err != nil {
		t.Fatal(err)
	}
	if got.ContentType != "image/jpeg" || len(got.Bytes) == 0 {
		t.Fatalf("fetched %s, %d bytes", got.ContentType, len(got.Bytes))
	}
	for path, want := range map[string]error{
		"/page": ErrPhotoResponse,
		"/gone": ErrPhotoResponse,
		"/huge": ErrPhotoSize,
		"/file": ErrPhotoAddress,
	} {
		if _, err := local.Fetch(ctx, srv.URL+path); !errors.Is(err, want) {
			t.Errorf("%s: %v, want %v", path, err, want)
		}
	}
}
