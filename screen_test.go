package main

import (
	"image"
	"testing"
)

func TestAverageColor_Uniform(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = 200
		img.Pix[i+1] = 100
		img.Pix[i+2] = 50
		img.Pix[i+3] = 255
	}

	got := AverageColor(img)
	if got.R != 200 || got.G != 100 || got.B != 50 {
		t.Errorf("expected RGB{200, 100, 50}, got RGB{%d, %d, %d}", got.R, got.G, got.B)
	}
}

func TestAverageColor_BlackAndWhite(t *testing.T) {
	// 2x1 image: one black pixel, one white pixel.
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Pix[0] = 0
	img.Pix[1] = 0
	img.Pix[2] = 0
	img.Pix[3] = 255
	img.Pix[4] = 255
	img.Pix[5] = 255
	img.Pix[6] = 255
	img.Pix[7] = 255

	got := AverageColor(img)
	if got.R != 127 || got.G != 127 || got.B != 127 {
		t.Errorf("expected RGB{127, 127, 127}, got RGB{%d, %d, %d}", got.R, got.G, got.B)
	}
}

func TestAverageColor_Empty(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 0, 0))
	got := AverageColor(img)
	if got.R != 0 || got.G != 0 || got.B != 0 {
		t.Errorf("expected RGB{0, 0, 0}, got RGB{%d, %d, %d}", got.R, got.G, got.B)
	}
}
