package collector

// Protocol mode markers (line-based, sent by remote script)
const (
	ModeLoadAvg  = "M LOADAVG"
	ModeMemStats = "M MEMSTATS"
	ModeNetStats = "M NETSTATS"
	ModeDiskStats = "M DISKSTATS"
	ModeCPUStats  = "M CPUSTATS"
)
