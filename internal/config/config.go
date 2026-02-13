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
	Hosts      []string // Each entry is "host" or "host:user"
	Title      string
	BarWidth   int
	CPUAverage int
	Extended   bool
	HasAgent   bool
	Height     int
	MaxWidth   int
	NetAverage int
	NetInt     string
	NetLink    string
	ShowCores  bool
	ShowMem    bool
	ShowNet    bool
	SSHOpts    string
	Cluster    string
}

// Default returns a Config with default values.
func Default() Config {
	return Config{
		BarWidth:   20,
		CPUAverage: 10,
		Extended:   false,
		HasAgent:   false,
		Height:    150,
		MaxWidth:  1900,
		NetAverage: 15,
		NetLink:   "gbit",
		ShowCores: false,
		ShowMem:   false,
		ShowNet:   false,
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

func (c *Config) parseReader(f *os.File) error {
	validKeys := map[string]bool{
		"title": true, "barwidth": true, "cpuaverage": true, "extended": true,
		"hasagent": true, "height": true, "maxwidth": true, "netaverage": true,
		"netint": true, "netlink": true, "showcores": true, "showmem": true,
		"shownet": true, "sshopts": true, "cluster": true,
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

func (c *Config) set(key, val string) {
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
	case "extended":
		c.Extended = parseBool(val)
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
	case "netint":
		c.NetInt = val
	case "netlink":
		c.NetLink = val
	case "showcores":
		c.ShowCores = parseBool(val)
	case "showmem":
		c.ShowMem = parseBool(val)
	case "shownet":
		c.ShowNet = parseBool(val)
	case "sshopts":
		c.SSHOpts = val
	case "cluster":
		c.Cluster = val
	}
}

func parseBool(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return s == "1" || s == "true" || s == "yes"
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

func (c *Config) writeTo(f *os.File) error {
	w := bufio.NewWriter(f)
	writeInt := func(key string, v int) { fmt.Fprintf(w, "%s=%d\n", key, v) }
	writeStr := func(key, v string) { fmt.Fprintf(w, "%s=%s\n", key, v) }
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
	writeStr("netint", c.NetInt)
	writeStr("netlink", c.NetLink)
	writeBool("showcores", c.ShowCores)
	writeBool("showmem", c.ShowMem)
	writeBool("shownet", c.ShowNet)
	writeStr("sshopts", c.SSHOpts)
	writeStr("cluster", c.Cluster)
	return w.Flush()
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
