// Package ios runs UxPlay as a pool of AirPlay receiver processes and
// retitles/tiles the mirrored-video windows UxPlay opens on its own.
//
// Unlike scrcpy (which we invoke once per Android device, fully under our
// control), UxPlay is a long-running *receiver*: it advertises itself over
// Bonjour/mDNS and, whenever an iPhone/iPad picks it from Control Center,
// opens its own GStreamer video-sink window. UxPlay's CLI does not expose a
// window-title flag, and its exact stdout log format for client
// connect/disconnect varies by build/version, so device-name extraction
// from logs is best-effort only.
//
// To make connect/disconnect detection reliable regardless of log format,
// the *authoritative* signal we use is the appearance/disappearance of a
// new top-level window owned by the uxplay.exe process (via winapi), which
// only happens exactly when a client starts/stops actively mirroring. We
// use that moment to set the window title (to a device name scraped from
// the logs, best effort) and move it into a tile slot.
//
// To let multiple iOS devices mirror at the same time, we run a small pool
// of independent uxplay.exe instances, each advertising a distinctly
// numbered receiver name on its own port range, since a single AirPlay
// receiver process generally only holds one active client at a time.
package ios

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"screenmirror/internal/logging"
	"screenmirror/internal/procutil"
	"screenmirror/internal/tile"
	"screenmirror/internal/winapi"
)

type Kind int

const (
	Connected Kind = iota
	Disconnected
)

// ClientEvent is reported purely for logging visibility; window management
// is handled internally by Manager.
type ClientEvent struct {
	ReceiverName string
	DeviceName   string
	Kind         Kind
}

type Manager struct {
	path     string
	poolSize int
	baseName string
	tiles    *tile.Manager
	logger   *logging.Logger
}

// NewManager builds a manager that will run poolSize independent uxplay
// receiver instances, named baseName (or "baseName-1", "baseName-2", ...
// when poolSize > 1).
func NewManager(path string, poolSize int, baseName string, tiles *tile.Manager, logger *logging.Logger) *Manager {
	if poolSize < 1 {
		poolSize = 1
	}
	return &Manager{path: path, poolSize: poolSize, baseName: baseName, tiles: tiles, logger: logger}
}

// Run starts every receiver instance and blocks until ctx is cancelled,
// restarting any instance that exits unexpectedly.
func (m *Manager) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for i := 0; i < m.poolSize; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			m.runInstance(ctx, idx)
		}(i)
	}
	wg.Wait()
}

func (m *Manager) instanceName(idx int) string {
	if m.poolSize == 1 {
		return m.baseName
	}
	return fmt.Sprintf("%s-%d", m.baseName, idx+1)
}

func (m *Manager) runInstance(ctx context.Context, idx int) {
	name := m.instanceName(idx)
	basePort := 7000 + idx*20
	backoff := time.Second

	for {
		if ctx.Err() != nil {
			return
		}
		start := time.Now()
		err := m.runOnce(ctx, name, basePort)
		if ctx.Err() != nil {
			return
		}
		if time.Since(start) > 10*time.Second {
			backoff = time.Second
		}
		m.logger.Warn("AirPlay receiver %q exited (%v); restarting in %s", name, err, backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}
		if backoff < 15*time.Second {
			backoff *= 2
		}
	}
}

func (m *Manager) runOnce(ctx context.Context, name string, basePort int) error {
	args := []string{
		"-n", name,
		"-nohold",
		"-p", fmt.Sprintf("%d-%d", basePort, basePort+19),
	}
	cmd := exec.CommandContext(ctx, m.path, args...)
	procutil.HideConsole(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	var nameMu sync.Mutex
	lastSeenName := ""

	scan := func(r io.Reader) {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 64*1024)
		for sc.Scan() {
			line := sc.Text()
			m.logger.Debug("[airplay:%s] %s", name, line)
			if dn := extractDeviceName(line); dn != "" {
				nameMu.Lock()
				lastSeenName = dn
				nameMu.Unlock()
			}
		}
	}

	if err := cmd.Start(); err != nil {
		return err
	}
	pid := uint32(cmd.Process.Pid)
	m.logger.Info("AirPlay receiver %q ready — pick it from an iPhone/iPad's Control Center > Screen Mirroring", name)

	go scan(stdout)
	go scan(stderr)

	done := make(chan struct{})
	go m.watchWindows(ctx, name, pid, &nameMu, &lastSeenName, done)

	err = cmd.Wait()
	close(done)
	return err
}

// watchWindows polls for a new top-level window owned by pid; its
// appearance/disappearance is treated as the client connect/disconnect
// signal (see package doc for why).
func (m *Manager) watchWindows(ctx context.Context, name string, pid uint32, nameMu *sync.Mutex, lastSeenName *string, done <-chan struct{}) {
	before := winapi.WindowsForPID(pid)

	var active uintptr
	var activeSlot int
	hasActive := false

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	cleanup := func() {
		if hasActive {
			m.tiles.Release(activeSlot)
		}
	}

	for {
		select {
		case <-ctx.Done():
			cleanup()
			return
		case <-done:
			cleanup()
			return
		case <-ticker.C:
		}

		cur := winapi.WindowsForPID(pid)

		if !hasActive {
			for hwnd := range cur {
				if before[hwnd] {
					continue
				}
				nameMu.Lock()
				dn := *lastSeenName
				nameMu.Unlock()
				if dn == "" {
					dn = "iOS Device (" + name + ")"
				}

				slot, x, y, w, h := m.tiles.Acquire()
				winapi.SetTitle(hwnd, dn)
				winapi.Move(hwnd, x, y, w, h)

				active = hwnd
				activeSlot = slot
				hasActive = true
				m.logger.Info("mirroring started: %s [AirPlay receiver %q]", dn, name)
				break
			}
		} else if !cur[active] {
			m.logger.Info("AirPlay client disconnected from receiver %q", name)
			m.tiles.Release(activeSlot)
			hasActive = false
			nameMu.Lock()
			*lastSeenName = ""
			nameMu.Unlock()
			before = cur
		}
	}
}

// extractDeviceName does best-effort scraping of a device/client name out
// of a raw uxplay log line. Exact log wording varies by UxPlay
// build/version; if nothing matches we fall back to a generic label
// upstream. Run with -v to see raw uxplay output if this needs tuning for
// your build.
var deviceNamePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)client\s*name[:=]\s*"?([^"\n]+?)"?\s*$`),
	regexp.MustCompile(`(?i)device\s*name[:=]\s*"?([^"\n]+?)"?\s*$`),
	regexp.MustCompile(`(?i)\bfrom\s+([A-Za-z0-9][\w' .-]{1,32}(?:'s [\w -]+)?)\s*$`),
}

func extractDeviceName(line string) string {
	for _, re := range deviceNamePatterns {
		if match := re.FindStringSubmatch(line); len(match) > 1 {
			name := strings.TrimSpace(match[1])
			if name != "" {
				return name
			}
		}
	}
	return ""
}
