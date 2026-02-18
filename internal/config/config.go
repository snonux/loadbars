package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"codeberg.org/snonux/loadbars/internal/constants"
)

// Config holds all loadbars configuration (file + CLI).
// Defaults match the Perl Shared.pm %C.
type Config struct {
	Hosts          []string // Each entry is "host" or "host:user"
	Title          string
	BarWidth       int
	CPUAverage     int
	Extended       bool
	HasAgent       bool
	Height         int
	MaxWidth       int
	NetAverage     int
	NetLink        string
	ShowAvgLine    bool
	ShowIOAvgLine  bool
	CPUMode        int // constants.CPUModeAverage / CPUModeCores / CPUModeOff
	ShowMem        bool
	ShowNet        bool
	ShowLoad       bool
	LoadMax        float64 // 0 = auto-scale; >0 = fixed full-height reference value
	ShowSeparators bool
	DiskMode       int     // constants.DiskModeAggregate / DiskModeDevices / DiskModeOff
	DiskMax        float64 // 0 = auto-scale; >0 = fixed bytes/sec reference
	DiskAverage    int     // smoothing sample count (like CPUAverage/NetAverage)
	MaxBarsPerRow  int
	SSHOpts        string
	Cluster        string
}

// Default returns a Config with default values.
func Default() Config {
	return Config{
		BarWidth:      1200,
		CPUAverage:    10,
		Extended:      false,
		HasAgent:      false,
		Height:        150,
		MaxWidth:      1900,
		NetAverage:    15,
		NetLink:       "gbit",
		CPUMode:       constants.CPUModeAverage, // start with aggregate bar only
		ShowMem:       false,
		ShowNet:       false,
		DiskMode:      constants.DiskModeOff,
		DiskMax:       0,
		DiskAverage:   10,
		MaxBarsPerRow: 0,
	}
}

// ConfFilePath returns the full path to the config file (~/.loadbarsrc).
func ConfFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, constants.ConfFile), nil
}

// Load reads config from the config file and merges into c. Unknown keys are ignored.
func (c *Config) Load() error {
	path, err := ConfFilePath()
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open config: %w", err)
	}
	defer f.Close()
	return c.parseReader(f)
}

// Write saves the current config to the config file (excluding title).
func (c *Config) Write() error {
	path, err := ConfFilePath()
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create config: %w", err)
	}
	defer f.Close()
	return c.writeTo(f)
}

// GetClusterHosts resolves a cluster name from /etc/clusters into a list of hosts.
func GetClusterHosts(cluster string) ([]string, error) {
	return GetClusterHostsFromFile(cluster, constants.CSSHConfFile)
}

// GetClusterHostsFromFile resolves a cluster from a clusters file (for testing or custom path).
// Supports recursive cluster references with cycle detection.
func GetClusterHostsFromFile(cluster, path string) ([]string, error) {
	return getClusterHostsRec(cluster, path, 1, nil)
}

func (c *Config) parseReader(f *os.File) error {
	validKeys := map[string]bool{
		"title": true, "barwidth": true, "cpuaverage": true, "extended": true,
		"hasagent": true, "height": true, "maxwidth": true, "netaverage": true,
		"netlink": true, "cpumode": true, "showcores": true, "showmem": true,
		"showavgline": true, "showioavgline": true, "shownet": true, "showload": true, "loadmax": true, "showseparators": true,
		"diskmode": true, "diskmax": true, "diskaverage": true,
		"maxbarsperrow": true, "sshopts": true, "cluster": true,
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if !validKeys[key] {
			continue
		}
		c.set(key, val)
	}
	return scanner.Err()
}

// set applies a single key=value pair to the config, delegating to focused helpers.
func (c *Config) set(key, val string) {
	c.setSizeAndTuning(key, val)
	c.setDisplayFlags(key, val)
}

// setSizeAndTuning handles window dimension, SSH, sampling, and identity keys.
func (c *Config) setSizeAndTuning(key, val string) {
	switch key {
	case "title":
		c.Title = val
	case "barwidth":
		if n, err := strconv.Atoi(val); err == nil {
			c.BarWidth = n
		}
	case "cpuaverage":
		if n, err := strconv.Atoi(val); err == nil {
			c.CPUAverage = n
		}
	case "hasagent":
		c.HasAgent = parseBool(val)
	case "height":
		if n, err := strconv.Atoi(val); err == nil {
			c.Height = n
		}
	case "maxwidth":
		if n, err := strconv.Atoi(val); err == nil {
			c.MaxWidth = n
		}
	case "netaverage":
		if n, err := strconv.Atoi(val); err == nil {
			c.NetAverage = n
		}
	case "netlink":
		c.NetLink = val
	case "maxbarsperrow":
		if n, err := strconv.Atoi(val); err == nil {
			c.MaxBarsPerRow = n
		}
	case "sshopts":
		c.SSHOpts = val
	case "cluster":
		c.Cluster = val
	}
}

