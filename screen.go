package main

import (
	"fmt"
	"image"

	"github.com/kbinani/screenshot"
)

// RGB holds an 8-bit color value.
type RGB struct {
	R, G, B uint8
}

func (c RGB) String() string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// CaptureScreen captures display 0 and returns the image.
func CaptureScreen() (*image.RGBA, error) {
	n := screenshot.NumActiveDisplays()
	if n == 0 {
		return nil, fmt.Errorf("no active displays found")
	}
	bounds := screenshot.GetDisplayBounds(0)
	img, err := screenshot.CaptureRect(bounds)
	if err != nil {
		return nil, fmt.Errorf("capturing screen: %w", err)
	}
	return img, nil
}

// AverageColor computes the mean RGB of an RGBA image.
// It samples every Nth pixel to keep computation fast on large displays.
func AverageColor(img *image.RGBA) RGB {
	w := img.Rect.Dx()
	h := img.Rect.Dy()
	pixels := w * h
	if pixels == 0 {
		return RGB{}
	}

	// Target ~10000 samples for performance.
	step := pixels / 10000
	if step < 1 {
		step = 1
	}

	var rSum, gSum, bSum uint64
	var count uint64

	pix := img.Pix
	stride := img.Stride
	for i := 0; i < pixels; i += step {
		off := (i/w)*stride + (i%w)*4
		rSum += uint64(pix[off])
		gSum += uint64(pix[off+1])
		bSum += uint64(pix[off+2])
		count++
	}

	return RGB{
		R: uint8(rSum / count),
		G: uint8(gSum / count),
		B: uint8(bSum / count),
	}
}
