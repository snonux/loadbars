package display

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"codeberg.org/snonux/loadbars/internal/config"
	"codeberg.org/snonux/loadbars/internal/stats"
	"github.com/veandco/go-sdl2/sdl"
)

// mouseIdleTimeout is how long the mouse must be idle before the tooltip
// and host inversion overlay are hidden.
const mouseIdleTimeout = 3 * time.Second

const (
	tooltipScale   int32 = 2  // pixel scale for bitmap font
	tooltipPadX    int32 = 6  // horizontal padding inside tooltip box
	tooltipPadY    int32 = 4  // vertical padding inside tooltip box
	tooltipOffsetX int32 = 12 // offset from cursor to tooltip
	tooltipOffsetY int32 = 12
)

// tooltipLines builds the text lines to display for the hovered bar.
func tooltipLines(bar *barDescriptor, snap map[string]*stats.HostStats, cfg *config.Config, state *runState) []string {
	h := snap[bar.host]
	if h == nil {
		return []string{bar.host}
	}
	switch bar.kind {
	case barCPU:
		return cpuTooltipLines(bar, h, state)
	case barMem:
		return memTooltipLines(bar, h, state)
	case barNet:
		return netTooltipLines(bar, cfg, state)
	case barLoad:
		return loadTooltipLines(bar, h, cfg, state)
	}
	return nil
}

// cpuTooltipLines returns tooltip text for a CPU bar showing smoothed percentages.
func cpuTooltipLines(bar *barDescriptor, h *stats.HostStats, state *runState) []string {
	lines := []string{fmt.Sprintf("%s [%s]", bar.host, bar.cpuName)}
	key := bar.host + ";" + bar.cpuName
	s := state.smoothedCPU[key]
	if s == nil {
		lines = append(lines, "No data yet")
		return lines
	}
	// Indices match cpuBarTargetPcts: 0=sys 1=usr 2=nice 3=idle 4=io 5=irq 6=softirq 7=guest 8=steal 9=guestnice
	lines = append(lines,
		fmt.Sprintf("Sys:    %5.1f%%", s[0]),
		fmt.Sprintf("Usr:    %5.1f%%", s[1]),
		fmt.Sprintf("Nice:   %5.1f%%", s[2]),
		fmt.Sprintf("IO:     %5.1f%%", s[4]),
		fmt.Sprintf("Steal:  %5.1f%%", s[8]),
		fmt.Sprintf("Idle:   %5.1f%%", s[3]),
	)
	return lines
}

// memTooltipLines returns tooltip text for a memory bar.
func memTooltipLines(bar *barDescriptor, h *stats.HostStats, state *runState) []string {
	lines := []string{fmt.Sprintf("%s [mem]", bar.host)}
	sm := state.smoothedMem[bar.host]
	if sm == nil || h.Mem == nil {
		lines = append(lines, "No data yet")
		return lines
	}
	memTotal := h.Mem["MemTotal"]
	memFree := h.Mem["MemFree"]
	swapTotal := h.Mem["SwapTotal"]
	swapFree := h.Mem["SwapFree"]
	lines = append(lines,
		fmt.Sprintf("RAM:  %s / %s", formatKB(memTotal-memFree), formatKB(memTotal)),
		fmt.Sprintf("      %5.1f%%", sm.ramUsed),
		fmt.Sprintf("Swap: %s / %s", formatKB(swapTotal-swapFree), formatKB(swapTotal)),
		fmt.Sprintf("      %5.1f%%", sm.swapUsed),
	)
	return lines
}

// netTooltipLines returns tooltip text for a network bar.
func netTooltipLines(bar *barDescriptor, cfg *config.Config, state *runState) []string {
	lines := []string{fmt.Sprintf("%s [net]", bar.host)}
	sm := state.smoothedNet[bar.host]
	if sm == nil {
		lines = append(lines, "No data yet")
		return lines
	}
	lines = append(lines,
		fmt.Sprintf("RX: %5.1f%%", sm.rxPct),
		fmt.Sprintf("TX: %5.1f%%", sm.txPct),
		fmt.Sprintf("Link: %s", cfg.NetLink),
	)
	return lines
}

