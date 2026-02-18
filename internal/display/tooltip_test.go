package display

import (
	"testing"
	"time"

	"codeberg.org/snonux/loadbars/internal/collector"
	"codeberg.org/snonux/loadbars/internal/constants"
	"codeberg.org/snonux/loadbars/internal/stats"
	"github.com/veandco/go-sdl2/sdl"
)

// --- font tests ---

func TestStringWidth(t *testing.T) {
	// Single character: 5 pixels wide at scale 1
	if w := stringWidth("A", 1); w != 5 {
		t.Errorf("stringWidth(A, 1) = %d, want 5", w)
	}
	// Two characters at scale 1: 5 + gap(1) + 5 = 11, minus trailing gap = 11
	if w := stringWidth("AB", 1); w != 11 {
		t.Errorf("stringWidth(AB, 1) = %d, want 11", w)
	}
	// Scale 2: single char = 5*2 = 10
	if w := stringWidth("A", 2); w != 10 {
		t.Errorf("stringWidth(A, 2) = %d, want 10", w)
	}
	// Empty string
	if w := stringWidth("", 1); w != 0 {
		t.Errorf("stringWidth('', 1) = %d, want 0", w)
	}
}

func TestDrawChar_RendersPixels(t *testing.T) {
	// Draw the letter 'I' which has a recognizable pattern (center column lit)
	renderer, surface, err := createTestRenderer(20, 20)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	renderer.SetDrawColor(0, 0, 0, 255)
	renderer.Clear()
	renderer.SetDrawColor(255, 255, 255, 255)
	drawChar(renderer, 'I', 0, 0, 1)
	renderer.Present()

	// 'I' glyph row 0 = 0x70 = 01110000 → pixels at columns 1,2,3 should be lit
	r, g, b := getPixelColor(surface, 1, 0)
	if r < 200 || g < 200 || b < 200 {
		t.Errorf("expected white pixel at (1,0) for 'I' glyph, got RGB(%d,%d,%d)", r, g, b)
	}
	// Column 0 should be black (not part of 'I' top row)
	r, g, b = getPixelColor(surface, 0, 0)
	if r > 50 || g > 50 || b > 50 {
		t.Errorf("expected black pixel at (0,0) for 'I' glyph, got RGB(%d,%d,%d)", r, g, b)
	}
}

func TestDrawString_MultipleChars(t *testing.T) {
	renderer, surface, err := createTestRenderer(40, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	renderer.SetDrawColor(0, 0, 0, 255)
	renderer.Clear()
	renderer.SetDrawColor(255, 255, 255, 255)
	totalW := drawString(renderer, "Hi", 0, 0, 1)
	renderer.Present()

	// "Hi" at scale 1: 2 chars × (5+1) = 12 pixels total advance
	if totalW != 12 {
		t.Errorf("drawString returned width %d, want 12", totalW)
	}
	// Second character starts at x=6, verify it has some lit pixels
	// 'i' glyph row 0 = 0x20 = col 2 → pixel at x=6+2=8
	r, _, _ := getPixelColor(surface, 8, 0)
	if r < 200 {
		t.Errorf("expected lit pixel from second char at x=8, got R=%d", r)
	}
}

// --- hit test tests ---

func TestBuildBarMap_SingleHost(t *testing.T) {
	snap := map[string]*stats.HostStats{
		"host1": {
			CPU: map[string]collector.CPULine{"cpu": {}},
		},
	}
	cfg := defaultTestConfig()
	state := newRunState(cfg, 200, 100)

	bars := buildBarMap(snap, cfg, state)
	if len(bars) != 1 {
		t.Fatalf("expected 1 bar, got %d", len(bars))
	}
	if bars[0].host != "host1" || bars[0].kind != barCPU || bars[0].cpuName != "cpu" {
		t.Errorf("unexpected bar: %+v", bars[0])
	}
	// Single bar should fill the whole window
	if bars[0].rect.W != 200 || bars[0].rect.H != 100 {
		t.Errorf("expected bar to fill window (200x100), got %dx%d", bars[0].rect.W, bars[0].rect.H)
	}
}

func TestBuildBarMap_WithMemAndNet(t *testing.T) {
	snap := map[string]*stats.HostStats{
		"alpha": {
			CPU: map[string]collector.CPULine{"cpu": {}},
			Mem: map[string]int64{"MemTotal": 1024},
			Net: map[string]stats.NetStamp{"eth0": {}},
		},
	}
	cfg := defaultTestConfig()
	state := newRunState(cfg, 300, 100)
	state.showMem = true
	state.showNet = true

	bars := buildBarMap(snap, cfg, state)
	if len(bars) != 3 {
		t.Fatalf("expected 3 bars (cpu+mem+net), got %d", len(bars))
	}
	if bars[0].kind != barCPU {
		t.Errorf("bar 0 should be CPU, got %d", bars[0].kind)
	}
	if bars[1].kind != barMem {
		t.Errorf("bar 1 should be Mem, got %d", bars[1].kind)
	}
	if bars[2].kind != barNet {
		t.Errorf("bar 2 should be Net, got %d", bars[2].kind)
	}
}

func TestBuildBarMap_MultiHost(t *testing.T) {
	snap := map[string]*stats.HostStats{
		"alpha": {CPU: map[string]collector.CPULine{"cpu": {}}},
		"beta":  {CPU: map[string]collector.CPULine{"cpu": {}}},
	}
	cfg := defaultTestConfig()
	state := newRunState(cfg, 200, 100)

	bars := buildBarMap(snap, cfg, state)
	if len(bars) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(bars))
	}
	// Sorted order: alpha first
	if bars[0].host != "alpha" || bars[1].host != "beta" {
		t.Errorf("expected alpha then beta, got %s then %s", bars[0].host, bars[1].host)
	}
}

