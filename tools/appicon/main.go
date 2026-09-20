// Command appicon renders build/appicon.png from the brand mark, laid out on
// Apple's macOS icon grid: an 824x824 rounded square centred in a 1024x1024
// transparent canvas.
package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"
)

const (
	canvas = 1024
	tile   = 824
	radius = 185.0
)

func main() {
	if len(os.Args) != 3 {
		log.Fatal("usage: appicon <src.png> <dst.png>")
	}

	src := decode(os.Args[1])
	scaled := resize(src, tile, tile)

	out := image.NewNRGBA(image.Rect(0, 0, canvas, canvas))
	offset := (canvas - tile) / 2

	for y := 0; y < tile; y++ {
		for x := 0; x < tile; x++ {
			a := cornerAlpha(float64(x)+0.5, float64(y)+0.5)
			if a == 0 {
				continue
			}
			r, g, b, _ := scaled.At(x, y).RGBA()
			out.SetNRGBA(x+offset, y+offset, color.NRGBA{
				R: uint8(r >> 8),
				G: uint8(g >> 8),
				B: uint8(b >> 8),
				A: uint8(a * 255),
			})
		}
	}

	f, err := os.Create(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, out); err != nil {
		log.Fatal(err)
	}
}

// cornerAlpha returns the coverage of the rounded-square mask at a pixel
// centre, antialiased over one pixel at the corner arcs.
func cornerAlpha(x, y float64) float64 {
	cx, cy := radius, radius
	if x > tile-radius {
		cx = tile - radius
	} else if x >= radius {
		cx = x
	}
	if y > tile-radius {
		cy = tile - radius
	} else if y >= radius {
		cy = y
	}

	d := math.Hypot(x-cx, y-cy)
	switch {
	case d <= radius-0.5:
		return 1
	case d >= radius+0.5:
		return 0
	default:
		return radius + 0.5 - d
	}
}

func decode(path string) image.Image {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		log.Fatal(err)
	}
	return img
}

// resize does a bilinear rescale, which is plenty for a downscale of a flat
// vector-style mark.
func resize(src image.Image, w, h int) *image.NRGBA {
	b := src.Bounds()
	rgba := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), src, b.Min, draw.Src)

	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	sx := float64(b.Dx()) / float64(w)
	sy := float64(b.Dy()) / float64(h)

	for y := 0; y < h; y++ {
		fy := (float64(y)+0.5)*sy - 0.5
		y0 := int(math.Floor(fy))
		wy := fy - float64(y0)

		for x := 0; x < w; x++ {
			fx := (float64(x)+0.5)*sx - 0.5
			x0 := int(math.Floor(fx))
			wx := fx - float64(x0)

			var acc [4]float64
			for dy := 0; dy <= 1; dy++ {
				for dx := 0; dx <= 1; dx++ {
					c := rgba.NRGBAAt(clamp(x0+dx, b.Dx()), clamp(y0+dy, b.Dy()))
					f := (1 - math.Abs(float64(dx)-wx)) * (1 - math.Abs(float64(dy)-wy))
					acc[0] += float64(c.R) * f
					acc[1] += float64(c.G) * f
					acc[2] += float64(c.B) * f
					acc[3] += float64(c.A) * f
				}
			}
			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8(acc[0] + 0.5),
				G: uint8(acc[1] + 0.5),
				B: uint8(acc[2] + 0.5),
				A: uint8(acc[3] + 0.5),
			})
		}
	}
	return dst
}

func clamp(v, max int) int {
	if v < 0 {
		return 0
	}
	if v >= max {
		return max - 1
	}
	return v
}
