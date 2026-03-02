package display

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"codeberg.org/snonux/loadbars/internal/config"
	"codeberg.org/snonux/loadbars/internal/constants"
	"codeberg.org/snonux/loadbars/internal/stats"
	"codeberg.org/snonux/loadbars/internal/version"
	"github.com/veandco/go-sdl2/sdl"
)

// smoothFactor controls how quickly bars blend toward their target values each frame.
// Lower values produce smoother animations.
const smoothFactor = 0.12

// linkScales lists the supported network link speeds in ascending order,
// used by the f/v hotkeys to cycle through link scale values.
var linkScales = []string{"mbit", "10mbit", "100mbit", "gbit", "10gbit"}

type displayFlags struct {
	showAvgLine    bool
	showIOAvgLine  bool
	cpuMode        int // constants.CPUModeAverage / CPUModeCores / CPUModeOff
	showMem        bool
	showNet        bool
	showLoad       bool
	showSeparators bool
	extended       bool
	diskMode       int // constants.DiskModeAggregate / DiskModeDevices / DiskModeOff
}

type smoothState struct {
	prevCPU      map[string]stats.CPULine
	smoothedCPU  map[string]*[10]float64
	smoothedMem  map[string]*struct{ ramUsed, swapUsed float64 }
	smoothedNet  map[string]*struct{ rxPct, txPct float64 }
	prevNet      map[string]stats.NetStamp // aggregated (summed) previous net stamp per host
	peakHistory  map[string][]float64
	prevDisk     map[string]stats.DiskStamp
	smoothedDisk map[string]*struct{ readPct, writePct float64 }
}

type peakState struct {
	loadPeak float64 // global max load1 across all hosts (for bar scaling)
	diskPeak float64 // auto-scale peak (bytes/sec) for disk bars
}

type mouseState struct {
	mouseX        int32     // last known mouse X position (for tooltip hit testing)
	mouseY        int32     // last known mouse Y position (for tooltip hit testing)
	mouseLastMove time.Time // timestamp of last mouse movement; tooltip hidden after 3s idle
}

// runState holds mutable state across the display loop (hotkeys, window size, smoothed data).
type runState struct {
	displayFlags
	smoothState
	peakState
	mouseState
	winW int32
	winH int32
}

// newRunState builds initial run state from config.
// When cfg.LoadMax > 0 the load bar uses a fixed scale; otherwise it
// starts at the auto-scale floor of 2.0 and tracks the live maximum.
func newRunState(cfg *config.Config, winW, winH int32) *runState {
	initLoadPeak := 2.0
	if cfg.LoadMax > 0 {
		initLoadPeak = cfg.LoadMax
	}
	initDiskPeak := 1048576.0 // 1 MB/s floor for auto-scale
	if cfg.DiskMax > 0 {
		initDiskPeak = cfg.DiskMax
	}
	return &runState{
		displayFlags: displayFlags{
			showAvgLine:    cfg.ShowAvgLine,
			showIOAvgLine:  cfg.ShowIOAvgLine,
			cpuMode:        cfg.CPUMode,
			showMem:        cfg.ShowMem,
			showNet:        cfg.ShowNet,
			showLoad:       cfg.ShowLoad,
			showSeparators: cfg.ShowSeparators,
			extended:       cfg.Extended,
			diskMode:       cfg.DiskMode,
		},
		smoothState: smoothState{
			prevCPU:      make(map[string]stats.CPULine),
			smoothedCPU:  make(map[string]*[10]float64),
			smoothedMem:  make(map[string]*struct{ ramUsed, swapUsed float64 }),
			smoothedNet:  make(map[string]*struct{ rxPct, txPct float64 }),
			prevNet:      make(map[string]stats.NetStamp),
			peakHistory:  make(map[string][]float64),
			prevDisk:     make(map[string]stats.DiskStamp),
			smoothedDisk: make(map[string]*struct{ readPct, writePct float64 }),
		},
		peakState: peakState{
			loadPeak: initLoadPeak,
			diskPeak: initDiskPeak,
		},
		mouseState: mouseState{
			mouseX: -1, // off-screen until first mouse move
			mouseY: -1,
		},
		winW: winW,
		winH: winH,
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
		case *sdl.MouseMotionEvent:
			state.mouseX, state.mouseY = ev.X, ev.Y
			state.mouseLastMove = time.Now()
		case *sdl.WindowEvent:
			if ev.Event == sdl.WINDOWEVENT_RESIZED {
				state.winW, state.winH = ev.Data1, ev.Data2
			}
		}
	}
	return false
}