func TestBuildBarMap_WithCores(t *testing.T) {
	snap := map[string]*stats.HostStats{
		"host1": {
			CPU: map[string]collector.CPULine{
				"cpu":  {},
				"cpu0": {},
				"cpu1": {},
			},
		},
	}
	cfg := defaultTestConfig()
	state := newRunState(cfg, 300, 100)
	state.cpuMode = constants.CPUModeCores

	bars := buildBarMap(snap, cfg, state)
	if len(bars) != 3 {
		t.Fatalf("expected 3 bars (cpu + cpu0 + cpu1), got %d", len(bars))
	}
	if bars[0].cpuName != "cpu" {
		t.Errorf("first CPU bar should be aggregate 'cpu', got %s", bars[0].cpuName)
	}
}

func TestHitTest_Hit(t *testing.T) {
	bars := []barDescriptor{
		{host: "h1", kind: barCPU, rect: sdl.Rect{X: 0, Y: 0, W: 100, H: 100}},
		{host: "h2", kind: barCPU, rect: sdl.Rect{X: 100, Y: 0, W: 100, H: 100}},
	}
	hit := hitTest(bars, 50, 50)
	if hit == nil || hit.host != "h1" {
		t.Errorf("expected hit on h1 at (50,50), got %v", hit)
	}
	hit = hitTest(bars, 150, 50)
	if hit == nil || hit.host != "h2" {
		t.Errorf("expected hit on h2 at (150,50), got %v", hit)
	}
}

func TestHitTest_Miss(t *testing.T) {
	bars := []barDescriptor{
		{host: "h1", kind: barCPU, rect: sdl.Rect{X: 0, Y: 0, W: 100, H: 100}},
	}
	hit := hitTest(bars, -1, -1)
	if hit != nil {
		t.Errorf("expected nil for off-screen coords, got %v", hit)
	}
	hit = hitTest(bars, 200, 50)
	if hit != nil {
		t.Errorf("expected nil for point outside bars, got %v", hit)
	}
}

func TestHitTest_EdgeCases(t *testing.T) {
	bars := []barDescriptor{
		{host: "h1", kind: barCPU, rect: sdl.Rect{X: 10, Y: 10, W: 50, H: 50}},
	}
	// Top-left corner (inclusive)
	hit := hitTest(bars, 10, 10)
	if hit == nil {
		t.Error("expected hit at top-left corner (10,10)")
	}
	// Bottom-right edge (exclusive)
	hit = hitTest(bars, 60, 60)
	if hit != nil {
		t.Error("expected miss at bottom-right edge (60,60)")
	}
}

// --- tooltip content tests ---

