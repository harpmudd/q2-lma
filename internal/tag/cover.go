package tag

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png" // a recording's art may be a PNG
	"sync"
)

// The Live Music Archive's llama logo: the cover of a song whose recording has no art of its own.
//
//go:embed logo.png
var logoPNG []byte

const (
	coverPx   = 600      // a side, at most, as q2-podcasts sizes covers for the Q2
	maxPixels = 12 << 20 // larger images aren't decoded: about 18 MB of the app's 48 MB
)

var logo = sync.OnceValue(func() []byte {
	b, _ := Cover(logoPNG)
	return b
})

// Logo is the fallback cover as a JPEG.
func Logo() []byte { return logo() }

// Cover is img (a JPEG or PNG) as a cover the Q2 shows: a baseline JPEG at most coverPx a side,
// transparency flattened onto black. It fails for an image it can't read or one too large to
// decode in the Q2's memory.
func Cover(img []byte) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(img))
	if err != nil {
		return nil, err
	}
	if cfg.Width*cfg.Height > maxPixels || cfg.Width == 0 || cfg.Height == 0 {
		return nil, ErrFormat
	}
	src, _, err := image.Decode(bytes.NewReader(img))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > coverPx || h > coverPx {
		if w > h {
			w, h = coverPx, h*coverPx/w
		} else {
			w, h = w*coverPx/h, coverPx
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
	shrink(dst, src)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 88}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// shrink draws src over dst (already black), each destination pixel the average of the source
// pixels it covers, in integers (the Q2's Go has no FPU).
func shrink(dst *image.RGBA, src image.Image) {
	b := src.Bounds()
	dw, dh, sw, sh := dst.Rect.Dx(), dst.Rect.Dy(), b.Dx(), b.Dy()
	for y := 0; y < dh; y++ {
		y0, y1 := y*sh/dh, max((y+1)*sh/dh, y*sh/dh+1)
		for x := 0; x < dw; x++ {
			x0, x1 := x*sw/dw, max((x+1)*sw/dw, x*sw/dw+1)
			var r, g, bl, a, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					cr, cg, cb, ca := src.At(b.Min.X+sx, b.Min.Y+sy).RGBA()
					r, g, bl, a, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca), n+1
				}
			}
			// premultiplied over black: the colour sums already carry their alpha
			_ = a
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n >> 8), uint8(g / n >> 8), uint8(bl / n >> 8), 255})
		}
	}
}
