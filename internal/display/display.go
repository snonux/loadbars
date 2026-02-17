package display

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"codeberg.org/snonux/loadbars/internal/collector"
	"codeberg.org/snonux/loadbars/internal/config"
	"codeberg.org/snonux/loadbars/internal/constants"
	"codeberg.org/snonux/loadbars/internal/stats"
	"codeberg.org/snonux/loadbars/internal/version"
	"github.com/veandco/go-sdl2/sdl"
)

const smoothFactor = 0.12 // blend toward target each frame; lower = smoother

// linkScales lists the supported network link speeds in ascending order,
// used by the f/v hotkeys to cycle through link scale values.
var linkScales = []string{"mbit", "10mbit", "100mbit", "gbit", "10gbit"}

// runState holds mutable state across the display loop (hotkeys, window size, smoothed data).
type runState struct {
	showAvgLine    bool
	showIOAvgLine  bool
	showCores      bool
	showMem        bool
	showNet        bool
	showSeparators bool
	extended       bool
	winW           int32
	winH           int32
	prevCPU        map[string]collector.CPULine
	smoothedCPU    map[string]*[10]float64
	smoothedMem    map[string]*struct{ ramUsed, swapUsed float64 }
	smoothedNet    map[string]*struct{ rxPct, txPct float64 }
	prevNet        map[string]stats.NetStamp // aggregated (summed) previous net stamp per host
	peakHistory    map[string][]float64
}

// newRunState builds initial run state from config.
func newRunState(cfg *config.Config, winW, winH int32) *runState {
	return &runState{
		showAvgLine:    cfg.ShowAvgLine,
		showIOAvgLine:  cfg.ShowIOAvgLine,
		showCores:      cfg.ShowCores,
		showMem:        cfg.ShowMem,
		showNet:        cfg.ShowNet,
		showSeparators: cfg.ShowSeparators,
		extended:       cfg.Extended,
		winW:           winW,
		winH:           winH,
		prevCPU:        make(map[string]collector.CPULine),
		smoothedCPU:    make(map[string]*[10]float64),
		smoothedMem:    make(map[string]*struct{ ramUsed, swapUsed float64 }),
		smoothedNet:    make(map[string]*struct{ rxPct, txPct float64 }),
		prevNet:        make(map[string]stats.NetStamp),
		peakHistory:    make(map[string][]float64),
	}
}

// Run runs the SDL display loop until ctx is cancelled or user presses 'q'.
func Run(ctx context.Context, cfg *config.Config, src stats.Source) error {
	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		return fmt.Errorf("sdl init: %w", err)
	}
	defer sdl.Quit()

	const minWindowWidth = 800
	width := clampInt(cfg.BarWidth, minWindowWidth, cfg.MaxWidth)
	height := cfg.Height
	if height < 1 {
		height = 1
	}
	title := cfg.Title
	if title == "" {
		title = "Loadbars " + version.Version + " (press h for help on stdout)"
	}
	window, renderer, err := sdl.CreateWindowAndRenderer(int32(width), int32(height), sdl.WINDOW_RESIZABLE)
	if err != nil {
		return fmt.Errorf("create window: %w", err)
	}
	defer window.Destroy()
	defer renderer.Destroy()
	window.SetTitle(title)

	// On macOS, bring the window to the foreground
	activateWindow()

	state := newRunState(cfg, int32(width), int32(height))
	ticker := time.NewTicker(time.Duration(constants.IntervalSDL * float64(time.Second)))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if handleEvents(window, cfg, state) {
			return nil
		}
		drawFrame(renderer, src, cfg, state)
		renderer.Present()
		sdl.Delay(10)
		<-ticker.C
	}
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// handleEvents processes all pending SDL events and updates state. Returns true if the user quit.
func handleEvents(window *sdl.Window, cfg *config.Config, state *runState) bool {
	for e := sdl.PollEvent(); e != nil; e = sdl.PollEvent() {
		switch ev := e.(type) {
		case *sdl.QuitEvent:
			return true
		case *sdl.KeyboardEvent:
			if ev.Type != sdl.KEYDOWN || ev.Repeat != 0 {
				continue
			}
			if handleKey(ev.Keysym.Sym, window, cfg, state) {
				return true
			}
		case *sdl.WindowEvent:
			if ev.Event == sdl.WINDOWEVENT_RESIZED {
				state.winW, state.winH = ev.Data1, ev.Data2
			}
		}
	}
	return false
}

