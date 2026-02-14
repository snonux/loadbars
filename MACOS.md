# macOS Support for Loadbars

## What was implemented

Loadbars now fully supports macOS with automatic window activation built into the binary.

### Changes made:

1. **Script embedding** - Both Linux and Darwin monitoring scripts are embedded in the binary
2. **OS detection** - Automatically uses the correct script based on the host:
   - `localhost` on macOS → Darwin script (sysctl, vm_stat, netstat, iostat)
   - `localhost` on Linux → Linux script (/proc filesystem)
   - All remote hosts → Linux script (assumes remote servers are Linux)
3. **Window activation** - macOS-specific code automatically brings SDL window to foreground
   - Uses build tags (`activate_darwin.go` and `activate.go`)
   - No external helper script needed

## Usage on macOS

Simply run the binary directly:

```bash
# Monitor localhost
./loadbars --showcores --showmem --shownet

# Monitor remote Linux servers
./loadbars server1.example.com server2.example.com --showcores

# Monitor both localhost and remotes
./loadbars localhost server1.example.com server2.example.com --showcores
```

The window will automatically appear in the foreground.

## Building on macOS

Requirements:
```bash
brew install sdl2
```

Build:
```bash
mage build
# or
go build -o loadbars ./cmd/loadbars
```

## Technical details

### Script selection logic
- `internal/collector/script.go` - Embeds both scripts
- `internal/collector/collector.go` - Selects script based on host:
  - For localhost: checks if `/proc` exists to determine Linux vs macOS
  - For remote hosts: always uses Linux script

### Window activation
- `internal/display/activate_darwin.go` - macOS-specific activation using `open -a`
- `internal/display/activate.go` - No-op for other platforms
- Called automatically after SDL window creation

### Darwin monitoring script
- Uses macOS native tools: `sysctl`, `vm_stat`, `netstat -ibn`, `iostat`
- Outputs same protocol format as Linux script (M LOADAVG, M MEMSTATS, etc.)
- Limitations: No per-core CPU stats on macOS (iostat limitation)

## Known limitations

- Remote macOS hosts are not supported (assumes all remote hosts are Linux)
- macOS script doesn't provide per-core CPU statistics (iostat limitation)
- Swap usage on macOS always shows 0 (macOS uses compressed memory differently)
