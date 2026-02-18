package display

import (
	"regexp"
	"sort"
	"strings"

	"codeberg.org/snonux/loadbars/internal/constants"
	"codeberg.org/snonux/loadbars/internal/stats"
	"github.com/veandco/go-sdl2/sdl"
)

// partitionSuffix matches trailing partition numbers on SCSI-style names (sda1, vda2, xvda3)
// and NVMe partition suffixes (nvme0n1p1). Loop, ram, and dm- devices are handled separately.
var partitionSuffix = regexp.MustCompile(`^(sd|vd|xvd|hd)[a-z]+\d+$`)
var nvmePartition = regexp.MustCompile(`^nvme\d+n\d+p\d+$`)

// isWholeDisk returns true if the device name represents a whole disk (not a partition,
// loop, ram, or device-mapper device). Used to filter /proc/diskstats entries.
func isWholeDisk(name string) bool {
	if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "dm-") || strings.HasPrefix(name, "zram") {
		return false
	}
	if partitionSuffix.MatchString(name) {
		return false
	}
	if nvmePartition.MatchString(name) {
		return false
	}
	return true
}

// sortedDiskNames returns the list of disk device names to display based on the disk mode.
// In aggregate mode, returns ["all"]; in device mode, returns sorted whole-disk names;
// in off mode, returns nil.
func sortedDiskNames(disk map[string]stats.DiskStamp, diskMode int) []string {
	switch diskMode {
	case constants.DiskModeAggregate:
		return []string{"all"}
	case constants.DiskModeDevices:
		var names []string
		for dev := range disk {
			if isWholeDisk(dev) {
				names = append(names, dev)
			}
		}
		sort.Strings(names)
		return names
	default:
		return nil
	}
}

// sumAllDisks sums sectors and picks the latest timestamp across all whole-disk devices.
func sumAllDisks(disk map[string]stats.DiskStamp) stats.DiskStamp {
	var sum stats.DiskStamp
	for dev, ds := range disk {
		if !isWholeDisk(dev) {
			continue
		}
		sum.SectorsRead += ds.SectorsRead
		sum.SectorsWrite += ds.SectorsWrite
		sum.IoTicks += ds.IoTicks
		if ds.Stamp > sum.Stamp {
			sum.Stamp = ds.Stamp
		}
	}
	return sum
}

// updateDiskPeak updates the auto-scale disk peak (bytes/sec) with slow decay.
// When diskMax > 0, the fixed value is used instead.
func updateDiskPeak(snap map[string]*stats.HostStats, state *runState, diskMax float64) {
	if diskMax > 0 {
		state.diskPeak = diskMax
		return
	}
	// Slow per-frame decay toward idle baseline
	state.diskPeak *= 0.9999
	const floorBps = 1048576.0 // 1 MB/s floor
	if state.diskPeak < floorBps {
		state.diskPeak = floorBps
	}
	// Scan current disk data to find if any host exceeds the peak
	for host, h := range snap {
		if h == nil || h.Disk == nil {
			continue
		}
		diskNames := sortedDiskNames(h.Disk, state.diskMode)
		for _, name := range diskNames {
			key := host + ";disk;" + name
			var cur stats.DiskStamp
			if name == "all" {
				cur = sumAllDisks(h.Disk)
			} else {
				cur = h.Disk[name]
			}
			prev, ok := state.prevDisk[key]
			if !ok || cur.Stamp <= prev.Stamp || prev.Stamp == 0 {
				continue
			}
			dt := cur.Stamp - prev.Stamp
			if dt <= 0 {
				continue
			}
			readBps := float64(cur.SectorsRead-prev.SectorsRead) * 512 / dt
			writeBps := float64(cur.SectorsWrite-prev.SectorsWrite) * 512 / dt
			totalBps := readBps + writeBps
			if totalBps > state.diskPeak {
				state.diskPeak = totalBps
			}
		}
	}
}