func TestTooltipLines_CPU(t *testing.T) {
	snap := map[string]*stats.HostStats{
		"myhost": {CPU: map[string]collector.CPULine{"cpu": {}}},
	}
	cfg := defaultTestConfig()
	state := newRunState(cfg, 200, 100)
	// Pre-populate smoothed data
	state.smoothedCPU["myhost;cpu"] = &[10]float64{10.0, 20.0, 5.0, 60.0, 3.0, 0, 0, 0, 2.0, 0}

	bar := &barDescriptor{host: "myhost", kind: barCPU, cpuName: "cpu"}
	lines := tooltipLines(bar, snap, cfg, state)

	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}
	if lines[0] != "myhost [cpu]" {
		t.Errorf("first line = %q, want %q", lines[0], "myhost [cpu]")
	}
	// Check that sys/usr/idle lines are present
	found := map[string]bool{}
	for _, l := range lines {
		if len(l) >= 3 {
			found[l[:3]] = true
		}
	}
	for _, prefix := range []string{"Sys", "Usr", "Nic", "IO:", "Ste", "Idl"} {
		if !found[prefix] {
			t.Errorf("missing line with prefix %q in tooltip", prefix)
		}
	}
}

func TestTooltipLines_Mem(t *testing.T) {
	snap := map[string]*stats.HostStats{
		"myhost": {
			CPU: map[string]collector.CPULine{"cpu": {}},
			Mem: map[string]int64{
				"MemTotal":  8 * 1024 * 1024, // 8 GB in KB
				"MemFree":   2 * 1024 * 1024,
				"SwapTotal": 4 * 1024 * 1024,
				"SwapFree":  3 * 1024 * 1024,
			},
		},
	}
	cfg := defaultTestConfig()
	state := newRunState(cfg, 200, 100)
	state.smoothedMem["myhost"] = &struct{ ramUsed, swapUsed float64 }{75.0, 25.0}

	bar := &barDescriptor{host: "myhost", kind: barMem}
	lines := tooltipLines(bar, snap, cfg, state)

	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d", len(lines))
	}
	if lines[0] != "myhost [mem]" {
		t.Errorf("first line = %q, want %q", lines[0], "myhost [mem]")
	}
}

func TestTooltipLines_Net(t *testing.T) {
	snap := map[string]*stats.HostStats{
		"myhost": {CPU: map[string]collector.CPULine{"cpu": {}}},
	}
	cfg := defaultTestConfig()
	cfg.NetLink = "gbit"
	state := newRunState(cfg, 200, 100)
	state.smoothedNet["myhost"] = &struct{ rxPct, txPct float64 }{12.5, 3.2}

	bar := &barDescriptor{host: "myhost", kind: barNet}
	lines := tooltipLines(bar, snap, cfg, state)

	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d", len(lines))
	}
	if lines[0] != "myhost [net]" {
		t.Errorf("first line = %q, want %q", lines[0], "myhost [net]")
	}
}

func TestTooltipLines_NoData(t *testing.T) {
	snap := map[string]*stats.HostStats{
		"myhost": {CPU: map[string]collector.CPULine{"cpu": {}}},
	}
	cfg := defaultTestConfig()
	state := newRunState(cfg, 200, 100)
	// Don't populate smoothedCPU → should get "No data yet"

	bar := &barDescriptor{host: "myhost", kind: barCPU, cpuName: "cpu"}
	lines := tooltipLines(bar, snap, cfg, state)

	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}
	if lines[1] != "No data yet" {
		t.Errorf("expected 'No data yet', got %q", lines[1])
	}
}

// --- formatKB tests ---

func TestFormatKB(t *testing.T) {
	tests := []struct {
		kb   int64
		want string
	}{
		{500, "500K"},
		{2048, "2.0M"},
		{1024 * 1024, "1.0G"},
		{8 * 1024 * 1024, "8.0G"},
	}
	for _, tc := range tests {
		got := formatKB(tc.kb)
		if got != tc.want {
			t.Errorf("formatKB(%d) = %q, want %q", tc.kb, got, tc.want)
		}
	}
}

// --- drawTooltip rendering test ---

