package main

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/grandcat/zeroconf"
)

// Bridge represents a discovered Philips Hue Bridge on the network.
type Bridge struct {
	ID       string
	Model    string
	Name     string
	IP       net.IP
	Port     int
	Hostname string
}

func (b Bridge) String() string {
	return fmt.Sprintf("%s (%s) at %s:%d", b.Name, b.ID, b.IP, b.Port)
}

// DiscoverBridges browses the network for Hue bridges via mDNS and returns
// discovered bridges on the bridges channel. The error channel receives any
// discovery errors. Both channels are closed when discovery finishes or the
// context is cancelled.
func DiscoverBridges(ctx context.Context) (<-chan Bridge, <-chan error) {
	bridges := make(chan Bridge)
	errs := make(chan error, 1)

	go func() {
		defer close(bridges)
		defer close(errs)

		resolver, err := zeroconf.NewResolver(nil)
		if err != nil {
			errs <- fmt.Errorf("creating mDNS resolver: %w", err)
			return
		}

		entries := make(chan *zeroconf.ServiceEntry)
		seen := make(map[string]bool)

		go func() {
			for entry := range entries {
				b := parseBridge(entry)
				if b.ID != "" && seen[b.ID] {
					continue
				}
				if b.ID != "" {
					seen[b.ID] = true
				}
				bridges <- b
			}
		}()

		err = resolver.Browse(ctx, "_hue._tcp", "local.", entries)
		if err != nil {
			errs <- fmt.Errorf("browsing for Hue bridges: %w", err)
		}

		<-ctx.Done()
	}()

	return bridges, errs
}

func parseBridge(entry *zeroconf.ServiceEntry) Bridge {
	b := Bridge{
		Name:     entry.Instance,
		Port:     entry.Port,
		Hostname: entry.HostName,
	}

	if len(entry.AddrIPv4) > 0 {
		b.IP = entry.AddrIPv4[0]
	} else if len(entry.AddrIPv6) > 0 {
		b.IP = entry.AddrIPv6[0]
	}

	for _, txt := range entry.Text {
		key, value, ok := strings.Cut(txt, "=")
		if !ok {
			continue
		}
		switch key {
		case "bridgeid":
			b.ID = value
		case "modelid":
			b.Model = value
		}
	}

	return b
}