// handleKey handles one key press; returns true to quit.
func handleKey(sym sdl.Keycode, window *sdl.Window, cfg *config.Config, state *runState) bool {
	switch sym {
	case sdl.K_q:
		return true
	case sdl.K_1:
		state.showCores = !state.showCores
		fmt.Println("==> Toggled show cores:", state.showCores)
	case sdl.K_2, sdl.K_m:
		state.showMem = !state.showMem
		fmt.Println("==> Toggled show mem:", state.showMem)
	case sdl.K_3, sdl.K_n:
		state.showNet = !state.showNet
		fmt.Println("==> Toggled show net:", state.showNet)
	case sdl.K_e:
		state.extended = !state.extended
		fmt.Println("==> Toggled extended (peak line):", state.extended)
	case sdl.K_g:
		state.showAvgLine = !state.showAvgLine
		fmt.Println("==> Toggled global avg line:", state.showAvgLine)
	case sdl.K_i:
		state.showIOAvgLine = !state.showIOAvgLine
		fmt.Println("==> Toggled global I/O avg line:", state.showIOAvgLine)
	case sdl.K_s:
		state.showSeparators = !state.showSeparators
		fmt.Println("==> Toggled host separators:", state.showSeparators)
	case sdl.K_a:
		cfg.CPUAverage++
		fmt.Println("==> CPU average samples:", cfg.CPUAverage)
	case sdl.K_y:
		if cfg.CPUAverage > 1 {
			cfg.CPUAverage--
		}
		fmt.Println("==> CPU average samples:", cfg.CPUAverage)
	case sdl.K_d:
		cfg.NetAverage++
		fmt.Println("==> Net average samples:", cfg.NetAverage)
	case sdl.K_c:
		if cfg.NetAverage > 1 {
			cfg.NetAverage--
		}
		fmt.Println("==> Net average samples:", cfg.NetAverage)
	case sdl.K_f:
		scaleLinkUp(cfg)
	case sdl.K_v:
		scaleLinkDown(cfg)
	case sdl.K_h:
		printHotkeys()
	case sdl.K_w:
		cfg.ShowAvgLine = state.showAvgLine
		cfg.ShowIOAvgLine = state.showIOAvgLine
		cfg.ShowCores = state.showCores
		cfg.ShowMem = state.showMem
		cfg.ShowNet = state.showNet
		cfg.ShowSeparators = state.showSeparators
		cfg.Extended = state.extended
		if err := cfg.Write(); err != nil {
			fmt.Fprintf(os.Stderr, "!!! Write config: %v\n", err)
		} else {
			fmt.Println("==> Config written to ~/.loadbarsrc")
		}
	case sdl.K_LEFT:
		state.winW -= 100
		if state.winW < 1 {
			state.winW = 1
		}
		window.SetSize(state.winW, state.winH)
	case sdl.K_RIGHT:
		state.winW += 100
		if state.winW > int32(cfg.MaxWidth) {
			state.winW = int32(cfg.MaxWidth)
		}
		window.SetSize(state.winW, state.winH)
	case sdl.K_UP:
		state.winH -= 100
		if state.winH < 1 {
			state.winH = 1
		}
		window.SetSize(state.winW, state.winH)
	case sdl.K_DOWN:
		state.winH += 100
		window.SetSize(state.winW, state.winH)
	}
	return false
}

// barBounds calculates the x position and width for a bar at the given index.
// This distributes remainder pixels evenly, ensuring all bars fill the window width.
func barBounds(winW int32, numBars int, barIndex int) (x int32, width int32) {
	if numBars <= 0 {
		return 0, winW
	}
	// Calculate start and end positions using scaled division to distribute remainder pixels
	startX := (winW * int32(barIndex)) / int32(numBars)
	endX := (winW * int32(barIndex+1)) / int32(numBars)
	return startX, endX - startX
}

