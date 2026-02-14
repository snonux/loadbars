package display

import (
	"fmt"
	"os"
	"testing"

	"codeberg.org/snonux/loadbars/internal/collector"
	"codeberg.org/snonux/loadbars/internal/config"
	"codeberg.org/snonux/loadbars/internal/constants"
	"codeberg.org/snonux/loadbars/internal/stats"
	"github.com/veandco/go-sdl2/sdl"
)

// mockSource implements stats.Source with fixed data for deterministic tests.
type mockSource struct {
	data map[string]*stats.HostStats
}

func (m *mockSource) Snapshot() map[string]*stats.HostStats {
	return m.data
}

// TestMain sets SDL_VIDEODRIVER=dummy so tests work headlessly (no display needed).
func TestMain(m *testing.M) {
	os.Setenv("SDL_VIDEODRIVER", "dummy")
	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		fmt.Fprintf(os.Stderr, "SDL init failed (SDL_VIDEODRIVER=dummy): %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	sdl.Quit()
	os.Exit(code)
}

// createTestRenderer creates a software renderer backed by an in-memory surface.
// Returns the renderer and surface; caller must defer Destroy/Free.
func createTestRenderer(w, h int32) (*sdl.Renderer, *sdl.Surface, error) {
	surface, err := sdl.CreateRGBSurface(0, w, h, 32, 0x00FF0000, 0x0000FF00, 0x000000FF, 0xFF000000)
	if err != nil {
		return nil, nil, fmt.Errorf("create surface: %w", err)
	}
	renderer, err := sdl.CreateSoftwareRenderer(surface)
	if err != nil {
		surface.Free()
		return nil, nil, fmt.Errorf("create software renderer: %w", err)
	}
	return renderer, surface, nil
}

// getPixelColor reads the RGB values at pixel (x, y) from the surface.
func getPixelColor(surface *sdl.Surface, x, y int32) (r, g, b uint8) {
	bpp := int32(surface.Format.BytesPerPixel)
	pixels := surface.Pixels()
	offset := y*surface.Pitch + x*bpp
	if offset < 0 || int(offset+bpp) > len(pixels) {
		return 0, 0, 0
	}
	// Read raw 32-bit pixel value (little-endian)
	pixel := uint32(pixels[offset]) | uint32(pixels[offset+1])<<8 |
		uint32(pixels[offset+2])<<16 | uint32(pixels[offset+3])<<24
	// Extract RGB using mask and shift derived from the mask itself
	r = uint8((pixel & surface.Format.Rmask) >> maskShift(surface.Format.Rmask))
	g = uint8((pixel & surface.Format.Gmask) >> maskShift(surface.Format.Gmask))
	b = uint8((pixel & surface.Format.Bmask) >> maskShift(surface.Format.Bmask))
	return r, g, b
}

// maskShift returns the bit position of the lowest set bit in mask.
func maskShift(mask uint32) uint {
	if mask == 0 {
		return 0
	}
	shift := uint(0)
	for mask&1 == 0 {
		mask >>= 1
		shift++
	}
	return shift
}

// assertPixelColor checks that the pixel at (x,y) matches the expected RGB within tolerance.
func assertPixelColor(t *testing.T, surface *sdl.Surface, x, y int32, expected constants.RGB, tolerance uint8, label string) {
	t.Helper()
	r, g, b := getPixelColor(surface, x, y)
	if diff(r, expected.R) > tolerance || diff(g, expected.G) > tolerance || diff(b, expected.B) > tolerance {
		t.Errorf("%s at (%d,%d): got RGB(%d,%d,%d), want RGB(%d,%d,%d) ±%d",
			label, x, y, r, g, b, expected.R, expected.G, expected.B, tolerance)
	}
}

func diff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

// defaultTestConfig returns a minimal config suitable for tests.
func defaultTestConfig() *config.Config {
	cfg := config.Default()
	cfg.NetLink = "gbit"
	cfg.CPUAverage = 1
	return &cfg
}

// makeCPUPair creates a (prev, cur) pair of CPULine such that the delta yields
// the desired system/user/idle percentages (the rest are zero).
// prev has a base total of 1000 (all idle); cur adds delta of 1000 with the desired distribution.
func makeCPUPair(systemPct, userPct, idlePct float64) (prev, cur collector.CPULine) {
	const base = 1000
	const delta = 1000
	// prev must have non-zero total for cpuBarTargetPcts to accept the sample
	prev = collector.CPULine{Idle: base}
	dSys := int64(systemPct * float64(delta) / 100)
	dUser := int64(userPct * float64(delta) / 100)
	dIdle := int64(idlePct * float64(delta) / 100)
	dNice := delta - dSys - dUser - dIdle
	if dNice < 0 {
		dNice = 0
	}
	cur = collector.CPULine{
		System: prev.System + dSys,
		User:   prev.User + dUser,
		Idle:   prev.Idle + dIdle,
		Nice:   prev.Nice + dNice,
	}
	return prev, cur
}

// renderOneCPUBar sets up state with pre-populated smoothed values, calls drawFrame,
// and returns the surface for pixel inspection.
func renderOneCPUBar(t *testing.T, systemPct, userPct, idlePct float64, extended bool) (*sdl.Surface, *sdl.Renderer) {
	t.Helper()
	const w, h int32 = 100, 100

	renderer, surface, err := createTestRenderer(w, h)
	if err != nil {
		t.Fatal(err)
	}

	prev, cur := makeCPUPair(systemPct, userPct, idlePct)
	cfg := defaultTestConfig()
	cfg.ShowCores = false
	cfg.ShowMem = false
	cfg.ShowNet = false
	cfg.Extended = extended

	src := &mockSource{
		data: map[string]*stats.HostStats{
			"host1": {
				CPU: map[string]collector.CPULine{"cpu": cur},
			},
		},
	}

	state := newRunState(cfg, w, h)
	// Pre-populate prevCPU so the delta calculation works on the first drawFrame call
	state.prevCPU["host1;cpu"] = prev

	drawFrame(renderer, src, cfg, state)
	return surface, renderer
}

func TestCPUBar_UserSystemIdle(t *testing.T) {
	// 30% system (blue from bottom), 50% user (yellow above), 20% idle (black on top)
	surface, renderer := renderOneCPUBar(t, 30, 50, 20, false)
	defer renderer.Destroy()
	defer surface.Free()

	const tol = 3
	// Bottom area should be system (blue)
	assertPixelColor(t, surface, 50, 95, constants.Blue, tol, "system/blue at bottom")
	// Middle area should be user (yellow) — system takes bottom 30px, user the next 50px
	assertPixelColor(t, surface, 50, 55, constants.Yellow, tol, "user/yellow in middle")
	// Top area should be idle (black)
	assertPixelColor(t, surface, 50, 5, constants.Black, tol, "idle/black at top")
}

func TestCPUBar_FullLoad(t *testing.T) {
	// 100% system — entire bar should be blue
	surface, renderer := renderOneCPUBar(t, 100, 0, 0, false)
	defer renderer.Destroy()
	defer surface.Free()

	const tol = 3
	assertPixelColor(t, surface, 50, 5, constants.Blue, tol, "full system top")
	assertPixelColor(t, surface, 50, 50, constants.Blue, tol, "full system mid")
	assertPixelColor(t, surface, 50, 95, constants.Blue, tol, "full system bottom")
}

func TestCPUBar_AllIdle(t *testing.T) {
	// 100% idle — entire bar should be black
	surface, renderer := renderOneCPUBar(t, 0, 0, 100, false)
	defer renderer.Destroy()
	defer surface.Free()

	const tol = 3
	assertPixelColor(t, surface, 50, 5, constants.Black, tol, "all idle top")
	assertPixelColor(t, surface, 50, 50, constants.Black, tol, "all idle mid")
	assertPixelColor(t, surface, 50, 95, constants.Black, tol, "all idle bottom")
}

func TestMemBar_RamAndSwap(t *testing.T) {
	const w, h int32 = 100, 100

	renderer, surface, err := createTestRenderer(w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	cfg := defaultTestConfig()
	cfg.ShowCores = false
	cfg.ShowMem = true
	cfg.ShowNet = false

	// 60% RAM used, 40% swap used
	src := &mockSource{
		data: map[string]*stats.HostStats{
			"host1": {
				CPU: map[string]collector.CPULine{
					"cpu": {User: 100, System: 100, Idle: 800}, // needed so countBars > 0
				},
				Mem: map[string]int64{
					"MemTotal": 1000,
					"MemFree":  400, // 60% used
					"SwapTotal": 1000,
					"SwapFree":  600, // 40% used
				},
			},
		},
	}

	state := newRunState(cfg, w, h)
	// Pre-populate prevCPU so CPU bar renders (needed for countBars)
	state.prevCPU["host1;cpu"] = collector.CPULine{}
	// Pre-populate smoothed mem so the first frame is close to target
	state.smoothedMem["host1"] = &struct{ ramUsed, swapUsed float64 }{
		ramUsed: 60, swapUsed: 40,
	}

	drawFrame(renderer, src, cfg, state)

	const tol = 5
	// Bar layout: 1 CPU bar + 1 mem bar = 2 bars total, each 50px wide
	// Mem bar starts at x=50, halfW=25
	// RAM (left half of mem bar, x=50..74): 60% used = 60px DarkGrey from bottom
	assertPixelColor(t, surface, 60, 95, constants.DarkGrey, tol, "RAM used at bottom")
	assertPixelColor(t, surface, 60, 10, constants.Black, tol, "RAM free at top")

	// Swap (right half of mem bar, x=75..99): 40% used = 40px Grey from bottom
	assertPixelColor(t, surface, 85, 95, constants.Grey, tol, "Swap used at bottom")
	assertPixelColor(t, surface, 85, 10, constants.Black, tol, "Swap free at top")
}

func TestNetBar_RxTx(t *testing.T) {
	const w, h int32 = 100, 100

	renderer, surface, err := createTestRenderer(w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	cfg := defaultTestConfig()
	cfg.ShowCores = false
	cfg.ShowMem = false
	cfg.ShowNet = true
	cfg.NetLink = "gbit"

	src := &mockSource{
		data: map[string]*stats.HostStats{
			"host1": {
				CPU: map[string]collector.CPULine{
					"cpu": {User: 100, System: 100, Idle: 800},
				},
				Net: map[string]stats.NetStamp{
					"eth0": {B: 12500000, Tb: 6250000, Stamp: 2e9}, // current sample
				},
			},
		},
	}

	state := newRunState(cfg, w, h)
	state.prevCPU["host1;cpu"] = collector.CPULine{}
	// Pre-populate prevNet so delta calculation works:
	// RX: delta=12500000 bytes in 1s = 10% of gbit, TX: 6250000 = 5% of gbit
	state.prevNet["host1"] = stats.NetStamp{B: 0, Tb: 0, Stamp: 1e9}
	// Pre-populate smoothed net so first frame is near target
	state.smoothedNet["host1"] = &struct{ rxPct, txPct float64 }{
		rxPct: 10, txPct: 5,
	}

	drawFrame(renderer, src, cfg, state)

	const tol = 5
	// Net bar: 1 CPU + 1 net = 2 bars, each 50px. Net bar at x=50, halfW=25
	// Left half (RX from top): 10% = 10px of LightGreen from top
	assertPixelColor(t, surface, 60, 2, constants.LightGreen, tol, "RX at top")
	assertPixelColor(t, surface, 60, 45, constants.Black, tol, "RX free area")

	// Right half (TX from bottom): 5% = 5px of LightGreen from bottom
	assertPixelColor(t, surface, 85, 98, constants.LightGreen, tol, "TX at bottom")
	assertPixelColor(t, surface, 85, 10, constants.Black, tol, "TX free area")
}

func TestMultiHost_BarCount(t *testing.T) {
	const w, h int32 = 600, 100

	renderer, surface, err := createTestRenderer(w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	cfg := defaultTestConfig()
	cfg.ShowCores = false
	cfg.ShowMem = true
	cfg.ShowNet = true

	// 2 hosts, each with 1 CPU bar + 1 mem bar + 1 net bar = 6 bars total
	// Use makeCPUPair to get valid prev/cur pairs for delta calculation
	alphaPrev, alphaCur := makeCPUPair(50, 0, 50)
	betaPrev, betaCur := makeCPUPair(0, 50, 50)

	src := &mockSource{
		data: map[string]*stats.HostStats{
			"alpha": {
				CPU: map[string]collector.CPULine{"cpu": alphaCur},
				Mem:  map[string]int64{"MemTotal": 100, "MemFree": 50, "SwapTotal": 0, "SwapFree": 0},
				Net:  map[string]stats.NetStamp{"eth0": {B: 0, Tb: 0, Stamp: 1e9}},
			},
			"beta": {
				CPU: map[string]collector.CPULine{"cpu": betaCur},
				Mem:  map[string]int64{"MemTotal": 100, "MemFree": 50, "SwapTotal": 0, "SwapFree": 0},
				Net:  map[string]stats.NetStamp{"eth0": {B: 0, Tb: 0, Stamp: 1e9}},
			},
		},
	}

	snap := src.Snapshot()
	numBars := countBars(snap, false, true, true)
	if numBars != 6 {
		t.Fatalf("expected 6 bars (2 hosts × 3), got %d", numBars)
	}

	state := newRunState(cfg, w, h)
	state.prevCPU["alpha;cpu"] = alphaPrev
	state.prevCPU["beta;cpu"] = betaPrev

	drawFrame(renderer, src, cfg, state)

	// 6 bars of 100px each in a 600px window
	barW := w / int32(numBars) // = 100

	// Verify alpha's CPU bar (bar 0, x=0..99) has some blue (50% system)
	assertPixelColor(t, surface, barW/2, 90, constants.Blue, 5, "alpha CPU system")
	// Verify beta's CPU bar (bar 3, x=300..399) has some yellow (50% user)
	assertPixelColor(t, surface, 3*barW+barW/2, 90, constants.Yellow, 5, "beta CPU user")
}

func TestCores_Toggle(t *testing.T) {
	// With showCores=true and 2 cores, we get cpu + cpu0 + cpu1 = 3 CPU bars
	hostStats := &stats.HostStats{
		CPU: map[string]collector.CPULine{
			"cpu":  {System: 500, User: 0, Idle: 500},
			"cpu0": {System: 500, User: 0, Idle: 500},
			"cpu1": {System: 0, User: 500, Idle: 500},
		},
	}

	snap := map[string]*stats.HostStats{"host1": hostStats}

	// showCores=true: should count 3 CPU bars
	nWith := countBars(snap, true, false, false)
	if nWith != 3 {
		t.Errorf("showCores=true: expected 3 bars, got %d", nWith)
	}

	// showCores=false: should count 1 CPU bar (aggregate only)
	nWithout := countBars(snap, false, false, false)
	if nWithout != 1 {
		t.Errorf("showCores=false: expected 1 bar, got %d", nWithout)
	}
}

func TestExtended_PeakLine(t *testing.T) {
	// 80% system + user → above UserOrangeThreshold (70), peak line should be orange
	surface, renderer := renderOneCPUBar(t, 40, 40, 20, true)
	defer renderer.Destroy()
	defer surface.Free()

	// Peak line at 80% from bottom = y = 100 - 80 = 20
	// Check that the peak line pixel is orange (not black)
	peakY := int32(100 - 80)
	r, g, b := getPixelColor(surface, 50, peakY)
	if r == 0 && g == 0 && b == 0 {
		t.Errorf("expected peak line at y=%d to be non-black, got RGB(%d,%d,%d)", peakY, r, g, b)
	}
	// The peak should be orange since 80% > UserOrangeThreshold (70)
	assertPixelColor(t, surface, 50, peakY, constants.Orange, 5, "peak line orange")
}

func TestExtended_PeakLine_Yellow(t *testing.T) {
	// 60% system + user → above UserYellowThreshold (50) but below UserOrangeThreshold (70)
	// Peak line should be Yellow0
	surface, renderer := renderOneCPUBar(t, 30, 30, 40, true)
	defer renderer.Destroy()
	defer surface.Free()

	peakY := int32(100 - 60)
	r, g, b := getPixelColor(surface, 50, peakY)
	if r == 0 && g == 0 && b == 0 {
		t.Errorf("expected peak line at y=%d to be non-black, got RGB(%d,%d,%d)", peakY, r, g, b)
	}
	assertPixelColor(t, surface, 50, peakY, constants.Yellow0, 5, "peak line yellow0")
}

func TestNetBar_NoInterface(t *testing.T) {
	// When no non-lo interface exists, net bar should be red
	const w, h int32 = 100, 100

	renderer, surface, err := createTestRenderer(w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	cfg := defaultTestConfig()
	cfg.ShowCores = false
	cfg.ShowMem = false
	cfg.ShowNet = true

	src := &mockSource{
		data: map[string]*stats.HostStats{
			"host1": {
				CPU: map[string]collector.CPULine{
					"cpu": {User: 100, System: 100, Idle: 800},
				},
				Net: map[string]stats.NetStamp{
					"lo": {B: 0, Tb: 0, Stamp: 1e9}, // only loopback
				},
			},
		},
	}

	state := newRunState(cfg, w, h)
	state.prevCPU["host1;cpu"] = collector.CPULine{}

	drawFrame(renderer, src, cfg, state)

	// Net bar at x=50 (CPU bar=0..49, net bar=50..99), should be red
	assertPixelColor(t, surface, 75, 50, constants.Red, 3, "no-interface red bar")
}

func TestRemainderPixels_AfterToggleMem(t *testing.T) {
	// Reproduces bug: with double-buffering, the back buffer retains stale
	// content from before a layout change. drawFrame must always clear the
	// entire window so remainder pixels (from integer division winW/numBars)
	// don't show old CPU bar fragments.
	//
	// We simulate the stale back-buffer by manually painting the remainder
	// area with a bright color before calling drawFrame, then verifying
	// drawFrame clears it to black.
	const w, h int32 = 200, 100

	renderer, surface, err := createTestRenderer(w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	// 4 hosts, each with cpu + 2 cores = 3 CPU names when showCores=true
	// Plus mem = 4 bars per host → 16 bars total
	// barWidth = 200/16 = 12, drawn = 192, remainder = 8px (x=192..199)
	hosts := map[string]*stats.HostStats{}
	for _, name := range []string{"host1", "host2", "host3", "host4"} {
		_, cur := makeCPUPair(50, 30, 20)
		hosts[name] = &stats.HostStats{
			CPU: map[string]collector.CPULine{
				"cpu":  cur,
				"cpu0": cur,
				"cpu1": cur,
			},
			Mem: map[string]int64{"MemTotal": 1000, "MemFree": 400, "SwapTotal": 0, "SwapFree": 0},
		}
	}
	src := &mockSource{data: hosts}

	cfg := defaultTestConfig()
	cfg.ShowCores = true
	cfg.ShowMem = true
	cfg.ShowNet = false

	state := newRunState(cfg, w, h)
	for _, name := range []string{"host1", "host2", "host3", "host4"} {
		prev, _ := makeCPUPair(50, 30, 20)
		state.prevCPU[name+";cpu"] = prev
		state.prevCPU[name+";cpu0"] = prev
		state.prevCPU[name+";cpu1"] = prev
	}

	// Draw one frame so the layout is established (numBars=16)
	drawFrame(renderer, src, cfg, state)

	// Simulate stale back-buffer content: paint the remainder area bright red.
	// In real double-buffered SDL, this area would contain old wider-bar content
	// from before the toggle. If drawFrame doesn't clear every frame, the
	// remainder keeps this stale color.
	renderer.SetDrawColor(255, 0, 0, 255)
	renderer.FillRect(&sdl.Rect{X: 192, Y: 0, W: 8, H: h})

	// Draw a second frame with the SAME layout (no numBars change).
	// The old code only cleared on layout changes, so this frame would skip
	// the clear and leave the red remainder pixels intact.
	drawFrame(renderer, src, cfg, state)

	// The remainder pixels (x=192..199) must be black, not stale red.
	const tol = 3
	for x := int32(192); x < w; x++ {
		assertPixelColor(t, surface, x, 50, constants.Black, tol,
			fmt.Sprintf("remainder pixel at x=%d must be cleared", x))
	}

	// Sanity: a drawn bar area should still have correct content
	assertPixelColor(t, surface, 185, 95, constants.DarkGrey, 5, "last mem bar has content")
}
