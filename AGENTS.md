# AGENTS.md – Guidance for AI coding agents

This file helps AI assistants (Cursor, Claude, etc.) work effectively in this repository.

## Project overview

**Loadbars** is a real-time server load monitoring tool. It connects to one or more hosts via SSH (or runs locally) and shows CPU, memory, and network usage as vertical colored bars in an SDL window. It does not record or graph history; it shows current state only (like `top` or `vmstat`).

- **Repo:** codeberg.org/snonux/loadbars  
- **Display:** SDL2 (go-sdl2). No in-window text/fonts; all bars are drawn with filled rectangles. User feedback (hotkeys, which interface, etc.) is printed to stdout.

## Tech stack

- **Go 1.25+**
- **github.com/veandco/go-sdl2** – SDL2 bindings for window and 2D rendering
- **Build/tasks:** Mage (`mage build`, `mage test`, `mage install`)

Remote hosts do not need Go: the client embeds the remote script in the binary and runs it via `bash -s` (stdin) locally or over SSH. No separate script file; install only the binary. Remotes need bash and `/proc` (Linux).

## Layout

```
cmd/loadbars/          # Entry point: flags, config load, app.Run()
internal/
  app/                 # App lifecycle, store, wires collector + display
  collector/           # Runs embedded script (local or ssh via bash -s), parses M LOADAVG / M MEMSTATS / M NETSTATS / M CPUSTATS
  config/              # Config struct, ~/.loadbarsrc load/save, cluster from /etc/clusters
  constants/           # Intervals, colors (RGB), link-speed constants
  display/             # SDL window, event loop, drawing (CPU/mem/net bars, hotkeys)
  stats/               # HostStats, Snapshot, NetStamp; read by display
  version/             # Version string (e.g. "0.8.0") – used in title bar and --version
scripts/
  loadbars-remote.sh   # Source copy; embedded into binary at build (internal/collector/scriptdata/)
```

- **Version:** Set in `internal/version/version.go`. Shown in window title and `--version`.
- **Config:** `internal/config`: `~/.loadbarsrc` (key=value), CLI flags override. Written on hotkey `w`.

## Commands

| Command           | Purpose                    |
|-------------------|----------------------------|
| `mage build`      | Build `loadbars` binary     |
| `mage test`       | Run Go tests               |
| `mage install`    | Copy binary to GOPATH/bin (~/go/bin) |
| `go build -o loadbars ./cmd/loadbars` | Build without Mage |
| `./loadbars --hosts localhost` | Run locally (no SSH)  |

Default when no hosts are given: `localhost`. No SSH required for local use.

## Display and hotkeys

- **Display** (`internal/display`): One loop – poll events, snapshot from store, count bars, clear only when layout/size changes, draw CPU then mem then net per host, present. Bar width = `winW / numBars`; no gap between bars (avoids 1px artifacts).
- **Hotkeys:** 1=cores, 2=mem, 3=net, e=extended (peak line), h=help, n=cycle net iface, q=quit, w=write config, a/y=cpu avg, d/c=net avg, arrows=resize. See README “Hotkeys” table.

Network interface: chosen by `netint` config or first non-`lo`; press `n` to cycle. When net is toggled on, stdout prints which interface is used and how to set `netint`/`netlink`.

## Conventions

- Prefer the existing `internal/*` package layout; avoid adding new toplevel Go packages unless necessary.
- Version is the single source in `internal/version/version.go`.
- Config keys are lowercase in `~/.loadbarsrc` (e.g. `netint=eth0`, `netlink=gbit`).
- No text/font rendering in the SDL window in the current design; keep feedback on stdout unless the user explicitly asks for in-window labels.

## Testing

- `go test ./...` or `mage test`. No UI tests; collector has parse tests for protocol (e.g. `ParseNetLine` for Linux-style `/proc/net/dev` output).

## macOS support

Loadbars supports macOS with automatic window activation and OS-specific monitoring scripts.

### Implementation details

- **Script embedding:** Both Linux and Darwin monitoring scripts are embedded in the binary
- **OS detection:** Automatically uses the correct script based on the host:
  - `localhost` on macOS → Darwin script (sysctl, vm_stat, netstat, iostat)
  - `localhost` on Linux → Linux script (/proc filesystem)
  - All remote hosts → Linux script (assumes remote servers are Linux)
- **Window activation:** macOS-specific code automatically brings SDL window to foreground
  - Uses build tags (`activate_darwin.go` and `activate.go`)
  - No external helper script needed

### Key files

- `internal/collector/script.go` - Embeds both scripts
- `internal/collector/collector.go` - Selects script based on host (checks if `/proc` exists for localhost)
- `internal/display/activate_darwin.go` - macOS-specific activation using `open -a`
- `internal/display/activate.go` - No-op for other platforms

### Known limitations

- Remote macOS hosts are not supported (assumes all remote hosts are Linux)
- macOS script doesn't provide per-core CPU statistics (iostat limitation)
- Swap usage on macOS always shows 0 (macOS uses compressed memory differently)

## Useful references

- **README.md** – User-facing usage, hotkeys, config, network interface.
- **CLAUDE.md** – Points to this file.
