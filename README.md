# loadbars - A small and humble tool to observe server loads

## Description

Loadbars is a tool that can be used to observe CPU loads of several remote servers at once in real time. It connects with SSH (using SSH public/private key auth) to several servers at once and vizualizes all server CPUs and memory statistics right next each other (either summarized or each core separately). Loadbars is not a tool for collecting CPU loads and drawing graphs for later analysis. However, since such tools require a significant amount of time before producing results, Loadbars lets you observe the current state immediately. Loadbars does not remember or record any load information. It just shows the current CPU usages like top or vmstat does.

![Loadbars](loadbars.gif)

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

### A few examples however

```bash
loadbars --extended 1 --showcores 1 --height 300 --hosts localhost

loadbars --hosts localhost,server1.example.com,server2.example.com

loadbars --cluster foocluster (foocluster is in /etc/clusters [ClusterSSH])
```

### More examples, using shell expansion

```bash
loadbars servername{01,02,03}.example.com

loadbars servername{01..50}.example.com --showcores 1
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
| **1** | Toggle CPU cores (one bar per core vs one aggregate bar per host) |
| **2** | Toggle memory bars (RAM left, Swap right per host) |
| **3** | Toggle network bars (RX/TX per host); stdout shows which interface is used |
| **e** | Toggle extended display (1px peak line on CPU bars: max system+user over last samples) |
| **h** | Print hotkey list to stdout |
| **n** | Cycle to next network interface (per host); only when net bars are shown |
| **q** | Quit |
| **w** | Write current settings to ~/.loadbarsrc |
| **a** | Increase CPU average samples (affects extended peak history length) |
| **y** | Decrease CPU average samples (min 1) |
| **d** | Increase net average samples |
| **c** | Decrease net average samples (min 1) |
| **f** | Link scale up (net utilization reference) |
| **v** | Link scale down (net utilization reference) |
| **Arrow keys** | Resize window (left/right: width, up/down: height) |

### CPU stuff

- `st` = Steal in % [see man proc] (extended), Color: Red
- `gt` = Guest in % [see man proc] (extended), Color: Red
- `sr` = Soft IRQ usage in % (extended), Color: White
- `ir` = IRQ usage in % (extended), Color: White
- `io` = IOwait cpu sage in %, Color: Purple
- `id` = Idle cpu usage in % (extended), Color: Black
- `ni` = Nice cpu usage in %, Color: Green
- `us` = User cpu usage in %, Color: Yellow, dark yellow if to>50%, orange if to>50%
- `sy` = System cpu sage in %, Color: Blue, lighter blue if >30%
- `to` = Total CPU usage, which is (100% - id)
- `pk` = Max us+sy peak of last avg. samples (extended)
- 1px horizontal line: Maximum sy+us+io of last 'avg' samples (extended)

### Memory stuff

- `Ram` = System ram usage in %, Color: Dark grey
- `Swp` = System swap usage in %, Color: Grey

### Network stuff

- `Rxb` = Incoming (received) traffic in %, Color: Light green, normal green if >100% while using low netlink reference. Bar comes from top and is half width.
- `Txb` = Outgoing (transmitted) traffic in %, Color: Light green, normal green if >100% while using low netlink reference. Bar comes from bottom and is half width.

When network bar is red: The interface does not exist on the specific remote host.

**Choosing the network interface:** By default loadbars uses the first non-loopback interface (e.g. `eth0`, `enp0s3`). If stats never change, you may be watching the wrong interface (e.g. `docker0` with little traffic). Set the interface explicitly:

- In **~/.loadbarsrc**: `netint=eth0` (use the name from `/proc/net/dev` or `ip link`)
- On the command line: `loadbars --netint eth0 --hosts localhost`
- In-app: press **3** to show net bars (stdout will print which interface is used), then **n** to cycle to the next interface.

**Link speed** (`netlink`): Used to compute utilization %. Default is `gbit`. Set e.g. `netlink=100mbit` or `netlink=10gbit` in ~/.loadbarsrc or `--netlink 100mbit`.

#### Config file support

Loadbars tries to read ~/.loadbarsrc and it's possible to configure any option you find in --help but without leading '--'. For comments just use the '#' sign. Sample config:

```
showcores=1   # Always show cores on startup
netint=eth0   # Interface for network bars (optional)
netlink=gbit  # Link speed for utilization % (optional)
```

will always show all CPU cores. If you press the 'w' hotkey during program execution your config file will be overwritten using the current settings.

## License

See package description or project website.

The Go build of loadbars links to **go-sdl2** (github.com/veandco/go-sdl2), which is licensed under the **BSD-3-Clause** license. That license is compatible with loadbars' use and does not impose additional restrictions on distribution. The full copyright notice and license text for go-sdl2 are in the [LICENSE](LICENSE) file.

## Author

Paul Buetow - <http://paul.buetow.org>
