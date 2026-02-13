package collector

// Protocol mode markers (line-based, sent by remote script)
const (
	ModeLoadAvg  = "M LOADAVG"
	ModeMemStats = "M MEMSTATS"
	ModeNetStats = "M NETSTATS"
	ModeCPUStats = "M CPUSTATS"
)