// setDisplayFlags handles keys that control what is shown and how CPU/load are scaled.
func (c *Config) setDisplayFlags(key, val string) {
	switch key {
	case "extended":
		c.Extended = parseBool(val)
	case "showavgline":
		c.ShowAvgLine = parseBool(val)
	case "showioavgline":
		c.ShowIOAvgLine = parseBool(val)
	case "cpumode":
		// 0=average, 1=cores, 2=off — clamp to valid range
		if n, err := strconv.Atoi(val); err == nil && n >= 0 && n < constants.CPUModeCount {
			c.CPUMode = n
		}
	case "showcores":
		// Backward-compatible: old boolean showcores maps to CPUMode
		if parseBool(val) {
			c.CPUMode = constants.CPUModeCores
		} else {
			c.CPUMode = constants.CPUModeAverage
		}
	case "showmem":
		c.ShowMem = parseBool(val)
	case "shownet":
		c.ShowNet = parseBool(val)
	case "showload":
		c.ShowLoad = parseBool(val)
	case "loadmax":
		// Accept any non-negative float; 0 means auto-scale.
		if f, err := strconv.ParseFloat(val, 64); err == nil && f >= 0 {
			c.LoadMax = f
		}
	case "showseparators":
		c.ShowSeparators = parseBool(val)
	case "diskmode":
		// 0=aggregate, 1=devices, 2=off — clamp to valid range
		if n, err := strconv.Atoi(val); err == nil && n >= 0 && n < constants.DiskModeCount {
			c.DiskMode = n
		}
	case "diskmax":
		// Accept any non-negative float; 0 means auto-scale.
		if f, err := strconv.ParseFloat(val, 64); err == nil && f >= 0 {
			c.DiskMax = f
		}
	case "diskaverage":
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			c.DiskAverage = n
		}
	}
}

func (c *Config) writeTo(f *os.File) error {
	w := bufio.NewWriter(f)
	writeInt := func(key string, v int) { fmt.Fprintf(w, "%s=%d\n", key, v) }
	writeStr := func(key, v string) { fmt.Fprintf(w, "%s=%s\n", key, v) }
	// writeFloat uses %g to strip trailing zeros (e.g. 8 → "8", 8.5 → "8.5").
	writeFloat := func(key string, v float64) { fmt.Fprintf(w, "%s=%g\n", key, v) }
	writeBool := func(key string, v bool) {
		val := "0"
		if v {
			val = "1"
		}
		fmt.Fprintf(w, "%s=%s\n", key, val)
	}
	writeInt("barwidth", c.BarWidth)
	writeInt("cpuaverage", c.CPUAverage)
	writeBool("extended", c.Extended)
	writeBool("hasagent", c.HasAgent)
	writeInt("height", c.Height)
	writeInt("maxwidth", c.MaxWidth)
	writeInt("netaverage", c.NetAverage)
	writeStr("netlink", c.NetLink)
	writeBool("showavgline", c.ShowAvgLine)
	writeBool("showioavgline", c.ShowIOAvgLine)
	writeInt("cpumode", c.CPUMode)
	writeBool("showmem", c.ShowMem)
	writeBool("shownet", c.ShowNet)
	writeBool("showload", c.ShowLoad)
	writeFloat("loadmax", c.LoadMax)
	writeBool("showseparators", c.ShowSeparators)
	writeInt("diskmode", c.DiskMode)
	writeFloat("diskmax", c.DiskMax)
	writeInt("diskaverage", c.DiskAverage)
	writeInt("maxbarsperrow", c.MaxBarsPerRow)
	writeStr("sshopts", c.SSHOpts)
	writeStr("cluster", c.Cluster)
	return w.Flush()
}

func parseBool(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return s == "1" || s == "true" || s == "yes"
}

func getClusterHostsRec(cluster, path string, depth int, seen map[string]bool) ([]string, error) {
	if depth > constants.CSSHMaxRecursion {
		return nil, fmt.Errorf("cluster recursion limit reached in %s (possible cycle)", path)
	}
	if seen == nil {
		seen = make(map[string]bool)
	}
	if seen[cluster] {
		return nil, fmt.Errorf("cluster cycle detected: %s", cluster)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	var line string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		ln := strings.TrimSpace(scanner.Text())
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		fields := strings.Fields(ln)
		if len(fields) >= 1 && fields[0] == cluster {
			if len(fields) > 1 {
				line = strings.Join(fields[1:], " ")
			}
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if line == "" {
		return []string{cluster}, nil
	}

	seen[cluster] = true
	defer delete(seen, cluster)

	var out []string
	for _, part := range strings.Fields(line) {
		hosts, err := getClusterHostsRec(part, path, depth+1, seen)
		if err != nil {
			return nil, err
		}
		out = append(out, hosts...)
	}
	return out, nil
}
