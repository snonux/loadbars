# loadbars - A small and humble tool to observe server loads

## Synopsis

```
loadbars [LIST OF HOSTNAMES] [OPTIONS]
```

### Tested platforms

This version of loadbars has been tested on Fedora Linux 43 and should work on
most modern Linux distributions (RHEL, CentOS, Ubuntu, Debian, etc.).

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

## Description

Loadbars is a tool that can be used to observe CPU loads of several remote servers at once in real time. It connects with SSH (using SSH public/private key auth) to several servers at once and vizualizes all server CPUs and memory statistics right next each other (either summarized or each core separately). Loadbars is not a tool for collecting CPU loads and drawing graphs for later analysis. However, since such tools require a significant amount of time before producing results, Loadbars lets you observe the current state immediately. Loadbars does not remember or record any load information. It just shows the current CPU usages like top or vmstat does.

## Build and run

Build the binary:

```bash
go build -o loadbars ./cmd/loadbars
./loadbars --hosts localhost
```

Or use [mage](https://magefile.org): `mage build` (default), `mage test`, `mage install` (set `DESTDIR` for install path), `mage uninstall` / `mage deinstall`.

Remote hosts need no Go: the binary pipes `scripts/loadbars-remote.sh` over SSH.

## Installation

### Dependencies (Fedora/RHEL/CentOS)

To run loadbars on Fedora Linux, you need the install the SDL2 development libraries:

```bash
sudo dnf install SDL2-devel
```

On Ubuntu/Debian:

```bash
sudo apt install libsdl2-dev
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

## Info

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

#### Config file support

Loadbars tries to read ~/.loadbarsrc and it's possible to configure any option you find in --help but without leading '--'. For comments just use the '#' sign. Sample config:

```
showcores=1 # Always show cores on startup
```

will always show all CPU cores. If you press the 'w' hotkey during program execution your config file will be overwritten using the current settings.

## License

See package description or project website.

The Go build of loadbars links to **go-sdl2** (github.com/veandco/go-sdl2), which is licensed under the **BSD-3-Clause** license. That license is compatible with loadbars' use and does not impose additional restrictions on distribution. The full copyright notice and license text for go-sdl2 are in the [NOTICE](NOTICE) file.

## Author

Paul Buetow - <http://paul.buetow.org>
