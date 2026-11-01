// icons draws the app's Streaming icons from the Live Music Archive logo (assets/lma-logo.png): the
// llama and its microphone on a round disc with transparent corners, as Q2 Pod's own Streaming
// icons are (assets/icons/stream_*.png). Two:
//
//	minimal: a black glyph on #8E8E93, the Configuration icon's grey (the glyph's colour is the
//	         user's choice). Minimal greys a card app's icon by its luminance (navigation.c,
//	         addon_is_icon); every built-in disc has a luminance near 142, so all of them, this one
//	         included, turn the same grey.
//	classic: a white glyph on black, as the user chose for Classic, which shows an icon's colours
//	         as they are.
//
// One file can't be both (black's luminance is black), so the app writes the one the theme needs
// (cmd/q2-lma/theme.go). Run from the repository's root:
//
//	go run ./tools/icons
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
)

const (
	cropX0, cropY0, cropX1, cropY1 = 60, 24, 250, 190 // the llama and microphone, above "LIVE"
	glyph                          = 0.64             // the glyph's width, of the disc's
	ss                             = 8                // samples a side per pixel
)

// discs is each icon's disc and glyph colours.
var discs = map[string][2]color.NRGBA{
	"minimal": {{0x8e, 0x8e, 0x93, 255}, {0, 0, 0, 255}},
	"classic": {{0, 0, 0, 255}, {255, 255, 255, 255}},
}

func main() {
	f, err := os.Open("assets/lma-logo.png")
	if err != nil {
		log.Fatal(err)
	}
	logo, err := png.Decode(f)
	if err != nil {
		log.Fatal(err)
	}
	os.MkdirAll("cmd/q2-lma/icons", 0o755)
	for name, c := range discs {
		for _, px := range []int{52, 40} {
			img := draw(logo, c[0], c[1], px)
			write(img, "cmd/q2-lma/icons/"+name+"-"+itoa(px)+".png")
			if name == "minimal" { // Q2 Pod's default theme: the release ships it
				out := map[int]string{52: "icon.png", 40: "icon-40.png"}[px]
				write(img, "contrib/shanlingq2/"+out)
			}
		}
	}
}

func itoa(n int) string { return string(rune('0'+n/10)) + string(rune('0'+n%10)) }

func write(img image.Image, p string) {
	out, err := os.Create(p)
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(out, img); err != nil {
		log.Fatal(err)
	}
	out.Close()
	log.Print(p)
}

// draw renders one icon px a side: per pixel, the disc's coverage and the glyph's, each from ss×ss
// samples.
func draw(logo image.Image, disc, ink color.NRGBA, px int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, px, px))
	cw, ch := float64(cropX1-cropX0), float64(cropY1-cropY0)
	gw := glyph * float64(px)
	scale := gw / cw
	gh := ch * scale
	gx, gy := (float64(px)-gw)/2, (float64(px)-gh)/2
	r := float64(px) / 2
	for y := 0; y < px; y++ {
		for x := 0; x < px; x++ {
			var in, white float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					fx := float64(x) + (float64(sx)+0.5)/ss
					fy := float64(y) + (float64(sy)+0.5)/ss
					dx, dy := fx-r, fy-r
					if dx*dx+dy*dy > r*r {
						continue
					}
					in++
					lx, ly := (fx-gx)/scale+cropX0, (fy-gy)/scale+cropY0
					if lx < cropX0 || ly < cropY0 || lx >= cropX1 || ly >= cropY1 {
						continue
					}
					cr, cg, cb, ca := logo.At(int(lx), int(ly)).RGBA()
					white += float64(cr+cg+cb) / 3 / 65535 * float64(ca) / 65535
				}
			}
			if in == 0 {
				continue
			}
			m := white / in // the logo's white is the glyph
			mix := func(d, g uint8) uint8 { return uint8(float64(d)*(1-m) + float64(g)*m + 0.5) }
			img.SetNRGBA(x, y, color.NRGBA{mix(disc.R, ink.R), mix(disc.G, ink.G), mix(disc.B, ink.B),
				uint8(in / (ss * ss) * 255)})
		}
	}
	return img
}
