// Package adbproto speaks the raw ADB host protocol directly to the local
// adb server (127.0.0.1:5037) instead of shelling out to `adb track-devices`
// and scraping its CLI output. This gives us exact control over the
// length-prefixed frame format the "host:track-devices" service uses, which
// is documented in AOSP's services.txt:
//
//	host:track-devices
//	  A variant of host:devices which doesn't close the connection.
//	  Instead, a new device list description is sent each time the
//	  device list changed.
//
// Wire format for every request: 4 ASCII hex chars giving the request's
// byte length, followed by the request text. The server replies with 4
// bytes ("OKAY" or "FAIL"); on track-devices, every subsequent update is
// again a 4-hex-char length prefix followed by that many bytes of a full
// device-list snapshot ("<serial>\t<state>\n" per line).
//
// The adb server must already be running (the caller is expected to have
// run `adb start-server` once at startup); this package only speaks the
// socket protocol, it never launches adb.exe itself.
package adbproto

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"screenmirror/internal/logging"
)

type EventKind int

const (
	Connected EventKind = iota
	Disconnected
)

// DeviceEvent reports a device transitioning into or out of the "device"
// (i.e. connected + authorized) state. State carries adb's raw state string
// at the time of the event ("device", "offline", "unauthorized", or "" if
// the device disappeared from the list entirely).
type DeviceEvent struct {
	Serial string
	State  string
	Kind   EventKind
}

type Tracker struct {
	addr   string
	logger *logging.Logger
}

// NewTracker builds a tracker connecting to the given adb server address
// (host:port). Pass "" to use the default 127.0.0.1:5037.
func NewTracker(addr string, logger *logging.Logger) *Tracker {
	if addr == "" {
		addr = "127.0.0.1:5037"
	}
	return &Tracker{addr: addr, logger: logger}
}

// Run connects to the adb server and streams device connect/disconnect
// events onto out until ctx is cancelled, automatically reconnecting with
// backoff if the adb server restarts or the connection drops.
func (t *Tracker) Run(ctx context.Context, out chan<- DeviceEvent) {
	known := map[string]string{}
	backoff := time.Second

	for {
		if ctx.Err() != nil {
			return
		}
		start := time.Now()
		err := t.runOnce(ctx, out, known)
		if ctx.Err() != nil {
			return
		}
		if time.Since(start) > 5*time.Second {
			backoff = time.Second
		}
		t.logger.Warn("adb device-tracking connection lost (%v); reconnecting in %s", err, backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}
		if backoff < 10*time.Second {
			backoff *= 2
		}
	}
}

func (t *Tracker) runOnce(ctx context.Context, out chan<- DeviceEvent, known map[string]string) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", t.addr)
	if err != nil {
		return fmt.Errorf("connecting to adb server at %s: %w", t.addr, err)
	}
	defer conn.Close()

	closeOnCancel := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-closeOnCancel:
		}
	}()
	defer close(closeOnCancel)

	const req = "host:track-devices"
	if _, err := conn.Write([]byte(fmt.Sprintf("%04x%s", len(req), req))); err != nil {
		return fmt.Errorf("sending track-devices request: %w", err)
	}

	status := make([]byte, 4)
	if _, err := io.ReadFull(conn, status); err != nil {
		return fmt.Errorf("reading adb status: %w", err)
	}
	switch string(status) {
	case "OKAY":
		// proceed
	case "FAIL":
		msg, _ := readFrame(conn)
		return fmt.Errorf("adb server rejected track-devices: %s", msg)
	default:
		return fmt.Errorf("unexpected adb response %q", status)
	}

	t.logger.Info("connected to adb server; watching for Android devices...")

	for {
		frame, err := readFrame(conn)
		if err != nil {
			return fmt.Errorf("reading device list: %w", err)
		}
		diffAndEmit(known, parseSnapshot(frame), out)
	}
}

func readFrame(conn net.Conn) (string, error) {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return "", err
	}
	n, err := strconv.ParseInt(string(lenBuf), 16, 32)
	if err != nil {
		return "", fmt.Errorf("bad frame length %q: %w", lenBuf, err)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func parseSnapshot(s string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		m[parts[0]] = parts[1]
	}
	return m
}

// diffAndEmit compares known (previous snapshot) against cur (latest
// snapshot), emits Connected/Disconnected events for anything that changed,
// and then updates known in place to match cur.
func diffAndEmit(known map[string]string, cur map[string]string, out chan<- DeviceEvent) {
	for serial, state := range cur {
		prevState, existed := known[serial]
		if state == "device" && (!existed || prevState != "device") {
			out <- DeviceEvent{Serial: serial, State: state, Kind: Connected}
		} else if existed && prevState == "device" && state != "device" {
			out <- DeviceEvent{Serial: serial, State: state, Kind: Disconnected}
		}
	}
	for serial, prevState := range known {
		if _, stillThere := cur[serial]; !stillThere && prevState == "device" {
			out <- DeviceEvent{Serial: serial, State: "", Kind: Disconnected}
		}
	}

	for k := range known {
		if _, ok := cur[k]; !ok {
			delete(known, k)
		}
	}
	for k, v := range cur {
		known[k] = v
	}
}
