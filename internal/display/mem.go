package display

import (
	"github.com/snonux/loadbars/internal/constants"
	"github.com/snonux/loadbars/internal/stats"
	"github.com/veandco/go-sdl2/sdl"
)

func drawMemBarSmoothed(renderer *sdl.Renderer, h *stats.HostStats, smoothed *struct{ ramUsed, swapUsed float64 }, factor float64, barW int32, x, y, barH int32) {
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
	ramUsedH := int32(smoothed.ramUsed * pxPerPct)
	if ramUsedH > 0 {
		renderer.SetDrawColor(constants.DarkGrey.R, constants.DarkGrey.G, constants.DarkGrey.B, 255)
		renderer.FillRect(&sdl.Rect{X: x, Y: y + barH - ramUsedH, W: halfW, H: ramUsedH})
	}
	if ramFreeH := barH - ramUsedH; ramFreeH > 0 {
		renderer.SetDrawColor(constants.Black.R, constants.Black.G, constants.Black.B, 255)
		renderer.FillRect(&sdl.Rect{X: x, Y: y, W: halfW, H: ramFreeH})
	}

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
