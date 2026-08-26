// Package orchestrator wires the Android watcher and iOS AirPlay receiver
// pool together with a shared window-tiling manager, and drives them until
// the process is asked to shut down.
package orchestrator

import (
	"context"
	"fmt"
	"os"
	"sync"

	"screenmirror/internal/android"
	"screenmirror/internal/ios"
	"screenmirror/internal/logging"
	"screenmirror/internal/pairing"
	"screenmirror/internal/tile"
	"screenmirror/internal/winapi"
)

type Options struct {
	AdbPath, ScrcpyPath, UxplayPath string
	IOSPoolSize                     int
	ReceiverBaseName                string
	Logger                          *logging.Logger
}

// Run blocks until ctx is cancelled, at which point it waits for all child
// processes and windows to be cleaned up before returning.
func Run(ctx context.Context, opt Options) {
	screenW, screenH := winapi.ScreenSize()
	tiles := tile.New(screenW, screenH)

	var wg sync.WaitGroup

	if opt.AdbPath != "" && opt.ScrcpyPath != "" {
		watcher := android.NewWatcher(opt.AdbPath, opt.Logger)
		scrcpyMgr := android.NewScrcpyManager(opt.ScrcpyPath, tiles, opt.Logger)
		events := watcher.Start(ctx)

		wg.Add(1)
		go func() {
			defer wg.Done()
			for ev := range events {
				switch ev.Kind {
				case android.Connected:
					scrcpyMgr.Start(ev.Serial, ev.Model)
				case android.Disconnected:
					scrcpyMgr.Stop(ev.Serial)
				}
			}
			scrcpyMgr.StopAll()
		}()

		opt.Logger.Info("Android watcher active — plug in or connect a device to start mirroring automatically")
	} else {
		opt.Logger.Warn("Android mirroring disabled: adb.exe or scrcpy.exe not found")
	}

	if opt.AdbPath != "" {
		startWirelessPairing(ctx, &wg, opt)
	}

	if opt.UxplayPath != "" {
		iosMgr := ios.NewManager(opt.UxplayPath, opt.IOSPoolSize, opt.ReceiverBaseName, tiles, opt.Logger)
		wg.Add(1)
		go func() {
			defer wg.Done()
			iosMgr.Run(ctx)
		}()
	} else {
		opt.Logger.Warn("iOS AirPlay mirroring disabled: uxplay.exe not found")
	}

	<-ctx.Done()
	opt.Logger.Info("shutting down...")
	wg.Wait()
}

// startWirelessPairing brings up the QR-code AirPlay-style pairing target
// (Android's "Pair device with QR code"), printing the QR to the console.
// Once a phone pairs, adb's own auto-mDNS-discovery connects to it with no
// further action needed here -- the already-running Android watcher above
// picks it up like any other device.
func startWirelessPairing(ctx context.Context, wg *sync.WaitGroup, opt Options) {
	pubKeyBytes, err := os.ReadFile(pairing.AdbKeyPath())
	if err != nil {
		opt.Logger.Warn("wireless QR pairing disabled: could not read %s: %v", pairing.AdbKeyPath(), err)
		return
	}

	server, err := pairing.NewServer(opt.ReceiverBaseName, string(pubKeyBytes), opt.Logger)
	if err != nil {
		opt.Logger.Warn("wireless QR pairing disabled: %v", err)
		return
	}

	err = server.Start(ctx, func(res pairing.Result) {
		opt.Logger.Info("phone paired successfully over Wi-Fi — it will connect automatically within a few seconds")
	})
	if err != nil {
		opt.Logger.Warn("wireless QR pairing disabled: %v", err)
		return
	}

	qrText := server.QRText()
	if art, err := pairing.RenderQRTerminal(qrText); err == nil {
		fmt.Println("\nScan this from an Android phone: Settings > Developer options > Wireless debugging > Pair device with QR code\n")
		fmt.Println(art)
	} else {
		opt.Logger.Debug("could not render QR code: %v", err)
	}
	opt.Logger.Info("wireless pairing ready: %s", qrText)

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		server.Stop()
	}()
}