// handleKey handles one key press; returns true to quit.
// handleKey handles one key press; returns true to quit.
// It delegates to focused helpers for toggle, adjust/save, and resize keys.
func handleKey(sym sdl.Keycode, window *sdl.Window, cfg *config.Config, state *runState) bool {
	if sym == sdl.K_q {
		return true
	}
	handleToggleKeys(sym, cfg, state)
	handleAdjustAndSave(sym, cfg, state)
	handleResizeKeys(sym, window, cfg, state)
	return false
}

// handleToggleKeys processes display-toggle hotkeys (1, 2/m, 3/n, 4/l, r, e, g, i, s).
func handleToggleKeys(sym sdl.Keycode, cfg *config.Config, state *runState) {
	switch sym {
	case sdl.K_1:
		cycleCPUMode(state)
	case sdl.K_2, sdl.K_m:
		toggleMem(state)
	case sdl.K_3, sdl.K_n:
		toggleNet(state)
	case sdl.K_4, sdl.K_l:
		toggleLoad(state)
	case sdl.K_5:
		cycleDiskMode(state)
	case sdl.K_r:
		resetAutoScalePeaks(cfg, state)
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
	}
}

func cycleCPUMode(state *runState) {
	state.cpuMode = (state.cpuMode + 1) % constants.CPUModeCount
	switch state.cpuMode {
	case constants.CPUModeAverage:
		fmt.Println("==> CPU: average bar only")
	case constants.CPUModeCores:
		fmt.Println("==> CPU: individual cores")
	case constants.CPUModeOff:
		fmt.Println("==> CPU: off")
	}
}

func cycleDiskMode(state *runState) {
	state.diskMode = (state.diskMode + 1) % constants.DiskModeCount
	switch state.diskMode {
	case constants.DiskModeAggregate:
		fmt.Println("==> Disk: aggregate (all devices)")
	case constants.DiskModeDevices:
		fmt.Println("==> Disk: per-device")
	case constants.DiskModeOff:
		fmt.Println("==> Disk: off")
	}
}

func toggleMem(state *runState) {
	state.showMem = !state.showMem
	fmt.Println("==> Toggled show mem:", state.showMem)
}

func toggleNet(state *runState) {
	state.showNet = !state.showNet
	fmt.Println("==> Toggled show net:", state.showNet)
}

func toggleLoad(state *runState) {
	state.showLoad = !state.showLoad
	fmt.Println("==> Toggled show load:", state.showLoad)
}

func resetAutoScalePeaks(cfg *config.Config, state *runState) {
	if cfg.LoadMax == 0 {
		state.loadPeak = 2.0
		fmt.Println("==> Load peak reset to auto-scale floor (2.0)")
	} else {
		fmt.Println("==> Load peak reset ignored (fixed loadmax =", cfg.LoadMax, ")")
	}
	if state.diskMode == constants.DiskModeOff {
		return
	}
	if cfg.DiskMax == 0 {
		const diskPeakFloorBps = 1048576.0 // 1 MB/s, same as in updateDiskPeak
		state.diskPeak = diskPeakFloorBps
		fmt.Println("==> Disk peak reset to auto-scale floor (1 MB/s)")
		return
	}
	fmt.Println("==> Disk peak reset ignored (fixed diskmax =", cfg.DiskMax, ")")
}

// handleAdjustAndSave processes sampling-adjust and config-write hotkeys (a, y, d, c, f, v, h, w).
func handleAdjustAndSave(sym sdl.Keycode, cfg *config.Config, state *runState) {
	switch sym {
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
	case sdl.K_b:
		cfg.DiskAverage++
		fmt.Println("==> Disk average samples:", cfg.DiskAverage)
	case sdl.K_x:
		if cfg.DiskAverage > 1 {
			cfg.DiskAverage--
		}
		fmt.Println("==> Disk average samples:", cfg.DiskAverage)
	case sdl.K_f:
		scaleLinkUp(cfg)
	case sdl.K_v:
		scaleLinkDown(cfg)
	case sdl.K_h:
		printHotkeys()
	case sdl.K_w:
		// Copy mutable display state back to config before persisting.
		cfg.ShowAvgLine = state.showAvgLine
		cfg.ShowIOAvgLine = state.showIOAvgLine
		cfg.CPUMode = state.cpuMode
		cfg.ShowMem = state.showMem
		cfg.ShowNet = state.showNet
		cfg.ShowLoad = state.showLoad
		cfg.ShowSeparators = state.showSeparators
		cfg.DiskMode = state.diskMode
		cfg.Extended = state.extended
		if err := cfg.Write(); err != nil {
			fmt.Fprintf(os.Stderr, "!!! Write config: %v\n", err)
		} else {
			fmt.Println("==> Config written to ~/.loadbarsrc")
		}
	}
}