// drawDiskBarSmoothed draws a single disk bar with read (top, purple) and write (bottom,
// darker purple). Returns the current DiskStamp to be stored as previous for the next frame.
func drawDiskBarSmoothed(renderer *sdl.Renderer, cur stats.DiskStamp, cfg *runState, smoothed *struct{ readPct, writePct float64 }, prev stats.DiskStamp, factor float64, barW, x, y, barH int32, extended bool) stats.DiskStamp {
	// Clear this slot to a dim purple so the bar is visible even when idle.
	// This distinguishes "disk bar present but idle" from background.
	renderer.SetDrawColor(0x18, 0x00, 0x28, 255)
	renderer.FillRect(&sdl.Rect{X: x, Y: y, W: barW, H: barH})

	// Only recompute when the collector has provided new data (same guard as net bars).
	if cur.Stamp > prev.Stamp && prev.Stamp > 0 {
		prev = smoothDiskUtilization(cur, prev, cfg, smoothed, factor)
	} else if prev.Stamp == 0 && cur.Stamp > 0 {
		// First sample: record it but can't compute delta yet.
		prev = cur
	}

	drawDiskHalves(renderer, smoothed, x, y, barW, barH)

	// In extended mode, overlay a utilization % line
	if extended && prev.Stamp > 0 {
		drawDiskUtilLine(renderer, cur, prev, cfg, x, y, barW, barH)
	}
	return prev
}

// smoothDiskUtilization computes read/write throughput as % of diskPeak and smooths.
func smoothDiskUtilization(cur, prev stats.DiskStamp, state *runState, smoothed *struct{ readPct, writePct float64 }, factor float64) stats.DiskStamp {
	peak := state.diskPeak
	if peak <= 0 {
		peak = 1048576 // 1 MB/s fallback
	}
	dt := cur.Stamp - prev.Stamp
	if dt > 0 {
		deltaRead := cur.SectorsRead - prev.SectorsRead
		deltaWrite := cur.SectorsWrite - prev.SectorsWrite
		if deltaRead < 0 {
			deltaRead = 0
		}
		if deltaWrite < 0 {
			deltaWrite = 0
		}
		readBps := float64(deltaRead) * 512 / dt
		writeBps := float64(deltaWrite) * 512 / dt
		targetRead := 100 * readBps / peak
		targetWrite := 100 * writeBps / peak
		smoothed.readPct += (targetRead - smoothed.readPct) * factor
		smoothed.writePct += (targetWrite - smoothed.writePct) * factor
	}
	return cur // advance the baseline
}

// drawDiskHalves draws read from top (purple) and write from bottom (darker purple).
func drawDiskHalves(renderer *sdl.Renderer, smoothed *struct{ readPct, writePct float64 }, x, y, barW, barH int32) {
	halfH := barH / 2
	pxPerPct := float64(barH) / 100.0

	// Read from top (purple)
	readH := int32(smoothed.readPct * pxPerPct)
	if readH > halfH {
		readH = halfH
	}
	if readH > 0 {
		renderer.SetDrawColor(constants.DiskRead.R, constants.DiskRead.G, constants.DiskRead.B, 255)
		renderer.FillRect(&sdl.Rect{X: x, Y: y, W: barW, H: readH})
	}

	// Write from bottom (darker purple)
	writeH := int32(smoothed.writePct * pxPerPct)
	if writeH > halfH {
		writeH = halfH
	}
	if writeH > 0 {
		renderer.SetDrawColor(constants.DiskWrite.R, constants.DiskWrite.G, constants.DiskWrite.B, 255)
		renderer.FillRect(&sdl.Rect{X: x, Y: y + barH - writeH, W: barW, H: writeH})
	}
}

// drawDiskUtilLine draws a 3px-thick horizontal line showing disk utilization %
// (fraction of time the device had I/O in progress) in extended mode.
func drawDiskUtilLine(renderer *sdl.Renderer, cur, prev stats.DiskStamp, state *runState, x, y, barW, barH int32) {
	dt := cur.Stamp - prev.Stamp
	if dt <= 0 {
		return
	}
	// IoTicks is cumulative ms; utilization = delta_io_ticks / (dt * 1000)
	deltaIo := cur.IoTicks - prev.IoTicks
	if deltaIo < 0 {
		deltaIo = 0
	}
	utilPct := float64(deltaIo) / (dt * 1000) * 100
	if utilPct > 100 {
		utilPct = 100
	}
	lineY := y + int32(utilPct/100*float64(barH))
	if lineY >= y+barH {
		lineY = y + barH - 1
	}
	renderer.SetDrawColor(constants.DiskUtil.R, constants.DiskUtil.G, constants.DiskUtil.B, 255)
	// Draw 3px band for visibility
	for dy := int32(-1); dy <= 1; dy++ {
		ly := lineY + dy
		if ly >= y && ly < y+barH {
			renderer.DrawLine(x, ly, x+barW-1, ly)
		}
	}
}
