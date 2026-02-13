package collector

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Mem key regex: "MemTotal:    12345 kB" -> MemTotal, 12345
var memRegex = regexp.MustCompile(`^([A-Za-z0-9_]+):\s*(\d+)`)

// ParseCPULine parses a /proc/stat line: "cpu 100 0 50 200 0 0 0 0 0 0" (name + 10 numbers).
// Older kernels may have fewer fields; missing ones are treated as 0.
func ParseCPULine(line string) (CPULine, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return CPULine{}, fmt.Errorf("cpu line too short: %q", line)
	}
	nums := make([]int64, 10)
	for i := 1; i < len(fields) && i-1 < 10; i++ {
		n, _ := strconv.ParseInt(fields[i], 10, 64)
		nums[i-1] = n
	}
	return CPULine{
		Name:      fields[0],
		User:      nums[0],
		Nice:      nums[1],
		System:    nums[2],
		Idle:      nums[3],
		Iowait:    nums[4],
		IRQ:       nums[5],
		SoftIRQ:   nums[6],
		Steal:     nums[7],
		Guest:     nums[8],
		GuestNice: nums[9],
	}, nil
}

// ParseMemLine parses a /proc/meminfo line: "MemTotal:       123456 kB".
func ParseMemLine(line string) (MemLine, bool) {
	m := memRegex.FindStringSubmatch(line)
	if m == nil {
		return MemLine{}, false
	}
	v, _ := strconv.ParseInt(m[2], 10, 64)
	return MemLine{Key: m[1], Value: v}, true
}

// ParseNetLine parses a protocol net line: "eth0:b=0;tb=0;p=0;tp=0 e=0;te=0;d=0;td=0".
// There may be a space between first block (b,tb,p,tp) and second (e,te,d,td).
func ParseNetLine(line string) (NetLine, error) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return NetLine{}, fmt.Errorf("net line missing colon: %q", line)
	}
	net := NetLine{Iface: strings.TrimSpace(parts[0])}
	rest := strings.ReplaceAll(parts[1], " ", ";")
	for _, pair := range strings.Split(rest, ";") {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.TrimSpace(kv[0])
		v, _ := strconv.ParseInt(strings.TrimSpace(kv[1]), 10, 64)
		switch k {
		case "b":
			net.B = v
		case "tb":
			net.Tb = v
		case "p":
			net.P = v
		case "tp":
			net.Tp = v
		case "e":
			net.E = v
		case "te":
			net.Te = v
		case "d":
			net.D = v
		case "td":
			net.Td = v
		}
	}
	return net, nil
}

// ParseLoadAvg parses "1.0;0.5;0.2" into Load1, Load5, Load15.
func ParseLoadAvg(line string) LoadAvg {
	parts := strings.SplitN(line, ";", 3)
	l := LoadAvg{}
	if len(parts) > 0 {
		l.Load1 = strings.TrimSpace(parts[0])
	}
	if len(parts) > 1 {
		l.Load5 = strings.TrimSpace(parts[1])
	}
	if len(parts) > 2 {
		l.Load15 = strings.TrimSpace(parts[2])
	}
	return l
}
