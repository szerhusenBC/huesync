package main

import (
	"fmt"
	"os/exec"

	"github.com/kbinani/screenshot"
)

const (
	captureWidth  = 64
	captureHeight = 36
	frameSize     = captureWidth * captureHeight * 3 // RGB24
)

// Capturer captures the screen and returns the average color.
type Capturer interface {
	CaptureColor() (RGB, error)
	Close() error
}

// x11Capturer wraps the existing kbinani/screenshot-based capture.
type x11Capturer struct{}

func (x11Capturer) CaptureColor() (RGB, error) {
	img, err := CaptureScreen()
	if err != nil {
		return RGB{}, err
	}
	return AverageColor(img), nil
}

func (x11Capturer) Close() error { return nil }

// NewCapturer tries PipeWire → FFmpeg → X11 and returns the first that works.
func NewCapturer() (Capturer, string, error) {
	c, method, err := newPipeWireCapturer()
	if err == nil {
		return c, method, nil
	}

	c, method, err = newFFmpegCapturer()
	if err == nil {
		return c, method, nil
	}

	return x11Capturer{}, "X11", nil
}

// averageRGB computes the mean color of a raw RGB24 buffer.
func averageRGB(buf []byte, pixels int) RGB {
	if pixels == 0 {
		return RGB{}
	}
	var rSum, gSum, bSum uint64
	for i := 0; i < pixels; i++ {
		off := i * 3
		rSum += uint64(buf[off])
		gSum += uint64(buf[off+1])
		bSum += uint64(buf[off+2])
	}
	n := uint64(pixels)
	return RGB{
		R: uint8(rSum / n),
		G: uint8(gSum / n),
		B: uint8(bSum / n),
	}
}

// hasExecutable reports whether the named program is on PATH.
func hasExecutable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// screenSize returns the dimensions of display 0 using kbinani/screenshot.
func screenSize() (int, int, error) {
	n := screenshot.NumActiveDisplays()
	if n == 0 {
		return 0, 0, fmt.Errorf("no active displays")
	}
	b := screenshot.GetDisplayBounds(0)
	return b.Dx(), b.Dy(), nil
}
