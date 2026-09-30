# AGENTS.md – Guidance for AI coding agents

This file helps AI assistants (Cursor, Claude, etc.) work effectively in this repository.

## Project overview

**Loadbars** is a real-time server load monitoring tool. It connects to one or more hosts via SSH (or runs locally) and shows CPU, memory, and network usage as vertical colored bars in an SDL window. It does not record or graph history; it shows current state only (like `top` or `vmstat`).

- **Repo:** github.com/snonux/loadbars  
- **Display:** SDL2 (go-sdl2). All bars are drawn with filled rectangles; the only in-window text is the hover tooltip, drawn with a small built-in bitmap font (`internal/display/font.go`). Other user feedback (hotkeys, toggles, errors) is printed to stdout.

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
  collector/           # Runs embedded script (local or ssh via bash -s), parses M LOADAVG / M MEMSTATS / M NETSTATS / M DISKSTATS / M CPUSTATS
  config/              # Config struct, ~/.loadbarsrc load/save, cluster from /etc/clusters
  constants/           # Intervals, colors (RGB), link-speed constants
  display/             # SDL window, event loop, drawing (CPU/mem/net bars, hotkeys)
  stats/               # HostStats, Snapshot, NetStamp; read by display
  version/             # Version string (e.g. "0.8.0") – used in title bar and --version
scripts/
  loadbars-remote.sh   # Copy for reference; the binary embeds internal/collector/loadbars-remote.sh (go:embed in script.go)
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

- **Display** (`internal/display`): One loop – poll events, snapshot from store, count bars, clear the whole window, draw CPU then mem, net, load and disk per host, present. Bar width = `winW / numBars`; no gap between bars (avoids 1px artifacts). When `maxbarsperrow` is set, bars wrap into multiple rows of equal height; the last row may have fewer (wider) bars.
- **Hotkeys:** 1=cpu mode, 2/m=mem, 3/n=net, 4/l=load, 5=disk mode, r=reset auto-scale, e=extended, g=avg line, i=io avg line, s=separators, h=help, q=quit, w=write config, a/y=cpu avg, d/c=net avg, b/x=disk avg, f/v=link scale, arrows=resize. See the hotkey table in docs/usage-guide.md.

Network bars aggregate RX/TX across all non-`lo` interfaces per host. Link speed is set via `netlink` config or `--netlink` flag.

## Conventions

- Prefer the existing `internal/*` package layout; avoid adding new toplevel Go packages unless necessary.
- Version is the single source in `internal/version/version.go`.
- Config keys are lowercase in `~/.loadbarsrc` (e.g. `netlink=gbit`, `maxbarsperrow=4`).
- Apart from the hover tooltip, no text in the SDL window; keep feedback on stdout unless the user explicitly asks for in-window labels.

## Testing

- `go test ./...` or `mage test`. No UI tests; collector has parse tests for protocol (e.g. `ParseNetLine` for Linux-style `/proc/net/dev` output).

## macOS support (client only)

Loadbars supports macOS as a client to monitor remote Linux servers via SSH.

### Implementation details

- **Window activation:** macOS-specific code automatically brings SDL window to foreground
  - Uses build tags (`activate_darwin.go` and `activate.go`)
  - No external helper script needed

### Key files

- `internal/collector/script.go` - Embeds the Linux monitoring script (`internal/collector/loadbars-remote.sh`)
- `internal/collector/collector.go` - Runs Linux script locally or over SSH; exits with error if local system lacks /proc
- `internal/display/activate_darwin.go` - macOS-specific activation using `open -a`
- `internal/display/activate.go` - No-op for other platforms

### Known limitations

- Local monitoring on macOS is not supported (requires Linux with /proc filesystem)
- All remote hosts must be Linux (assumes all remote hosts are Linux)

## Useful references

- **README.md** – Short intro, build instructions and a few examples; links to the guide.
- **docs/usage-guide.md** – Feature-by-feature guide with GIFs in `docs/img/`, recorded by `scripts/record-guide-gifs.py` using the fake fleet in `scripts/demo/ssh`. Re-record the affected GIFs when the display changes.
- **CLAUDE.md** – Points to this file.
