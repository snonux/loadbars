#!/usr/bin/env python3
"""Record the animated GIFs used in docs/usage-guide.md.

Each scene starts loadbars in a headless X server (Xvfb), drives it with
xdotool (key presses, mouse hovers), records the window with ffmpeg and
adds two caption strips: the command line on top, and what is being
pressed at the bottom (the loadbars window itself shows no text).

Remote servers are simulated by scripts/demo/ssh, a stand-in for ssh that
streams a synthetic but distinct load profile per host name, so every GIF
can be reproduced on one machine. Loadbars itself runs unmodified.

Requirements: Xvfb, xdotool, ffmpeg, python3, a DejaVu Sans Mono font,
and optionally gifsicle (smaller output).

Usage:
    go build -o loadbars ./cmd/loadbars
    ./scripts/record-guide-gifs.py              # all scenes
    ./scripts/record-guide-gifs.py cpu-modes    # selected scenes
    ./scripts/record-guide-gifs.py --list
"""

import argparse
import os
import shutil
import subprocess
import sys
import tempfile
import time
from dataclasses import dataclass, field
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OUT_DIR = ROOT / "docs" / "img"
DEMO_DIR = ROOT / "scripts" / "demo"
FONT = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"
FPS = 10
STRIP = 30  # caption strip height in pixels
DISPLAY_NUM = ":97"
SCREEN = (1920, 1080)


@dataclass
class Scene:
    """One GIF: loadbars arguments plus a timeline of actions.

    Actions are (seconds, kind, arg, caption) tuples. kind is one of:
      "key"     press a key (arg: xdotool key name)
      "mouse"   move the pointer to (x, y) inside the window (arg: tuple)
      "note"    change the bottom caption only
      "load"    start/stop a local CPU burner (arg: number of busy processes)
    """

    name: str
    args: list
    duration: float
    actions: list = field(default_factory=list)
    caption: str = ""  # bottom caption shown until the first action
    command: str = ""  # shown in the top strip; defaults to "loadbars <args>"
    capture: tuple = None  # (w, h) to record instead of the window size
    cursor: bool = False
    warmup: float = 3.0  # seconds before recording starts (bars settle)
    load: int = 0  # local busy processes already running when recording starts


FLEET6 = "backup01,build01,cache01,db01,web01,web02"

