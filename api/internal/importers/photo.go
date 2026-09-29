package importers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"net/http"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// MaxPhotoBytes caps a photo file before it is scaled down.
const MaxPhotoBytes = 8 << 20

const (
	maxPhotoSide   = 1200
	maxPhotoPixels = 40_000_000
)

var (
	ErrPhotoFormat = errors.New("photo: not a JPEG, PNG, WebP or GIF image")
	ErrPhotoPixels = errors.New("photo: more than 40 megapixels")
)

// Photo is a robot photo as the catalog stores it.
type Photo struct {
	Bytes       []byte
	ContentType string
	SHA256      string
}

// NormalizePhoto decodes a JPEG, PNG, WebP or GIF image and scales it down to at most 1 200 px on the longer side.
// Opaque images become JPEG, images with transparency PNG; metadata such as GPS tags is dropped.
func NormalizePhoto(data []byte) (Photo, error) {
	switch http.DetectContentType(data) {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
	default:
		return Photo{}, ErrPhotoFormat
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Photo{}, ErrPhotoFormat
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return Photo{}, ErrPhotoFormat
	}
	if cfg.Width*cfg.Height > maxPhotoPixels {
		return Photo{}, ErrPhotoPixels
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Photo{}, ErrPhotoFormat
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > maxPhotoSide || h > maxPhotoSide {
		if w >= h {
			w, h = maxPhotoSide, max(1, h*maxPhotoSide/w)
		} else {
			w, h = max(1, w*maxPhotoSide/h), maxPhotoSide
		}
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)

	var buf bytes.Buffer
	ct := "image/jpeg"
	if dst.Opaque() {
		err = jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85})
	} else {
		ct = "image/png"
		err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, dst)
	}
	if err != nil {
		return Photo{}, fmt.Errorf("importers.photo.encode: %w", err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return Photo{Bytes: buf.Bytes(), ContentType: ct, SHA256: hex.EncodeToString(sum[:])}, nil
}