// barRect computes the x, y, width, and height for a bar in a multi-row layout.
// When maxPerRow <= 0 or maxPerRow >= numBars, all bars fit in a single row (full height).
// Otherwise, bars wrap into multiple rows of equal height. The last row may have
// fewer bars, which become wider to fill the full window width.
func barRect(winW, winH int32, numBars, maxPerRow, barIndex int) (x, y, w, h int32) {
	if maxPerRow <= 0 || maxPerRow >= numBars {
		// Single row: full window height
		bx, bw := barBounds(winW, numBars, barIndex)
		return bx, 0, bw, winH
	}
	numRows := (numBars + maxPerRow - 1) / maxPerRow // ceil(numBars / maxPerRow)
	row := barIndex / maxPerRow
	col := barIndex % maxPerRow
	// Count how many bars are in this row (last row may have fewer)
	barsInRow := maxPerRow
	if row == numRows-1 {
		barsInRow = numBars - row*maxPerRow
	}
	// Divide window height evenly across rows
	rowY := (winH * int32(row)) / int32(numRows)
	rowH := (winH*int32(row+1))/int32(numRows) - rowY
	bx, bw := barBounds(winW, barsInRow, col)
	return bx, rowY, bw, rowH
}

// drawFrame updates state from snapshot, clears if layout changed, and draws all bars.
// When showAvgLine/showIOAvgLine are enabled, global average lines are drawn on top.
func drawFrame(renderer *sdl.Renderer, src stats.Source, cfg *config.Config, state *runState) {
	snap := src.Snapshot()
	numBars := countBars(snap, state.showCores, state.showMem, state.showNet)
	// Always clear the entire window before drawing. SDL2 uses double-buffering,
	// so skipping clear leaves stale content in the back buffer.
	renderer.SetDrawColor(0, 0, 0, 255)
	renderer.Clear()
	drawBars(renderer, snap, cfg, state, numBars)
	if state.showAvgLine {
		drawGlobalAvgLine(renderer, snap, state, numBars, cfg.MaxBarsPerRow)
	}
	if state.showIOAvgLine {
		drawGlobalIOAvgLine(renderer, snap, state, numBars, cfg.MaxBarsPerRow)
	}
}

func countBars(snap map[string]*stats.HostStats, showCores, showMem, showNet bool) int {
	n := 0
	for _, host := range sortedHosts(snap) {
		if h := snap[host]; h != nil {
			n += len(sortedCPUNames(h.CPU, showCores))
			if showMem {
				n++
			}
			if showNet {
				n++
			}
		}
	}
	if n == 0 {
		n = 1
	}
	return n
}

// drawBars draws CPU, memory, and network bars for all hosts in snap.
// Bars wrap into multiple rows when cfg.MaxBarsPerRow is set.
func drawBars(renderer *sdl.Renderer, snap map[string]*stats.HostStats, cfg *config.Config, state *runState, numBars int) {
	barIndex := 0
	hosts := sortedHosts(snap)
	maxPerRow := cfg.MaxBarsPerRow
	// Track separator rects (position + row height) for drawing after all bars
	type sepRect struct{ x, y, h int32 }
	var separators []sepRect
	for i, host := range hosts {
		h := snap[host]
		if h == nil {
			continue
		}
		drawHostBars(renderer, h, host, cfg, state, numBars, maxPerRow, &barIndex)
		// Record separator position between hosts (not after the last one)
		if state.showSeparators && i < len(hosts)-1 {
			sx, sy, _, sh := barRect(state.winW, state.winH, numBars, maxPerRow, barIndex)
			separators = append(separators, sepRect{sx, sy, sh})
		}
	}
	// Draw 1px red vertical separators on top of all bars (same color as CPU steal)
	for _, sep := range separators {
		renderer.SetDrawColor(constants.Red.R, constants.Red.G, constants.Red.B, 255)
		renderer.FillRect(&sdl.Rect{X: sep.x, Y: sep.y, W: 1, H: sep.h})
	}
}

