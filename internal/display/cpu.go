package display

import (
	"sort"

	"codeberg.org/snonux/loadbars/internal/constants"
	"codeberg.org/snonux/loadbars/internal/stats"
	"github.com/veandco/go-sdl2/sdl"
)

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

func sortedCPUNames(cpu map[string]stats.CPULine, cpuMode int) []string {
	if cpuMode == constants.CPUModeOff {
		return nil
	}
	var names []string
	for name := range cpu {
		if name == "cpu" {
			names = append(names, "cpu")
			continue
		}
		if cpuMode == constants.CPUModeCores {
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

// cpuBarTargetPcts returns the 9 segment percentages (system, user, nice, idle, iowait, irq, softirq, guest, steal) from cur/prev delta.
func cpuBarTargetPcts(cur, prev stats.CPULine) (out [10]float64, ok bool) {
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

func drawCPUBarFromPcts(renderer *sdl.Renderer, s *[10]float64, barW int32, x, y, barH int32, extended bool, peakPct float64) {
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
	fill(constants.Blue.R, constants.Blue.G, constants.Blue.B, (*s)[0])
	fill(constants.Yellow.R, constants.Yellow.G, constants.Yellow.B, (*s)[1])
	fill(constants.Green.R, constants.Green.G, constants.Green.B, (*s)[2])
	fill(constants.LimeGreen.R, constants.LimeGreen.G, constants.LimeGreen.B, (*s)[9])
	fill(constants.Black.R, constants.Black.G, constants.Black.B, (*s)[3])
	fill(constants.Purple.R, constants.Purple.G, constants.Purple.B, (*s)[4])
	fill(constants.White.R, constants.White.G, constants.White.B, (*s)[5])
	fill(constants.White.R, constants.White.G, constants.White.B, (*s)[6])
	fill(constants.Red.R, constants.Red.G, constants.Red.B, (*s)[7])
	fill(constants.Red.R, constants.Red.G, constants.Red.B, (*s)[8])
	if !extended || peakPct <= 0 {
		return
	}
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
