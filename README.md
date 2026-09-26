# loadbars - A small and humble tool to observe server loads

## Description

Loadbars is a tool that can be used to observe CPU loads of several remote servers at once in real time. It connects with SSH (using SSH public/private key auth) to several servers at once and vizualizes all server CPUs and memory statistics right next each other (either summarized or each core separately). Loadbars is not a tool for collecting CPU loads and drawing graphs for later analysis. However, since such tools require a significant amount of time before producing results, Loadbars lets you observe the current state immediately. Loadbars does not remember or record any load information. It just shows the current CPU usages like top or vmstat does.

![Loadbars](loadbars.gif)

**New to loadbars? Read the [usage guide](docs/usage-guide.md)**: every feature explained with an animated GIF, including how to watch many remote servers at once.

### Tested platforms

This version of loadbars has been tested on:
- Fedora Linux 43 and most modern Linux distributions (RHEL, CentOS, Ubuntu, Debian, etc.)
- macOS (Darwin) - can connect to remote Linux servers via SSH (local monitoring not supported)

**Note:** Local monitoring requires Linux with /proc filesystem. Remote hosts must be Linux (using /proc filesystem). macOS can be used as a client to monitor remote Linux servers.

## Build and run

### SDL2 Dependencies

Loadbars requires SDL2 for the display window. Install it for your platform:

#### Fedora Linux / RHEL / CentOS

```bash
sudo dnf install SDL2-devel
```

#### macOS

```bash
brew install sdl2
```

### Using Mage (recommended)

Build the binary:

```bash
mage build
./loadbars --hosts localhost
```

Install to GOPATH/bin (e.g. ~/go/bin):

```bash
mage install
```

Run tests:

```bash
mage test
```

### I like flying elephants

For any startup params help check out `--help` on command line or `h` during program
execution.

### Command-line flags

All options can also be set in `~/.loadbarsrc` (key=value, no leading `--`). CLI flags override the config file.

| Flag | Description | Default |
|------|-------------|---------|
| `--hosts <list>` | Comma-separated list of hosts; optional `user@` in front (e.g. `root@server1,server2`) | (none; default hosts: localhost) |
| `--cluster <name>` | Cluster name from `/etc/clusters` (ClusterSSH-style); hosts are read from that file | (none) |
| `--barwidth <n>` | Initial window width in pixels | 1200 |
| `--height <n>` | Window height in pixels | 150 |
| `--maxwidth <n>` | Maximum window width in pixels | 1900 |
| `--cpuaverage <n>` | Number of CPU samples used for average (and extended peak history) | 10 |
| `--netaverage <n>` | Number of network samples used for average | 15 |
| `--netlink <speed>` | Link speed for network utilization %: `mbit`, `10mbit`, `100mbit`, `gbit`, `10gbit` or a number of Mbit/s | gbit |
| `--cpumode <n>` | CPU display mode: 0 = aggregate bar, 1 = per-core bars, 2 = off | 0 |
| `--showcores` | Same as `--cpumode 1` | off |
| `--showmem` | Show memory bars (RAM left, Swap right per host) | off |
| `--shownet` | Show network bars (RX/TX across non-lo interfaces per host) | off |
| `--extended` | Show extended display (1px peak line on CPU bars, utilization line on disk bars) | off |
| `--showavgline` | Show the global CPU average line across all hosts | off |
| `--showioavgline` | Show the global I/O (iowait+IRQ) average line across all hosts | off |
| `--showseparators` | Show separator lines between hosts | off |
| `--title <text>` | Set title bar text | (empty) |
| `--sshopts <opts>` | Extra SSH options passed to `ssh` (e.g. `-o ConnectTimeout=5`) | (empty) |
| `--hasagent` | Accepted for compatibility with older versions; has no effect | off |
| `--showload` | Show load average bars (1-min teal fill, 5-min yellow line, 15-min white line per host) | off |
| `--loadmax <n>` | Fix the load bar full-height reference to `n` (e.g. `8` = core count); 0 = auto-scale | 0 |
| `--diskmode <n>` | Disk I/O display mode: 0=aggregate, 1=per-device, 2=off | 2 (off) |
| `--diskmax <n>` | Fix the disk bar full-height reference to `n` bytes/sec; 0 = auto-scale | 0 |
| `--diskaverage <n>` | Number of disk samples used for average | 10 |
| `--maxbarsperrow <n>` | Max bars per row; 0 = unlimited (single row) | 0 |
| `--help` | Show usage and exit | — |
| `--version` | Print version and exit | — |

Hosts can also be given as positional arguments, mixed freely with flags: `loadbars server1 server2 --showcores`.

Boolean flags take no value (`--showmem`); use `--showmem=false` to switch off something enabled in `~/.loadbarsrc`.

### A few examples however

```bash
loadbars --extended --showcores --height 300 --hosts localhost

loadbars --hosts localhost,server1.example.com,server2.example.com

loadbars --cluster foocluster (foocluster is in /etc/clusters [ClusterSSH])
```

### More examples, using shell expansion

```bash
loadbars servername{01,02,03}.example.com

loadbars servername{01..50}.example.com --showcores
```


### Running from Source

To run loadbars directly from the source directory:

```bash
./loadbars --hosts localhost
```

Or with remote servers:

```bash
./loadbars --hosts root@server1,root@server2 --showcores 1
```

### SSH Configuration

Loadbars requires SSH public/private key authentication. Make sure:

- You have SSH keys set up (~/.ssh/id_rsa or similar)
- Your public key is in ~/.ssh/authorized_keys on remote servers
- SSH agent is running (ssh-agent), or passwordless keys are configured

## More usage

### Hotkeys

Press these keys while loadbars is running (see also `h` for a short list on stdout):