SCENES = [
    Scene(
        "quickstart",
        ["--barwidth", "800", "--height", "160"],
        12,
        [(2.5, "load", 3, "Starting 2 more busy processes..."),
         (5.5, "load", 4, "...all 4 cores busy"),
         (8.0, "load", 0, "Stopping them: the bar drains")],
        caption="No hosts given: monitors this machine, 1 busy process",
        load=1,
        command="loadbars",
    ),
    Scene(
        "cpu-modes",
        ["--hosts", "db01,web01", "--barwidth", "900", "--height", "160"],
        13,
        [(3, "key", "1", "Key 1: one bar per core (plus the aggregate)"),
         (7.5, "key", "1", "Key 1 again: CPU bars off"),
         (9.5, "key", "1", "Key 1 again: back to one aggregate bar per host")],
        caption="Default: one aggregate CPU bar per host",
    ),
    Scene(
        "cpu-extended",
        ["--hosts", "web01,web02", "--cpumode", "1", "--barwidth", "900", "--height", "160"],
        11,
        [(3, "key", "e", "Key e: extended mode, 1px peak line per bar"),
         (7.5, "key", "y", "Key y (x3): fewer samples, shorter peak history")],
        caption="Per-core CPU bars of two web servers",
    ),
    Scene(
        "memory",
        ["--hosts", "build01,cache01,db01", "--barwidth", "900", "--height", "160"],
        11,
        [(2.5, "key", "2", "Key 2 (or m): RAM (left) and swap (right) per host")],
        caption="CPU only",
    ),
    Scene(
        "network",
        ["--hosts", "backup01,cache01,web01", "--netaverage", "5", "--barwidth", "900", "--height", "160"],
        20,
        [(2.5, "key", "3", "Key 3 (or n): RX from the top, TX from the bottom"),
         (11, "key", "v", "Key v: lower link reference (gbit -> 100mbit), bars saturate"),
         (17, "key", "f", "Key f: back to gbit")],
        caption="CPU only",
    ),
    Scene(
        "load",
        ["--hosts", "build01,db01,web01", "--barwidth", "900", "--height", "160"],
        16,
        [(2.5, "key", "4", "Key 4 (or l): load average, 1-min fill, 5/15-min lines"),
         (11, "key", "r", "Key r: reset the auto-scale peak")],
        caption="CPU only",
    ),
    Scene(
        "disk",
        ["--hosts", "backup01,build01,db01", "--barwidth", "900", "--height", "160"],
        15,
        [(2.5, "key", "5", "Key 5: disk I/O, reads from the top, writes from the bottom"),
         (7, "key", "5", "Key 5 again: one bar per disk (db01 has two NVMe drives)"),
         (11, "key", "e", "Key e: extended mode adds the utilisation line")],
        caption="CPU only",
    ),
    Scene(
        "tooltips",
        ["--hosts", "cache01,db01,web01", "--showmem", "--shownet", "--showload",
         "--diskmode", "0", "--barwidth", "900", "--height", "220"],
        15,
        [(1.0, "mouse", (330, 110), "Hover a bar: exact values, and the whole host is highlighted"),
         (3.5, "mouse", (390, 110), "Memory bar of db01"),
         (6.0, "mouse", (450, 110), "Network bar of db01"),
         (8.5, "mouse", (510, 110), "Load average bar of db01"),
         (11.0, "mouse", (570, 110), "Disk I/O bar of db01"),
         (13.0, "mouse", (30, 110), "CPU bar of cache01")],
        caption="Hover over any bar",
        cursor=True,
    ),
    Scene(
        "multi-server",
        ["--hosts", FLEET6, "--showmem", "--shownet", "--barwidth", "1200", "--height", "180"],
        16,
        [(3, "key", "s", "Key s: separator lines between hosts"),
         (6.5, "key", "g", "Key g: red line, average CPU across all hosts"),
         (10, "key", "i", "Key i: pink line, average iowait+IRQ across all hosts"),
         (13, "mouse", (700, 90), "Hover: the whole host is highlighted")],
        caption="Six servers side by side, sorted by name",
        command="loadbars --hosts " + FLEET6 + " --showmem --shownet",
        cursor=True,
    ),
    Scene(
        "many-servers",
        ["--hosts", ",".join(
            [f"web{i:02d}" for i in range(1, 13)] + [f"db{i:02d}" for i in range(1, 5)]
            + [f"cache{i:02d}" for i in range(1, 5)] + [f"build{i:02d}" for i in range(1, 5)]),
         "--showmem", "--maxbarsperrow", "16", "--showseparators",
         "--barwidth", "1200", "--height", "300"],
        12,
        [(5, "mouse", (1030, 60), "Hover: find out which host a bar belongs to"),
         (9, "mouse", (560, 260), "Hover: find out which host a bar belongs to")],
        caption="24 servers, 48 bars wrapped into rows of 16",
        cursor=True,
        command="loadbars web{01..12} db{01..04} cache{01..04} build{01..04} "
                "--showmem --maxbarsperrow 16 --showseparators",
    ),
    Scene(
        "window",
        ["--hosts", "db01,web01", "--showmem", "--barwidth", "800", "--height", "150"],
        12,
        [(2.5, "key", "Right", "Right arrow: 100px wider"),
         (3.5, "key", "Right", "Right arrow: 100px wider"),
         (5.0, "key", "Down", "Down arrow: 100px taller"),
         (7.0, "key", "Left", "Left arrow: 100px narrower"),
         (8.5, "key", "Up", "Up arrow: 100px shorter"),
         (10.0, "key", "w", "Key w: save the current view to ~/.loadbarsrc")],
        caption="Resize with the arrow keys",
        capture=(1000, 250),
    ),
]


def run(cmd, **kw):
    return subprocess.run(cmd, check=True, **kw)


def xdo(*args):
    return subprocess.run(["xdotool", *args], capture_output=True, text=True).stdout.strip()


def wait_for_window(timeout=30):
    end = time.time() + timeout
    while time.time() < end:
        wid = xdo("search", "--name", "Loadbars")
        if wid:
            return wid.splitlines()[0]
        time.sleep(0.1)
    raise RuntimeError("loadbars window did not appear")


def window_size(wid):
    geo = dict(line.split("=") for line in xdo("getwindowgeometry", "--shell", wid).splitlines())
    return int(geo["WIDTH"]), int(geo["HEIGHT"])


def escape(text):
    """Escape text for ffmpeg drawtext inside a filtergraph."""
    return text.replace("\\", "\\\\").replace("'", "’").replace(":", "\\:").replace(",", "\\,").replace("%", "\\%")


def captions(scene):
    """Bottom caption intervals [(start, end, text)] relative to the recording."""
    points = [(0.0, scene.caption)] + [(a[0], a[3]) for a in scene.actions if a[3]]
    out = []
    for i, (start, text) in enumerate(points):
        end = points[i + 1][0] if i + 1 < len(points) else scene.duration + 1
        if text:
            out.append((start, end, text))
    return out


class Burner:
    """Busy-loop processes to put real CPU load on localhost."""

    def __init__(self):
        self.procs = []

    def set(self, n):
        while len(self.procs) < n:
            self.procs.append(subprocess.Popen([sys.executable, "-c", "while True: pass"]))
        while len(self.procs) > n:
            self.procs.pop().kill()


