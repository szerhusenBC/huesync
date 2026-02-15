package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	portalDest      = "org.freedesktop.portal.Desktop"
	portalPath      = "/org/freedesktop/portal/desktop"
	screenCastIface = "org.freedesktop.portal.ScreenCast"
	requestIface    = "org.freedesktop.portal.Request"

	portalTimeout = 120 * time.Second // user may need time to pick a screen
)

type pipeWireCapturer struct {
	cancel context.CancelFunc
	cmd    *exec.Cmd
	dbConn *dbus.Conn // kept alive to hold the ScreenCast session
	pwFile *os.File   // PipeWire remote fd from the portal
	done   chan struct{}
	ready  chan struct{} // closed when first frame is available

	mu    sync.Mutex
	frame []byte
}

func newPipeWireCapturer() (Capturer, string, error) {
	if !hasExecutable("gst-launch-1.0") {
		return nil, "", fmt.Errorf("gst-launch-1.0 not found")
	}

	dbConn, nodeID, pwFile, err := acquirePipeWireNode()
	if err != nil {
		return nil, "", fmt.Errorf("pipewire portal: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// GStreamer child process inherits pwFile via ExtraFiles.
	// ExtraFiles[0] becomes fd 3 in the child.
	cmd := exec.CommandContext(ctx, "gst-launch-1.0", "-q",
		"pipewiresrc", fmt.Sprintf("path=%d", nodeID), "fd=3",
		"!", "videoconvert",
		"!", "videoscale",
		"!", fmt.Sprintf("video/x-raw,format=RGB,width=%d,height=%d", captureWidth, captureHeight),
		"!", "fdsink", "fd=1",
	)
	cmd.ExtraFiles = []*os.File{pwFile}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		pwFile.Close()
		dbConn.Close()
		return nil, "", fmt.Errorf("gstreamer stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		pwFile.Close()
		dbConn.Close()
		return nil, "", fmt.Errorf("starting gstreamer: %w", err)
	}

	c := &pipeWireCapturer{
		cancel: cancel,
		cmd:    cmd,
		dbConn: dbConn,
		pwFile: pwFile,
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
		pwFile.Close()
		dbConn.Close()
		return nil, "", fmt.Errorf("gstreamer: timed out waiting for first frame")
	}

	return c, "PipeWire", nil
}

func (c *pipeWireCapturer) readFrames(r io.Reader) {
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

func (c *pipeWireCapturer) CaptureColor() (RGB, error) {
	c.mu.Lock()
	f := c.frame
	c.mu.Unlock()
	if f == nil {
		return RGB{}, fmt.Errorf("no frame captured yet")
	}
	return averageRGB(f, captureWidth*captureHeight), nil
}

func (c *pipeWireCapturer) Close() error {
	c.cancel()
	<-c.done
	err := c.cmd.Wait()
	c.pwFile.Close()
	c.dbConn.Close()
	return err
}

// acquirePipeWireNode negotiates a ScreenCast session via the XDG Desktop Portal
// and returns the D-Bus connection (must stay open), the PipeWire node ID,
// and a PipeWire remote file descriptor for GStreamer.
func acquirePipeWireNode() (*dbus.Conn, uint32, *os.File, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, 0, nil, fmt.Errorf("connecting to session bus: %w", err)
	}
	if !conn.SupportsUnixFDs() {
		conn.Close()
		return nil, 0, nil, fmt.Errorf("D-Bus connection does not support Unix FD passing")
	}

	portal := conn.Object(portalDest, dbus.ObjectPath(portalPath))
	sender := senderToToken(conn.Names()[0])

	// --- CreateSession ---
	sessionToken := "huesync_session"
	reqToken := "huesync_req_create"
	reqPath := dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/portal/desktop/request/%s/%s", sender, reqToken))

	sigCh := subscribeSignal(conn, reqPath)
	defer conn.RemoveSignal(sigCh)

	call := portal.Call(screenCastIface+".CreateSession", 0, map[string]dbus.Variant{
		"handle_token":         dbus.MakeVariant(reqToken),
		"session_handle_token": dbus.MakeVariant(sessionToken),
	})
	if call.Err != nil {
		conn.Close()
		return nil, 0, nil, fmt.Errorf("CreateSession: %w", call.Err)
	}

	resp, err := waitForResponse(sigCh, portalTimeout)
	if err != nil {
		conn.Close()
		return nil, 0, nil, fmt.Errorf("CreateSession response: %w", err)
	}

	sessionHandle, ok := resp["session_handle"]
	if !ok {
		conn.Close()
		return nil, 0, nil, fmt.Errorf("CreateSession: no session_handle in response")
	}
	sessionPath := dbus.ObjectPath(sessionHandle.Value().(string))

	// --- SelectSources ---
	reqToken = "huesync_req_select"
	reqPath = dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/portal/desktop/request/%s/%s", sender, reqToken))

	sigCh2 := subscribeSignal(conn, reqPath)
	defer conn.RemoveSignal(sigCh2)

	call = portal.Call(screenCastIface+".SelectSources", 0, sessionPath, map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(reqToken),
		"types":        dbus.MakeVariant(uint32(1)), // 1 = monitor
		"multiple":     dbus.MakeVariant(false),
	})
	if call.Err != nil {
		conn.Close()
		return nil, 0, nil, fmt.Errorf("SelectSources: %w", call.Err)
	}

	if _, err := waitForResponse(sigCh2, portalTimeout); err != nil {
		conn.Close()
		return nil, 0, nil, fmt.Errorf("SelectSources response: %w", err)
	}

	// --- Start ---
	reqToken = "huesync_req_start"
	reqPath = dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/portal/desktop/request/%s/%s", sender, reqToken))

	sigCh3 := subscribeSignal(conn, reqPath)
	defer conn.RemoveSignal(sigCh3)

	call = portal.Call(screenCastIface+".Start", 0, sessionPath, "", map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(reqToken),
	})
	if call.Err != nil {
		conn.Close()
		return nil, 0, nil, fmt.Errorf("Start: %w", call.Err)
	}

	startResp, err := waitForResponse(sigCh3, portalTimeout)
	if err != nil {
		conn.Close()
		return nil, 0, nil, fmt.Errorf("Start response: %w", err)
	}

	nodeID, err := extractNodeID(startResp)
	if err != nil {
		conn.Close()
		return nil, 0, nil, err
	}

	// --- OpenPipeWireRemote ---
	// Returns a Unix fd that grants access to the PipeWire stream.
	// pipewiresrc needs this fd to connect to the portal's capture.
	var pwFd dbus.UnixFD
	err = portal.Call(screenCastIface+".OpenPipeWireRemote", 0, sessionPath, map[string]dbus.Variant{}).Store(&pwFd)
	if err != nil {
		conn.Close()
		return nil, 0, nil, fmt.Errorf("OpenPipeWireRemote: %w", err)
	}

	pwFile := os.NewFile(uintptr(pwFd), "pipewire-remote")
	if pwFile == nil {
		conn.Close()
		return nil, 0, nil, fmt.Errorf("invalid PipeWire fd")
	}

	return conn, nodeID, pwFile, nil
}

