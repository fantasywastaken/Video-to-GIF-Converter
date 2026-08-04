package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiBlue   = "\x1b[34m"
	ansiCyan   = "\x1b[36m"
	ansiGray   = "\x1b[90m"
)

var useColor = true

type config struct {
	input    string
	output   string
	fps      int
	width    int
	start    string
	duration string
	quiet    bool
	loop     int
}

var (
	timeRegex     = regexp.MustCompile(`time=(\d+):(\d+):(\d+(?:\.\d+)?)`)
	durationRegex = regexp.MustCompile(`Duration:\s*(\d+):(\d+):(\d+(?:\.\d+)?)`)
)

func main() {
	if os.Getenv("NO_COLOR") != "" {
		useColor = false
	}

	cfg, showHelp, err := parseArgs(os.Args[1:])
	if showHelp {
		printUsage(os.Stdout)
		os.Exit(0)
	}
	if err != nil {
		fatal(err.Error())
	}

	if err := ensureFFmpeg(); err != nil {
		fatal(err.Error())
	}
	if err := ensureInput(cfg.input); err != nil {
		fatal(err.Error())
	}
	if err := convert(cfg); err != nil {
		fatal(err.Error())
	}
}

func parseArgs(args []string) (config, bool, error) {
	var cfg config
	fs := flag.NewFlagSet("vid2gif", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.StringVar(&cfg.output, "output", "", "Output GIF file path (defaults to <input>.gif)")
	fs.StringVar(&cfg.output, "o", "", "Alias for --output")
	fs.IntVar(&cfg.fps, "fps", 15, "Frames per second in the output GIF")
	fs.IntVar(&cfg.width, "width", 480, "Output width in pixels; aspect ratio is preserved (-1 keeps original)")
	fs.StringVar(&cfg.start, "start", "", "Start time (HH:MM:SS or seconds)")
	fs.StringVar(&cfg.duration, "duration", "", "Duration to convert (HH:MM:SS or seconds)")
	fs.BoolVar(&cfg.quiet, "quiet", false, "Suppress progress output")
	fs.IntVar(&cfg.loop, "loop", 0, "Loop count (0 = forever, -1 = no loop)")

	var help bool
	fs.BoolVar(&help, "help", false, "Show this help message")
	fs.BoolVar(&help, "h", false, "Alias for --help")

	var input string
	var rest []string
	if len(args) == 0 {
		return cfg, true, nil
	}
	if !strings.HasPrefix(args[0], "-") {
		input = args[0]
		rest = args[1:]
	} else {
		rest = args
	}

	fs.StringVar(&input, "input", input, "Input video file (alternative to positional argument)")
	fs.StringVar(&input, "i", input, "Alias for --input")

	if err := fs.Parse(rest); err != nil {
		return cfg, false, fmt.Errorf("invalid arguments: %w", err)
	}

	if help {
		return cfg, true, nil
	}
	if input == "" {
		return cfg, true, errors.New("no input file specified")
	}
	if cfg.fps <= 0 {
		return cfg, false, errors.New("--fps must be greater than 0")
	}
	if cfg.width <= 0 && cfg.width != -1 {
		return cfg, false, errors.New("--width must be greater than 0 (or -1 to keep original)")
	}
	if cfg.start != "" {
		if _, err := parseTime(cfg.start); err != nil {
			return cfg, false, fmt.Errorf("invalid --start value %q: %w", cfg.start, err)
		}
	}
	if cfg.duration != "" {
		if _, err := parseTime(cfg.duration); err != nil {
			return cfg, false, fmt.Errorf("invalid --duration value %q: %w", cfg.duration, err)
		}
	}

	cfg.input = input
	if cfg.output == "" {
		ext := filepath.Ext(input)
		base := strings.TrimSuffix(input, ext)
		cfg.output = base + ".gif"
	}
	return cfg, false, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, color(ansiBold, "vid2gif")+" - Convert video files to optimized GIFs")
	fmt.Fprintln(w)
	fmt.Fprintln(w, color(ansiBold, "Usage:"))
	fmt.Fprintln(w, "  vid2gif <input> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, color(ansiBold, "Flags:"))
	fmt.Fprintln(w, "  --output, -o <path>     Output GIF file path (default: <input>.gif)")
	fmt.Fprintln(w, "  --fps <n>               Frames per second (default: 15)")
	fmt.Fprintln(w, "  --width <px>            Output width in pixels, preserves aspect ratio (default: 480)")
	fmt.Fprintln(w, "  --start <time>          Start time as HH:MM:SS or seconds (default: 0)")
	fmt.Fprintln(w, "  --duration <time>       Duration as HH:MM:SS or seconds (default: full clip)")
	fmt.Fprintln(w, "  --loop <n>              Loop count: 0 = forever, -1 = no loop (default: 0)")
	fmt.Fprintln(w, "  --quiet                 Suppress progress output")
	fmt.Fprintln(w, "  --help, -h              Show this help")
	fmt.Fprintln(w)
	fmt.Fprintln(w, color(ansiBold, "Example:"))
	fmt.Fprintln(w, "  vid2gif input.mp4 --output out.gif --fps 15 --width 480 --start 00:00:05 --duration 10")
	fmt.Fprintln(w)
	fmt.Fprintln(w, color(ansiDim, "Requires ffmpeg in PATH (https://ffmpeg.org/download.html)."))
}

func ensureFFmpeg() error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return errors.New("ffmpeg not found in PATH. Install it from https://ffmpeg.org/download.html")
	}
	return nil
}