// handleResizeKeys processes window-resize hotkeys (arrow keys).
// window may be nil in tests; the guard prevents a nil-pointer panic.
func handleResizeKeys(sym sdl.Keycode, window *sdl.Window, cfg *config.Config, state *runState) {
	if window == nil {
		return
	}
	switch sym {
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
	numBars := countBars(snap, state.cpuMode, state.showMem, state.showNet, state.showLoad, state.diskMode)
	// Always clear the entire window before drawing. SDL2 uses double-buffering,
	// so skipping clear leaves stale content in the back buffer.
	renderer.SetDrawColor(0, 0, 0, 255)
	renderer.Clear()
	if state.showLoad {
		// Update the global load peak before drawing so bar scale is current.
		updateLoadPeak(snap, state, cfg.LoadMax)
	}
	if state.diskMode != constants.DiskModeOff {
		// Update the global disk peak before drawing so bar scale is current.
		updateDiskPeak(snap, state, cfg.DiskMax)
	}
	drawBars(renderer, snap, cfg, state, numBars)
	if state.showAvgLine {
		drawGlobalAvgLine(renderer, snap, state, numBars, cfg.MaxBarsPerRow)
	}
	if state.showIOAvgLine {
		drawGlobalIOAvgLine(renderer, snap, state, numBars, cfg.MaxBarsPerRow)
	}
	// Draw mouse-over tooltip and host highlight inversion on top of all bars
	drawOverlay(renderer, snap, cfg, state)
}

// countBars returns the total number of bars to display. The total is computed
// dynamically per snapshot: each host contributes its own CPU/mem/net/load bars
// plus its own disk device count (so servers with different numbers of devices
// are all included).
func countBars(snap map[string]*stats.HostStats, cpuMode int, showMem, showNet, showLoad bool, diskMode int) int {
	n := 0
	for _, host := range sortedHosts(snap) {
		if h := snap[host]; h != nil {
			n += len(sortedCPUNames(h.CPU, cpuMode))
			if showMem {
				n++
			}
			if showNet {
				n++
			}
			if showLoad {
				n++
			}
			// Per-host device count: aggregate=1 bar, devices=whole-disk count, off=0
			n += len(sortedDiskNames(h.Disk, diskMode))
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
	bars := buildBarMap(snap, cfg, state)
	// Track separator rects (position + row height) for drawing after all bars
	type sepRect struct{ x, y, h int32 }
	var separators []sepRect
	prevHost := ""
	for i := range bars {
		bar := bars[i]
		h := snap[bar.host]
		if h == nil {
			continue
		}
		if state.showSeparators && prevHost != "" && bar.host != prevHost {
			separators = append(separators, sepRect{bar.rect.X, bar.rect.Y, bar.rect.H})
		}
		drawBar(renderer, h, bar, cfg, state)
		prevHost = bar.host
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
	numRows := countRows(numBars, maxPerRow)
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
	numRows := countRows(numBars, maxPerRow)
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

func countRows(numBars, maxPerRow int) int {
	if maxPerRow > 0 && maxPerRow < numBars {
		return (numBars + maxPerRow - 1) / maxPerRow
	}
	return 1
}

func drawBar(renderer *sdl.Renderer, h *stats.HostStats, bar barDescriptor, cfg *config.Config, state *runState) {
	x, y, barW, barH := bar.rect.X, bar.rect.Y, bar.rect.W, bar.rect.H
	switch bar.kind {
	case barCPU:
		key := bar.host + ";" + bar.cpuName
		cur := h.CPU[bar.cpuName]
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
		drawCPUBarFromPcts(renderer, s, barW, x, y, barH, state.extended, peakPct)
	case barMem:
		if state.smoothedMem[bar.host] == nil {
			state.smoothedMem[bar.host] = &struct{ ramUsed, swapUsed float64 }{}
		}
		drawMemBarSmoothed(renderer, h, state.smoothedMem[bar.host], smoothFactor, barW, x, y, barH)
	case barNet:
		if state.smoothedNet[bar.host] == nil {
			state.smoothedNet[bar.host] = &struct{ rxPct, txPct float64 }{}
		}
		state.prevNet[bar.host] = drawNetBarSmoothed(renderer, h, cfg, state.smoothedNet[bar.host], state.prevNet[bar.host], smoothFactor, barW, x, y, barH)
	case barLoad:
		drawLoadAvgBar(renderer, h, state.loadPeak, barW, x, y, barH)
	case barDisk:
		key := bar.host + ";disk;" + bar.diskName
		cur := h.Disk[bar.diskName]
		if bar.diskName == "all" {
			cur = sumAllDisks(h.Disk)
		}
		if state.smoothedDisk[key] == nil {
			state.smoothedDisk[key] = &struct{ readPct, writePct float64 }{}
		}
		state.prevDisk[key] = drawDiskBarSmoothed(renderer, cur, state, state.smoothedDisk[key], state.prevDisk[key], smoothFactor, barW, x, y, barH, state.extended)
	}
}

func sortedHosts(snap map[string]*stats.HostStats) []string {
	out := make([]string, 0, len(snap))
	for h := range snap {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

func printHotkeys() {
	fmt.Println("=> Hotkeys: 1=cores 2/m=mem 3/n=net 4/l=load 5=disk r=reset load/disk peak e=extended g=avg line i=io avg s=separators h=help q=quit w=write config a/y=cpu avg d/c=net avg b/x=disk avg f/v=link scale arrows=resize")
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

// updateLoadPeak maintains the load scale used by the bar renderer.
// When loadMax > 0, the scale is pinned to that fixed value every frame
// (no decay, no tracking). When loadMax == 0, auto-scale is used: the
// global peak decays slowly (× 0.9999 per frame) with a floor of 2.0,
// and is updated with the current maximum 1-min load across all hosts.
func updateLoadPeak(snap map[string]*stats.HostStats, state *runState, loadMax float64) {
	if loadMax > 0 {
		state.loadPeak = loadMax // fixed scale: override every frame, skip auto logic
		return
	}
	state.loadPeak *= 0.9999 // slow per-frame decay toward idle baseline
	if state.loadPeak < 2.0 {
		state.loadPeak = 2.0
	}
	for _, h := range snap {
		if h == nil {
			continue
		}
		if l1, err := strconv.ParseFloat(strings.TrimSpace(h.LoadAvg1), 64); err == nil {
			if l1 > state.loadPeak {
				state.loadPeak = l1
			}
		}
	}
}

// drawLoadAvgBar renders a load-average bar for one host.
// The teal fill extends from the top downward proportional to the smoothed 1-min
// load average relative to the global loadPeak scale.
// A yellow 1px line marks the 5-min average and a white 1px line marks the
// 15-min average, giving a visual indication of load trend direction:
// when load is rising the reference lines appear inside the fill;
// when load is falling they hang below it.
func drawLoadAvgBar(renderer *sdl.Renderer, h *stats.HostStats, loadPeak float64, barW int32, x, y, barH int32) {
	// Clear this slot to black before drawing.
	renderer.SetDrawColor(constants.Black.R, constants.Black.G, constants.Black.B, 255)
	renderer.FillRect(&sdl.Rect{X: x, Y: y, W: barW, H: barH})

	// Load averages are already kernel-computed time averages; no further smoothing needed.
	l1, err1 := strconv.ParseFloat(strings.TrimSpace(h.LoadAvg1), 64)
	l5, err5 := strconv.ParseFloat(strings.TrimSpace(h.LoadAvg5), 64)
	l15, err15 := strconv.ParseFloat(strings.TrimSpace(h.LoadAvg15), 64)
	if err1 != nil || err5 != nil || err15 != nil {
		return // no valid data yet
	}

	clamp := func(v, lo, hi float64) float64 {
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}

	// Teal fill from top downward for 1-min load.
	l1H := int32(clamp(l1/loadPeak, 0, 1) * float64(barH))
	if l1H > 0 {
		renderer.SetDrawColor(constants.Teal.R, constants.Teal.G, constants.Teal.B, 255)
		renderer.FillRect(&sdl.Rect{X: x, Y: y, W: barW, H: l1H})
	}

	// Yellow 1px line for 5-min average.
	l5Y := y + int32(clamp(l5/loadPeak, 0, 1)*float64(barH))
	if l5Y < y+barH {
		renderer.SetDrawColor(constants.Yellow.R, constants.Yellow.G, constants.Yellow.B, 255)
		renderer.DrawLine(x, l5Y, x+barW-1, l5Y)
	}

	// White 1px line for 15-min average.
	l15Y := y + int32(clamp(l15/loadPeak, 0, 1)*float64(barH))
	if l15Y < y+barH {
		renderer.SetDrawColor(constants.White.R, constants.White.G, constants.White.B, 255)
		renderer.DrawLine(x, l15Y, x+barW-1, l15Y)
	}
}