func TestDrawTooltip_RendersBox(t *testing.T) {
	renderer, surface, err := createTestRenderer(200, 200)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	renderer.SetDrawColor(0, 0, 0, 255)
	renderer.Clear()

	lines := []string{"Host: test", "CPU: 50%"}
	drawTooltip(renderer, lines, 10, 10, 200, 200)
	renderer.Present()

	// The tooltip background (#181818) should be visible near cursor + offset
	bx := int32(10 + tooltipOffsetX + 1)
	by := int32(10 + tooltipOffsetY + 1)
	r, g, b := getPixelColor(surface, bx, by)
	// Should be near #181818 (dark grey background) or text color
	if r > 0x30 && g > 0x30 && b > 0x30 {
		// If it's bright, it might be text — that's also fine
	}
	// Just verify it's not pure black (meaning something was drawn)
	if r == 0 && g == 0 && b == 0 {
		t.Errorf("expected tooltip content at (%d,%d), but pixel is pure black", bx, by)
	}
}

func TestDrawTooltip_ClampsToWindow(t *testing.T) {
	renderer, surface, err := createTestRenderer(100, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	renderer.SetDrawColor(0, 0, 0, 255)
	renderer.Clear()

	// Place cursor near bottom-right corner; tooltip should flip to stay in bounds
	lines := []string{"Very long text line here"}
	drawTooltip(renderer, lines, 90, 90, 100, 100)
	renderer.Present()

	// Tooltip should be positioned to the left/above cursor
	// Check that something was drawn in the upper area (not just bottom-right)
	foundDrawn := false
	for y := int32(0); y < 80; y += 10 {
		for x := int32(0); x < 80; x += 10 {
			r, g, b := getPixelColor(surface, x, y)
			if r != 0 || g != 0 || b != 0 {
				foundDrawn = true
				break
			}
		}
		if foundDrawn {
			break
		}
	}
	if !foundDrawn {
		t.Error("expected tooltip to be clamped and visible in upper area, but found nothing drawn")
	}
}

// --- invertHostBars test ---

func TestInvertHostBars_InvertsCorrectHost(t *testing.T) {
	renderer, surface, err := createTestRenderer(200, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	// Draw a blue bar for host1 (left half) and a green bar for host2 (right half)
	renderer.SetDrawColor(0, 0, 200, 255)
	renderer.FillRect(&sdl.Rect{X: 0, Y: 0, W: 100, H: 100})
	renderer.SetDrawColor(0, 200, 0, 255)
	renderer.FillRect(&sdl.Rect{X: 100, Y: 0, W: 100, H: 100})

	bars := []barDescriptor{
		{host: "host1", kind: barCPU, rect: sdl.Rect{X: 0, Y: 0, W: 100, H: 100}},
		{host: "host2", kind: barCPU, rect: sdl.Rect{X: 100, Y: 0, W: 100, H: 100}},
	}

	// Invert host1 only
	invertHostBars(renderer, bars, "host1")
	renderer.Present()

	// host1 area: blue (0,0,200) should become inverted (255, 255, 55)
	r1, g1, b1 := getPixelColor(surface, 50, 50)
	// host2 area: green (0,200,0) should remain unchanged
	r2, g2, b2 := getPixelColor(surface, 150, 50)

	// Host1 blue was inverted: R should jump from 0 to ~255
	if r1 < 200 {
		t.Errorf("host1 after inversion: expected R>200 (inverted blue), got R=%d (full: %d,%d,%d)", r1, r1, g1, b1)
	}
	// Host2 green should stay green (R near 0)
	if r2 > 50 {
		t.Errorf("host2 should not be inverted: expected R<50, got R=%d (full: %d,%d,%d)", r2, r2, g2, b2)
	}
	_ = g1
	_ = b1
	_ = g2
	_ = b2
}

// --- drawOverlay integration test ---

func TestDrawOverlay_NoTooltipWhenMouseOffScreen(t *testing.T) {
	renderer, surface, err := createTestRenderer(100, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	snap := map[string]*stats.HostStats{
		"host1": {CPU: map[string]collector.CPULine{"cpu": {}}},
	}
	cfg := defaultTestConfig()
	state := newRunState(cfg, 100, 100)
	// mouseX/mouseY default to -1 (off-screen)

	// Draw a solid blue background to detect any overlay changes
	renderer.SetDrawColor(0, 0, constants.Blue.B, 255)
	renderer.FillRect(&sdl.Rect{X: 0, Y: 0, W: 100, H: 100})

	drawOverlay(renderer, snap, cfg, state)
	renderer.Present()

	// Center pixel should still be blue (no inversion or tooltip drawn)
	r, g, b := getPixelColor(surface, 50, 50)
	if r > 10 || g > 10 {
		t.Errorf("expected blue pixel (no overlay) at (50,50), got RGB(%d,%d,%d)", r, g, b)
	}
}

func TestDrawOverlay_TooltipWhenMouseOnBar(t *testing.T) {
	renderer, surface, err := createTestRenderer(200, 200)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	snap := map[string]*stats.HostStats{
		"host1": {CPU: map[string]collector.CPULine{"cpu": {}}},
	}
	cfg := defaultTestConfig()
	state := newRunState(cfg, 200, 200)
	state.mouseX = 50
	state.mouseY = 50
	state.mouseLastMove = time.Now() // simulate recent mouse activity
	state.smoothedCPU["host1;cpu"] = &[10]float64{10, 20, 5, 60, 3, 0, 0, 0, 2, 0}

	// Draw a solid blue background
	renderer.SetDrawColor(0, 0, 200, 255)
	renderer.FillRect(&sdl.Rect{X: 0, Y: 0, W: 200, H: 200})

	drawOverlay(renderer, snap, cfg, state)
	renderer.Present()

	// Tooltip area should have tooltip background or text drawn.
	// Note: custom blend modes (used for host inversion) may not work with
	// software renderers, so we only verify the tooltip box was rendered.
	tx := int32(50 + tooltipOffsetX + 2)
	ty := int32(50 + tooltipOffsetY + 2)
	if tx < 200 && ty < 200 {
		tr, tg, tb := getPixelColor(surface, tx, ty)
		// Should be tooltip background ~(0x18,0x18,0x18) or text color, not the original blue
		isOriginalBlue := tr < 10 && tg < 10 && tb > 150
		if isOriginalBlue {
			t.Errorf("tooltip pixel at (%d,%d): still original blue RGB(%d,%d,%d), expected tooltip content", tx, ty, tr, tg, tb)
		}
	}
}

// --- mouse idle timeout test ---

func TestDrawOverlay_HiddenAfterIdleTimeout(t *testing.T) {
	renderer, surface, err := createTestRenderer(200, 200)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Destroy()
	defer surface.Free()

	snap := map[string]*stats.HostStats{
		"host1": {CPU: map[string]collector.CPULine{"cpu": {}}},
	}
	cfg := defaultTestConfig()
	state := newRunState(cfg, 200, 200)
	state.mouseX = 50
	state.mouseY = 50
	// Set mouseLastMove to 4 seconds ago (beyond the 3s idle timeout)
	state.mouseLastMove = time.Now().Add(-4 * time.Second)
	state.smoothedCPU["host1;cpu"] = &[10]float64{10, 20, 5, 60, 3, 0, 0, 0, 2, 0}

	// Draw a solid blue background
	renderer.SetDrawColor(0, 0, 200, 255)
	renderer.FillRect(&sdl.Rect{X: 0, Y: 0, W: 200, H: 200})

	drawOverlay(renderer, snap, cfg, state)
	renderer.Present()

	// Mouse is idle > 3s, so no tooltip or inversion should be drawn.
	// The pixel should remain the original blue.
	r, g, b := getPixelColor(surface, 50, 50)
	if r > 10 || g > 10 {
		t.Errorf("expected original blue at (50,50) after idle timeout, got RGB(%d,%d,%d)", r, g, b)
	}
}

// --- multi-row hit test ---

func TestBuildBarMap_MultiRow(t *testing.T) {
	snap := map[string]*stats.HostStats{
		"a": {CPU: map[string]collector.CPULine{"cpu": {}}},
		"b": {CPU: map[string]collector.CPULine{"cpu": {}}},
		"c": {CPU: map[string]collector.CPULine{"cpu": {}}},
	}
	cfg := defaultTestConfig()
	cfg.MaxBarsPerRow = 2
	state := newRunState(cfg, 200, 200)

	bars := buildBarMap(snap, cfg, state)
	if len(bars) != 3 {
		t.Fatalf("expected 3 bars, got %d", len(bars))
	}
	// First row: bars 0 and 1 (hosts a, b) should have Y=0
	if bars[0].rect.Y != 0 || bars[1].rect.Y != 0 {
		t.Errorf("first row bars should start at Y=0, got Y=%d and Y=%d", bars[0].rect.Y, bars[1].rect.Y)
	}
	// Second row: bar 2 (host c) should have Y > 0
	if bars[2].rect.Y <= 0 {
		t.Errorf("second row bar should have Y>0, got Y=%d", bars[2].rect.Y)
	}
}
