package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
)

var hueClient = &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

// ErrLinkButtonNotPressed is returned by PairBridge when the user has not yet
// pressed the link button on the Hue bridge.
var ErrLinkButtonNotPressed = errors.New("link button not pressed")

// ErrUnauthorized is returned when the bridge rejects the API credentials.
var ErrUnauthorized = errors.New("unauthorized")

// PairBridge registers a new application with the Hue bridge at the given IP.
// The user must press the link button on the bridge before calling this.
func PairBridge(ip net.IP) (username, clientkey string, err error) {
	url := bridgeURL(ip, "/api")

	body := strings.NewReader(`{"devicetype":"huesync#device","generateclientkey":true}`)
	resp, err := hueClient.Post(url, "application/json", body)
	if err != nil {
		return "", "", fmt.Errorf("pairing request: %w", err)
	}
	defer resp.Body.Close()

	var result []pairResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("decoding pair response: %w", err)
	}

	if len(result) == 0 {
		return "", "", fmt.Errorf("empty pair response")
	}

	r := result[0]
	if r.Error != nil {
		if r.Error.Type == 101 {
			return "", "", ErrLinkButtonNotPressed
		}
		return "", "", fmt.Errorf("bridge error %d: %s", r.Error.Type, r.Error.Description)
	}

	if r.Success == nil {
		return "", "", fmt.Errorf("unexpected pair response: no success or error")
	}

	return r.Success.Username, r.Success.Clientkey, nil
}

// EntertainmentArea represents a Hue entertainment configuration.
type EntertainmentArea struct {
	ID         string
	Name       string
	Type       string
	Status     string
	ChannelIDs []uint8
	Lights     int
}

func (a EntertainmentArea) String() string {
	return fmt.Sprintf("%s (%d channels, %d lights)", a.Name, len(a.ChannelIDs), a.Lights)
}

// FetchEntertainmentAreas retrieves entertainment configurations from the bridge.
func FetchEntertainmentAreas(ip net.IP, username string) ([]EntertainmentArea, error) {
	url := bridgeURL(ip, "/clip/v2/resource/entertainment_configuration")

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("hue-application-key", username)

	resp, err := hueClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching entertainment areas: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 {
		return nil, ErrUnauthorized
	}

	var result entertainmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding entertainment response: %w", err)
	}

	areas := make([]EntertainmentArea, len(result.Data))
	for i, d := range result.Data {
		channelIDs := make([]uint8, len(d.Channels))
		for j, raw := range d.Channels {
			var ch channelData
			if err := json.Unmarshal(raw, &ch); err != nil {
				return nil, fmt.Errorf("decoding channel %d: %w", j, err)
			}
			channelIDs[j] = ch.ChannelID
		}
		areas[i] = EntertainmentArea{
			ID:         d.ID,
			Name:       d.Metadata.Name,
			Type:       d.ConfigurationType,
			Status:     d.Status,
			ChannelIDs: channelIDs,
			Lights:     len(d.LightServices),
		}
	}

	return areas, nil
}

func bridgeURL(ip net.IP, path string) string {
	host := ip.String()
	if ip.To4() == nil {
		host = "[" + host + "]"
	}
	return "https://" + host + path
}

// JSON mapping structs

type pairResponse struct {
	Success *pairSuccess `json:"success"`
	Error   *pairError   `json:"error"`
}

type pairSuccess struct {
	Username  string `json:"username"`
	Clientkey string `json:"clientkey"`
}

type pairError struct {
	Type        int    `json:"type"`
	Description string `json:"description"`
}

type entertainmentResponse struct {
	Data []entertainmentData `json:"data"`
}

type entertainmentData struct {
	ID                string              `json:"id"`
	Metadata          entertainmentMeta   `json:"metadata"`
	ConfigurationType string              `json:"configuration_type"`
	Status            string              `json:"status"`
	Channels          []json.RawMessage   `json:"channels"`
	LightServices     []json.RawMessage   `json:"light_services"`
}

type entertainmentMeta struct {
	Name string `json:"name"`
}

type channelData struct {
	ChannelID uint8 `json:"channel_id"`
}
