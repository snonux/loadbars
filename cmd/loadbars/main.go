package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/snonux/loadbars/internal/app"
	"github.com/snonux/loadbars/internal/config"
	"github.com/snonux/loadbars/internal/constants"
	"github.com/snonux/loadbars/internal/version"
)

// cliOptions holds the command-line switches that are not part of config.Config.
type cliOptions struct {
	showHelp bool
	showVer  bool
}

func main() {
	cfg := config.Default()

	// The config file is read first so that command-line flags override it.
	if err := cfg.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "loadbars: config: %v\n", err)
		os.Exit(constants.EUnknown)
	}

	opts, err := parseArgs(os.Args[1:], &cfg, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		opts.showHelp = true
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "loadbars: %v\n", err)
		os.Exit(constants.EUnknown)
	}

	if opts.showVer {
		fmt.Printf("Loadbars %s %s\n", version.Version, constants.Copyright)
		os.Exit(constants.Success)
	}
	if opts.showHelp {
		printUsage()
		os.Exit(constants.Success)
	}

	if cfg.Cluster != "" {
		clusterHosts, err := config.GetClusterHosts(cfg.Cluster)
		if err != nil {
			fmt.Fprintf(os.Stderr, "loadbars: cluster: %v\n", err)
			os.Exit(constants.EUnknown)
		}
		for _, h := range clusterHosts {
			cfg.Hosts = append(cfg.Hosts, normalizeHost(h))
		}
	}

	// No hosts given: run locally without SSH
	if len(cfg.Hosts) == 0 {
		cfg.Hosts = []string{"localhost"}
	}

	if err := app.Run(&cfg); err != nil {
		fmt.Fprintf(os.Stderr, "loadbars: %v\n", err)
		os.Exit(constants.EUnknown)
	}
	os.Exit(constants.Success)
}

// parseArgs parses command-line arguments into cfg. Values already in cfg (defaults
// merged with ~/.loadbarsrc) act as flag defaults, so explicit flags override them.
// Hosts may be given with --hosts and/or as positional arguments, and positional
// hosts may be mixed freely with flags (e.g. "loadbars host1 host2 --showmem").
func parseArgs(args []string, cfg *config.Config, errOut io.Writer) (cliOptions, error) {
	var opts cliOptions
	fs := flag.NewFlagSet("loadbars", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {}

	var hosts string
	var showCores bool
	fs.StringVar(&hosts, "hosts", "", "Comma-separated list of hosts; optional user@ in front")
	fs.StringVar(&cfg.Cluster, "cluster", cfg.Cluster, "Cluster name from /etc/clusters")
	fs.BoolVar(&opts.showHelp, "help", false, "Show usage")
	fs.BoolVar(&opts.showVer, "version", false, "Show version")
	fs.IntVar(&cfg.BarWidth, "barwidth", cfg.BarWidth, "Initial window width")
	fs.IntVar(&cfg.Height, "height", cfg.Height, "Set window height")
	fs.IntVar(&cfg.MaxWidth, "maxwidth", cfg.MaxWidth, "Set max width")
	fs.IntVar(&cfg.CPUAverage, "cpuaverage", cfg.CPUAverage, "Num of CPU samples for avg")
	fs.IntVar(&cfg.NetAverage, "netaverage", cfg.NetAverage, "Num of net samples for avg")
	fs.IntVar(&cfg.DiskAverage, "diskaverage", cfg.DiskAverage, "Num of disk samples for avg")
	fs.StringVar(&cfg.NetLink, "netlink", cfg.NetLink, "Link speed (mbit, 10mbit, 100mbit, gbit, 10gbit or number)")
	fs.IntVar(&cfg.CPUMode, "cpumode", cfg.CPUMode, "CPU display mode (0=average, 1=cores, 2=off)")
	fs.BoolVar(&showCores, "showcores", false, "Show per-core CPU bars (same as --cpumode 1)")
	fs.BoolVar(&cfg.ShowMem, "showmem", cfg.ShowMem, "Toggle mem display")
	fs.BoolVar(&cfg.ShowNet, "shownet", cfg.ShowNet, "Toggle net display")
	fs.BoolVar(&cfg.ShowLoad, "showload", cfg.ShowLoad, "Toggle load average display")
	fs.BoolVar(&cfg.ShowAvgLine, "showavgline", cfg.ShowAvgLine, "Show global CPU average line")
	fs.BoolVar(&cfg.ShowIOAvgLine, "showioavgline", cfg.ShowIOAvgLine, "Show global I/O average line")
	fs.BoolVar(&cfg.ShowSeparators, "showseparators", cfg.ShowSeparators, "Show host separator lines")
	fs.BoolVar(&cfg.Extended, "extended", cfg.Extended, "Toggle extended display")
	fs.StringVar(&cfg.Title, "title", cfg.Title, "Set title bar text")
	fs.StringVar(&cfg.SSHOpts, "sshopts", cfg.SSHOpts, "Set SSH options")
	fs.BoolVar(&cfg.HasAgent, "hasagent", cfg.HasAgent, "SSH key already known by agent")
	fs.IntVar(&cfg.MaxBarsPerRow, "maxbarsperrow", cfg.MaxBarsPerRow, "Max bars per row (0=unlimited)")
	fs.IntVar(&cfg.DiskMode, "diskmode", cfg.DiskMode, "Disk display mode (0=aggregate, 1=devices, 2=off)")
	fs.Float64Var(&cfg.DiskMax, "diskmax", cfg.DiskMax, "Fixed disk bar full-height reference in bytes/sec (0 = auto-scale)")
	fs.Float64Var(&cfg.LoadMax, "loadmax", cfg.LoadMax, "Fixed load bar full-height reference value (0 = auto-scale)")

	// The flag package stops at the first non-flag argument, so parse repeatedly and
	// collect positional hosts in between. A bare "--" ends flag parsing.
	var positional []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return opts, err
		}
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		if rest[0] == "--" {
			positional = append(positional, rest[1:]...)
			break
		}
		positional = append(positional, rest[0])
		rest = rest[1:]
	}

	if showCores {
		cfg.CPUMode = constants.CPUModeCores
	}
	for _, h := range strings.Split(hosts, ",") {
		if h = strings.TrimSpace(h); h != "" {
			cfg.Hosts = append(cfg.Hosts, normalizeHost(h))
		}
	}
	for _, h := range positional {
		if h = strings.TrimSpace(h); h != "" {
			cfg.Hosts = append(cfg.Hosts, normalizeHost(h))
		}
	}
	return opts, validate(cfg)
}

