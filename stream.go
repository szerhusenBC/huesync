package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/pion/dtls/v2"
)

// ActivateArea tells the bridge to start entertainment mode for the given area.
func ActivateArea(ip net.IP, username, areaID string) error {
	url := bridgeURL(ip, "/clip/v2/resource/entertainment_configuration/"+areaID)
	body := strings.NewReader(`{"action":"start"}`)

	req, err := newHueRequest("PUT", url, body, username)
	if err != nil {
		return fmt.Errorf("creating activate request: %w", err)
	}

	resp, err := hueClient.Do(req)
	if err != nil {
		return fmt.Errorf("activating area: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("activate area: HTTP %d", resp.StatusCode)
	}
	return nil
}

// DeactivateArea tells the bridge to stop entertainment mode for the given area.
func DeactivateArea(ip net.IP, username, areaID string) error {
	url := bridgeURL(ip, "/clip/v2/resource/entertainment_configuration/"+areaID)
	body := strings.NewReader(`{"action":"stop"}`)

	req, err := newHueRequest("PUT", url, body, username)
	if err != nil {
		return fmt.Errorf("creating deactivate request: %w", err)
	}

	resp, err := hueClient.Do(req)
	if err != nil {
		return fmt.Errorf("deactivating area: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("deactivate area: HTTP %d", resp.StatusCode)
	}
	return nil
}

// Streamer sends color data to the Hue bridge over DTLS.
type Streamer struct {
	conn       net.Conn
	areaID     string
	channelIDs []uint8
	seq        uint8
}

// NewStreamer establishes a DTLS connection to the Hue bridge for entertainment streaming.
func NewStreamer(ip net.IP, username, clientkey, areaID string, channelIDs []uint8) (*Streamer, error) {
	psk, err := hex.DecodeString(clientkey)
	if err != nil {
		return nil, fmt.Errorf("decoding clientkey: %w", err)
	}

	addr := &net.UDPAddr{IP: ip, Port: 2100}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := dtls.DialWithContext(ctx, "udp", addr, &dtls.Config{
		PSK: func(hint []byte) ([]byte, error) {
			return psk, nil
		},
		PSKIdentityHint: []byte(username),
		CipherSuites:    []dtls.CipherSuiteID{dtls.TLS_PSK_WITH_AES_128_GCM_SHA256},
		InsecureSkipVerify: true,
	})
	if err != nil {
		return nil, fmt.Errorf("DTLS handshake: %w", err)
	}

	return &Streamer{
		conn:       conn,
		areaID:     areaID,
		channelIDs: channelIDs,
	}, nil
}

// SendColor sends the given color to all channels.
func (s *Streamer) SendColor(c RGB) error {
	msg := BuildHueStreamMessage(s.areaID, s.channelIDs, c, s.seq)
	s.seq++
	_, err := s.conn.Write(msg)
	if err != nil {
		return fmt.Errorf("writing to DTLS: %w", err)
	}
	return nil
}

// Close closes the DTLS connection.
func (s *Streamer) Close() error {
	return s.conn.Close()
}

// BuildHueStreamMessage constructs a HueStream v2 binary message.
func BuildHueStreamMessage(areaID string, channelIDs []uint8, c RGB, seq uint8) []byte {
	// Header: 52 bytes + 7 bytes per channel
	msg := make([]byte, 52+7*len(channelIDs))

	// "HueStream" magic (9 bytes)
	copy(msg[0:9], "HueStream")

	// Version
	msg[9] = 0x02  // major
	msg[10] = 0x00 // minor

	// Sequence number
	msg[11] = seq

	// Reserved
	msg[12] = 0x00
	msg[13] = 0x00

	// Color space: 0x00 = RGB
	msg[14] = 0x00

	// Reserved
	msg[15] = 0x00

	// Entertainment configuration ID (36 ASCII chars, UUID format)
	copy(msg[16:52], padOrTruncate(areaID, 36))

	// 8-bit to 16-bit color conversion
	r16 := uint16(c.R) * 257
	g16 := uint16(c.G) * 257
	b16 := uint16(c.B) * 257

	// Per-channel data (7 bytes each)
	offset := 52
	for _, ch := range channelIDs {
		msg[offset] = ch
		msg[offset+1] = byte(r16 >> 8)
		msg[offset+2] = byte(r16)
		msg[offset+3] = byte(g16 >> 8)
		msg[offset+4] = byte(g16)
		msg[offset+5] = byte(b16 >> 8)
		msg[offset+6] = byte(b16)
		offset += 7
	}

	return msg
}

func padOrTruncate(s string, n int) []byte {
	b := make([]byte, n)
	copy(b, s)
	return b
}

func newHueRequest(method, url string, body io.Reader, username string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("hue-application-key", username)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}
