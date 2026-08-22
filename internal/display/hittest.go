package display

import (
	"github.com/snonux/loadbars/internal/config"
	"github.com/snonux/loadbars/internal/stats"
	"github.com/veandco/go-sdl2/sdl"
)

// barKind identifies what type of data a bar represents.
type barKind int

const (
	barCPU barKind = iota
	barMem
	barNet
	barLoad
	barDisk
)

// barDescriptor describes a single bar's position, host, and type.
type barDescriptor struct {
	host     string  // hostname this bar belongs to
	kind     barKind // CPU, mem, net, load, or disk
	cpuName  string  // CPU name (e.g. "cpu", "cpu0"); only set for barCPU
	diskName string  // disk device name (e.g. "sda", "all"); only set for barDisk
	rect     sdl.Rect
}

// iterateBars emits bars in the canonical host/CPU/mem/net/load/disk order.
func iterateBars(snap map[string]*stats.HostStats, state *runState, visit func(h *stats.HostStats, bar barDescriptor)) {
	for _, host := range sortedHosts(snap) {
		h := snap[host]
		if h == nil {
			continue
		}
		for _, name := range sortedCPUNames(h.CPU, state.cpuMode) {
			visit(h, barDescriptor{host: host, kind: barCPU, cpuName: name})
		}
		if state.showMem {
			visit(h, barDescriptor{host: host, kind: barMem})
		}
		if state.showNet {
			visit(h, barDescriptor{host: host, kind: barNet})
		}
		if state.showLoad {
			visit(h, barDescriptor{host: host, kind: barLoad})
		}
		for _, dname := range sortedDiskNames(h.Disk, state.diskMode) {
			visit(h, barDescriptor{host: host, kind: barDisk, diskName: dname})
		}
	}
}

// buildBarMap produces bar descriptors with screen rectangles.
func buildBarMap(snap map[string]*stats.HostStats, cfg *config.Config, state *runState) []barDescriptor {
	numBars := countBars(snap, state.cpuMode, state.showMem, state.showNet, state.showLoad, state.diskMode)
	bars := make([]barDescriptor, 0, numBars)
	barIndex := 0
	iterateBars(snap, state, func(_ *stats.HostStats, bar barDescriptor) {
		x, y, w, bh := barRect(state.winW, state.winH, numBars, cfg.MaxBarsPerRow, barIndex)
		bar.rect = sdl.Rect{X: x, Y: y, W: w, H: bh}
		bars = append(bars, bar)
		barIndex++
	})
	return bars
}

// hitTest returns the bar descriptor under the given point, or nil if none.
func hitTest(bars []barDescriptor, mx, my int32) *barDescriptor {
	for i := range bars {
		r := &bars[i].rect
		if mx >= r.X && mx < r.X+r.W && my >= r.Y && my < r.Y+r.H {
			return &bars[i]
		}
	}
	return nil
}