func ensureInput(input string) error {
	info, err := os.Stat(input)
	if err != nil {
		return fmt.Errorf("cannot open input %q: %w", input, err)
	}
	if info.IsDir() {
		return fmt.Errorf("input must be a file, not a directory: %s", input)
	}
	return nil
}

func convert(cfg config) error {
	tempFile, err := os.CreateTemp("", "vid2gif_palette_*.png")
	if err != nil {
		return fmt.Errorf("could not create temp palette: %w", err)
	}
	palettePath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(palettePath)

	totalDur := detectTargetDuration(cfg)

	info(cfg, fmt.Sprintf("Input:  %s", cfg.input))
	info(cfg, fmt.Sprintf("Output: %s", cfg.output))
	info(cfg, fmt.Sprintf("Config: %d fps  |  width %dpx  |  start %s  |  duration %s",
		cfg.fps, cfg.width, orDash(cfg.start), orDash(cfg.duration)))
	if totalDur > 0 {
		info(cfg, fmt.Sprintf("Detected clip length: %s", formatDur(totalDur)))
	}
	fmt.Println()

	stage(cfg, "Pass 1/2", "Generating optimized color palette")
	if err := runPass1(cfg, palettePath, totalDur); err != nil {
		return fmt.Errorf("palette generation failed: %w", err)
	}
	fmt.Println()

	stage(cfg, "Pass 2/2", "Encoding GIF with palette")
	if err := runPass2(cfg, palettePath, totalDur); err != nil {
		return fmt.Errorf("gif encoding failed: %w", err)
	}
	fmt.Println()

	if stat, err := os.Stat(cfg.output); err == nil {
		fmt.Printf("%s Wrote %s (%s)\n",
			color(ansiGreen+ansiBold, "[OK]"),
			cfg.output, humanBytes(stat.Size()))
	} else {
		fmt.Println(color(ansiGreen+ansiBold, "[OK] Done."))
	}
	return nil
}

func runPass1(cfg config, palettePath string, totalDur time.Duration) error {
	args := []string{"-y", "-hide_banner"}
	if cfg.start != "" {
		args = append(args, "-ss", cfg.start)
	}
	if cfg.duration != "" {
		args = append(args, "-t", cfg.duration)
	}
	args = append(args, "-i", cfg.input)
	vf := fmt.Sprintf("fps=%d,scale=%d:-1:flags=lanczos,palettegen=stats_mode=diff", cfg.fps, cfg.width)
	args = append(args, "-vf", vf, palettePath)
	return runFFmpeg(cfg, args, totalDur, "palette")
}

func runPass2(cfg config, palettePath string, totalDur time.Duration) error {
	args := []string{"-y", "-hide_banner"}
	if cfg.start != "" {
		args = append(args, "-ss", cfg.start)
	}
	if cfg.duration != "" {
		args = append(args, "-t", cfg.duration)
	}
	args = append(args, "-i", cfg.input, "-i", palettePath)
	lavfi := fmt.Sprintf("[0:v]fps=%d,scale=%d:-1:flags=lanczos[v];[v][1:v]paletteuse=dither=sierra2_4a", cfg.fps, cfg.width)
	args = append(args, "-filter_complex", lavfi, "-loop", strconv.Itoa(cfg.loop), cfg.output)
	return runFFmpeg(cfg, args, totalDur, "gif")
}

