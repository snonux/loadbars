package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"codeberg.org/snonux/loadbars/internal/app"
	"codeberg.org/snonux/loadbars/internal/config"
	"codeberg.org/snonux/loadbars/internal/constants"
	"codeberg.org/snonux/loadbars/internal/version"
)

func main() {
	cfg := config.Default()

	var (
		hosts    = flag.String("hosts", "", "Comma-separated list of hosts; optional user@ in front")
		cluster  = flag.String("cluster", "", "Cluster name from /etc/clusters")
		showHelp = flag.Bool("help", false, "Show usage")
		showVer  = flag.Bool("version", false, "Show version")
	)
	flag.IntVar(&cfg.BarWidth, "barwidth", cfg.BarWidth, "Set bar width")
	flag.IntVar(&cfg.Height, "height", cfg.Height, "Set window height")
	flag.IntVar(&cfg.MaxWidth, "maxwidth", cfg.MaxWidth, "Set max width")
	flag.IntVar(&cfg.CPUAverage, "cpuaverage", cfg.CPUAverage, "Num of CPU samples for avg")
	flag.IntVar(&cfg.NetAverage, "netaverage", cfg.NetAverage, "Num of net samples for avg")
	flag.StringVar(&cfg.NetInt, "netint", cfg.NetInt, "Interface to show netstats for")
	flag.StringVar(&cfg.NetLink, "netlink", cfg.NetLink, "Link speed (mbit, 10mbit, 100mbit, gbit, 10gbit or number)")
	flag.BoolVar(&cfg.ShowCores, "showcores", cfg.ShowCores, "Toggle core display")
	flag.BoolVar(&cfg.ShowMem, "showmem", cfg.ShowMem, "Toggle mem display")
	flag.BoolVar(&cfg.ShowNet, "shownet", cfg.ShowNet, "Toggle net display")
	flag.BoolVar(&cfg.Extended, "extended", cfg.Extended, "Toggle extended display")
	flag.StringVar(&cfg.Title, "title", cfg.Title, "Set title bar text")
	flag.StringVar(&cfg.SSHOpts, "sshopts", cfg.SSHOpts, "Set SSH options")
	flag.BoolVar(&cfg.HasAgent, "hasagent", cfg.HasAgent, "SSH key already known by agent")

	flag.Parse()

	if *showVer {
		fmt.Printf("Loadbars %s %s\n", version.Version, constants.Copyright)
		os.Exit(constants.Success)
	}

	if err := cfg.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "loadbars: config: %v\n", err)
		os.Exit(constants.EUnknown)
	}

	// Flags override config file; re-parse into cfg for hosts/cluster
	if *cluster != "" {
		cfg.Cluster = *cluster
	}
	if *hosts != "" {
		// Hosts from flag: optional user@host → we keep "user:host" or "host"
		for _, h := range strings.Split(*hosts, ",") {
			h = strings.TrimSpace(h)
			if h == "" {
				continue
			}
			cfg.Hosts = append(cfg.Hosts, normalizeHost(h))
		}
	}
	// Hosts from positional args (before first flag-like arg)
	for i := 0; i < flag.NArg(); i++ {
		arg := flag.Arg(i)
		if strings.HasPrefix(arg, "-") {
			break
		}
		cfg.Hosts = append(cfg.Hosts, normalizeHost(arg))
	}
	if cfg.Cluster != "" {
		clusterHosts, err := config.GetClusterHosts(cfg.Cluster)
		if err != nil {
			fmt.Fprintf(os.Stderr, "loadbars: cluster: %v\n", err)
			os.Exit(constants.EUnknown)
		}
		cfg.Hosts = append(cfg.Hosts, clusterHosts...)
	}

	if *showHelp {
		printUsage()
		os.Exit(constants.Success)
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

Options:
  --hosts <list>     Comma-separated hosts (optional user@host)
  --cluster <name>   Cluster from %s
  --barwidth <n>     Initial window width (default 1200)
  --height <n>       Window height (default 150)
  --showcores        Show per-CPU bars
  --showmem          Show memory bars
  --shownet          Show network bars
  --netint <iface>   Network interface for net bars (e.g. eth0, enp0s3).
                     Default: first non-lo. Press 'n' in-app to cycle.
  --netlink <speed>  Link speed for %% (gbit, 10gbit, mbit, 100mbit, or number).
                     Default: gbit.
  --extended         Extended display (peak line)
  --help             This help
  --version          Print version

Config: netint and netlink can be set in ~/.loadbarsrc. Press '3' then see
stdout for which interface is used; press 'n' to cycle. Press 'h' for hotkeys.
`, version.Version, constants.CSSHConfFile)
}
