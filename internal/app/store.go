package app

import (
	"sync"

	"github.com/snonux/loadbars/internal/collector"
	"github.com/snonux/loadbars/internal/stats"
)

// Store holds current stats from all hosts and implements collector.StatsStore.
type Store struct {
	mu sync.RWMutex
	// host -> *hostData
	hosts map[string]*hostData
}

type hostData struct {
	load1, load5, load15 string
	mem                  map[string]int64
	net                  map[string]stats.NetStamp
	cpu                  map[string]collector.CPULine
	disk                 map[string]stats.DiskStamp
}

// Compile-time interface satisfaction checks.
var _ stats.Source = (*Store)(nil)
var _ collector.StatsStore = (*Store)(nil)

// NewStore creates an empty store.
func NewStore() *Store {
	return &Store{hosts: make(map[string]*hostData)}
}

// SetLoadAvg sets the load average for the given host.
func (s *Store) SetLoadAvg(host, load1, load5, load15 string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.getOrCreate(host)
	d.load1, d.load5, d.load15 = load1, load5, load15
}

// SetCPU sets the CPU line for the given host and CPU name.
func (s *Store) SetCPU(host, name string, line collector.CPULine) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.getOrCreate(host)
	d.cpu[name] = line
}

// SetMem sets a meminfo key/value for the given host.
func (s *Store) SetMem(host, key string, value int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.getOrCreate(host)
	d.mem[key] = value
}

// SetNet sets the network stamp for the given host and interface.
func (s *Store) SetNet(host, iface string, net collector.NetLine, stamp float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.getOrCreate(host)
	d.net[iface] = stats.NetStamp{B: net.B, Tb: net.Tb, Stamp: stamp}
}

// SetDisk sets the disk stamp for the given host and device.
func (s *Store) SetDisk(host, device string, disk collector.DiskLine, stamp float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.getOrCreate(host)
	d.disk[device] = stats.DiskStamp{
		SectorsRead:  disk.SectorsRead,
		SectorsWrite: disk.SectorsWrite,
		IoTicks:      disk.IoTicks,
		Stamp:        stamp,
	}
}

// Snapshot returns a copy of current stats for all hosts for the display.
func (s *Store) Snapshot() map[string]*stats.HostStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]*stats.HostStats, len(s.hosts))
	for h, d := range s.hosts {
		mem := make(map[string]int64, len(d.mem))
		for k, v := range d.mem {
			mem[k] = v
		}
		net := make(map[string]stats.NetStamp, len(d.net))
		for k, v := range d.net {
			net[k] = v
		}
		cpu := make(map[string]collector.CPULine, len(d.cpu))
		for k, v := range d.cpu {
			cpu[k] = v
		}
		disk := make(map[string]stats.DiskStamp, len(d.disk))
		for k, v := range d.disk {
			disk[k] = v
		}
		out[h] = &stats.HostStats{
			LoadAvg1:  d.load1,
			LoadAvg5:  d.load5,
			LoadAvg15: d.load15,
			Mem:       mem,
			Net:       net,
			CPU:       cpu,
			Disk:      disk,
		}
	}
	return out
}

func (s *Store) getOrCreate(host string) *hostData {
	if s.hosts[host] == nil {
		s.hosts[host] = &hostData{
			mem:  make(map[string]int64),
			net:  make(map[string]stats.NetStamp),
			cpu:  make(map[string]collector.CPULine),
			disk: make(map[string]stats.DiskStamp),
		}
	}
	return s.hosts[host]
}
