#!/bin/bash
# record-gif.sh - Record a loadbars session as an animated GIF.
#
# Wayland: Uses grim for frame capture + slurp for region selection.
# X11:     Uses ffmpeg x11grab + xdotool for automatic window detection.
# GIF conversion: gifski (preferred, high quality) or ffmpeg palettegen (fallback).
#
# Usage: ./scripts/record-gif.sh [options] [-- loadbars-args...]
#   Arguments before -- are for this script; arguments after -- are passed to loadbars.
#
# Examples:
#   ./scripts/record-gif.sh
#   ./scripts/record-gif.sh --duration 20 --output demo.gif -- --hosts host1,host2 --showcores
#   ./scripts/record-gif.sh --duration 15 --fps 15 -- --hosts localhost

set -euo pipefail

# Defaults
DURATION=10
OUTPUT="loadbars.gif"
FPS=10
LOADBARS_BIN="./loadbars"
LOADBARS_ARGS=()
TMPDIR_REC=""
LOADBARS_PID=""
IS_WAYLAND=false

# Colored output to stderr so stdout stays clean
info()  { echo -e "\033[1;34m[info]\033[0m  $*" >&2; }
warn()  { echo -e "\033[1;33m[warn]\033[0m  $*" >&2; }
error() { echo -e "\033[1;31m[error]\033[0m $*" >&2; }

# Cleanup on exit/interrupt: kill loadbars, remove temp files
cleanup() {
    if [[ -n "$LOADBARS_PID" ]] && kill -0 "$LOADBARS_PID" 2>/dev/null; then
        info "Stopping loadbars (PID $LOADBARS_PID)"
        kill "$LOADBARS_PID" 2>/dev/null || true
        wait "$LOADBARS_PID" 2>/dev/null || true
    fi
    if [[ -n "$TMPDIR_REC" && -d "$TMPDIR_REC" ]]; then
        rm -rf "$TMPDIR_REC"
    fi
}
trap cleanup EXIT INT TERM

# --- Detect display server ---

detect_display_server() {
    if [[ "${XDG_SESSION_TYPE:-}" == "wayland" || -n "${WAYLAND_DISPLAY:-}" ]]; then
        IS_WAYLAND=true
    fi
}

# --- Dependency checks ---

