package stats

// CPULine is one line of /proc/stat: cpu name + counters (user, nice, system, idle, ...).
type CPULine struct {
	Name                                                                    string
	User, Nice, System, Idle, Iowait, IRQ, SoftIRQ, Steal, Guest, GuestNice int64
}

// Total returns sum of all CPU counters.
func (c CPULine) Total() int64 {
	return c.User + c.Nice + c.System + c.Idle + c.Iowait + c.IRQ + c.SoftIRQ + c.Steal + c.Guest + c.GuestNice
}

// MemLine is one key from /proc/meminfo (e.g. MemTotal, MemFree).
type MemLine struct {
	Key   string
	Value int64
}

// NetLine is one interface line: iface and key=value pairs (b, tb, p, tp, e, te, d, td).
type NetLine struct {
	Iface string
	B     int64 // rx bytes
	Tb    int64 // tx bytes
	P     int64
	Tp    int64
	E     int64
	Te    int64
	D     int64
	Td    int64
}

// LoadAvg is 1/5/15 min load average.
type LoadAvg struct {
	Load1, Load5, Load15 string
}

// DiskLine is one device from /proc/diskstats with cumulative counters.
type DiskLine struct {
	Device       string
	SectorsRead  int64 // cumulative sectors read (each sector = 512 bytes)
	SectorsWrite int64 // cumulative sectors written
	ReadTicks    int64 // cumulative ms spent reading
	WriteTicks   int64 // cumulative ms spent writing
	IoTicks      int64 // cumulative ms the device had I/O in progress
}
