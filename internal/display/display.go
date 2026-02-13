package display

import (
	"context"
	"fmt"
	"os"
	"sort"
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
	redrawBg := true

	// Previous CPU state for delta (key = host;cpuName)
	prevCPU := make(map[string]collector.CPULine)
	// We need collector.CPULine - use stats.HostStats.CPU which is map[string]collector.CPULine. So we need to import collector for CPULine.
	// Actually we have stats.HostStats which has CPU map[string]collector.CPULine. So we need to import collector in display for the type. Let me add the import and a type alias or use the type from collector. So display will import collector for CPULine.
	_ = prevCPU

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
					redrawBg = true
				case sdl.K_2:
					showMem = !showMem
				case sdl.K_3:
					showNet = !showNet
				case sdl.K_e:
					extended = !extended
					redrawBg = true
				case sdl.K_h:
					printHotkeys()
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
					redrawBg = true
				case sdl.K_RIGHT:
					winW += 100
					if winW > int32(cfg.MaxWidth) {
						winW = int32(cfg.MaxWidth)
					}
					window.SetSize(winW, winH)
					redrawBg = true
				case sdl.K_UP:
					winH -= 100
					if winH < 1 {
						winH = 1
					}
					window.SetSize(winW, winH)
					redrawBg = true
				case sdl.K_DOWN:
					winH += 100
					window.SetSize(winW, winH)
					redrawBg = true
				}
			case *sdl.WindowEvent:
				if ev.Event == sdl.WINDOWEVENT_RESIZED {
					winW, winH = ev.Data1, ev.Data2
					redrawBg = true
				}
			}
		}

		snap := src.Snapshot()
		numStats := len(snap)
		if cfg.ShowMem {
			numStats += len(snap)
		}
		if cfg.ShowNet {
			numStats += len(snap)
		}
		if numStats == 0 {
			numStats = 1
		}

		barWidth := (winW / int32(numStats)) - 1
		if barWidth < 1 {
			barWidth = 1
		}

		if redrawBg {
			renderer.SetDrawColor(0, 0, 0, 255)
			renderer.Clear()
			redrawBg = false
		}

		x := int32(0)
		hosts := sortedHosts(snap)
		for _, host := range hosts {
			h := snap[host]
			if h == nil {
				continue
			}
			// Draw CPU bars for this host (aggregate or per-core)
			cpuNames := sortedCPUNames(h.CPU, showCores)
			for _, name := range cpuNames {
				drawCPUBar(renderer, h.CPU[name], prevCPU[host+";"+name], barWidth, &x, winH)
				prevCPU[host+";"+name] = h.CPU[name]
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

func drawCPUBar(renderer *sdl.Renderer, cur, prev collector.CPULine, barW int32, x *int32, winH int32) {
	defer func() { *x += barW + 1 }()
	// Compute delta and normalize to %
	totalCur := cur.Total()
	totalPrev := prev.Total()
	if totalPrev == 0 || totalCur <= totalPrev {
		return
	}
	scale := float64(totalCur-totalPrev) / 100.0
	if scale <= 0 {
		return
	}
	userPct := int((cur.User - prev.User) / int64(scale))
	nicePct := int((cur.Nice - prev.Nice) / int64(scale))
	sysPct := int((cur.System - prev.System) / int64(scale))
	idlePct := int((cur.Idle - prev.Idle) / int64(scale))
	iowaitPct := int((cur.Iowait - prev.Iowait) / int64(scale))
	irqPct := int((cur.IRQ - prev.IRQ) / int64(scale))
	softirqPct := int((cur.SoftIRQ - prev.SoftIRQ) / int64(scale))
	guestPct := int((cur.Guest - prev.Guest) / int64(scale))
	stealPct := int((cur.Steal - prev.Steal) / int64(scale))

	norm := func(v int) int {
		if v < 0 {
			return 0
		}
		if v > 100 {
			return 100
		}
		return v
	}
	userPct = norm(userPct)
	nicePct = norm(nicePct)
	sysPct = norm(sysPct)
	idlePct = norm(idlePct)
	iowaitPct = norm(iowaitPct)
	irqPct = norm(irqPct)
	softirqPct = norm(softirqPct)
	guestPct = norm(guestPct)
	stealPct = norm(stealPct)

	barH := float64(winH) / 100.0
	y := float64(winH)
	fill := func(r, g, b uint8, h int) {
		hh := int32(float64(h) * barH)
		if hh < 1 && h > 0 {
			hh = 1
		}
		y -= float64(hh)
		renderer.SetDrawColor(r, g, b, 255)
		rect := sdl.Rect{X: *x, Y: int32(y), W: barW, H: hh}
		renderer.FillRect(&rect)
	}
	// Order bottom to top: system, user, nice, idle, iowait, irq, softirq, guest, steal (match Perl)
	fill(constants.Blue.R, constants.Blue.G, constants.Blue.B, sysPct)
	fill(constants.Yellow.R, constants.Yellow.G, constants.Yellow.B, userPct)
	fill(constants.Green.R, constants.Green.G, constants.Green.B, nicePct)
	fill(constants.Black.R, constants.Black.G, constants.Black.B, idlePct)
	fill(constants.Purple.R, constants.Purple.G, constants.Purple.B, iowaitPct)
	fill(constants.White.R, constants.White.G, constants.White.B, irqPct)
	fill(constants.White.R, constants.White.G, constants.White.B, softirqPct)
	fill(constants.Red.R, constants.Red.G, constants.Red.B, guestPct)
	fill(constants.Red.R, constants.Red.G, constants.Red.B, stealPct)
}

func printHotkeys() {
	fmt.Println("=> Hotkeys: 1=cores 2=mem 3=net e=extended h=help n=next net q=quit w=write config a/y=cpu avg d/c=net avg f/v=link scale arrows=resize")
}
