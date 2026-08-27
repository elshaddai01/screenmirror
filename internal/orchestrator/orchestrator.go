// Package orchestrator wires the Android watcher and iOS AirPlay receiver
// pool together with a shared window-tiling manager, and drives them until
// the process is asked to shut down.
package orchestrator

import (
	"context"
	"sync"

	"screenmirror/internal/android"
	"screenmirror/internal/ios"
	"screenmirror/internal/logging"
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