// drawGlobalAvgLine draws a 1px red horizontal line at the Y position
// corresponding to the mean CPU usage across all hosts. When bars are
// split into multiple rows, one line is drawn per row at the correct
// proportional position within that row.
func drawGlobalAvgLine(renderer *sdl.Renderer, snap map[string]*stats.HostStats, state *runState, numBars, maxPerRow int) {
	var totalUsage float64
	var hostCount int
	for _, host := range sortedHosts(snap) {
		h := snap[host]
		if h == nil {
			continue
		}
		key := host + ";cpu"
		s := state.smoothedCPU[key]
		if s == nil {
			continue
		}
		// Sum all segments except idle (index 3) to get total CPU usage
		var usage float64
		for i := 0; i < 10; i++ {
			if i != 3 {
				usage += (*s)[i]
			}
		}
		totalUsage += usage
		hostCount++
	}
	if hostCount == 0 {
		return
	}
	avgPct := totalUsage / float64(hostCount)
	renderer.SetDrawColor(constants.Red.R, constants.Red.G, constants.Red.B, 255)
	// Draw one line per row, positioned proportionally within each row's height
	numRows := 1
	if maxPerRow > 0 && maxPerRow < numBars {
		numRows = (numBars + maxPerRow - 1) / maxPerRow
	}
	for row := 0; row < numRows; row++ {
		rowY := (state.winH * int32(row)) / int32(numRows)
		rowH := (state.winH*int32(row+1))/int32(numRows) - rowY
		lineY := rowY + rowH - int32(avgPct*float64(rowH)/100)
		if lineY < rowY {
			lineY = rowY
		}
		if lineY >= rowY+rowH {
			lineY = rowY + rowH - 1
		}
		renderer.FillRect(&sdl.Rect{X: 0, Y: lineY, W: state.winW, H: 1})
	}
}

// drawGlobalIOAvgLine draws a 1px pink horizontal line from the top of the window
// at the Y position corresponding to the mean I/O overhead (iowait + IRQ + softIRQ,
// indices 4, 5, 6 in the smoothed CPU array) across all hosts. When bars are split
// into multiple rows, one line is drawn per row.
func drawGlobalIOAvgLine(renderer *sdl.Renderer, snap map[string]*stats.HostStats, state *runState, numBars, maxPerRow int) {
	var totalIO float64
	var hostCount int
	for _, host := range sortedHosts(snap) {
		h := snap[host]
		if h == nil {
			continue
		}
		key := host + ";cpu"
		s := state.smoothedCPU[key]
		if s == nil {
			continue
		}
		// Sum iowait (4) + IRQ (5) + softIRQ (6) for I/O overhead
		totalIO += (*s)[4] + (*s)[5] + (*s)[6]
		hostCount++
	}
	if hostCount == 0 {
		return
	}
	avgPct := totalIO / float64(hostCount)
	renderer.SetDrawColor(constants.Pink.R, constants.Pink.G, constants.Pink.B, 255)
	// Draw one line per row, positioned proportionally from the top of each row
	numRows := 1
	if maxPerRow > 0 && maxPerRow < numBars {
		numRows = (numBars + maxPerRow - 1) / maxPerRow
	}
	for row := 0; row < numRows; row++ {
		rowY := (state.winH * int32(row)) / int32(numRows)
		rowH := (state.winH*int32(row+1))/int32(numRows) - rowY
		lineY := rowY + int32(avgPct*float64(rowH)/100)
		if lineY < rowY {
			lineY = rowY
		}
		if lineY >= rowY+rowH {
			lineY = rowY + rowH - 1
		}
		renderer.FillRect(&sdl.Rect{X: 0, Y: lineY, W: state.winW, H: 1})
	}
}

// drawHostBars draws CPU, mem, and net bars for one host and advances barIndex.
// maxPerRow controls multi-row wrapping (0 = single row).
func drawHostBars(renderer *sdl.Renderer, h *stats.HostStats, host string, cfg *config.Config, state *runState, numBars, maxPerRow int, barIndex *int) {
	cpuNames := sortedCPUNames(h.CPU, state.showCores)
	for _, name := range cpuNames {
		key := host + ";" + name
		cur := h.CPU[name]
		prev := state.prevCPU[key]
		state.prevCPU[key] = cur
		target, ok := cpuBarTargetPcts(cur, prev)
		s := state.smoothedCPU[key]
		if s == nil {
			s = &[10]float64{}
			state.smoothedCPU[key] = s
			if ok {
				*s = target
			}
		} else if ok {
			for i := 0; i < 10; i++ {
				(*s)[i] += (target[i] - (*s)[i]) * smoothFactor
			}
			normalizePcts(s)
		}
		peakPct := peakPctForBar(state, key, cfg.CPUAverage, s)
		x, y, barW, barH := barRect(state.winW, state.winH, numBars, maxPerRow, *barIndex)
		*barIndex++
		drawCPUBarFromPcts(renderer, s, barW, x, y, barH, state.extended, peakPct)
	}
	if state.showMem {
		if state.smoothedMem[host] == nil {
			state.smoothedMem[host] = &struct{ ramUsed, swapUsed float64 }{}
		}
		x, y, barW, barH := barRect(state.winW, state.winH, numBars, maxPerRow, *barIndex)
		*barIndex++
		drawMemBarSmoothed(renderer, h, state.smoothedMem[host], smoothFactor, barW, x, y, barH)
	}
	if state.showNet {
		if state.smoothedNet[host] == nil {
			state.smoothedNet[host] = &struct{ rxPct, txPct float64 }{}
		}
		x, y, barW, barH := barRect(state.winW, state.winH, numBars, maxPerRow, *barIndex)
		*barIndex++
		state.prevNet[host] = drawNetBarSmoothed(renderer, h, cfg, state.smoothedNet[host], state.prevNet[host], smoothFactor, barW, x, y, barH)
	}
}

