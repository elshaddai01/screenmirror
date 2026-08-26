// Command screenmirror is a zero-configuration Windows mirroring tool: the
// instant it starts, it watches for Android devices via ADB and for iOS
// devices via an AirPlay receiver, and opens a titled, tiled window for
// each device the moment it starts mirroring.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"

	"screenmirror/internal/deps"
	"screenmirror/internal/logging"
	"screenmirror/internal/orchestrator"
	"screenmirror/internal/procutil"
)

func main() {
	verbose := flag.Bool("v", false, "verbose/debug logging")
	iosPool := flag.Int("ios-pool", 2, "number of simultaneous AirPlay (iOS) receiver slots")
	receiverName := flag.String("name", "", "AirPlay receiver name advertised on the network (default: ScreenMirror-<hostname>)")
	flag.Parse()

	logger := logging.New(*verbose)
	logger.Info("ScreenMirror starting...")

	adbPath, scrcpyPath, uxplayPath, setupMsg := deps.Check()
	if setupMsg != "" {
		fmt.Println(setupMsg)
	}
	if adbPath == "" || scrcpyPath == "" {
		logger.Warn("Android mirroring cannot start without both adb.exe and scrcpy.exe on PATH")
	}
	if adbPath != "" {
		ensureAdbServer(adbPath, logger)
	}

	name := *receiverName
	if name == "" {
		if h, err := os.Hostname(); err == nil {
			name = "ScreenMirror-" + h
		} else {
			name = "ScreenMirror-PC"
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	orchestrator.Run(ctx, orchestrator.Options{
		AdbPath:          adbPath,
		ScrcpyPath:       scrcpyPath,
		UxplayPath:       uxplayPath,
		IOSPoolSize:      *iosPool,
		ReceiverBaseName: name,
		Logger:           logger,
	})

	logger.Info("ScreenMirror stopped.")
}

func ensureAdbServer(adbPath string, logger *logging.Logger) {
	cmd := exec.Command(adbPath, "start-server")
	procutil.HideConsole(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		logger.Warn("adb start-server failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
}
