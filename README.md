# huesync

Sync your screen colors to Philips Hue lights in real time.

huesync continuously captures your screen, calculates the average color, and streams it to lights in a Philips Hue entertainment area over DTLS — turning your room into an ambient display.

## Features

- Automatic Hue bridge discovery via mDNS
- Interactive TUI for bridge selection, pairing, and area selection
- Configurable capture delay
- Credentials persisted across sessions (`~/.huesync/credentials.json`)
- Streams via the Hue Entertainment API (DTLS/PSK)

## Requirements

- Go 1.25+
- A Philips Hue bridge with at least one entertainment area configured
- Linux (X11) — uses X11 screen capture

## Install

```sh
go install github.com/szerhusenBC/huesync@latest
```

Or build from source:

```sh
git clone https://github.com/szerhusenBC/huesync.git
cd huesync
go build .
```

## Usage

```sh
./huesync
```

The TUI will guide you through:

1. **Bridge discovery** — scans your network for Hue bridges
2. **Pairing** — press the link button on your bridge, then press Enter
3. **Area selection** — pick an entertainment area (auto-selected if only one exists)
4. **Capture delay** — set the screen capture interval in milliseconds (default: 100)
5. **Streaming** — screen colors are sent to your lights in real time

Press `q` to stop streaming and exit.

## How it works

1. The primary display is captured at the configured interval
2. The average RGB color is computed by sampling ~10,000 pixels
3. The color is sent to all channels in the entertainment area via the HueStream v2 protocol over DTLS

## License

[MIT](LICENSE)
