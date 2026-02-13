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
	"github.com/veandco/go-sdl2/sdl"
)

// Run runs the SDL display loop until ctx is cancelled or user presses 'q'.
func Run(ctx context.Context, cfg *config.Config, src stats.Source) error {
	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		return fmt.Errorf("sdl init: %w", err)
	}
	defer sdl.Quit()

	width := cfg.BarWidth
	if width < 1 {
		width = 1
	}
	if width > cfg.MaxWidth {
		width = cfg.MaxWidth
	}
	height := cfg.Height
	if height < 1 {
		height = 1
	}

	title := cfg.Title
	if title == "" {
		title = "Loadbars (press h for help on stdout)"
	}

	window, renderer, err := sdl.CreateWindowAndRenderer(int32(width), int32(height), sdl.WINDOW_RESIZABLE)
	if err != nil {
		return fmt.Errorf("create window: %w", err)
	}
	defer window.Destroy()
	defer renderer.Destroy()

	window.SetTitle(title)

	// Mutable copy of config for hotkey toggles (only what display needs)
	showCores := cfg.ShowCores
	showMem := cfg.ShowMem
	showNet := cfg.ShowNet
	extended := cfg.Extended
	winW, winH := int32(width), int32(height)

	// Previous CPU state for delta (key = host;cpuName)
	prevCPU := make(map[string]collector.CPULine)
	// Smoothed values for transitions (blend toward target each frame)
	const smoothFactor = 0.12 // lower = smoother, less flicker from noisy samples
	smoothedCPU := make(map[string]*[9]float64)
	smoothedMem := make(map[string]*struct{ ramUsed, swapUsed float64 })
	smoothedNet := make(map[string]*struct{ rxPct, txPct float64 })
	prevNet := make(map[string]stats.NetStamp)
	netIntIndex := make(map[string]int) // for cycling interface per host
	var cycleNetNext bool
	var printNetInfoOnce bool = showNet // print chosen interface when net view is on (once at start or after toggling on)
	// Peak history for extended mode: per CPU bar key, ring of (system+user) %
	peakHistory := make(map[string][]float64)

	lastNumBars := -1
	lastWinW, lastWinH := int32(0), int32(0)
	ticker := time.NewTicker(time.Duration(constants.IntervalSDL * float64(time.Second)))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Poll all pending events
		for e := sdl.PollEvent(); e != nil; e = sdl.PollEvent() {
			switch ev := e.(type) {
			case *sdl.QuitEvent:
				return nil
			case *sdl.KeyboardEvent:
				if ev.Type != sdl.KEYDOWN || ev.Repeat != 0 {
					continue
				}
				sym := ev.Keysym.Sym
				switch sym {
				case sdl.K_q:
					return nil
				case sdl.K_1:
					showCores = !showCores
					fmt.Println("==> Toggled show cores:", showCores)
				case sdl.K_2:
					showMem = !showMem
					fmt.Println("==> Toggled show mem:", showMem)
				case sdl.K_3:
					showNet = !showNet
					fmt.Println("==> Toggled show net:", showNet)
					if showNet {
						printNetInfoOnce = true
					}
				case sdl.K_e:
					extended = !extended
					fmt.Println("==> Toggled extended (peak line):", extended)
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
				case sdl.K_h:
					printHotkeys()
				case sdl.K_n:
					cycleNetNext = true
					if showNet {
						fmt.Println("==> Cycling to next network interface (per host)")
					}
				case sdl.K_w:
					cfg.ShowCores = showCores
					cfg.ShowMem = showMem
					cfg.ShowNet = showNet
					cfg.Extended = extended
					if err := cfg.Write(); err != nil {
						fmt.Fprintf(os.Stderr, "!!! Write config: %v\n", err)
					} else {
						fmt.Println("==> Config written to ~/.loadbarsrc")
					}
				case sdl.K_LEFT:
					winW -= 100
					if winW < 1 {
						winW = 1
					}
					window.SetSize(winW, winH)
				case sdl.K_RIGHT:
					winW += 100
					if winW > int32(cfg.MaxWidth) {
						winW = int32(cfg.MaxWidth)
					}
					window.SetSize(winW, winH)
				case sdl.K_UP:
					winH -= 100
					if winH < 1 {
						winH = 1
					}
					window.SetSize(winW, winH)
				case sdl.K_DOWN:
					winH += 100
					window.SetSize(winW, winH)
				}
			case *sdl.WindowEvent:
				if ev.Event == sdl.WINDOWEVENT_RESIZED {
					winW, winH = ev.Data1, ev.Data2
				}
			}
		}

		snap := src.Snapshot()
		if cycleNetNext {
			for _, host := range sortedHosts(snap) {
				netIntIndex[host]++
			}
			cycleNetNext = false
		}
		// One-time: print which interface is used for net stats and how to configure
		if printNetInfoOnce && showNet {
			printNetInfoOnce = false
			printNetInterfaceHelp(snap, cfg, netIntIndex)
		}
		// Count total bars we will draw (only non-nil hosts) so layout matches draw order
		numBars := 0
		for _, host := range sortedHosts(snap) {
			if h := snap[host]; h != nil {
				numBars += len(sortedCPUNames(h.CPU, showCores))
				if showMem {
					numBars++
				}
				if showNet {
					numBars++
				}
			}
		}
		if numBars == 0 {
			numBars = 1
		}

		barWidth := winW / int32(numBars)
		if barWidth < 1 {
			barWidth = 1
		}

		// Clear only when layout changes (bar count or window size) to avoid full-screen flicker
		if numBars != lastNumBars || winW != lastWinW || winH != lastWinH {
			renderer.SetDrawColor(0, 0, 0, 255)
			renderer.Clear()
			lastNumBars = numBars
			lastWinW, lastWinH = winW, winH
		}

		x := int32(0)
		hosts := sortedHosts(snap)
		for _, host := range hosts {
			h := snap[host]
			if h == nil {
				continue
			}
			// Draw CPU bars for this host (aggregate or per-core), with smoothing
			cpuNames := sortedCPUNames(h.CPU, showCores)
			for _, name := range cpuNames {
				key := host + ";" + name
				cur := h.CPU[name]
				prev := prevCPU[key]
				prevCPU[key] = cur
				target, ok := cpuBarTargetPcts(cur, prev)
				s := smoothedCPU[key]
				if s == nil {
					s = &[9]float64{}
					smoothedCPU[key] = s
					if ok {
						*s = target
					}
				} else if ok {
					for i := 0; i < 9; i++ {
						(*s)[i] += (target[i] - (*s)[i]) * smoothFactor
					}
					normalizePcts9(s)
				}
				// Peak line (extended): max of (system+user) over last CPUAverage samples
				var peakPct float64
				if extended && s != nil {
					userSys := (*s)[0] + (*s)[1]
					hist := peakHistory[key]
					hist = append(hist, userSys)
					n := cfg.CPUAverage
					if n < 1 {
						n = 1
					}
					for len(hist) > n {
						hist = hist[1:]
					}
					peakHistory[key] = hist
					for _, v := range hist {
						if v > peakPct {
							peakPct = v
						}
					}
				}
				// Always draw (smoothed or last state) so we never leave a blank bar and cause flicker
				drawCPUBarFromPcts(renderer, s, barWidth, &x, winH, extended, peakPct)
			}
			// Draw memory bar(s) for this host when showMem, with smoothing
			if showMem {
				if smoothedMem[host] == nil {
					smoothedMem[host] = &struct{ ramUsed, swapUsed float64 }{}
				}
				drawMemBarSmoothed(renderer, h, smoothedMem[host], smoothFactor, barWidth, &x, winH)
			}
			// Draw network bar(s) for this host when showNet
			if showNet {
				if smoothedNet[host] == nil {
					smoothedNet[host] = &struct{ rxPct, txPct float64 }{}
				}
				prevNet[host] = drawNetBarSmoothed(renderer, h, cfg, smoothedNet[host], prevNet[host], netIntIndex, host, smoothFactor, barWidth, &x, winH)
			}
		}

		renderer.Present()
		sdl.Delay(10)

		<-ticker.C
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
func cpuBarTargetPcts(cur, prev collector.CPULine) (out [9]float64, ok bool) {
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

func normalizePcts9(s *[9]float64) {
	var sum float64
	for i := 0; i < 9; i++ {
		sum += (*s)[i]
	}
	if sum <= 0 {
		return
	}
	for i := 0; i < 9; i++ {
		(*s)[i] = (*s)[i] * 100 / sum
	}
}

// drawCPUBarFromPcts draws one CPU bar from 9 smoothed segment percentages. If s is nil, advances x only.
// When extended is true and peakPct > 0, draws a 1px peak line (max system+user over history).
func drawCPUBarFromPcts(renderer *sdl.Renderer, s *[9]float64, barW int32, x *int32, winH int32, extended bool, peakPct float64) {
	defer func() { *x += barW }()
	// Clear this slot so we never leave previous (e.g. mem/net) content visible
	renderer.SetDrawColor(constants.Black.R, constants.Black.G, constants.Black.B, 255)
	renderer.FillRect(&sdl.Rect{X: *x, Y: 0, W: barW, H: winH})
	if s == nil {
		return
	}
	barH := float64(winH) / 100.0
	y := float64(winH)
	fill := func(r, g, b uint8, pct float64) {
		hh := int32(pct * barH)
		if hh < 1 && pct > 0 {
			hh = 1
		}
		y -= float64(hh)
		renderer.SetDrawColor(r, g, b, 255)
		renderer.FillRect(&sdl.Rect{X: *x, Y: int32(y), W: barW, H: hh})
	}
	fill(constants.Blue.R, constants.Blue.G, constants.Blue.B, (*s)[0])   // system
	fill(constants.Yellow.R, constants.Yellow.G, constants.Yellow.B, (*s)[1]) // user
	fill(constants.Green.R, constants.Green.G, constants.Green.B, (*s)[2])   // nice
	fill(constants.Black.R, constants.Black.G, constants.Black.B, (*s)[3])    // idle
	fill(constants.Purple.R, constants.Purple.G, constants.Purple.B, (*s)[4]) // iowait
	fill(constants.White.R, constants.White.G, constants.White.B, (*s)[5])   // irq
	fill(constants.White.R, constants.White.G, constants.White.B, (*s)[6])   // softirq
	fill(constants.Red.R, constants.Red.G, constants.Red.B, (*s)[7])         // guest
	fill(constants.Red.R, constants.Red.G, constants.Red.B, (*s)[8])         // steal
	// Extended: 1px peak line at max (system+user) over history
	if extended && peakPct > 0 {
		peakY := winH - int32(peakPct*barH)
		if peakY < 0 {
			peakY = 0
		}
		if peakY >= winH {
			peakY = winH - 1
		}
		if peakPct > float64(constants.UserOrangeThreshold) {
			renderer.SetDrawColor(constants.Orange.R, constants.Orange.G, constants.Orange.B, 255)
		} else if peakPct > float64(constants.UserYellowThreshold) {
			renderer.SetDrawColor(constants.Yellow0.R, constants.Yellow0.G, constants.Yellow0.B, 255)
		} else {
			renderer.SetDrawColor(constants.Yellow.R, constants.Yellow.G, constants.Yellow.B, 255)
		}
		renderer.FillRect(&sdl.Rect{X: *x, Y: peakY, W: barW, H: 1})
	}
}

// drawMemBarSmoothed blends mem stats toward target and draws one memory bar (RAM left, Swap right).
func drawMemBarSmoothed(renderer *sdl.Renderer, h *stats.HostStats, smoothed *struct{ ramUsed, swapUsed float64 }, factor float64, barW int32, x *int32, winH int32) {
	defer func() { *x += barW }()
	// Clear this slot so we never leave previous (e.g. CPU/net) content visible
	renderer.SetDrawColor(constants.Black.R, constants.Black.G, constants.Black.B, 255)
	renderer.FillRect(&sdl.Rect{X: *x, Y: 0, W: barW, H: winH})
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
	barH := float64(winH) / 100.0

	// RAM: used (dark grey) from bottom, free (black) on top
	ramUsedH := int32(smoothed.ramUsed * barH)
	if ramUsedH > 0 {
		renderer.SetDrawColor(constants.DarkGrey.R, constants.DarkGrey.G, constants.DarkGrey.B, 255)
		renderer.FillRect(&sdl.Rect{X: *x, Y: winH - ramUsedH, W: halfW, H: ramUsedH})
	}
	if ramFreeH := winH - ramUsedH; ramFreeH > 0 {
		renderer.SetDrawColor(constants.Black.R, constants.Black.G, constants.Black.B, 255)
		renderer.FillRect(&sdl.Rect{X: *x, Y: 0, W: halfW, H: ramFreeH})
	}

	// Swap: used (grey) from bottom, free (black) on top
	swapUsedH := int32(smoothed.swapUsed * barH)
	if swapUsedH > 0 {
		renderer.SetDrawColor(constants.Grey.R, constants.Grey.G, constants.Grey.B, 255)
		renderer.FillRect(&sdl.Rect{X: *x + halfW, Y: winH - swapUsedH, W: halfW, H: swapUsedH})
	}
	if swapFreeH := winH - swapUsedH; swapFreeH > 0 {
		renderer.SetDrawColor(constants.Black.R, constants.Black.G, constants.Black.B, 255)
		renderer.FillRect(&sdl.Rect{X: *x + halfW, Y: 0, W: halfW, H: swapFreeH})
	}
}

func printHotkeys() {
	fmt.Println("=> Hotkeys: 1=cores 2=mem 3=net e=extended h=help n=next net q=quit w=write config a/y=cpu avg d/c=net avg f/v=link scale arrows=resize")
}

// printNetInterfaceHelp prints which interface is used per host and how to set netint (when net view is toggled on).
func printNetInterfaceHelp(snap map[string]*stats.HostStats, cfg *config.Config, netIntIndex map[string]int) {
	for _, host := range sortedHosts(snap) {
		h := snap[host]
		if h == nil || h.Net == nil || len(h.Net) == 0 {
			fmt.Printf("Net: %s => (no interfaces yet, wait for data)\n", host)
			continue
		}
		iface := chooseNetIface(h, cfg, host, netIntIndex)
		all := make([]string, 0, len(h.Net))
		for name := range h.Net {
			all = append(all, name)
		}
		sort.Strings(all)
		if iface == "" {
			fmt.Printf("Net: %s => (no non-lo interface; seen: %s)\n", host, strings.Join(all, ", "))
			continue
		}
		hint := "set netint=IFACE in ~/.loadbarsrc or --netint IFACE"
		if cfg.NetInt != "" {
			hint = "using netint=" + cfg.NetInt + " from config"
		}
		fmt.Printf("Net: %s => %s (all: %s). %s\n", host, iface, strings.Join(all, ", "), hint)
	}
	fmt.Println("=> Link speed: netlink=" + cfg.NetLink + " (gbit/mbit/10mbit/100mbit/10gbit or number). Change in ~/.loadbarsrc or --netlink")
}

// netLinkBytesPerSec returns link speed in bytes/sec from cfg.NetLink (e.g. "gbit", "10gbit", "100mbit", or numeric mbit).
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

// chooseNetIface returns the interface name to use for this host: cfg.NetInt if set and present, else first non-lo, cycling with n key.
func chooseNetIface(h *stats.HostStats, cfg *config.Config, host string, netIntIndex map[string]int) string {
	if h.Net == nil || len(h.Net) == 0 {
		return ""
	}
	if cfg.NetInt != "" {
		if _, ok := h.Net[cfg.NetInt]; ok {
			return cfg.NetInt
		}
	}
	names := make([]string, 0, len(h.Net))
	for iface := range h.Net {
		if iface == "lo" {
			continue
		}
		names = append(names, iface)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return ""
	}
	idx := netIntIndex[host] % len(names)
	if idx < 0 {
		idx += len(names)
	}
	return names[idx]
}

func drawNetBarSmoothed(renderer *sdl.Renderer, h *stats.HostStats, cfg *config.Config, smoothed *struct{ rxPct, txPct float64 }, prev stats.NetStamp, netIntIndex map[string]int, host string, factor float64, barW int32, x *int32, winH int32) stats.NetStamp {
	defer func() { *x += barW }()
	// Clear this slot so we never leave previous (e.g. CPU/mem) content visible
	renderer.SetDrawColor(constants.Black.R, constants.Black.G, constants.Black.B, 255)
	renderer.FillRect(&sdl.Rect{X: *x, Y: 0, W: barW, H: winH})
	iface := chooseNetIface(h, cfg, host, netIntIndex)
	if iface == "" {
		renderer.SetDrawColor(constants.Red.R, constants.Red.G, constants.Red.B, 255)
		renderer.FillRect(&sdl.Rect{X: *x, Y: 0, W: barW, H: winH})
		return prev
	}
	cur, ok := h.Net[iface]
	if !ok {
		renderer.SetDrawColor(constants.Red.R, constants.Red.G, constants.Red.B, 255)
		renderer.FillRect(&sdl.Rect{X: *x, Y: 0, W: barW, H: winH})
		return prev
	}
	linkBps := netLinkBytesPerSec(cfg)
	if linkBps <= 0 {
		linkBps = int64(constants.BytesGbit)
	}
	var targetRx, targetTx float64
	if prev.Stamp > 0 && cur.Stamp > prev.Stamp {
		dt := float64(cur.Stamp-prev.Stamp) / 1e9
		if dt > 0 {
			deltaB := cur.B - prev.B
			deltaTb := cur.Tb - prev.Tb
			if deltaB < 0 {
				deltaB = 0
			}
			if deltaTb < 0 {
				deltaTb = 0
			}
			targetRx = 100 * float64(deltaB) / (float64(linkBps) * dt)
			targetTx = 100 * float64(deltaTb) / (float64(linkBps) * dt)
		}
	}
	smoothed.rxPct += (targetRx - smoothed.rxPct) * factor
	smoothed.txPct += (targetTx - smoothed.txPct) * factor

	halfW := barW / 2
	barH := float64(winH) / 100.0
	// Left half: RX from top (light green = used)
	rxH := int32(smoothed.rxPct * barH)
	if rxH > winH/2 {
		rxH = winH / 2
	}
	if rxH > 0 {
		renderer.SetDrawColor(constants.LightGreen.R, constants.LightGreen.G, constants.LightGreen.B, 255)
		renderer.FillRect(&sdl.Rect{X: *x, Y: 0, W: halfW, H: rxH})
	}
	if halfW > 0 && winH/2-rxH > 0 {
		renderer.SetDrawColor(constants.Black.R, constants.Black.G, constants.Black.B, 255)
		renderer.FillRect(&sdl.Rect{X: *x, Y: rxH, W: halfW, H: winH/2 - rxH})
	}
	// Right half: TX from bottom (light green = used)
	txH := int32(smoothed.txPct * barH)
	if txH > winH/2 {
		txH = winH / 2
	}
	if txH > 0 {
		renderer.SetDrawColor(constants.LightGreen.R, constants.LightGreen.G, constants.LightGreen.B, 255)
		renderer.FillRect(&sdl.Rect{X: *x + halfW, Y: winH - txH, W: halfW, H: txH})
	}
	if halfW > 0 && (winH - txH) > 0 {
		renderer.SetDrawColor(constants.Black.R, constants.Black.G, constants.Black.B, 255)
		renderer.FillRect(&sdl.Rect{X: *x + halfW, Y: 0, W: halfW, H: winH - txH})
	}
	return cur
}
