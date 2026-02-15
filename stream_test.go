package main

import (
	"testing"
)

func TestBuildHueStreamMessage_Header(t *testing.T) {
	areaID := "abcdefgh-1234-5678-9abc-def012345678"
	channels := []uint8{0, 1}
	color := RGB{R: 255, G: 128, B: 0}

	msg := BuildHueStreamMessage(areaID, channels, color, 42)

	// Total length: 52 header + 7*2 channels = 66
	if len(msg) != 66 {
		t.Fatalf("expected length 66, got %d", len(msg))
	}

	// Magic
	if string(msg[0:9]) != "HueStream" {
		t.Errorf("expected magic 'HueStream', got %q", string(msg[0:9]))
	}

	// Version
	if msg[9] != 0x02 {
		t.Errorf("expected major version 0x02, got 0x%02x", msg[9])
	}
	if msg[10] != 0x00 {
		t.Errorf("expected minor version 0x00, got 0x%02x", msg[10])
	}

	// Sequence
	if msg[11] != 42 {
		t.Errorf("expected sequence 42, got %d", msg[11])
	}

	// Color space
	if msg[14] != 0x00 {
		t.Errorf("expected color space 0x00, got 0x%02x", msg[14])
	}

	// Area ID
	if string(msg[16:52]) != areaID {
		t.Errorf("expected area ID %q, got %q", areaID, string(msg[16:52]))
	}
}

func TestBuildHueStreamMessage_ChannelData(t *testing.T) {
	areaID := "abcdefgh-1234-5678-9abc-def012345678"
	channels := []uint8{0, 3}
	color := RGB{R: 255, G: 0, B: 128}

	msg := BuildHueStreamMessage(areaID, channels, color, 0)

	// Channel 0 starts at offset 52
	if msg[52] != 0 {
		t.Errorf("expected channel ID 0, got %d", msg[52])
	}

	// R=255 → 255*257 = 65535 = 0xFFFF
	r16 := uint16(msg[53])<<8 | uint16(msg[54])
	if r16 != 65535 {
		t.Errorf("expected R16=65535, got %d", r16)
	}

	// G=0 → 0
	g16 := uint16(msg[55])<<8 | uint16(msg[56])
	if g16 != 0 {
		t.Errorf("expected G16=0, got %d", g16)
	}

	// B=128 → 128*257 = 32896 = 0x8080
	b16 := uint16(msg[57])<<8 | uint16(msg[58])
	if b16 != 32896 {
		t.Errorf("expected B16=32896, got %d", b16)
	}

	// Channel 3 starts at offset 59
	if msg[59] != 3 {
		t.Errorf("expected channel ID 3, got %d", msg[59])
	}
}

func TestBuildHueStreamMessage_SingleChannel(t *testing.T) {
	areaID := "12345678-1234-1234-1234-123456789012"
	channels := []uint8{5}
	color := RGB{R: 0, G: 0, B: 0}

	msg := BuildHueStreamMessage(areaID, channels, color, 255)

	// Total length: 52 + 7 = 59
	if len(msg) != 59 {
		t.Fatalf("expected length 59, got %d", len(msg))
	}

	// Sequence wrapping
	if msg[11] != 255 {
		t.Errorf("expected sequence 255, got %d", msg[11])
	}

	// Channel ID
	if msg[52] != 5 {
		t.Errorf("expected channel ID 5, got %d", msg[52])
	}

	// All colors zero
	for i := 53; i < 59; i++ {
		if msg[i] != 0 {
			t.Errorf("expected byte %d to be 0, got %d", i, msg[i])
		}
	}
}