def record(scene, binary, workdir):
    env = dict(os.environ, DISPLAY=DISPLAY_NUM, SDL_RENDER_DRIVER="software",
               PATH=f"{DEMO_DIR}:{os.environ['PATH']}", HOME=str(workdir))
    log = open(workdir / f"{scene.name}.log", "w")
    lb = subprocess.Popen([str(binary), *scene.args], env=env, stdout=log, stderr=log)
    burner = Burner()
    burner.set(scene.load)
    try:
        try:
            wid = wait_for_window()
        except RuntimeError:
            log.flush()
            sys.stderr.write((workdir / f"{scene.name}.log").read_text())
            raise
        xdo("windowmove", "--sync", wid, "0", "0")
        xdo("windowfocus", wid)
        xdo("mousemove", str(SCREEN[0] - 1), str(SCREEN[1] - 1))  # park the pointer
        time.sleep(scene.warmup)
        w, h = scene.capture or window_size(wid)
        w, h = w - w % 2, h - h % 2

        raw = workdir / f"{scene.name}.mkv"
        ff = subprocess.Popen(
            ["ffmpeg", "-loglevel", "error", "-y", "-f", "x11grab",
             "-draw_mouse", "1" if scene.cursor else "0", "-framerate", str(FPS),
             "-video_size", f"{w}x{h}", "-i", f"{DISPLAY_NUM}+0,0",
             "-t", str(scene.duration), "-c:v", "ffv1", str(raw)])
        start = time.time()
        for at, kind, arg, _ in scene.actions:
            time.sleep(max(0.0, start + at - time.time()))
            if kind == "key":
                repeat = 3 if arg == "y" else 1
                for _ in range(repeat):
                    xdo("key", "--window", wid, arg)
            elif kind == "mouse":
                xdo("mousemove", str(arg[0]), str(arg[1]))
            elif kind == "load":
                burner.set(arg)
        ff.wait()
    finally:
        burner.set(0)
        lb.terminate()
        lb.wait()
    return raw, w, h


def visible_args(args):
    """Drop the window-size flags every scene sets; they only distract in the caption."""
    out, skip = [], False
    for a in args:
        if skip:
            skip = False
        elif a in ("--barwidth", "--height"):
            skip = True
        else:
            out.append(a)
    return out


def to_gif(scene, raw, w, h, out):
    cmd = escape("$ " + (scene.command or "loadbars " + " ".join(visible_args(scene.args))))
    filters = [
        f"pad={w}:{h + 2 * STRIP}:0:{STRIP}:color=0x1a1a1a",
        f"drawtext=fontfile={FONT}:text='{cmd}':x=8:y=({STRIP}-th)/2:fontsize=14:fontcolor=0x9fdf9f",
    ]
    for start, end, text in captions(scene):
        filters.append(
            f"drawtext=fontfile={FONT}:text='{escape(text)}':x=8:y={h + STRIP}+({STRIP}-th)/2:"
            f"fontsize=15:fontcolor=white:enable='between(t,{start},{end})'")
    chain = ",".join(filters)
    palette = raw.with_suffix(".png")
    run(["ffmpeg", "-loglevel", "error", "-y", "-i", str(raw),
         "-vf", chain + ",palettegen=max_colors=64:stats_mode=full", "-update", "1", str(palette)])
    run(["ffmpeg", "-loglevel", "error", "-y", "-i", str(raw), "-i", str(palette),
         "-filter_complex", f"[0:v]{chain}[v];[v][1:v]paletteuse=dither=none",
         "-loop", "0", str(out)])
    if shutil.which("gifsicle"):
        run(["gifsicle", "-O3", "--batch", str(out)])


def main():
    names = [s.name for s in SCENES]
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("scenes", nargs="*", help=f"scenes to record (default: all): {', '.join(names)}")
    ap.add_argument("--list", action="store_true", help="list scenes and exit")
    ap.add_argument("--bin", default=str(ROOT / "loadbars"), help="loadbars binary")
    opts = ap.parse_args()
    if opts.list:
        print("\n".join(names))
        return
    unknown = set(opts.scenes) - set(names)
    if unknown:
        ap.error(f"unknown scene(s): {', '.join(sorted(unknown))}")
    for tool in ("Xvfb", "xdotool", "ffmpeg"):
        if not shutil.which(tool):
            sys.exit(f"missing dependency: {tool}")
    binary = Path(opts.bin)
    if not binary.exists():
        sys.exit(f"{binary} not found; build it first: go build -o loadbars ./cmd/loadbars")

    OUT_DIR.mkdir(parents=True, exist_ok=True)
    xvfb = subprocess.Popen(["Xvfb", DISPLAY_NUM, "-screen", "0", f"{SCREEN[0]}x{SCREEN[1]}x24", "-nolisten", "tcp"],
                            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    os.environ["DISPLAY"] = DISPLAY_NUM
    time.sleep(1)
    try:
        with tempfile.TemporaryDirectory(prefix="loadbars-gifs.") as tmp:
            for scene in SCENES:
                if opts.scenes and scene.name not in opts.scenes:
                    continue
                out = OUT_DIR / f"{scene.name}.gif"
                print(f"==> {scene.name}", flush=True)
                try:
                    raw, w, h = record(scene, binary, Path(tmp))
                except RuntimeError as err:  # the window occasionally fails to map; retry once
                    print(f"    {err}, retrying", flush=True)
                    raw, w, h = record(scene, binary, Path(tmp))
                to_gif(scene, raw, w, h, out)
                print(f"    {out.relative_to(ROOT)} ({out.stat().st_size // 1024} KiB)", flush=True)
    finally:
        xvfb.terminate()


if __name__ == "__main__":
    main()
