package collector

import "codeberg.org/snonux/loadbars/internal/stats"

// Collector parsing uses the shared stats protocol types.
type CPULine = stats.CPULine
type MemLine = stats.MemLine
type NetLine = stats.NetLine
type LoadAvg = stats.LoadAvg
type DiskLine = stats.DiskLine
