package main

import "testing"

func TestAverageRGB_Uniform(t *testing.T) {
	// 4 pixels, all the same color.
	buf := []byte{
		200, 100, 50,
		200, 100, 50,
		200, 100, 50,
		200, 100, 50,
	}
	got := averageRGB(buf, 4)
	if got.R != 200 || got.G != 100 || got.B != 50 {
		t.Errorf("expected RGB{200, 100, 50}, got %v", got)
	}
}

func TestAverageRGB_BlackAndWhite(t *testing.T) {
	// 2 pixels: black + white → 127,127,127.
	buf := []byte{
		0, 0, 0,
		255, 255, 255,
	}
	got := averageRGB(buf, 2)
	if got.R != 127 || got.G != 127 || got.B != 127 {
		t.Errorf("expected RGB{127, 127, 127}, got %v", got)
	}
}

func TestAverageRGB_Empty(t *testing.T) {
	got := averageRGB(nil, 0)
	if got.R != 0 || got.G != 0 || got.B != 0 {
		t.Errorf("expected RGB{0, 0, 0}, got %v", got)
	}
}

func TestAverageRGB_FullFrame(t *testing.T) {
	// Simulate a full 64x36 frame of solid red.
	pixels := captureWidth * captureHeight
	buf := make([]byte, pixels*3)
	for i := 0; i < pixels; i++ {
		buf[i*3] = 255
		buf[i*3+1] = 0
		buf[i*3+2] = 0
	}
	got := averageRGB(buf, pixels)
	if got.R != 255 || got.G != 0 || got.B != 0 {
		t.Errorf("expected RGB{255, 0, 0}, got %v", got)
	}
}
