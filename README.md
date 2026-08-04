# Video-to-GIF-Converter

A high-quality video-to-GIF converter that wraps `ffmpeg` with a two-pass, palette-optimized pipeline and a live terminal progress bar.

### ⚙️ How It Works

Naive one-pass conversions produce blotchy, banded GIFs because GIF is limited to a 256-color palette per frame. This tool follows the well-known ffmpeg two-pass technique:

1. **Pass 1** runs a `palettegen` filter over the trimmed, downscaled frames to build a *single* optimized 256-color palette (`stats_mode=diff` weights colors by change between frames, ideal for animation).
2. **Pass 2** re-encodes the same segment through `paletteuse` with `sierra2_4a` error-diffusion dithering, producing a crisp GIF that reuses that shared palette across every frame.

Trimming (`--start`, `--duration`) is applied *before* the input decode using ffmpeg's fast keyframe seek, so long clips convert quickly. The tool auto-detects ffmpeg on `PATH`, probes the input duration up front, and streams a colored progress bar by parsing `time=HH:MM:SS.xx` markers out of ffmpeg's stderr with a custom scanner that respects both `\n` and `\r` (ffmpeg overwrites the progress line with `\r`).

## 📁 Setup

Requirements:

- Go 1.21+
- `ffmpeg` on your `PATH` (download at <https://ffmpeg.org/download.html>)

Build:

```bash
cd Video-to-GIF-Converter
go build -o vid2gif
```

On Windows PowerShell, the binary is `vid2gif.exe`.

### 🚀 Usage

```bash
vid2gif <input> [flags]
```

Example:

```bash
vid2gif input.mp4 --output out.gif --fps 15 --width 480 --start 00:00:05 --duration 10
```

Flags:

| Flag                | Default          | Description                                                       |
| ------------------- | ---------------- | ----------------------------------------------------------------- |
| `--output`, `-o`    | `<input>.gif`    | Output GIF file path                                              |
| `--fps`             | `15`             | Frames per second                                                 |
| `--width`           | `480`            | Output width in pixels (aspect ratio preserved; `-1` keeps original) |
| `--start`           | `0`              | Start time (`HH:MM:SS`, `MM:SS`, or plain seconds)                |
| `--duration`        | full clip        | Duration to convert (`HH:MM:SS`, `MM:SS`, or plain seconds)       |
| `--loop`            | `0`              | Loop count (`0` = forever, `-1` = play once, `N` = loop N times)   |
| `--quiet`           | off              | Suppress progress and info output                                 |
| `--help`, `-h`      | -                | Show help                                                         |

Environment variables:

- `NO_COLOR` - set to any value to disable ANSI colors in the output.

### ✨ Features

- **Two-pass palette pipeline** for high-quality, low-artifact GIFs.
- **Live colored progress bar** parsed from ffmpeg's stderr, per pass.
- **Positional input** *or* `--input`/`-i` flag - use whichever style you like.
- **Fast seek** with `--start` / `--duration` using ffmpeg's pre-decode `-ss` and `-t`.
- **Sensible defaults**: 15 fps, 480 px wide, infinite loop - matches most social-media requirements.
- **Clear errors** when ffmpeg is missing, the input is unreadable, or any flag value is invalid.
- **NO_COLOR aware** for CI logs and colorless terminals.
- **Zero external Go dependencies** - pure standard library.
- **Cross-platform**: works on Windows, macOS, and Linux wherever ffmpeg does.