// validate rejects option values the display cannot handle.
func validate(cfg *config.Config) error {
	switch {
	case cfg.CPUMode < 0 || cfg.CPUMode >= constants.CPUModeCount:
		return fmt.Errorf("--cpumode must be 0, 1 or 2 (got %d)", cfg.CPUMode)
	case cfg.DiskMode < 0 || cfg.DiskMode >= constants.DiskModeCount:
		return fmt.Errorf("--diskmode must be 0, 1 or 2 (got %d)", cfg.DiskMode)
	case cfg.CPUAverage < 1 || cfg.NetAverage < 1 || cfg.DiskAverage < 1:
		return errors.New("--cpuaverage, --netaverage and --diskaverage must be at least 1")
	case cfg.LoadMax < 0 || cfg.DiskMax < 0:
		return errors.New("--loadmax and --diskmax must not be negative")
	case cfg.MaxBarsPerRow < 0:
		return errors.New("--maxbarsperrow must not be negative")
	}
	return nil
}

// normalizeHost converts "user@host" to "host:user", or returns "host" if no user.
func normalizeHost(h string) string {
	h = strings.TrimSpace(h)
	if idx := strings.Index(h, "@"); idx >= 0 {
		user, host := strings.TrimSpace(h[:idx]), strings.TrimSpace(h[idx+1:])
		return host + ":" + user
	}
	return h
}

// printUsage prints usage and options to stderr.
func printUsage() {
	fmt.Fprintf(os.Stderr, `Loadbars %s - real-time server load monitoring

Usage: loadbars [HOSTS...] [OPTIONS]

Hosts:
  --hosts <list>          Comma-separated hosts (optional user@host); hosts can
                          also be given as positional arguments
  --cluster <name>        Cluster from %s (ClusterSSH format)

What to show:
  --cpumode <n>           CPU bars: 0=aggregate (default), 1=per-core, 2=off
  --showcores             Same as --cpumode 1
  --showmem               Show memory bars (RAM left, swap right)
  --shownet               Show network bars (all non-lo interfaces summed)
  --showload              Show load average bars
  --diskmode <n>          Disk I/O bars: 0=aggregate, 1=per-device, 2=off (default)
  --extended              Extended display (CPU peak line, disk utilisation line)
  --showavgline           Global CPU average line across all hosts
  --showioavgline         Global I/O (iowait+IRQ) average line across all hosts
  --showseparators        Separator lines between hosts

Scaling and smoothing:
  --netlink <speed>       Link speed for net %% (mbit, 10mbit, 100mbit, gbit,
                          10gbit or bytes/sec). Default: gbit
  --loadmax <n>           Fixed full-height load value (0 = auto-scale)
  --diskmax <n>           Fixed full-height disk bytes/sec (0 = auto-scale)
  --cpuaverage <n>        CPU samples to average (default 10)
  --netaverage <n>        Net samples to average (default 15)
  --diskaverage <n>       Disk samples to average (default 10)

Window:
  --barwidth <n>          Initial window width (default 1200, min 800)
  --height <n>            Window height (default 150)
  --maxwidth <n>          Maximum window width (default 1900)
  --maxbarsperrow <n>     Wrap bars into rows of at most n bars (0 = one row)
  --title <text>          Window title

SSH:
  --sshopts <opts>        Extra options for ssh (e.g. "-o ConnectTimeout=5")
  --hasagent              Accepted for compatibility; has no effect

  --help                  This help
  --version               Print version

All options can also be set in ~/.loadbarsrc as key=value (no leading --);
command-line flags override the file. Press 'h' in the window for hotkeys.
`, version.Version, constants.CSSHConfFile)
}
