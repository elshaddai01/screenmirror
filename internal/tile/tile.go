// Package tile assigns new mirror windows a slot in a simple grid over the
// primary monitor so windows spawned in quick succession don't all stack
// exactly on top of each other.
package tile

import "sync"

type Manager struct {
	mu               sync.Mutex
	cols, rows       int
	cellW, cellH     int
	marginX, marginY int
	used             map[int]bool
	next             int
}

// New builds a tiling grid sized to the given screen resolution.
func New(screenW, screenH int) *Manager {
	cols, rows := 3, 2
	if screenW < 1600 {
		cols = 2
	}
	if screenH < 900 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	return &Manager{
		cols:    cols,
		rows:    rows,
		cellW:   screenW / cols,
		cellH:   screenH / rows,
		marginX: 16,
		marginY: 16,
		used:    map[int]bool{},
	}
}

// Acquire reserves the next free grid cell and returns its slot id plus a
// window rect (x, y, w, h) to place a new mirror window at. Once every cell
// in the grid is occupied, slots are reused round-robin (new windows land on
// top of the oldest slot rather than growing off-screen).
func (m *Manager) Acquire() (slot, x, y, w, h int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	total := m.cols * m.rows
	slot = -1
	for i := 0; i < total; i++ {
		if !m.used[i] {
			slot = i
			break
		}
	}
	if slot == -1 {
		slot = m.next % total
	}
	m.used[slot] = true
	m.next++

	col := slot % m.cols
	row := (slot / m.cols) % m.rows
	x = col*m.cellW + m.marginX
	y = row*m.cellH + m.marginY
	w = m.cellW - 2*m.marginX
	h = m.cellH - 2*m.marginY
	if w < 320 {
		w = 320
	}
	if h < 480 {
		h = 480
	}
	return
}

// Release frees a previously acquired slot so a future device can reuse it.
func (m *Manager) Release(slot int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.used, slot)
}
