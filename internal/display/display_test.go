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
					"eth0": {B: 12500000, Tb: 6250000, Stamp: 2.0}, // current sample
				},
			},
		},
	}

	state := newRunState(cfg, w, h)
	state.prevCPU["host1;cpu"] = collector.CPULine{}
	// Pre-populate prevNet so delta calculation works:
	// RX: delta=12500000 bytes in 1s = 10% of gbit, TX: 6250000 = 5% of gbit
	state.prevNet["host1"] = stats.NetStamp{B: 0, Tb: 0, Stamp: 1.0}
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

func TestNetBar_AggregatesAllInterfaces(t *testing.T) {
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

	// Two non-lo interfaces: combined RX = 12500000+12500000 = 25000000 → 20% of gbit
	src := &mockSource{
		data: map[string]*stats.HostStats{
			"host1": {
				CPU: map[string]collector.CPULine{
					"cpu": {User: 100, System: 100, Idle: 800},
				},
				Net: map[string]stats.NetStamp{
					"eth0":  {B: 12500000, Tb: 6250000, Stamp: 2.0},
					"wlan0": {B: 12500000, Tb: 6250000, Stamp: 2.0},
					"lo":    {B: 99999999, Tb: 99999999, Stamp: 2.0}, // lo must be excluded
				},
			},
		},
	}

	state := newRunState(cfg, w, h)
	state.prevCPU["host1;cpu"] = collector.CPULine{}
	// Previous aggregated stamp: all zeros at t=1
	state.prevNet["host1"] = stats.NetStamp{B: 0, Tb: 0, Stamp: 1.0}
	// Pre-populate smoothed to 20% RX, 10% TX (the expected combined values)
	state.smoothedNet["host1"] = &struct{ rxPct, txPct float64 }{
		rxPct: 20, txPct: 10,
	}

	drawFrame(renderer, src, cfg, state)

	const tol = 5
	// Net bar: 1 CPU + 1 net = 2 bars, each 50px. Net bar at x=50, halfW=25
	// Left half (RX from top): 20% = 20px of LightGreen from top
	assertPixelColor(t, surface, 60, 5, constants.LightGreen, tol, "aggregated RX at top")
	assertPixelColor(t, surface, 60, 45, constants.Black, tol, "RX free area")

	// Right half (TX from bottom): 10% = 10px of LightGreen from bottom
	assertPixelColor(t, surface, 85, 98, constants.LightGreen, tol, "aggregated TX at bottom")
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
				Net:  map[string]stats.NetStamp{"eth0": {B: 0, Tb: 0, Stamp: 1.0}},
			},
			"beta": {
				CPU: map[string]collector.CPULine{"cpu": betaCur},
				Mem:  map[string]int64{"MemTotal": 100, "MemFree": 50, "SwapTotal": 0, "SwapFree": 0},
				Net:  map[string]stats.NetStamp{"eth0": {B: 0, Tb: 0, Stamp: 1.0}},
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
					"lo": {B: 0, Tb: 0, Stamp: 1.0}, // only loopback
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
	// Tests that bars fill the entire window width (no remainder pixels).
	// With double-buffering, the back buffer retains stale content from
	// before a layout change. drawFrame must overwrite the entire window.
	//
	// We simulate the stale back-buffer by manually painting with a bright
	// color before calling drawFrame, then verifying drawFrame properly
	// overwrites all pixels including the rightmost edge.
	const w, h int32 = 200, 100

	renderer, surface, err := createTestRenderer(w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	// 4 hosts, each with cpu + 2 cores = 3 CPU names when showCores=true
	// Plus mem = 4 bars per host → 16 bars total
	// With remainder distribution: bars alternate between 12 and 13 pixels,
	// filling all 200 pixels. Bar 15 (last mem) spans x=187..199 (width 13).
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

	// Simulate stale back-buffer content: paint rightmost area bright red.
	// In real double-buffered SDL, this area would contain old content from
	// before the toggle. drawFrame must clear/overwrite the entire window.
	renderer.SetDrawColor(255, 0, 0, 255)
	renderer.FillRect(&sdl.Rect{X: 187, Y: 0, W: 13, H: h})

	// Draw a second frame with the SAME layout (no numBars change).
	// This verifies that drawFrame properly overwrites all pixels, including
	// the rightmost bar (which now extends to the window edge).
	drawFrame(renderer, src, cfg, state)

	// Bar 15 (last mem bar) spans x=187..199 (width 13).
	// Left half (RAM): x=187..192 (halfW=6), right half (swap): x=193..199
	// Verify RAM portion has proper content (dark grey), not stale red.
	const tol = 5
	assertPixelColor(t, surface, 190, 95, constants.DarkGrey, tol, "last mem bar RAM at x=190")
	assertPixelColor(t, surface, 192, 95, constants.DarkGrey, tol, "last mem bar RAM at x=192")
	// Rightmost pixel is in swap half; with no swap, it's black (free space)
	assertPixelColor(t, surface, 199, 95, constants.Black, tol, "rightmost pixel (swap half)")
}

// --- Hotkey handler tests ---

// newHotkeyTestEnv creates a test environment with 1 host, 2 CPU cores, memory,
// and 2 net interfaces. Returns all components needed for handleKey + drawFrame
// pixel inspection tests.
func newHotkeyTestEnv(t *testing.T, showCores, showMem, showNet bool) (
	renderer *sdl.Renderer, surface *sdl.Surface,
	cfg *config.Config, state *runState, src *mockSource,
) {
	t.Helper()
	const w, h int32 = 200, 100

	renderer, surface, err := createTestRenderer(w, h)
	if err != nil {
		t.Fatal(err)
	}

	cfg = defaultTestConfig()
	cfg.ShowCores = showCores
	cfg.ShowMem = showMem
	cfg.ShowNet = showNet

	prev, cur := makeCPUPair(50, 30, 20)
	prev0, cur0 := makeCPUPair(60, 20, 20)
	prev1, cur1 := makeCPUPair(40, 40, 20)

	src = &mockSource{
		data: map[string]*stats.HostStats{
			"host1": {
				CPU: map[string]collector.CPULine{
					"cpu":  cur,
					"cpu0": cur0,
					"cpu1": cur1,
				},
				Mem: map[string]int64{
					"MemTotal":  1000,
					"MemFree":   400,
					"SwapTotal": 1000,
					"SwapFree":  600,
				},
				Net: map[string]stats.NetStamp{
					"eth0": {B: 12500000, Tb: 6250000, Stamp: 2.0},
					"wlan0": {B: 1000000, Tb: 500000, Stamp: 2.0},
				},
			},
		},
	}

	state = newRunState(cfg, w, h)
	state.prevCPU["host1;cpu"] = prev
	state.prevCPU["host1;cpu0"] = prev0
	state.prevCPU["host1;cpu1"] = prev1
	state.prevNet["host1"] = stats.NetStamp{B: 0, Tb: 0, Stamp: 1.0}
	state.smoothedNet["host1"] = &struct{ rxPct, txPct float64 }{
		rxPct: 10, txPct: 5,
	}
	state.smoothedMem["host1"] = &struct{ ramUsed, swapUsed float64 }{
		ramUsed: 60, swapUsed: 40,
	}

	return renderer, surface, cfg, state, src
}

func TestHandleKey_Quit(t *testing.T) {
	cfg := defaultTestConfig()
	state := newRunState(cfg, 200, 100)
	if !handleKey(sdl.K_q, nil, cfg, state) {
		t.Error("expected handleKey(q) to return true (quit)")
	}
}

func TestHandleKey_UnknownKey(t *testing.T) {
	cfg := defaultTestConfig()
	state := newRunState(cfg, 200, 100)
	if handleKey(sdl.K_x, nil, cfg, state) {
		t.Error("expected handleKey(x) to return false")
	}
	// State should be unchanged
	if state.showCores != cfg.ShowCores || state.showMem != cfg.ShowMem || state.showNet != cfg.ShowNet {
		t.Error("unknown key should not change state")
	}
}

func TestHandleKey_ToggleCores(t *testing.T) {
	renderer, surface, cfg, state, src := newHotkeyTestEnv(t, false, false, false)
	defer renderer.Destroy()
	defer surface.Free()

	// Before: showCores=false → 1 CPU bar (aggregate)
	drawFrame(renderer, src, cfg, state)
	// The single bar spans full width; check it has color at x=100
	assertPixelColor(t, surface, 100, 95, constants.Blue, 5, "aggregate CPU bar before toggle")

	// Press '1' to toggle cores on
	handleKey(sdl.K_1, nil, cfg, state)
	if !state.showCores {
		t.Fatal("expected showCores=true after pressing 1")
	}

	// After: showCores=true → 3 CPU bars (cpu + cpu0 + cpu1)
	drawFrame(renderer, src, cfg, state)
	// With 3 bars at width 200: barWidth=66, bars at x=0, x=66, x=132
	// Third bar (cpu1) should have color
	assertPixelColor(t, surface, 140, 95, constants.Blue, 5, "cpu1 bar after toggle")
}

func TestHandleKey_ToggleMem(t *testing.T) {
	renderer, surface, cfg, state, src := newHotkeyTestEnv(t, false, false, false)
	defer renderer.Destroy()
	defer surface.Free()

	// Before: no mem bar
	drawFrame(renderer, src, cfg, state)

	// Press '2' to toggle mem on
	handleKey(sdl.K_2, nil, cfg, state)
	if !state.showMem {
		t.Fatal("expected showMem=true after pressing 2")
	}

	// After: CPU bar + mem bar = 2 bars, each 100px wide
	drawFrame(renderer, src, cfg, state)
	// Mem bar starts at x=100, left half is RAM (DarkGrey at bottom for 60% used)
	assertPixelColor(t, surface, 110, 95, constants.DarkGrey, 5, "mem bar RAM after toggle")
}

func TestHandleKey_ToggleMemAlias(t *testing.T) {
	cfg := defaultTestConfig()
	state := newRunState(cfg, 200, 100)
	if state.showMem {
		t.Fatal("expected showMem=false initially")
	}
	// 'm' should toggle mem just like '2'
	handleKey(sdl.K_m, nil, cfg, state)
	if !state.showMem {
		t.Fatal("expected showMem=true after pressing m")
	}
	handleKey(sdl.K_m, nil, cfg, state)
	if state.showMem {
		t.Fatal("expected showMem=false after pressing m again")
	}
}

func TestHandleKey_ToggleNet(t *testing.T) {
	renderer, surface, cfg, state, src := newHotkeyTestEnv(t, false, false, false)
	defer renderer.Destroy()
	defer surface.Free()

	drawFrame(renderer, src, cfg, state)

	// Press '3' to toggle net on
	handleKey(sdl.K_3, nil, cfg, state)
	if !state.showNet {
		t.Fatal("expected showNet=true after pressing 3")
	}

	// After: CPU bar + net bar = 2 bars, each 100px wide
	drawFrame(renderer, src, cfg, state)
	// Net bar starts at x=100, RX (left half from top): LightGreen
	assertPixelColor(t, surface, 110, 2, constants.LightGreen, 5, "net bar RX after toggle")
}

func TestHandleKey_ToggleNetAlias(t *testing.T) {
	cfg := defaultTestConfig()
	state := newRunState(cfg, 200, 100)
	if state.showNet {
		t.Fatal("expected showNet=false initially")
	}
	// 'n' should toggle net just like '3'
	handleKey(sdl.K_n, nil, cfg, state)
	if !state.showNet {
		t.Fatal("expected showNet=true after pressing n")
	}
	handleKey(sdl.K_n, nil, cfg, state)
	if state.showNet {
		t.Fatal("expected showNet=false after pressing n again")
	}
}

func TestHandleKey_ToggleExtended(t *testing.T) {
	renderer, surface, cfg, state, src := newHotkeyTestEnv(t, false, false, false)
	defer renderer.Destroy()
	defer surface.Free()

	// Before: extended=false, no peak line
	if state.extended {
		t.Fatal("expected extended=false initially")
	}

	// Press 'e' to enable extended/peak line
	handleKey(sdl.K_e, nil, cfg, state)
	if !state.extended {
		t.Fatal("expected extended=true after pressing e")
	}

	// After: peak line should appear. Draw two frames so peak history builds up.
	drawFrame(renderer, src, cfg, state)
	drawFrame(renderer, src, cfg, state)
	// CPU is 80% (50 system + 30 user), peak line at y = 100 - 80 = 20
	peakY := int32(100 - 80)
	r, g, b := getPixelColor(surface, 100, peakY)
	// Peak line should be orange (80% > UserOrangeThreshold)
	if r == 0 && g == 0 && b == 0 {
		t.Errorf("expected peak line at y=%d after toggle, got black", peakY)
	}
}

func TestHandleKey_CPUAverage(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.CPUAverage = 5
	state := newRunState(cfg, 200, 100)

	// 'a' increases CPU average
	handleKey(sdl.K_a, nil, cfg, state)
	if cfg.CPUAverage != 6 {
		t.Errorf("expected CPUAverage=6 after 'a', got %d", cfg.CPUAverage)
	}

	// 'y' decreases CPU average
	handleKey(sdl.K_y, nil, cfg, state)
	if cfg.CPUAverage != 5 {
		t.Errorf("expected CPUAverage=5 after 'y', got %d", cfg.CPUAverage)
	}

	// 'y' should clamp at 1
	cfg.CPUAverage = 1
	handleKey(sdl.K_y, nil, cfg, state)
	if cfg.CPUAverage != 1 {
		t.Errorf("expected CPUAverage=1 (clamped), got %d", cfg.CPUAverage)
	}
}

func TestHandleKey_NetAverage(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.NetAverage = 5
	state := newRunState(cfg, 200, 100)

	// 'd' increases net average
	handleKey(sdl.K_d, nil, cfg, state)
	if cfg.NetAverage != 6 {
		t.Errorf("expected NetAverage=6 after 'd', got %d", cfg.NetAverage)
	}

	// 'c' decreases net average
	handleKey(sdl.K_c, nil, cfg, state)
	if cfg.NetAverage != 5 {
		t.Errorf("expected NetAverage=5 after 'c', got %d", cfg.NetAverage)
	}

	// 'c' should clamp at 1
	cfg.NetAverage = 1
	handleKey(sdl.K_c, nil, cfg, state)
	if cfg.NetAverage != 1 {
		t.Errorf("expected NetAverage=1 (clamped), got %d", cfg.NetAverage)
	}
}

func TestHandleKey_WriteConfig(t *testing.T) {
	// Set HOME to a temp dir so we don't touch real ~/.loadbarsrc
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg := defaultTestConfig()
	state := newRunState(cfg, 200, 100)
	// Modify state values that should be copied to config
	state.showAvgLine = true
	state.showCores = true
	state.showMem = true
	state.showNet = true
	state.extended = true

	handleKey(sdl.K_w, nil, cfg, state)

	if !cfg.ShowAvgLine {
		t.Error("expected ShowAvgLine=true in config after 'w'")
	}
	if !cfg.ShowCores {
		t.Error("expected ShowCores=true in config after 'w'")
	}
	if !cfg.ShowMem {
		t.Error("expected ShowMem=true in config after 'w'")
	}
	if !cfg.ShowNet {
		t.Error("expected ShowNet=true in config after 'w'")
	}
	if !cfg.Extended {
		t.Error("expected Extended=true in config after 'w'")
	}
}

func TestHandleKey_LinkScaleUp(t *testing.T) {
	renderer, surface, cfg, state, src := newHotkeyTestEnv(t, false, false, true)
	defer renderer.Destroy()
	defer surface.Free()

	cfg.NetLink = "100mbit"

	// Draw before: net bar with 100mbit scale
	drawFrame(renderer, src, cfg, state)

	// Press 'f' to scale up
	handleKey(sdl.K_f, nil, cfg, state)
	if cfg.NetLink != "gbit" {
		t.Errorf("expected NetLink=gbit after 'f', got %s", cfg.NetLink)
	}

	// Draw after: same traffic, higher link → smaller bars
	drawFrame(renderer, src, cfg, state)

	// At 10gbit, pressing 'f' should clamp
	cfg.NetLink = "10gbit"
	handleKey(sdl.K_f, nil, cfg, state)
	if cfg.NetLink != "10gbit" {
		t.Errorf("expected NetLink=10gbit (clamped), got %s", cfg.NetLink)
	}
}

func TestHandleKey_LinkScaleDown(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.NetLink = "gbit"
	state := newRunState(cfg, 200, 100)

	handleKey(sdl.K_v, nil, cfg, state)
	if cfg.NetLink != "100mbit" {
		t.Errorf("expected NetLink=100mbit after 'v', got %s", cfg.NetLink)
	}

	// At mbit, pressing 'v' should clamp
	cfg.NetLink = "mbit"
	handleKey(sdl.K_v, nil, cfg, state)
	if cfg.NetLink != "mbit" {
		t.Errorf("expected NetLink=mbit (clamped), got %s", cfg.NetLink)
	}
}

func TestHandleKey_ToggleAvgLine(t *testing.T) {
	cfg := defaultTestConfig()
	state := newRunState(cfg, 200, 100)
	if state.showAvgLine {
		t.Fatal("expected showAvgLine=false initially")
	}
	handleKey(sdl.K_g, nil, cfg, state)
	if !state.showAvgLine {
		t.Fatal("expected showAvgLine=true after pressing g")
	}
	handleKey(sdl.K_g, nil, cfg, state)
	if state.showAvgLine {
		t.Fatal("expected showAvgLine=false after pressing g again")
	}
}

func TestGlobalAvgLine_SingleHost(t *testing.T) {
	// One host at 80% CPU → red line at y = 100 - 80 = 20
	const w, h int32 = 100, 100

	renderer, surface, err := createTestRenderer(w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	prev, cur := makeCPUPair(40, 40, 20) // 80% used (40 sys + 40 user)
	cfg := defaultTestConfig()
	cfg.ShowCores = false
	cfg.ShowMem = false
	cfg.ShowNet = false

	src := &mockSource{
		data: map[string]*stats.HostStats{
			"host1": {CPU: map[string]collector.CPULine{"cpu": cur}},
		},
	}

	state := newRunState(cfg, w, h)
	state.showAvgLine = true
	state.prevCPU["host1;cpu"] = prev

	drawFrame(renderer, src, cfg, state)

	// Red line at y=20 (100 - 80%)
	assertPixelColor(t, surface, 50, 20, constants.Red, 3, "avg line at y=20")
	// Check it spans the full width
	assertPixelColor(t, surface, 0, 20, constants.Red, 3, "avg line at x=0")
	assertPixelColor(t, surface, 99, 20, constants.Red, 3, "avg line at x=99")
}

func TestGlobalAvgLine_MultiHost(t *testing.T) {
	// Two hosts: 80% + 40% → average 60% → red line at y = 100 - 60 = 40
	const w, h int32 = 100, 100

	renderer, surface, err := createTestRenderer(w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	prev1, cur1 := makeCPUPair(40, 40, 20) // 80% used
	prev2, cur2 := makeCPUPair(20, 20, 60) // 40% used

	cfg := defaultTestConfig()
	cfg.ShowCores = false
	cfg.ShowMem = false
	cfg.ShowNet = false

	src := &mockSource{
		data: map[string]*stats.HostStats{
			"alpha": {CPU: map[string]collector.CPULine{"cpu": cur1}},
			"beta":  {CPU: map[string]collector.CPULine{"cpu": cur2}},
		},
	}

	state := newRunState(cfg, w, h)
	state.showAvgLine = true
	state.prevCPU["alpha;cpu"] = prev1
	state.prevCPU["beta;cpu"] = prev2

	drawFrame(renderer, src, cfg, state)

	// Average 60% → line at y=40
	assertPixelColor(t, surface, 50, 40, constants.Red, 3, "avg line at y=40")
}

func TestGlobalAvgLine_Disabled(t *testing.T) {
	// With showAvgLine=false, the line position should remain black (background)
	const w, h int32 = 100, 100

	renderer, surface, err := createTestRenderer(w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	prev, cur := makeCPUPair(40, 40, 20) // 80% used
	cfg := defaultTestConfig()
	cfg.ShowCores = false
	cfg.ShowMem = false
	cfg.ShowNet = false

	src := &mockSource{
		data: map[string]*stats.HostStats{
			"host1": {CPU: map[string]collector.CPULine{"cpu": cur}},
		},
	}

	state := newRunState(cfg, w, h)
	state.showAvgLine = false
	state.prevCPU["host1;cpu"] = prev

	drawFrame(renderer, src, cfg, state)

	// At y=20, the CPU bar has user color (yellow), not red
	r, g, b := getPixelColor(surface, 50, 20)
	if r == constants.Red.R && g == constants.Red.G && b == constants.Red.B {
		t.Errorf("expected no red avg line at y=20 when disabled, got RGB(%d,%d,%d)", r, g, b)
	}
}

func TestHandleKey_ArrowResize(t *testing.T) {
	// Arrow keys require a window for SetSize. Create a real dummy SDL window.
	window, err := sdl.CreateWindow("test", 0, 0, 200, 100, sdl.WINDOW_HIDDEN)
	if err != nil {
		t.Skipf("cannot create SDL window in test environment: %v", err)
	}
	defer window.Destroy()

	cfg := defaultTestConfig()
	cfg.MaxWidth = 500
	state := newRunState(cfg, 200, 100)

	// Right arrow → width +100
	handleKey(sdl.K_RIGHT, window, cfg, state)
	if state.winW != 300 {
		t.Errorf("expected winW=300 after right arrow, got %d", state.winW)
	}

	// Left arrow → width -100
	handleKey(sdl.K_LEFT, window, cfg, state)
	if state.winW != 200 {
		t.Errorf("expected winW=200 after left arrow, got %d", state.winW)
	}

	// Left arrow past minimum → clamp at 1
	state.winW = 50
	handleKey(sdl.K_LEFT, window, cfg, state)
	if state.winW != 1 {
		t.Errorf("expected winW=1 (clamped), got %d", state.winW)
	}

	// Right arrow past MaxWidth → clamp at MaxWidth
	state.winW = 450
	handleKey(sdl.K_RIGHT, window, cfg, state)
	if state.winW != 500 {
		t.Errorf("expected winW=500 (clamped at MaxWidth), got %d", state.winW)
	}

	// Up arrow → height -100
	state.winH = 200
	handleKey(sdl.K_UP, window, cfg, state)
	if state.winH != 100 {
		t.Errorf("expected winH=100 after up arrow, got %d", state.winH)
	}

	// Down arrow → height +100
	handleKey(sdl.K_DOWN, window, cfg, state)
	if state.winH != 200 {
		t.Errorf("expected winH=200 after down arrow, got %d", state.winH)
	}

	// Up arrow past minimum → clamp at 1
	state.winH = 50
	handleKey(sdl.K_UP, window, cfg, state)
	if state.winH != 1 {
		t.Errorf("expected winH=1 (clamped), got %d", state.winH)
	}
}

// makeCPUPairWithIO creates a (prev, cur) pair where the delta yields the desired
// system, user, idle, iowait, irq, and softirq percentages.
func makeCPUPairWithIO(systemPct, userPct, idlePct, iowaitPct, irqPct, softirqPct float64) (prev, cur collector.CPULine) {
	const base = 1000
	const delta = 1000
	prev = collector.CPULine{Idle: base}
	dSys := int64(systemPct * float64(delta) / 100)
	dUser := int64(userPct * float64(delta) / 100)
	dIdle := int64(idlePct * float64(delta) / 100)
	dIowait := int64(iowaitPct * float64(delta) / 100)
	dIRQ := int64(irqPct * float64(delta) / 100)
	dSoftIRQ := int64(softirqPct * float64(delta) / 100)
	dNice := delta - dSys - dUser - dIdle - dIowait - dIRQ - dSoftIRQ
	if dNice < 0 {
		dNice = 0
	}
	cur = collector.CPULine{
		System:  prev.System + dSys,
		User:    prev.User + dUser,
		Idle:    prev.Idle + dIdle,
		Nice:    prev.Nice + dNice,
		Iowait:  prev.Iowait + dIowait,
		IRQ:     prev.IRQ + dIRQ,
		SoftIRQ: prev.SoftIRQ + dSoftIRQ,
	}
	return prev, cur
}

func TestHandleKey_ToggleIOAvgLine(t *testing.T) {
	cfg := defaultTestConfig()
	state := newRunState(cfg, 200, 100)
	if state.showIOAvgLine {
		t.Fatal("expected showIOAvgLine=false initially")
	}
	handleKey(sdl.K_i, nil, cfg, state)
	if !state.showIOAvgLine {
		t.Fatal("expected showIOAvgLine=true after pressing i")
	}
	handleKey(sdl.K_i, nil, cfg, state)
	if state.showIOAvgLine {
		t.Fatal("expected showIOAvgLine=false after pressing i again")
	}
}

func TestGlobalIOAvgLine_SingleHost(t *testing.T) {
	// One host with 20% iowait + 5% irq + 5% softirq = 30% → pink line at y=30 from top
	const w, h int32 = 100, 100

	renderer, surface, err := createTestRenderer(w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	prev, cur := makeCPUPairWithIO(10, 10, 20, 20, 5, 5)
	cfg := defaultTestConfig()

	src := &mockSource{
		data: map[string]*stats.HostStats{
			"host1": {CPU: map[string]collector.CPULine{"cpu": cur}},
		},
	}

	state := newRunState(cfg, w, h)
	state.showIOAvgLine = true
	state.prevCPU["host1;cpu"] = prev

	drawFrame(renderer, src, cfg, state)

	// Pink line at y=30 (30% from top in a 100px window)
	assertPixelColor(t, surface, 50, 30, constants.Pink, 3, "IO avg line at y=30")
	// Spans full width
	assertPixelColor(t, surface, 0, 30, constants.Pink, 3, "IO avg line at x=0")
	assertPixelColor(t, surface, 99, 30, constants.Pink, 3, "IO avg line at x=99")
}

func TestGlobalIOAvgLine_MultiHost(t *testing.T) {
	// Two hosts: host1=30% IO, host2=0% IO → average 15% → pink line at y=15
	const w, h int32 = 100, 100

	renderer, surface, err := createTestRenderer(w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	prev1, cur1 := makeCPUPairWithIO(10, 10, 20, 20, 5, 5) // 30% IO
	prev2, cur2 := makeCPUPair(40, 40, 20)                   // 0% IO

	cfg := defaultTestConfig()

	src := &mockSource{
		data: map[string]*stats.HostStats{
			"alpha": {CPU: map[string]collector.CPULine{"cpu": cur1}},
			"beta":  {CPU: map[string]collector.CPULine{"cpu": cur2}},
		},
	}

	state := newRunState(cfg, w, h)
	state.showIOAvgLine = true
	state.prevCPU["alpha;cpu"] = prev1
	state.prevCPU["beta;cpu"] = prev2

	drawFrame(renderer, src, cfg, state)

	// Average 15% → pink line at y=15
	assertPixelColor(t, surface, 50, 15, constants.Pink, 3, "IO avg line at y=15")
}

func TestGlobalIOAvgLine_Disabled(t *testing.T) {
	// With showIOAvgLine=false, no pink line should appear
	const w, h int32 = 100, 100

	renderer, surface, err := createTestRenderer(w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	prev, cur := makeCPUPairWithIO(10, 10, 20, 20, 5, 5) // 30% IO
	cfg := defaultTestConfig()

	src := &mockSource{
		data: map[string]*stats.HostStats{
			"host1": {CPU: map[string]collector.CPULine{"cpu": cur}},
		},
	}

	state := newRunState(cfg, w, h)
	state.showIOAvgLine = false
	state.prevCPU["host1;cpu"] = prev

	drawFrame(renderer, src, cfg, state)

	// At y=30, there should be no pink line
	r, g, b := getPixelColor(surface, 50, 30)
	if r == constants.Pink.R && g == constants.Pink.G && b == constants.Pink.B {
		t.Errorf("expected no pink IO avg line at y=30 when disabled, got RGB(%d,%d,%d)", r, g, b)
	}
}

func TestHandleKey_WriteConfig_IOAvgLine(t *testing.T) {
	// Verify that 'w' hotkey persists showIOAvgLine to config
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg := defaultTestConfig()
	state := newRunState(cfg, 200, 100)
	state.showIOAvgLine = true

	handleKey(sdl.K_w, nil, cfg, state)

	if !cfg.ShowIOAvgLine {
		t.Error("expected ShowIOAvgLine=true in config after 'w'")
	}
}

func TestHandleKey_ToggleSeparators(t *testing.T) {
	cfg := defaultTestConfig()
	state := newRunState(cfg, 200, 100)
	if state.showSeparators {
		t.Fatal("expected showSeparators=false initially")
	}
	handleKey(sdl.K_s, nil, cfg, state)
	if !state.showSeparators {
		t.Fatal("expected showSeparators=true after pressing s")
	}
	handleKey(sdl.K_s, nil, cfg, state)
	if state.showSeparators {
		t.Fatal("expected showSeparators=false after pressing s again")
	}
}

func TestSeparator_TwoHosts_Enabled(t *testing.T) {
	// Two hosts (100% system = blue) with separators enabled: yellow pixel at boundary
	const w, h int32 = 200, 100

	renderer, surface, err := createTestRenderer(w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	prev1, cur1 := makeCPUPair(100, 0, 0) // all system → blue
	prev2, cur2 := makeCPUPair(100, 0, 0)

	cfg := defaultTestConfig()
	cfg.ShowCores = false
	cfg.ShowMem = false
	cfg.ShowNet = false
	cfg.ShowSeparators = true

	src := &mockSource{
		data: map[string]*stats.HostStats{
			"alpha": {CPU: map[string]collector.CPULine{"cpu": cur1}},
			"beta":  {CPU: map[string]collector.CPULine{"cpu": cur2}},
		},
	}

	state := newRunState(cfg, w, h)
	state.prevCPU["alpha;cpu"] = prev1
	state.prevCPU["beta;cpu"] = prev2

	drawFrame(renderer, src, cfg, state)

	// 2 bars at 200px → each 100px. Separator at x=100 (start of second host's bars)
	assertPixelColor(t, surface, 100, 50, constants.Red, 3, "separator red at x=100")
}

func TestSeparator_TwoHosts_Disabled(t *testing.T) {
	// Two hosts (100% system = blue) with separators disabled: no yellow at boundary
	const w, h int32 = 200, 100

	renderer, surface, err := createTestRenderer(w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	prev1, cur1 := makeCPUPair(100, 0, 0) // all system → blue
	prev2, cur2 := makeCPUPair(100, 0, 0)

	cfg := defaultTestConfig()
	cfg.ShowCores = false
	cfg.ShowMem = false
	cfg.ShowNet = false
	cfg.ShowSeparators = false

	src := &mockSource{
		data: map[string]*stats.HostStats{
			"alpha": {CPU: map[string]collector.CPULine{"cpu": cur1}},
			"beta":  {CPU: map[string]collector.CPULine{"cpu": cur2}},
		},
	}

	state := newRunState(cfg, w, h)
	state.prevCPU["alpha;cpu"] = prev1
	state.prevCPU["beta;cpu"] = prev2

	drawFrame(renderer, src, cfg, state)

	// At x=100, should be blue (second host's system bar), NOT red separator
	assertPixelColor(t, surface, 100, 50, constants.Blue, 3, "no separator, should be blue")
}

func TestSeparator_SingleHost(t *testing.T) {
	// Single host: no separator should be drawn even when enabled
	const w, h int32 = 100, 100

	renderer, surface, err := createTestRenderer(w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	prev, cur := makeCPUPair(50, 30, 20)
	cfg := defaultTestConfig()
	cfg.ShowCores = false
	cfg.ShowMem = false
	cfg.ShowNet = false
	cfg.ShowSeparators = true

	src := &mockSource{
		data: map[string]*stats.HostStats{
			"host1": {CPU: map[string]collector.CPULine{"cpu": cur}},
		},
	}

	state := newRunState(cfg, w, h)
	state.prevCPU["host1;cpu"] = prev

	drawFrame(renderer, src, cfg, state)

	// No separator at the edges — just verify no red separator at x=0 or x=99
	r, g, b := getPixelColor(surface, 0, 50)
	if r == constants.Red.R && g == constants.Red.G && b == constants.Red.B {
		t.Errorf("unexpected red separator at x=0 with single host")
	}
	r, g, b = getPixelColor(surface, 99, 50)
	if r == constants.Red.R && g == constants.Red.G && b == constants.Red.B {
		t.Errorf("unexpected red separator at x=99 with single host")
	}
}

// --- barRect tests ---

func TestBarRect_Unlimited(t *testing.T) {
	// maxPerRow=0 means unlimited → single row, same as barBounds with y=0, h=winH
	const winW, winH int32 = 600, 200
	numBars := 6
	for i := 0; i < numBars; i++ {
		x, y, w, h := barRect(winW, winH, numBars, 0, i)
		bx, bw := barBounds(winW, numBars, i)
		if x != bx || w != bw {
			t.Errorf("bar %d: barRect x/w = (%d,%d), barBounds = (%d,%d)", i, x, w, bx, bw)
		}
		if y != 0 || h != winH {
			t.Errorf("bar %d: expected y=0, h=%d; got y=%d, h=%d", i, winH, y, h)
		}
	}
}

func TestBarRect_MultiRow(t *testing.T) {
	// 6 bars, maxPerRow=4 → 2 rows: row 0 has 4 bars, row 1 has 2 bars
	const winW, winH int32 = 400, 200
	numBars, maxPerRow := 6, 4

	// Row 0: bars 0-3, each 100px wide, y=0, h=100
	for i := 0; i < 4; i++ {
		x, y, w, h := barRect(winW, winH, numBars, maxPerRow, i)
		if y != 0 || h != 100 {
			t.Errorf("bar %d: expected y=0 h=100, got y=%d h=%d", i, y, h)
		}
		expectedX := int32(i) * 100
		if x != expectedX || w != 100 {
			t.Errorf("bar %d: expected x=%d w=100, got x=%d w=%d", i, expectedX, x, w)
		}
	}

	// Row 1: bars 4-5, each 200px wide (2 bars fill 400px), y=100, h=100
	for i := 4; i < 6; i++ {
		x, y, w, h := barRect(winW, winH, numBars, maxPerRow, i)
		if y != 100 || h != 100 {
			t.Errorf("bar %d: expected y=100 h=100, got y=%d h=%d", i, y, h)
		}
		col := i - 4
		expectedX := int32(col) * 200
		if x != expectedX || w != 200 {
			t.Errorf("bar %d: expected x=%d w=200, got x=%d w=%d", i, expectedX, x, w)
		}
	}
}

func TestBarRect_LastRowWider(t *testing.T) {
	// 5 bars, maxPerRow=3 → 2 rows: row 0 has 3 bars (133px), row 1 has 2 bars (200px)
	const winW, winH int32 = 400, 100
	numBars, maxPerRow := 5, 3

	// Last row bars should be wider since fewer bars fill the full width
	_, _, w0, _ := barRect(winW, winH, numBars, maxPerRow, 0)
	_, _, w3, _ := barRect(winW, winH, numBars, maxPerRow, 3)
	if w3 <= w0 {
		t.Errorf("last row bars should be wider: row0 bar w=%d, row1 bar w=%d", w0, w3)
	}
}

func TestBarRect_SingleBar(t *testing.T) {
	// Edge case: 1 bar fills the entire window
	const winW, winH int32 = 300, 150
	x, y, w, h := barRect(winW, winH, 1, 0, 0)
	if x != 0 || y != 0 || w != winW || h != winH {
		t.Errorf("expected (0,0,%d,%d), got (%d,%d,%d,%d)", winW, winH, x, y, w, h)
	}
	// Also with maxPerRow=1
	x, y, w, h = barRect(winW, winH, 1, 1, 0)
	if x != 0 || y != 0 || w != winW || h != winH {
		t.Errorf("maxPerRow=1: expected (0,0,%d,%d), got (%d,%d,%d,%d)", winW, winH, x, y, w, h)
	}
}

func TestBarRect_MaxPerRowOne(t *testing.T) {
	// maxPerRow=1: each bar gets its own row, full width
	const winW, winH int32 = 200, 300
	numBars := 3
	for i := 0; i < numBars; i++ {
		x, y, w, h := barRect(winW, winH, numBars, 1, i)
		if x != 0 || w != winW {
			t.Errorf("bar %d: expected x=0 w=%d, got x=%d w=%d", i, winW, x, w)
		}
		expectedY := winH * int32(i) / 3
		expectedH := winH*int32(i+1)/3 - expectedY
		if y != expectedY || h != expectedH {
			t.Errorf("bar %d: expected y=%d h=%d, got y=%d h=%d", i, expectedY, expectedH, y, h)
		}
	}
}

func TestBarRect_NegativeMaxPerRow(t *testing.T) {
	// Negative maxPerRow treated as unlimited (same as 0)
	const winW, winH int32 = 400, 100
	numBars := 4
	for i := 0; i < numBars; i++ {
		x, y, w, h := barRect(winW, winH, numBars, -1, i)
		bx, bw := barBounds(winW, numBars, i)
		if x != bx || w != bw || y != 0 || h != winH {
			t.Errorf("bar %d: negative maxPerRow should act as unlimited", i)
		}
	}
}

func TestBarRect_MaxPerRowExceedsNumBars(t *testing.T) {
	// maxPerRow >= numBars → single row
	const winW, winH int32 = 400, 100
	numBars := 3
	for i := 0; i < numBars; i++ {
		x, y, w, h := barRect(winW, winH, numBars, 10, i)
		bx, bw := barBounds(winW, numBars, i)
		if x != bx || w != bw || y != 0 || h != winH {
			t.Errorf("bar %d: maxPerRow >= numBars should act as single row", i)
		}
	}
}

func TestMultiRow_DrawFrame(t *testing.T) {
	// Verify that multi-row layout draws bars in correct positions
	const w, h int32 = 200, 200

	renderer, surface, err := createTestRenderer(w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	// 4 hosts × 1 CPU bar = 4 bars, maxPerRow=2 → 2 rows of 2 bars
	cfg := defaultTestConfig()
	cfg.MaxBarsPerRow = 2

	prev1, cur1 := makeCPUPair(100, 0, 0) // all system → blue
	prev2, cur2 := makeCPUPair(0, 100, 0)  // all user → yellow
	prev3, cur3 := makeCPUPair(0, 0, 100)  // all idle → black
	prev4, cur4 := makeCPUPair(100, 0, 0)  // all system → blue

	src := &mockSource{
		data: map[string]*stats.HostStats{
			"alpha": {CPU: map[string]collector.CPULine{"cpu": cur1}},
			"beta":  {CPU: map[string]collector.CPULine{"cpu": cur2}},
			"gamma": {CPU: map[string]collector.CPULine{"cpu": cur3}},
			"delta": {CPU: map[string]collector.CPULine{"cpu": cur4}},
		},
	}

	state := newRunState(cfg, w, h)
	state.prevCPU["alpha;cpu"] = prev1
	state.prevCPU["beta;cpu"] = prev2
	state.prevCPU["gamma;cpu"] = prev3
	state.prevCPU["delta;cpu"] = prev4

	drawFrame(renderer, src, cfg, state)

	const tol = 5
	// Row 0 (y=0..99): 2 bars each 100px wide
	// Bar 0 (alpha, blue) at x=50, y=90
	assertPixelColor(t, surface, 50, 90, constants.Blue, tol, "row0 bar0 blue")
	// Bar 1 (beta, yellow) at x=150, y=90
	assertPixelColor(t, surface, 150, 90, constants.Yellow, tol, "row0 bar1 yellow")

	// Row 1 (y=100..199): 2 bars each 100px wide
	// Hosts sorted alphabetically: alpha, beta, delta, gamma
	// Bar 2 (delta, blue) at x=50, y=190
	assertPixelColor(t, surface, 50, 190, constants.Blue, tol, "row1 bar2 blue")
	// Bar 3 (gamma, idle/black) at x=150, y=150
	assertPixelColor(t, surface, 150, 150, constants.Black, tol, "row1 bar3 black")
}

func TestHandleKey_WriteConfig_Separators(t *testing.T) {
	// Verify that 'w' hotkey persists showSeparators to config
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg := defaultTestConfig()
	state := newRunState(cfg, 200, 100)
	state.showSeparators = true

	handleKey(sdl.K_w, nil, cfg, state)

	if !cfg.ShowSeparators {
		t.Error("expected ShowSeparators=true in config after 'w'")
	}
}
