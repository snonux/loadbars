package collector

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/snonux/loadbars/internal/config"
)

// StatsStore is the interface for receiving parsed stats (implemented by app).
type StatsStore interface {
	SetLoadAvg(host, load1, load5, load15 string)
	SetCPU(host, name string, line CPULine)
	SetMem(host, key string, value int64)
	SetNet(host, iface string, net NetLine, stamp float64)
	SetDisk(host, device string, disk DiskLine, stamp float64)
}

// Run starts a collector for one host: runs the embedded remote script (local or over SSH)
// and parses the output stream into store. Host may be "host" or "host:user".
// It runs until ctx is cancelled or the command exits.
func Run(ctx context.Context, host string, cfg *config.Config, store StatsStore) error {
	hostKey, user := splitHostUser(host)

	// Select script: only Linux is supported for local monitoring.
	scriptBytes := LinuxScript
	if isLocal(hostKey) {
		scriptBytes = getLocalScript()
		if scriptBytes == nil {
			return fmt.Errorf("%s: local stats gathering requires Linux with /proc filesystem", hostKey)
		}
	}

	var (
		scanner *bufio.Scanner
		cmd     *exec.Cmd
		err     error
	)
	if isLocal(hostKey) {
		scanner, cmd, err = startLocalScanner(ctx, scriptBytes)
	} else {
		scanner, cmd, err = startRemoteScanner(ctx, hostKey, user, scriptBytes, cfg)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", hostKey, err)
	}
	defer cmd.Wait()

	return parseCollectorStream(ctx, scanner, hostKey, store)
}

// startLocalScanner spawns "bash -s" with scriptBytes on stdin and returns a scanner
// over its stdout along with the Cmd for deferred Wait.
func startLocalScanner(ctx context.Context, scriptBytes []byte) (*bufio.Scanner, *exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "bash", "-s")
	cmd.Stdin = bytes.NewReader(scriptBytes)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	return bufio.NewScanner(stdout), cmd, nil
}

// startRemoteScanner spawns "ssh host bash -s" with scriptBytes on stdin and returns
// a scanner over its stdout along with the Cmd for deferred Wait.
func startRemoteScanner(ctx context.Context, hostKey, user string, scriptBytes []byte, cfg *config.Config) (*bufio.Scanner, *exec.Cmd, error) {
	args := []string{"-o", "StrictHostKeyChecking=no"}
	if cfg.SSHOpts != "" {
		args = append(args, strings.Fields(cfg.SSHOpts)...)
	}
	if user != "" {
		args = append(args, "-l", user)
	}
	args = append(args, hostKey, "bash -s")
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stdin = bytes.NewReader(scriptBytes)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	return bufio.NewScanner(stdout), cmd, nil
}

// parseCollectorStream reads lines from scanner and dispatches parsed stats to store.
// Always collects all CPU lines (cpu, cpu0, cpu1, ...) so display can toggle per-core
// view with key 1 without a reconnect.
func parseCollectorStream(ctx context.Context, scanner *bufio.Scanner, hostKey string, store StatsStore) error {
	mode := ""
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
		dispatchCollectorLine(mode, line, hostKey, store)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%s: read: %w", hostKey, err)
	}
	return nil
}

// dispatchCollectorLine routes one parsed line to the appropriate store setter
// based on the current protocol mode marker.
func dispatchCollectorLine(mode, line, hostKey string, store StatsStore) {
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
			net, err := ParseNetLine(iface + ":" + line[idx+1:])
			if err == nil {
				store.SetNet(hostKey, net.Iface, net, float64(time.Now().UnixNano())/1e9)
			}
		}
	case ModeDiskStats:
		if d, err := ParseDiskLine(line); err == nil {
			store.SetDisk(hostKey, d.Device, d, float64(time.Now().UnixNano())/1e9)
		}
	case ModeCPUStats:
		if strings.HasPrefix(line, "cpu") {
			if cu, err := ParseCPULine(line); err == nil {
				store.SetCPU(hostKey, cu.Name, cu)
			}
		}
	}
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

// getLocalScript returns the appropriate script for the local OS
func getLocalScript() []byte {
	// Check if /proc exists (Linux/Unix).
	if st, err := os.Stat("/proc"); err == nil && st.IsDir() {
		return LinuxScript
	}
	// /proc not found - unsupported OS for local stats gathering
	return nil
}
