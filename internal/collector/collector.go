package collector

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"codeberg.org/snonux/loadbars/internal/config"
)

// StatsStore is the interface for receiving parsed stats (implemented by app).
type StatsStore interface {
	SetLoadAvg(host, load1, load5, load15 string)
	SetCPU(host, name string, line CPULine)
	SetMem(host, key string, value int64)
	SetNet(host, iface string, net NetLine, stamp float64)
}

// Run starts a collector for one host: runs the remote (or local) script and parses the stream into store.
// Host may be "host" or "host:user". It runs until ctx is cancelled or the command exits.
func Run(ctx context.Context, host string, cfg *config.Config, store StatsStore, scriptPath string) error {
	hostKey, user := splitHostUser(host)
	var scanner *bufio.Scanner
	if isLocal(hostKey) {
		cmd := exec.CommandContext(ctx, "bash", scriptPath)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("%s: %w", hostKey, err)
		}
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("%s: %w", hostKey, err)
		}
		defer cmd.Wait()
		scanner = bufio.NewScanner(stdout)
	} else {
		args := []string{"-o", "StrictHostKeyChecking=no"}
		if cfg.SSHOpts != "" {
			args = append(args, strings.Fields(cfg.SSHOpts)...)
		}
		if user != "" {
			args = append(args, "-l", user)
		}
		args = append(args, hostKey, "bash -s")
		cmd := exec.CommandContext(ctx, "ssh", args...)
		scriptFile, err := os.Open(scriptPath)
		if err != nil {
			return fmt.Errorf("%s: open script: %w", hostKey, err)
		}
		defer scriptFile.Close()
		cmd.Stdin = scriptFile
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("%s: %w", hostKey, err)
		}
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("%s: %w", hostKey, err)
		}
		defer cmd.Wait()
		scanner = bufio.NewScanner(stdout)
	}

	mode := ""
	cpustring := "cpu"
	if !cfg.ShowCores {
		cpustring = "cpu "
	}
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "M ") {
			mode = line
			continue
		}
		switch mode {
		case ModeLoadAvg:
			l := ParseLoadAvg(line)
			store.SetLoadAvg(hostKey, l.Load1, l.Load5, l.Load15)
		case ModeMemStats:
			if mem, ok := ParseMemLine(line); ok {
				store.SetMem(hostKey, mem.Key, mem.Value)
			}
		case ModeNetStats:
			if idx := strings.Index(line, ":"); idx >= 0 {
				iface := strings.TrimSpace(line[:idx])
				rest := line[idx+1:]
				net, err := ParseNetLine(iface + ":" + rest)
				if err != nil {
					continue
				}
				store.SetNet(hostKey, net.Iface, net, float64(time.Now().UnixNano())/1e9)
			}
		case ModeCPUStats:
			if strings.HasPrefix(line, cpustring) {
				cu, err := ParseCPULine(line)
				if err != nil {
					continue
				}
				store.SetCPU(hostKey, cu.Name, cu)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%s: read: %w", hostKey, err)
	}
	return nil
}

// splitHostUser splits "host:user" into (host, user). If no colon, returns (host, "").
func splitHostUser(host string) (h, u string) {
	idx := strings.Index(host, ":")
	if idx < 0 {
		return strings.TrimSpace(host), ""
	}
	return strings.TrimSpace(host[:idx]), strings.TrimSpace(host[idx+1:])
}

func isLocal(h string) bool {
	return h == "localhost" || h == "127.0.0.1"
}