// loadTooltipLines returns tooltip text for a load-average bar showing
// the 1/5/15-min averages and the current scale reference.
// When cfg.LoadMax > 0 the scale is fixed and the label reads "Max:";
// otherwise it tracks the auto-scale peak and reads "Peak:".
func loadTooltipLines(bar *barDescriptor, h *stats.HostStats, cfg *config.Config, state *runState) []string {
	lines := []string{fmt.Sprintf("%s [load]", bar.host)}
	l1, err1 := strconv.ParseFloat(strings.TrimSpace(h.LoadAvg1), 64)
	l5, err5 := strconv.ParseFloat(strings.TrimSpace(h.LoadAvg5), 64)
	l15, err15 := strconv.ParseFloat(strings.TrimSpace(h.LoadAvg15), 64)
	if err1 != nil || err5 != nil || err15 != nil {
		lines = append(lines, "No data yet")
		return lines
	}
	scaleLabel := "Peak: "
	if cfg.LoadMax > 0 {
		scaleLabel = "Max:  "
	}
	lines = append(lines,
		fmt.Sprintf("1min:  %.2f", l1),
		fmt.Sprintf("5min:  %.2f", l5),
		fmt.Sprintf("15min: %.2f", l15),
		fmt.Sprintf(scaleLabel+"%.2f", state.loadPeak),
	)
	return lines
}

// formatKB formats a value in KB as a human-readable string (KB, MB, or GB).
func formatKB(kb int64) string {
	switch {
	case kb >= 1024*1024:
		return fmt.Sprintf("%.1fG", float64(kb)/(1024*1024))
	case kb >= 1024:
		return fmt.Sprintf("%.1fM", float64(kb)/1024)
	default:
		return fmt.Sprintf("%dK", kb)
	}
}

// drawTooltip renders the tooltip box with text lines near the cursor position,
// clamped to stay within the window.
func drawTooltip(renderer *sdl.Renderer, lines []string, mx, my, winW, winH int32) {
	if len(lines) == 0 {
		return
	}
	// Compute box dimensions from the longest line
	lineH := glyphH*tooltipScale + 2 // 2px spacing between lines
	maxW := int32(0)
	for _, l := range lines {
		w := stringWidth(l, tooltipScale)
		if w > maxW {
			maxW = w
		}
	}
	boxW := maxW + tooltipPadX*2
	boxH := int32(len(lines))*lineH + tooltipPadY*2 - 2 // subtract trailing spacing

	// Position: prefer below-right of cursor, clamp to window
	bx := mx + tooltipOffsetX
	by := my + tooltipOffsetY
	if bx+boxW > winW {
		bx = mx - tooltipOffsetX - boxW
	}
	if by+boxH > winH {
		by = my - tooltipOffsetY - boxH
	}
	if bx < 0 {
		bx = 0
	}
	if by < 0 {
		by = 0
	}

	// Dark background
	renderer.SetDrawColor(0x18, 0x18, 0x18, 240)
	renderer.FillRect(&sdl.Rect{X: bx, Y: by, W: boxW, H: boxH})
	// Grey border (1px)
	renderer.SetDrawColor(0x60, 0x60, 0x60, 255)
	renderer.DrawRect(&sdl.Rect{X: bx, Y: by, W: boxW, H: boxH})

	// Light text
	renderer.SetDrawColor(0xE0, 0xE0, 0xE0, 255)
	ty := by + tooltipPadY
	for _, l := range lines {
		drawString(renderer, l, bx+tooltipPadX, ty, tooltipScale)
		ty += lineH
	}
}

// invertHostBars draws a color-inversion overlay on all bars belonging to the given host.
// Uses SDL custom blend mode: src factor = ONE_MINUS_DST_COLOR, so drawing white
// produces (1 - dst) = inverted colors.
func invertHostBars(renderer *sdl.Renderer, bars []barDescriptor, host string) {
	invertBlend := sdl.ComposeCustomBlendMode(
		sdl.BLENDFACTOR_ONE_MINUS_DST_COLOR, sdl.BLENDFACTOR_ZERO, sdl.BLENDOPERATION_ADD,
		sdl.BLENDFACTOR_ONE, sdl.BLENDFACTOR_ZERO, sdl.BLENDOPERATION_ADD,
	)
	renderer.SetDrawBlendMode(invertBlend)
	renderer.SetDrawColor(255, 255, 255, 255)
	for i := range bars {
		if bars[i].host == host {
			renderer.FillRect(&bars[i].rect)
		}
	}
	renderer.SetDrawBlendMode(sdl.BLENDMODE_NONE)
}

// drawOverlay handles the full mouse-over effect: builds the bar map, performs
// hit testing, inverts the hovered host's bars, and draws the tooltip.
func drawOverlay(renderer *sdl.Renderer, snap map[string]*stats.HostStats, cfg *config.Config, state *runState) {
	// Hide tooltip and inversion when mouse has been idle for 3 seconds
	if state.mouseLastMove.IsZero() || time.Since(state.mouseLastMove) > mouseIdleTimeout {
		return
	}
	bars := buildBarMap(snap, cfg, state)
	hit := hitTest(bars, state.mouseX, state.mouseY)
	if hit == nil {
		return
	}
	invertHostBars(renderer, bars, hit.host)
	lines := tooltipLines(hit, snap, cfg, state)
	drawTooltip(renderer, lines, state.mouseX, state.mouseY, state.winW, state.winH)
}
