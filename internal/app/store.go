package app

import (
	"sync"

	"github.com/loadbars/loadbars/internal/collector"
	"github.com/loadbars/loadbars/internal/stats"
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
}

// NewStore creates an empty store.
func NewStore() *Store {
	return &Store{hosts: make(map[string]*hostData)}
}

func (s *Store) getOrCreate(host string) *hostData {
	if s.hosts[host] == nil {
		s.hosts[host] = &hostData{
			mem: make(map[string]int64),
			net: make(map[string]stats.NetStamp),
			cpu: make(map[string]collector.CPULine),
		}
	}
	return s.hosts[host]
}

// SetLoadAvg implements collector.StatsStore.
func (s *Store) SetLoadAvg(host, load1, load5, load15 string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.getOrCreate(host)
	d.load1, d.load5, d.load15 = load1, load5, load15
}

// SetCPU implements collector.StatsStore.
func (s *Store) SetCPU(host, name string, line collector.CPULine) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.getOrCreate(host)
	d.cpu[name] = line
}

// SetMem implements collector.StatsStore.
func (s *Store) SetMem(host, key string, value int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.getOrCreate(host)
	d.mem[key] = value
}

// SetNet implements collector.StatsStore.
func (s *Store) SetNet(host, iface string, net collector.NetLine, stamp float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.getOrCreate(host)
	d.net[iface] = stats.NetStamp{B: net.B, Tb: net.Tb, Stamp: stamp}
}

// Snapshot returns a copy of current stats for all hosts (for display).
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
		out[h] = &stats.HostStats{
			LoadAvg1:  d.load1,
			LoadAvg5:  d.load5,
			LoadAvg15: d.load15,
			Mem:       mem,
			Net:       net,
			CPU:       cpu,
		}
	}
	return out
}

var _ stats.Source = (*Store)(nil)
var _ collector.StatsStore = (*Store)(nil)