func peakPctForBar(state *runState, key string, cpuAvg int, s *[10]float64) float64 {
	if !state.extended || s == nil {
		return 0
	}
	userSys := (*s)[0] + (*s)[1]
	hist := state.peakHistory[key]
	hist = append(hist, userSys)
	n := cpuAvg
	if n < 1 {
		n = 1
	}
	for len(hist) > n {
		hist = hist[1:]
	}
	state.peakHistory[key] = hist
	var max float64
	for _, v := range hist {
		if v > max {
			max = v
		}
	}
	return max
}

func sortedHosts(snap map[string]*stats.HostStats) []string {
	out := make([]string, 0, len(snap))
	for h := range snap {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

func sortedCPUNames(cpu map[string]collector.CPULine, showCores bool) []string {
	var names []string
	for name := range cpu {
		if name == "cpu" {
			names = append(names, "cpu")
			continue
		}
		if showCores {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool {
		if names[i] == "cpu" {
			return true
		}
		if names[j] == "cpu" {
			return false
		}
		return names[i] < names[j]
	})
	return names
}

// cpuBarTargetPcts returns the 9 segment percentages (system, user, nice, idle, iowait, irq, softirq, guest, steal) from cur/prev delta. ok is false if no valid sample.
func cpuBarTargetPcts(cur, prev collector.CPULine) (out [10]float64, ok bool) {
	totalCur := cur.Total()
	totalPrev := prev.Total()
	if totalPrev == 0 || totalCur <= totalPrev {
		return out, false
	}
	scale := float64(totalCur-totalPrev) / 100.0
	if scale <= 0 {
		return out, false
	}
	out[0] = float64(cur.System-prev.System) / scale
	out[1] = float64(cur.User-prev.User) / scale
	out[2] = float64(cur.Nice-prev.Nice) / scale
	out[3] = float64(cur.Idle-prev.Idle) / scale
	out[4] = float64(cur.Iowait-prev.Iowait) / scale
	out[5] = float64(cur.IRQ-prev.IRQ) / scale
	out[6] = float64(cur.SoftIRQ-prev.SoftIRQ) / scale
	out[7] = float64(cur.Guest-prev.Guest) / scale
	out[8] = float64(cur.Steal-prev.Steal) / scale
	out[9] = float64(cur.GuestNice-prev.GuestNice) / scale
	for i := range out {
		if out[i] < 0 {
			out[i] = 0
		}
		if out[i] > 100 {
			out[i] = 100
		}
	}
	return out, true
}

func normalizePcts(s *[10]float64) {
	var sum float64
	for i := 0; i < 10; i++ {
		sum += (*s)[i]
	}
	if sum <= 0 {
		return
	}
	for i := 0; i < 10; i++ {
		(*s)[i] = (*s)[i] * 100 / sum
	}
}

// drawCPUBarFromPcts draws one CPU bar from 10 smoothed segment percentages.
// The bar occupies the region (x, y) with dimensions (barW, barH).
// If s is nil, only clears the slot. When extended is true and peakPct > 0,
// draws a 1px peak line (max system+user over history).
func drawCPUBarFromPcts(renderer *sdl.Renderer, s *[10]float64, barW int32, x, y, barH int32, extended bool, peakPct float64) {
	// Clear this slot so we never leave previous (e.g. mem/net) content visible
	renderer.SetDrawColor(constants.Black.R, constants.Black.G, constants.Black.B, 255)
	renderer.FillRect(&sdl.Rect{X: x, Y: y, W: barW, H: barH})
	if s == nil {
		return
	}
	pxPerPct := float64(barH) / 100.0
	curY := float64(y + barH)
	fill := func(r, g, b uint8, pct float64) {
		hh := int32(pct * pxPerPct)
		if hh < 1 && pct > 0 {
			hh = 1
		}
		curY -= float64(hh)
		renderer.SetDrawColor(r, g, b, 255)
		renderer.FillRect(&sdl.Rect{X: x, Y: int32(curY), W: barW, H: hh})
	}
	fill(constants.Blue.R, constants.Blue.G, constants.Blue.B, (*s)[0])             // system
	fill(constants.Yellow.R, constants.Yellow.G, constants.Yellow.B, (*s)[1])       // user
	fill(constants.Green.R, constants.Green.G, constants.Green.B, (*s)[2])          // nice
	fill(constants.LimeGreen.R, constants.LimeGreen.G, constants.LimeGreen.B, (*s)[9]) // guestnice
	fill(constants.Black.R, constants.Black.G, constants.Black.B, (*s)[3])          // idle
	fill(constants.Purple.R, constants.Purple.G, constants.Purple.B, (*s)[4])       // iowait
	fill(constants.White.R, constants.White.G, constants.White.B, (*s)[5])          // irq
	fill(constants.White.R, constants.White.G, constants.White.B, (*s)[6])          // softirq
	fill(constants.Red.R, constants.Red.G, constants.Red.B, (*s)[7])                // guest
	fill(constants.Red.R, constants.Red.G, constants.Red.B, (*s)[8])                // steal
	// Extended: 1px peak line at max (system+user) over history
	if extended && peakPct > 0 {
		peakY := y + barH - int32(peakPct*pxPerPct)
		if peakY < y {
			peakY = y
		}
		if peakY >= y+barH {
			peakY = y + barH - 1
		}
		if peakPct > float64(constants.UserOrangeThreshold) {
			renderer.SetDrawColor(constants.Orange.R, constants.Orange.G, constants.Orange.B, 255)
		} else if peakPct > float64(constants.UserYellowThreshold) {
			renderer.SetDrawColor(constants.Yellow0.R, constants.Yellow0.G, constants.Yellow0.B, 255)
		} else {
			renderer.SetDrawColor(constants.Yellow.R, constants.Yellow.G, constants.Yellow.B, 255)
		}
		renderer.FillRect(&sdl.Rect{X: x, Y: peakY, W: barW, H: 1})
	}
}

// drawMemBarSmoothed blends mem stats toward target and draws one memory bar (RAM left, Swap right).
// The bar occupies the region (x, y) with dimensions (barW, barH).
func drawMemBarSmoothed(renderer *sdl.Renderer, h *stats.HostStats, smoothed *struct{ ramUsed, swapUsed float64 }, factor float64, barW int32, x, y, barH int32) {
	// Clear this slot so we never leave previous (e.g. CPU/net) content visible
	renderer.SetDrawColor(constants.Black.R, constants.Black.G, constants.Black.B, 255)
	renderer.FillRect(&sdl.Rect{X: x, Y: y, W: barW, H: barH})
	if h.Mem == nil {
		return
	}
	var targetRam, targetSwap float64
	if memTotal := h.Mem["MemTotal"]; memTotal > 0 {
		targetRam = 100 - 100*float64(h.Mem["MemFree"])/float64(memTotal)
		if targetRam < 0 {
			targetRam = 0
		}
		if targetRam > 100 {
			targetRam = 100
		}
	}
	if swapTotal := h.Mem["SwapTotal"]; swapTotal > 0 {
		targetSwap = 100 - 100*float64(h.Mem["SwapFree"])/float64(swapTotal)
		if targetSwap < 0 {
			targetSwap = 0
		}
		if targetSwap > 100 {
			targetSwap = 100
		}
	}
	smoothed.ramUsed += (targetRam - smoothed.ramUsed) * factor
	smoothed.swapUsed += (targetSwap - smoothed.swapUsed) * factor

	halfW := barW / 2
	pxPerPct := float64(barH) / 100.0

	// RAM: used (dark grey) from bottom, free (black) on top
	ramUsedH := int32(smoothed.ramUsed * pxPerPct)
	if ramUsedH > 0 {
		renderer.SetDrawColor(constants.DarkGrey.R, constants.DarkGrey.G, constants.DarkGrey.B, 255)
		renderer.FillRect(&sdl.Rect{X: x, Y: y + barH - ramUsedH, W: halfW, H: ramUsedH})
	}
	if ramFreeH := barH - ramUsedH; ramFreeH > 0 {
		renderer.SetDrawColor(constants.Black.R, constants.Black.G, constants.Black.B, 255)
		renderer.FillRect(&sdl.Rect{X: x, Y: y, W: halfW, H: ramFreeH})
	}

	// Swap: used (grey) from bottom, free (black) on top
	swapUsedH := int32(smoothed.swapUsed * pxPerPct)
	if swapUsedH > 0 {
		renderer.SetDrawColor(constants.Grey.R, constants.Grey.G, constants.Grey.B, 255)
		renderer.FillRect(&sdl.Rect{X: x + halfW, Y: y + barH - swapUsedH, W: halfW, H: swapUsedH})
	}
	if swapFreeH := barH - swapUsedH; swapFreeH > 0 {
		renderer.SetDrawColor(constants.Black.R, constants.Black.G, constants.Black.B, 255)
		renderer.FillRect(&sdl.Rect{X: x + halfW, Y: y, W: halfW, H: swapFreeH})
	}
}

func printHotkeys() {
	fmt.Println("=> Hotkeys: 1=cores 2/m=mem 3/n=net e=extended g=avg line i=io avg s=separators h=help q=quit w=write config a/y=cpu avg d/c=net avg f/v=link scale arrows=resize")
}

// scaleLinkUp moves cfg.NetLink to the next higher link speed in linkScales.
// Clamps at the maximum (10gbit).
func scaleLinkUp(cfg *config.Config) {
	idx := linkScaleIndex(cfg.NetLink)
	if idx < len(linkScales)-1 {
		cfg.NetLink = linkScales[idx+1]
	}
	fmt.Println("==> Link scale:", cfg.NetLink)
}

// scaleLinkDown moves cfg.NetLink to the next lower link speed in linkScales.
// Clamps at the minimum (mbit).
func scaleLinkDown(cfg *config.Config) {
	idx := linkScaleIndex(cfg.NetLink)
	if idx > 0 {
		cfg.NetLink = linkScales[idx-1]
	}
	fmt.Println("==> Link scale:", cfg.NetLink)
}

// linkScaleIndex returns the index of the current NetLink value in linkScales.
// Defaults to 3 (gbit) if the value is not recognized.
func linkScaleIndex(netLink string) int {
	s := strings.ToLower(strings.TrimSpace(netLink))
	for i, v := range linkScales {
		if s == v {
			return i
		}
	}
	return 3 // default: gbit
}

func netLinkBytesPerSec(cfg *config.Config) int64 {
	s := strings.ToLower(strings.TrimSpace(cfg.NetLink))
	switch s {
	case "gbit", "1gbit":
		return int64(constants.BytesGbit)
	case "10gbit":
		return int64(constants.Bytes10Gbit)
	case "mbit", "1mbit":
		return int64(constants.BytesMbit)
	case "10mbit":
		return int64(constants.Bytes10Mbit)
	case "100mbit":
		return int64(constants.Bytes100Mbit)
	case "":
		return int64(constants.BytesGbit)
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n * int64(constants.BytesMbit)
	}
	return int64(constants.BytesGbit)
}

// sumNonLoNet aggregates RX (B) and TX (Tb) bytes across all non-lo interfaces,
// using the latest timestamp from any interface.
func sumNonLoNet(h *stats.HostStats) (sum stats.NetStamp, hasIface bool) {
	if h.Net == nil {
		return sum, false
	}
	for iface, ns := range h.Net {
		if iface == "lo" {
			continue
		}
		hasIface = true
		sum.B += ns.B
		sum.Tb += ns.Tb
		if ns.Stamp > sum.Stamp {
			sum.Stamp = ns.Stamp
		}
	}
	return sum, hasIface
}

// drawNetBarSmoothed sums RX/TX across all non-lo interfaces, computes utilization
// vs link speed, smooths toward target, and draws one net bar (RX left from top, TX right from bottom).
// The bar occupies the region (x, y) with dimensions (barW, barH).
// Smoothed values and prevNet are only updated when new collector data arrives
// (cur.Stamp > prev.Stamp), so the bar holds steady between collector cycles
// instead of decaying toward zero on frames with no new data.
func drawNetBarSmoothed(renderer *sdl.Renderer, h *stats.HostStats, cfg *config.Config, smoothed *struct{ rxPct, txPct float64 }, prev stats.NetStamp, factor float64, barW int32, x, y, barH int32) stats.NetStamp {
	// Clear this slot so we never leave previous (e.g. CPU/mem) content visible
	renderer.SetDrawColor(constants.Black.R, constants.Black.G, constants.Black.B, 255)
	renderer.FillRect(&sdl.Rect{X: x, Y: y, W: barW, H: barH})
	cur, hasIface := sumNonLoNet(h)
	if !hasIface {
		// No non-lo interface: show red bar
		renderer.SetDrawColor(constants.Red.R, constants.Red.G, constants.Red.B, 255)
		renderer.FillRect(&sdl.Rect{X: x, Y: y, W: barW, H: barH})
		return prev
	}
	// Only recompute and smooth when the collector has provided new data.
	// The collector updates net stamps every ~2.8s, but drawFrame runs every
	// ~0.14s. Without this guard, the 19 intermediate frames would set
	// target to 0 (no delta) and smooth the bar toward zero, making real
	// traffic invisible.
	if cur.Stamp > prev.Stamp && prev.Stamp > 0 {
		linkBps := netLinkBytesPerSec(cfg)
		if linkBps <= 0 {
			linkBps = int64(constants.BytesGbit)
		}
		dt := cur.Stamp - prev.Stamp
		if dt > 0 {
			deltaB := cur.B - prev.B
			deltaTb := cur.Tb - prev.Tb
			if deltaB < 0 {
				deltaB = 0
			}
			if deltaTb < 0 {
				deltaTb = 0
			}
			targetRx := 100 * float64(deltaB) / (float64(linkBps) * dt)
			targetTx := 100 * float64(deltaTb) / (float64(linkBps) * dt)
			smoothed.rxPct += (targetRx - smoothed.rxPct) * factor
			smoothed.txPct += (targetTx - smoothed.txPct) * factor
		}
		prev = cur // only advance prev when we consumed new data
	} else if prev.Stamp == 0 {
		// First sample: record it but don't draw yet (no delta available)
		prev = cur
	}

	halfW := barW / 2
	pxPerPct := float64(barH) / 100.0
	halfH := barH / 2
	// Left half: RX from top (light green = used)
	rxH := int32(smoothed.rxPct * pxPerPct)
	if rxH > halfH {
		rxH = halfH
	}
	if rxH > 0 {
		renderer.SetDrawColor(constants.LightGreen.R, constants.LightGreen.G, constants.LightGreen.B, 255)
		renderer.FillRect(&sdl.Rect{X: x, Y: y, W: halfW, H: rxH})
	}
	if halfW > 0 && halfH-rxH > 0 {
		renderer.SetDrawColor(constants.Black.R, constants.Black.G, constants.Black.B, 255)
		renderer.FillRect(&sdl.Rect{X: x, Y: y + rxH, W: halfW, H: halfH - rxH})
	}
	// Right half: TX from bottom (light green = used)
	txH := int32(smoothed.txPct * pxPerPct)
	if txH > halfH {
		txH = halfH
	}
	if txH > 0 {
		renderer.SetDrawColor(constants.LightGreen.R, constants.LightGreen.G, constants.LightGreen.B, 255)
		renderer.FillRect(&sdl.Rect{X: x + halfW, Y: y + barH - txH, W: halfW, H: txH})
	}
	if halfW > 0 && (barH-txH) > 0 {
		renderer.SetDrawColor(constants.Black.R, constants.Black.G, constants.Black.B, 255)
		renderer.FillRect(&sdl.Rect{X: x + halfW, Y: y, W: halfW, H: barH - txH})
	}
	return prev
}
