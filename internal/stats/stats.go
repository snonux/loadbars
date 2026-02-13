package stats

import (
	"github.com/loadbars/loadbars/internal/collector"
)

// NetStamp holds network stats and timestamp for delta calculation.
type NetStamp struct {
	B     int64
	Tb    int64
	Stamp float64
}

// HostStats holds the latest stats for one host (read-only snapshot).
type HostStats struct {
	LoadAvg1, LoadAvg5, LoadAvg15 string
	Mem                            map[string]int64
	Net                            map[string]NetStamp
	CPU                            map[string]collector.CPULine
}

// Source is the interface the display uses to read current stats.
type Source interface {
	Snapshot() map[string]*HostStats
}
