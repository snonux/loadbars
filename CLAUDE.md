# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Loadbars is a Perl-based real-time server load monitoring tool that connects to multiple remote servers via SSH and visualizes CPU, memory, and network statistics using SDL graphics. It uses threading to monitor multiple hosts concurrently and displays the data in a graphical window.

**Technology Stack:**
- Perl 5.14+
- SDL (Simple DirectMedia Layer) via libsdl-perl for graphics
- Threading for concurrent server monitoring
- SSH for remote connections (requires public/private key authentication)

## Build and Development Commands

**Version management:**
```bash
make version  # Extract version from debian/changelog to .version file
```

**Code formatting:**
```bash
make perltidy  # Format all Perl code using perltidy
```

**Documentation generation:**
```bash
make documentation  # Generate man pages and text docs from POD
```

**Full release process:**
```bash
make release  # Generate docs, tag, and push to git
```

**Testing the application:**
```bash
# Run locally
./loadbars --hosts localhost

# Run with multiple hosts
./loadbars --hosts localhost,server1.example.com --showcores 1

# Profile with NYTProf
make profile  # Runs loadbars with profiling and generates HTML report
```

**Installation:**
```bash
make install DESTDIR=/path/to/install  # Install to custom directory
```

## Architecture

### Module Structure

The codebase follows a modular Perl architecture with all core logic in `lib/Loadbars/`:

- **Main.pm** (~900 lines): Core application logic
  - SDL window management and rendering
  - CPU/memory/network parsing from `/proc/stat`, `/proc/meminfo`, `/sys/class/net`
  - Thread creation and management for remote SSH connections
  - Main event loop and keyboard handling
  - Drawing functions for bars, text, and statistics

- **Config.pm**: Configuration file handling
  - Reads from `~/.loadbarsrc`
  - Supports ClusterSSH integration via `/etc/clusters`
  - Recursive cluster expansion with cycle detection

- **HelpDispatch.pm**: Command-line argument parsing and help system
  - Creates GetOptions dispatch table
  - Generates usage information

- **Constants.pm**: Application constants
  - Color definitions for SDL
  - Timing intervals (CPU: 0.14s, Network: 3.0s, Memory: 1.0s)
  - Configuration file paths

- **Shared.pm**: Shared state variables used across modules
  - Global configuration hash `%C`
  - Internal state hash `%I`
  - Thread PID tracking `%PIDS`

- **Utils.pm**: Utility functions for string manipulation and helpers

### Entry Point Flow

1. `loadbars` script parses command-line arguments
2. Loads configuration from `~/.loadbarsrc` if exists
3. Resolves cluster definitions from `/etc/clusters` if using `--cluster`
4. Creates one thread per host for SSH connections
5. Each thread runs `vmstat` remotely and pipes data back
6. Main loop reads thread data, parses statistics, and renders SDL graphics
7. Handles keyboard input for interactive controls (toggle cores, extended mode, etc.)

### Library Path Resolution

The main script checks for `./lib/Loadbars` first (development mode), then falls back to `/usr/share/loadbars/lib` (installed mode). This allows running from source tree or installed location.

### Threading Model

Each monitored host gets its own Perl thread that:
- Establishes SSH connection
- Runs `vmstat` and other commands remotely
- Pipes output back to shared data structures
- Main thread reads shared data and updates display

### Key Design Patterns

- **Shared state**: Uses `threads::shared` for communication between threads
- **SDL rendering**: Double-buffered rendering with fixed intervals
- **Parser design**: CPU/memory/network stats parsed from `/proc` filesystem format
- **Color coding**: Different colors represent different metrics (system=blue, user=yellow, iowait=purple, etc.)

## Important Implementation Details

**Version source**: The `.version` file is generated from the VERSION variable in the Makefile using the `make version` target. Version is displayed at startup and in man pages.

**Font handling**: Custom fonts are stored in `fonts/` directory and installed to `/usr/share/loadbars/fonts`.

**Configuration persistence**: Pressing 'w' during execution writes current settings to `~/.loadbarsrc`.

**SSH requirements**: Requires SSH agent or passwordless SSH keys for remote connections. The tool automatically runs `ssh-add` if no agent is detected.

**Remote data collection**: Uses `vmstat`, `/proc/stat`, `/proc/meminfo`, and `/sys/class/net` on remote hosts via SSH.