// subscribeSignal registers a D-Bus signal match for the portal Response signal
// at the given path and returns a channel that receives matching signals.
func subscribeSignal(conn *dbus.Conn, path dbus.ObjectPath) chan *dbus.Signal {
	ch := make(chan *dbus.Signal, 1)
	conn.Signal(ch)
	conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0,
		fmt.Sprintf("type='signal',interface='%s',member='Response',path='%s'", requestIface, path))
	return ch
}

// waitForResponse waits for a portal Response signal and returns the results map.
// A non-zero response code indicates the user denied or the request failed.
func waitForResponse(ch chan *dbus.Signal, timeout time.Duration) (map[string]dbus.Variant, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case sig := <-ch:
			if sig == nil {
				return nil, fmt.Errorf("signal channel closed")
			}
			if len(sig.Body) < 2 {
				continue
			}
			code, ok := sig.Body[0].(uint32)
			if !ok {
				continue
			}
			if code != 0 {
				return nil, fmt.Errorf("portal request denied (code %d)", code)
			}
			results, ok := sig.Body[1].(map[string]dbus.Variant)
			if !ok {
				return nil, fmt.Errorf("unexpected response type")
			}
			return results, nil
		case <-ctx.Done():
			return nil, fmt.Errorf("timed out waiting for portal response")
		}
	}
}

// senderToToken converts a D-Bus sender name like ":1.42" to "1_42" for use
// in request object paths.
func senderToToken(sender string) string {
	s := strings.TrimPrefix(sender, ":")
	return strings.ReplaceAll(s, ".", "_")
}

// extractNodeID pulls the PipeWire node ID from the Start response.
// The streams field is typed as a(ua{sv}) — an array of (uint32, dict) structs.
func extractNodeID(resp map[string]dbus.Variant) (uint32, error) {
	streamsVariant, ok := resp["streams"]
	if !ok {
		return 0, fmt.Errorf("no streams in Start response")
	}

	// The variant wraps [][]interface{} where each inner slice is [uint32, map[string]dbus.Variant].
	streams, ok := streamsVariant.Value().([][]interface{})
	if !ok {
		// Some D-Bus libs may present this as []interface{}.
		rawSlice, ok2 := streamsVariant.Value().([]interface{})
		if !ok2 || len(rawSlice) == 0 {
			return 0, fmt.Errorf("unexpected streams type: %T", streamsVariant.Value())
		}
		inner, ok2 := rawSlice[0].([]interface{})
		if !ok2 || len(inner) == 0 {
			return 0, fmt.Errorf("unexpected stream entry type: %T", rawSlice[0])
		}
		nodeID, ok2 := inner[0].(uint32)
		if !ok2 {
			return 0, fmt.Errorf("unexpected node ID type: %T", inner[0])
		}
		return nodeID, nil
	}

	if len(streams) == 0 {
		return 0, fmt.Errorf("no streams returned")
	}

	entry := streams[0]
	if len(entry) == 0 {
		return 0, fmt.Errorf("empty stream entry")
	}

	nodeID, ok := entry[0].(uint32)
	if !ok {
		return 0, fmt.Errorf("unexpected node ID type: %T", entry[0])
	}
	return nodeID, nil
}