func runFFmpeg(cfg config, args []string, totalDur time.Duration, label string) error {
	cmd := exec.Command("ffmpeg", args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 128*1024), 4*1024*1024)
	scanner.Split(scanCRLF)

	var lastPct int
	var errBuf []string
	lastPct = -1

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "error") ||
			strings.Contains(lower, "invalid data") ||
			strings.HasPrefix(lower, "conversion failed") ||
			strings.HasPrefix(lower, "cannot") {
			errBuf = append(errBuf, line)
		}
		m := timeRegex.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if cfg.quiet {
			continue
		}
		cur := parseHMSFloat(m[1], m[2], m[3])
		if totalDur <= 0 {
			printSpinner(label, cur)
			continue
		}
		pct := int(float64(cur) / float64(totalDur) * 100)
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		if pct != lastPct {
			printProgress(label, pct, cur, totalDur)
			lastPct = pct
		}
	}

	if err := cmd.Wait(); err != nil {
		if len(errBuf) > 0 {
			return fmt.Errorf("%s (%s)", err.Error(), strings.Join(errBuf, "; "))
		}
		return err
	}
	if !cfg.quiet && totalDur > 0 {
		printProgress(label, 100, totalDur, totalDur)
	}
	return nil
}

func detectTargetDuration(cfg config) time.Duration {
	if cfg.duration != "" {
		if d, err := parseTime(cfg.duration); err == nil {
			return d
		}
	}
	full, err := probeDuration(cfg.input)
	if err != nil {
		return 0
	}
	if cfg.start != "" {
		if s, err := parseTime(cfg.start); err == nil {
			full -= s
			if full < 0 {
				full = 0
			}
		}
	}
	return full
}

func probeDuration(input string) (time.Duration, error) {
	cmd := exec.Command("ffmpeg", "-hide_banner", "-i", input)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return 0, err
	}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	data, _ := io.ReadAll(stderr)
	_ = cmd.Wait()
	m := durationRegex.FindStringSubmatch(string(data))
	if m == nil {
		return 0, errors.New("could not parse duration")
	}
	return parseHMSFloat(m[1], m[2], m[3]), nil
}

func parseHMSFloat(h, m, s string) time.Duration {
	hi, _ := strconv.Atoi(h)
	mi, _ := strconv.Atoi(m)
	sf, _ := strconv.ParseFloat(s, 64)
	return time.Duration(hi)*time.Hour + time.Duration(mi)*time.Minute + time.Duration(sf*float64(time.Second))
}

func parseTime(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty duration")
	}
	if strings.Contains(s, ":") {
		parts := strings.Split(s, ":")
		switch len(parts) {
		case 3:
			return parseHMSFloat(parts[0], parts[1], parts[2]), nil
		case 2:
			return parseHMSFloat("0", parts[0], parts[1]), nil
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(f * float64(time.Second)), nil
}

func scanCRLF(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i, b := range data {
		if b == '\r' || b == '\n' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func printProgress(label string, pct int, cur, total time.Duration) {
	width := 32
	filled := pct * width / 100
	if filled > width {
		filled = width
	}
	var barBuilder strings.Builder
	for i := 0; i < width; i++ {
		switch {
		case i < filled-1:
			barBuilder.WriteString("=")
		case i == filled-1 && pct < 100:
			barBuilder.WriteString(">")
		case i == filled-1 && pct == 100:
			barBuilder.WriteString("=")
		default:
			barBuilder.WriteString(" ")
		}
	}
	fmt.Printf("\r  %s [%s%s%s] %s%3d%%%s  %s%s / %s%s   ",
		color(ansiCyan, padRight(label, 8)),
		color(ansiGreen, ""), barBuilder.String(), color(ansiReset, ""),
		color(ansiBold, ""), pct, color(ansiReset, ""),
		color(ansiDim, ""), formatDur(cur), formatDur(total), color(ansiReset, ""))
}

func printSpinner(label string, cur time.Duration) {
	fmt.Printf("\r  %s  processing... %s%s%s   ",
		color(ansiCyan, padRight(label, 8)),
		color(ansiDim, ""), formatDur(cur), color(ansiReset, ""))
}

func stage(cfg config, tag, desc string) {
	if cfg.quiet {
		return
	}
	fmt.Printf("%s %s\n", color(ansiBlue+ansiBold, tag), desc)
}

func info(cfg config, s string) {
	if cfg.quiet {
		return
	}
	fmt.Println(color(ansiDim, s))
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, color(ansiRed+ansiBold, "[ERR]")+" "+msg)
	os.Exit(1)
}

func color(code, s string) string {
	if !useColor {
		return s
	}
	if s == "" {
		return code
	}
	return code + s + ansiReset
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func formatDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Seconds())
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
