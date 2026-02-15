package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

type ffmpegCapturer struct {
	cancel context.CancelFunc
	cmd    *exec.Cmd
	done   chan struct{}
	ready  chan struct{} // closed when first frame is available

	mu    sync.Mutex
	frame []byte
}

func newFFmpegCapturer() (Capturer, string, error) {
	if !hasExecutable("ffmpeg") {
		return nil, "", fmt.Errorf("ffmpeg not found")
	}

	display := os.Getenv("DISPLAY")
	if display == "" {
		return nil, "", fmt.Errorf("DISPLAY not set")
	}

	w, h, err := screenSize()
	if err != nil {
		return nil, "", err
	}

	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-nostdin",
		"-loglevel", "error",
		"-f", "x11grab",
		"-framerate", "30",
		"-video_size", fmt.Sprintf("%dx%d", w, h),
		"-i", display+".0",
		"-vf", fmt.Sprintf("scale=%d:%d", captureWidth, captureHeight),
		"-f", "rawvideo",
		"-pix_fmt", "rgb24",
		"pipe:1",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, "", fmt.Errorf("ffmpeg stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, "", fmt.Errorf("starting ffmpeg: %w", err)
	}

	c := &ffmpegCapturer{
		cancel: cancel,
		cmd:    cmd,
		done:   make(chan struct{}),
		ready:  make(chan struct{}),
	}

	go c.readFrames(stdout)

	// Wait for the first frame so CaptureColor is immediately usable.
	select {
	case <-c.ready:
	case <-time.After(5 * time.Second):
		c.cancel()
		<-c.done
		_ = c.cmd.Wait()
		return nil, "", fmt.Errorf("ffmpeg: timed out waiting for first frame")
	}

	return c, "FFmpeg", nil
}

func (c *ffmpegCapturer) readFrames(r io.Reader) {
	defer close(c.done)
	buf := make([]byte, frameSize)
	first := true
	for {
		_, err := io.ReadFull(r, buf)
		if err != nil {
			return
		}
		c.mu.Lock()
		if c.frame == nil {
			c.frame = make([]byte, frameSize)
		}
		copy(c.frame, buf)
		c.mu.Unlock()
		if first {
			close(c.ready)
			first = false
		}
	}
}

func (c *ffmpegCapturer) CaptureColor() (RGB, error) {
	c.mu.Lock()
	f := c.frame
	c.mu.Unlock()
	if f == nil {
		return RGB{}, fmt.Errorf("no frame captured yet")
	}
	return averageRGB(f, captureWidth*captureHeight), nil
}

func (c *ffmpegCapturer) Close() error {
	c.cancel()
	<-c.done
	return c.cmd.Wait()
}
