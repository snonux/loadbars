package display

import (
	"strconv"
	"strings"

	"codeberg.org/snonux/loadbars/internal/config"
	"codeberg.org/snonux/loadbars/internal/constants"
	"codeberg.org/snonux/loadbars/internal/stats"
	"github.com/veandco/go-sdl2/sdl"
)

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

func drawNetBarSmoothed(renderer *sdl.Renderer, h *stats.HostStats, cfg *config.Config, smoothed *struct{ rxPct, txPct float64 }, prev stats.NetStamp, factor float64, barW int32, x, y, barH int32) stats.NetStamp {
	renderer.SetDrawColor(constants.Black.R, constants.Black.G, constants.Black.B, 255)
	renderer.FillRect(&sdl.Rect{X: x, Y: y, W: barW, H: barH})
	cur, hasIface := sumNonLoNet(h)
	if !hasIface {
		renderer.SetDrawColor(constants.Red.R, constants.Red.G, constants.Red.B, 255)
		renderer.FillRect(&sdl.Rect{X: x, Y: y, W: barW, H: barH})
		return prev
	}
	if cur.Stamp > prev.Stamp && prev.Stamp > 0 {
		prev = smoothNetUtilization(cur, prev, cfg, smoothed, factor)
	} else if prev.Stamp == 0 {
		prev = cur
	}
	drawNetHalves(renderer, smoothed, x, y, barW, barH)
	return prev
}

func smoothNetUtilization(cur, prev stats.NetStamp, cfg *config.Config, smoothed *struct{ rxPct, txPct float64 }, factor float64) stats.NetStamp {
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
	return cur
}

func drawNetHalves(renderer *sdl.Renderer, smoothed *struct{ rxPct, txPct float64 }, x, y, barW, barH int32) {
	halfW := barW / 2
	pxPerPct := float64(barH) / 100.0
	halfH := barH / 2
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
}
