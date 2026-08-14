// Command dither generates the 1-bit ordered-dither PNG assets used by the
// site. Every "gray" on the page is a lie told with ink pixels; this is the
// program that tells it. Run from the repo root:
//
//	go run ./tools/dither
//
// It writes deterministic, tiny PNGs into sendkey/public/assets/px/. The
// files are committed, so this only needs re-running when the palette or the
// patterns change.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
)

// ink matches --ink in brand.css. The background stays transparent so one
// tile works on paper, on cards, and on anything else.
var ink = color.NRGBA{R: 0x14, G: 0x14, B: 0x12, A: 0xff}

// bayer8 is the classic 8x8 ordered-dither threshold matrix. Cell values
// 0..63; a pixel is inked when value*64 > threshold*levels, which turns a
// target density into a fixed, tileable pattern.
var bayer8 = [8][8]int{
	{0, 32, 8, 40, 2, 34, 10, 42},
	{48, 16, 56, 24, 50, 18, 58, 26},
	{12, 44, 4, 36, 14, 46, 6, 38},
	{60, 28, 52, 20, 62, 30, 54, 22},
	{3, 35, 11, 43, 1, 33, 9, 41},
	{51, 19, 59, 27, 49, 17, 57, 25},
	{15, 47, 7, 39, 13, 45, 5, 37},
	{63, 31, 55, 23, 61, 29, 53, 21},
}

// tile renders a w x h image where density(x, y) in [0,1] decides how much
// ink the Bayer matrix lays down around that point.
func tile(w, h int, density func(x, y int) float64) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			d := density(x, y)
			if d < 0 {
				d = 0
			}
			if d > 1 {
				d = 1
			}
			if d*64 > float64(bayer8[y%8][x%8]) {
				img.SetNRGBA(x, y, ink)
			}
		}
	}
	return img
}

func write(dir, name string, img *image.NRGBA) {
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatalf("encode %s: %v", path, err)
	}
	st, _ := f.Stat()
	fmt.Printf("wrote %s (%d bytes)\n", path, st.Size())
}

func main() {
	dir := filepath.Join("sendkey", "public", "assets", "px")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}

	// Uniform texture tiles: constant density through the matrix gives a
	// stable repeating pattern, not noise.
	write(dir, "d06.png", tile(8, 8, func(x, y int) float64 { return 0.06 }))
	write(dir, "d12.png", tile(8, 8, func(x, y int) float64 { return 0.125 }))
	write(dir, "d25.png", tile(8, 8, func(x, y int) float64 { return 0.25 }))

	// Vertical fade, transparent at the top to solid ink at the bottom.
	// Tiled horizontally it dissolves a paper section into an ink one.
	const fh = 64
	write(dir, "fade.png", tile(8, fh, func(x, y int) float64 {
		return float64(y) / float64(fh-1)
	}))
}
