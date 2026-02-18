package display

import (
	"codeberg.org/snonux/loadbars/internal/config"
	"codeberg.org/snonux/loadbars/internal/stats"
	"github.com/veandco/go-sdl2/sdl"
)

// barKind identifies what type of data a bar represents.
type barKind int

const (
	barCPU barKind = iota
	barMem
	barNet
)

// barDescriptor describes a single bar's position, host, and type.
type barDescriptor struct {
	host    string  // hostname this bar belongs to
	kind    barKind // CPU, mem, or net
	cpuName string  // CPU name (e.g. "cpu", "cpu0"); only set for barCPU
	rect    sdl.Rect
}

// buildBarMap replays the same host/bar iteration as drawBars to produce
// a slice of bar descriptors with their screen rectangles.
func buildBarMap(snap map[string]*stats.HostStats, cfg *config.Config, state *runState) []barDescriptor {
	numBars := countBars(snap, state.cpuMode, state.showMem, state.showNet)
	maxPerRow := cfg.MaxBarsPerRow
	hosts := sortedHosts(snap)

	bars := make([]barDescriptor, 0, numBars)
	barIndex := 0
	for _, host := range hosts {
		h := snap[host]
		if h == nil {
			continue
		}
		cpuNames := sortedCPUNames(h.CPU, state.cpuMode)
		for _, name := range cpuNames {
			x, y, w, bh := barRect(state.winW, state.winH, numBars, maxPerRow, barIndex)
			bars = append(bars, barDescriptor{
				host:    host,
				kind:    barCPU,
				cpuName: name,
				rect:    sdl.Rect{X: x, Y: y, W: w, H: bh},
			})
			barIndex++
		}
		if state.showMem {
			x, y, w, bh := barRect(state.winW, state.winH, numBars, maxPerRow, barIndex)
			bars = append(bars, barDescriptor{
				host: host,
				kind: barMem,
				rect: sdl.Rect{X: x, Y: y, W: w, H: bh},
			})
			barIndex++
		}
		if state.showNet {
			x, y, w, bh := barRect(state.winW, state.winH, numBars, maxPerRow, barIndex)
			bars = append(bars, barDescriptor{
				host: host,
				kind: barNet,
				rect: sdl.Rect{X: x, Y: y, W: w, H: bh},
			})
			barIndex++
		}
	}
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
