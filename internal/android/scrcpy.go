package android

import (
	"fmt"
	"os/exec"
	"sync"

	"screenmirror/internal/logging"
	"screenmirror/internal/procutil"
	"screenmirror/internal/tile"
)

// ScrcpyManager spawns and tracks one scrcpy child process per mirrored
// Android device, giving each its own titled, positioned window and
// cleaning up automatically when the device disconnects or the window is
// closed by the user.
type ScrcpyManager struct {
	path   string
	tiles  *tile.Manager
	logger *logging.Logger

	mu      sync.Mutex
	running map[string]*runningDevice
}

type runningDevice struct {
	cmd      *exec.Cmd
	slot     int
	stopping bool
}

func NewScrcpyManager(path string, tiles *tile.Manager, logger *logging.Logger) *ScrcpyManager {
	return &ScrcpyManager{
		path:    path,
		tiles:   tiles,
		logger:  logger,
		running: map[string]*runningDevice{},
	}
}

// Start launches scrcpy for the given serial if it isn't already running.
func (m *ScrcpyManager) Start(serial, model string) {
	m.mu.Lock()
	if _, exists := m.running[serial]; exists {
		m.mu.Unlock()
		return
	}

	title := model
	if title == "" {
		title = serial
	}

	slot, x, y, w, h := m.tiles.Acquire()
	args := []string{
		"-s", serial,
		"--window-title=" + title,
		fmt.Sprintf("--window-x=%d", x),
		fmt.Sprintf("--window-y=%d", y),
		fmt.Sprintf("--window-width=%d", w),
		fmt.Sprintf("--window-height=%d", h),
	}
	cmd := exec.Command(m.path, args...)
	procutil.HideConsole(cmd)

	rd := &runningDevice{cmd: cmd, slot: slot}
	if err := cmd.Start(); err != nil {
		m.logger.Error("failed to start scrcpy for %s (%s): %v", title, serial, err)
		m.tiles.Release(slot)
		m.mu.Unlock()
		return
	}
	m.running[serial] = rd
	m.mu.Unlock()

	m.logger.Info("mirroring started: %s [%s]", title, serial)

	go func() {
		err := cmd.Wait()
		m.mu.Lock()
		delete(m.running, serial)
		m.tiles.Release(rd.slot)
		wasStopping := rd.stopping
		m.mu.Unlock()

		if !wasStopping {
			if err != nil {
				m.logger.Info("mirror window closed: %s [%s] (%v)", title, serial, err)
			} else {
				m.logger.Info("mirror window closed: %s [%s]", title, serial)
			}
		}
	}()
}

// Stop terminates the scrcpy process for the given serial, if running.
func (m *ScrcpyManager) Stop(serial string) {
	m.mu.Lock()
	rd, exists := m.running[serial]
	if !exists {
		m.mu.Unlock()
		return
	}
	rd.stopping = true
	proc := rd.cmd.Process
	m.mu.Unlock()

	if proc != nil {
		_ = proc.Kill()
	}
	m.logger.Info("device disconnected, closing mirror window [%s]", serial)
}

// StopAll terminates every running scrcpy process (used on shutdown).
func (m *ScrcpyManager) StopAll() {
	m.mu.Lock()
	serials := make([]string, 0, len(m.running))
	for s := range m.running {
		serials = append(serials, s)
	}
	m.mu.Unlock()

	for _, s := range serials {
		m.Stop(s)
	}
}
