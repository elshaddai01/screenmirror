// Package android turns raw adb device-tracking events into
// mirroring-ready events (with a human-readable model name resolved) and
// manages the scrcpy child process for each mirrored device.
package android

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"screenmirror/internal/adbproto"
	"screenmirror/internal/logging"
	"screenmirror/internal/procutil"
)

type Kind int

const (
	Connected Kind = iota
	Disconnected
)

type DeviceEvent struct {
	Serial string
	Model  string
	Kind   Kind
}

type Watcher struct {
	adbPath string
	logger  *logging.Logger
}

func NewWatcher(adbPath string, logger *logging.Logger) *Watcher {
	return &Watcher{adbPath: adbPath, logger: logger}
}

// Start begins tracking devices and returns a channel of higher-level
// events. The channel is closed once ctx is cancelled.
func (w *Watcher) Start(ctx context.Context) <-chan DeviceEvent {
	raw := make(chan adbproto.DeviceEvent, 16)
	out := make(chan DeviceEvent, 16)

	tracker := adbproto.NewTracker("", w.logger)
	go tracker.Run(ctx, raw)

	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-raw:
				if !ok {
					return
				}
				switch ev.Kind {
				case adbproto.Connected:
					model := w.lookupModel(ctx, ev.Serial)
					w.logger.Debug("device %s authorized (model=%s)", ev.Serial, model)
					out <- DeviceEvent{Serial: ev.Serial, Model: model, Kind: Connected}
				case adbproto.Disconnected:
					out <- DeviceEvent{Serial: ev.Serial, Kind: Disconnected}
				}
			}
		}
	}()

	return out
}

func (w *Watcher) lookupModel(ctx context.Context, serial string) string {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, w.adbPath, "-s", serial, "shell", "getprop", "ro.product.model")
	procutil.HideConsole(cmd)
	outb, err := cmd.Output()
	if err != nil {
		w.logger.Debug("could not read model name for %s: %v", serial, err)
		return serial
	}
	model := strings.TrimSpace(string(outb))
	if model == "" {
		return serial
	}
	return model
}