check_deps_wayland() {
    local missing=()
    command -v grim  >/dev/null 2>&1 || missing+=("grim")
    command -v slurp >/dev/null 2>&1 || missing+=("slurp")
    command -v ffmpeg >/dev/null 2>&1 || missing+=("ffmpeg")

    if (( ${#missing[@]} > 0 )); then
        error "Missing required dependencies: ${missing[*]}"
        error "Install with: sudo dnf install ${missing[*]}"
        exit 1
    fi
}

check_deps_x11() {
    local missing=()
    command -v ffmpeg  >/dev/null 2>&1 || missing+=("ffmpeg")
    command -v xdotool >/dev/null 2>&1 || missing+=("xdotool")

    if (( ${#missing[@]} > 0 )); then
        error "Missing required dependencies: ${missing[*]}"
        error "Install with: sudo dnf install ${missing[*]}"
        exit 1
    fi
}

check_gifski() {
    if ! command -v gifski >/dev/null 2>&1; then
        warn "gifski not found — will use ffmpeg palettegen fallback (lower quality)."
        warn "For better GIFs, install gifski: https://gif.ski"
    fi
}

# --- Argument parsing ---

parse_args() {
    while (( $# > 0 )); do
        case "$1" in
            --duration) DURATION="$2"; shift 2 ;;
            --output)   OUTPUT="$2";   shift 2 ;;
            --fps)      FPS="$2";      shift 2 ;;
            --bin)      LOADBARS_BIN="$2"; shift 2 ;;
            --)         shift; LOADBARS_ARGS=("$@"); break ;;
            -h|--help)
                cat >&2 <<'HELP'
Usage: record-gif.sh [options] [-- loadbars-args...]

Options:
  --duration SECS  Recording duration (default: 10)
  --output FILE    Output GIF path (default: loadbars.gif)
  --fps N          Frames per second (default: 10)
  --bin PATH       Path to loadbars binary (default: ./loadbars)
  --               Separator; remaining args passed to loadbars

Wayland: uses grim + slurp (click to select the loadbars window)
X11:     uses ffmpeg x11grab + xdotool (automatic window detection)

Examples:
  record-gif.sh --duration 20 --output demo.gif -- --hosts h1,h2
  record-gif.sh --fps 15 -- --hosts localhost --showcores
HELP
                exit 0
                ;;
            *)
                error "Unknown option: $1 (use -- to separate loadbars args)"
                exit 1
                ;;
        esac
    done
}

# --- Wayland recording: grim frame capture ---

# Prompt user to select the loadbars window region with slurp
select_region_wayland() {
    info "Click and drag to select the loadbars window region..."
    local region
    region=$(slurp 2>/dev/null) || {
        error "Region selection cancelled."
        exit 1
    }
    echo "$region"
}

# Capture frames with grim at the target fps for the given duration
record_frames_wayland() {
    local region="$1"
    local framedir="$TMPDIR_REC/frames"
    mkdir -p "$framedir"

    local interval
    interval=$(python3 -c "print(1.0 / $FPS)")
    local total_frames=$(( DURATION * FPS ))

    info "Recording ${DURATION}s at ${FPS} fps (${total_frames} frames)..."
    local i=0
    local start_time
    start_time=$(date +%s%N)

    while (( i < total_frames )); do
        local frame_num
        printf -v frame_num "%04d" "$i"
        grim -g "$region" -t ppm "$framedir/frame${frame_num}.ppm" 2>/dev/null

        i=$(( i + 1 ))

        # Sleep to maintain target fps (subtract capture time from interval)
        local now elapsed_ns target_ns sleep_s
        now=$(date +%s%N)
        elapsed_ns=$(( now - start_time ))
        target_ns=$(( i * 1000000000 / FPS ))
        if (( target_ns > elapsed_ns )); then
            sleep_s=$(python3 -c "print(($target_ns - $elapsed_ns) / 1e9)")
            sleep "$sleep_s"
        fi
    done

    local end_time
    end_time=$(date +%s%N)
    local actual_duration=$(( (end_time - start_time) / 1000000 ))
    info "Captured ${i} frames in ${actual_duration}ms."
    echo "$framedir"
}

# --- X11 recording: ffmpeg x11grab ---

# Wait for the loadbars SDL window to appear on X11
wait_for_window_x11() {
    local attempts=0
    local max_attempts=50  # 50 * 100ms = 5 seconds
    info "Waiting for Loadbars window..."
    while (( attempts < max_attempts )); do
        local wid
        wid=$(xdotool search --name "Loadbars" 2>/dev/null | head -1) || true
        if [[ -n "$wid" ]]; then
            echo "$wid"
            return 0
        fi
        sleep 0.1
        (( attempts++ ))
    done
    error "Timed out waiting for Loadbars window (5s)."
    return 1
}

# Record using ffmpeg x11grab targeting the window region
record_x11() {
    local wid="$1"
    local raw_video="$TMPDIR_REC/capture.mp4"

    eval "$(xdotool getwindowgeometry --shell "$wid")"
    info "Window geometry: ${WIDTH}x${HEIGHT} at +${X}+${Y}"

    local display="${DISPLAY:-:0}"
    info "Recording ${DURATION}s at ${FPS} fps..."
    ffmpeg -loglevel warning \
        -video_size "${WIDTH}x${HEIGHT}" \
        -framerate "$FPS" \
        -f x11grab \
        -i "${display}+${X},${Y}" \
        -t "$DURATION" \
        -c:v libx264 -preset ultrafast -crf 0 \
        -y "$raw_video"

    echo "$raw_video"
}

# --- GIF conversion ---

# Convert PPM frames (from Wayland grim capture) to GIF
frames_to_gif() {
    local framedir="$1"
    local output="$2"
    local frame_count
    frame_count=$(ls "$framedir"/frame*.ppm 2>/dev/null | wc -l)

    if (( frame_count == 0 )); then
        error "No frames captured."
        exit 1
    fi

    if command -v gifski >/dev/null 2>&1; then
        info "Converting ${frame_count} frames to GIF with gifski..."
        # gifski needs PNG input, convert from PPM
        local pngdir="$TMPDIR_REC/png"
        mkdir -p "$pngdir"
        ffmpeg -loglevel warning -framerate "$FPS" -i "$framedir/frame%04d.ppm" \
            "$pngdir/frame%04d.png"
        gifski --repeat 0 --fps "$FPS" --quality 90 -o "$output" "$pngdir"/frame*.png
    else
        # Use ffmpeg palettegen+paletteuse
        info "Converting ${frame_count} frames to GIF with ffmpeg..."
        local palette="$TMPDIR_REC/palette.png"
        ffmpeg -loglevel warning -framerate "$FPS" -i "$framedir/frame%04d.ppm" \
            -vf "palettegen=stats_mode=full" \
            -update 1 -frames:v 1 -y "$palette"
        ffmpeg -loglevel warning -framerate "$FPS" -i "$framedir/frame%04d.ppm" -i "$palette" \
            -filter_complex "[0:v][1:v]paletteuse=dither=sierra2_4a" \
            -loop 0 -y "$output"
    fi
}

# Convert video file (from X11 ffmpeg capture) to GIF
video_to_gif() {
    local input="$1"
    local output="$2"

    if command -v gifski >/dev/null 2>&1; then
        info "Converting to GIF with gifski..."
        local framedir="$TMPDIR_REC/frames"
        mkdir -p "$framedir"
        ffmpeg -loglevel warning -i "$input" -vf "fps=$FPS" "$framedir/frame%04d.png"
        gifski --repeat 0 --fps "$FPS" --quality 90 -o "$output" "$framedir"/frame*.png
    else
        info "Converting to GIF with ffmpeg..."
        local palette="$TMPDIR_REC/palette.png"
        ffmpeg -loglevel warning -i "$input" \
            -vf "fps=$FPS,palettegen=stats_mode=full" \
            -update 1 -frames:v 1 -y "$palette"
        ffmpeg -loglevel warning -i "$input" -i "$palette" \
            -filter_complex "[0:v]fps=$FPS[x];[x][1:v]paletteuse=dither=sierra2_4a" \
            -loop 0 -y "$output"
    fi
}

# --- Main ---

main() {
    parse_args "$@"
    detect_display_server
    check_gifski

    # Verify loadbars binary exists
    if [[ ! -x "$LOADBARS_BIN" ]]; then
        error "Loadbars binary not found or not executable: $LOADBARS_BIN"
        error "Build it first: mage build  (or: go build -o loadbars ./cmd/loadbars)"
        exit 1
    fi

    TMPDIR_REC=$(mktemp -d /tmp/loadbars-rec.XXXXXX)

    # Launch loadbars in background
    info "Starting loadbars: $LOADBARS_BIN ${LOADBARS_ARGS[*]:-}"
    "$LOADBARS_BIN" "${LOADBARS_ARGS[@]:+${LOADBARS_ARGS[@]}}" &
    LOADBARS_PID=$!
    sleep 2  # give loadbars time to open its window

    if $IS_WAYLAND; then
        check_deps_wayland

        # User selects the loadbars window region interactively
        local region
        region=$(select_region_wayland)
        info "Selected region: $region"

        # Capture frames with grim
        local framedir
        framedir=$(record_frames_wayland "$region")

        # Convert frames to GIF
        frames_to_gif "$framedir" "$OUTPUT"
    else
        check_deps_x11

        # Automatically find the loadbars window via xdotool
        local wid
        wid=$(wait_for_window_x11) || exit 1

        # Record with ffmpeg x11grab (software rendering for capture compatibility)
        export SDL_RENDER_DRIVER=software
        local raw_video
        raw_video=$(record_x11 "$wid")

        # Convert video to GIF
        video_to_gif "$raw_video" "$OUTPUT"
    fi

    # Report result
    local size
    size=$(du -h "$OUTPUT" | cut -f1)
    info "Done! Output: $OUTPUT ($size)"
}

main "$@"