| Key | Action |
|-----|--------|
| **1** | Toggle CPU display mode: aggregate bar → per-core bars → off → aggregate |
| **2** / **m** | Toggle memory bars (RAM left, Swap right per host) |
| **3** / **n** | Toggle network bars (RX/TX summed across all non-lo interfaces per host) |
| **4** / **l** | Toggle load average bars (1-min teal fill, 5-min yellow line, 15-min white line) |
| **5** | Toggle disk I/O bars: aggregate (all devices) → per-device → off → aggregate |
| **r** | Reset load auto-scale peak to floor (2.0) and disk auto-scale peak to floor (1 MB/s) — no effect when `loadmax`/`diskmax` is fixed |
| **e** | Toggle extended display (1px peak line on CPU bars: max system+user over last samples) |
| **g** | Toggle global average CPU line (1px red line showing mean CPU usage across all hosts) |
| **i** | Toggle global I/O average line (1px pink line showing mean iowait+IRQ across all hosts) |
| **s** | Toggle host separator lines (1px red vertical line between hosts) |
| **h** | Print hotkey list to stdout |
| **q** | Quit |
| **w** | Write current settings (including the current window size) to ~/.loadbarsrc |
| **a** | Increase CPU average samples (affects extended peak history length) |
| **y** | Decrease CPU average samples (min 1) |
| **d** | Increase net average samples |
| **c** | Decrease net average samples (min 1) |
| **b** | Increase disk average samples |
| **x** | Decrease disk average samples (min 1) |
| **f** | Link scale up (net utilization reference) |
| **v** | Link scale down (net utilization reference) |
| **Arrow keys** | Resize window (left/right: width, up/down: height) |

### CPU stuff

Each CPU bar is a stack filled from the bottom (see the [usage guide](docs/usage-guide.md#2-cpu-bars) for pictures):

- `sy` = System CPU usage in %, Color: Blue
- `us` = User CPU usage in %, Color: Yellow
- `ni` = Nice CPU usage in %, Color: Green (guest nice: lime green)
- `id` = Idle CPU in %, Color: Black
- `io` = IOwait CPU usage in %, Color: Purple
- `ir`/`sr` = IRQ and soft IRQ usage in %, Color: White
- `gt`/`st` = Guest and steal in % [see man proc], Color: Red
- Extended mode (`e`): 1px horizontal line at the maximum `us`+`sy` of the last `cpuaverage` samples; yellow, dark yellow above 50%, orange above 70%

### Memory stuff

- `Ram` = System ram usage in %, Color: Dark grey
- `Swp` = System swap usage in %, Color: Grey

### Network stuff

- `Rxb` = Incoming (received) traffic in % of the `netlink` reference, Color: Light green. Left half of the bar, grows down from the top.
- `Txb` = Outgoing (transmitted) traffic in % of the `netlink` reference, Color: Light green. Right half of the bar, grows up from the bottom.

When network bar is red: No non-loopback interface exists on the specific remote host.

### Load average stuff

- **Teal fill** from top downward: 1-minute load average, height proportional to the scale reference.
- **Yellow 1px line**: 5-minute load average. Appears inside the fill when load is rising, below it when falling.
- **White 1px line**: 15-minute load average. Same direction convention as the 5-min line.
- **Scale reference** (shown in hover tooltip):
  - *Auto-scale* (`loadmax=0`, default): the reference tracks the global maximum 1-min load across all hosts, decaying slowly over time (floor 2.0). Press **r** to reset it back to 2.0 immediately.
  - *Fixed scale* (`loadmax=N`): the bar's full height always equals `N`. Useful when you know the core count and want a stable reference across sessions. Tooltip shows `Max:` instead of `Peak:`.

### Disk I/O stuff

- **Purple fill** from top downward: read throughput as % of the peak/max reference.
- **Darker purple fill** from bottom upward: write throughput as % of the peak/max reference.
- Loop, ram, zram and device-mapper devices are ignored.
- In **extended mode** (`e` key), a 3px light-red line shows disk utilization % (fraction of time the device had I/O in progress).
- **Aggregate mode** (`5` key, first press): one bar per host summing all whole-disk devices. Partitions (`sda1`, `nvme0n1p1`) are excluded.
- **Per-device mode** (`5` key, second press): one bar per whole-disk device per host.
- **Scale reference**: auto-scales based on observed peak throughput (floor 1 MB/s), or fixed via `diskmax=N` (bytes/sec).
- **Config keys**: `diskmode` (0/1/2), `diskmax` (bytes/sec), `diskaverage` (smoothing samples).

**Aggregated interfaces:** Loadbars sums RX/TX across all non-loopback interfaces (e.g. `eth0`, `wlan0`, `enp0s3`) and shows the combined total. Loopback (`lo`) is always excluded.

**Link speed** (`netlink`): Used to compute utilization %. Default is `gbit`. Set e.g. `netlink=100mbit` or `netlink=10gbit` in ~/.loadbarsrc or `--netlink 100mbit`.

#### Config file support

Loadbars tries to read ~/.loadbarsrc and it's possible to configure any option you find in --help but without leading '--'. For comments just use the '#' sign. Sample config:

```
showcores=1   # Always show cores on startup
netlink=gbit  # Link speed for utilization % (optional)
```

will always show all CPU cores. If you press the 'w' hotkey during program execution your config file will be overwritten using the current settings.

## License

See package description or project website.

The Go build of loadbars links to **go-sdl2** (github.com/veandco/go-sdl2), which is licensed under the **BSD-3-Clause** license. That license is compatible with loadbars' use and does not impose additional restrictions on distribution. The full copyright notice and license text for go-sdl2 are in the [LICENSE](LICENSE) file.

## Author

Paul Buetow - <http://paul.buetow.org>
