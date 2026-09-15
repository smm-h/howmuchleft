package render

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/smm-h/howmuchleft/internal/platform"
)

// ANSI escape constants matching the Node.js colors object.
const (
	Reset   = "\x1b[0m"
	Bold    = "\x1b[1m"
	Dim     = "\x1b[2m"
	Green   = "\x1b[32m"
	Yellow  = "\x1b[33m"
	Orange  = "\x1b[38;5;208m"
	Red     = "\x1b[31m"
	Cyan    = "\x1b[36m"
	Magenta = "\x1b[35m"
	White   = "\x1b[37m"
	Gray    = "\x1b[90m"
)

// Per-process caches for truecolor and dark mode detection.
var (
	truecolorOnce   sync.Once
	truecolorCached bool

	darkModeOnce   sync.Once
	darkModeCached bool
)

// IsTruecolorSupported checks COLORTERM env var for "truecolor" or "24bit".
// Result is cached per-process.
func IsTruecolorSupported() bool {
	truecolorOnce.Do(func() {
		ct := os.Getenv("COLORTERM")
		truecolorCached = ct == "truecolor" || ct == "24bit"
	})
	return truecolorCached
}

// ResetTruecolorCache allows tests to reset the cached value.
func ResetTruecolorCache() {
	truecolorOnce = sync.Once{}
}

// ResetDarkModeCache allows tests to reset the cached value.
func ResetDarkModeCache() {
	darkModeOnce = sync.Once{}
}

// darkModeCacheTTLMs bounds how long a detected desktop dark/light preference
// is reused before the detector runs again. Detection costs a subprocess on
// every platform, and the answer is an OS setting a person changes by hand, so
// running the detector on every render buys nothing: ten seconds keeps the
// subprocess off all but a handful of renders and still follows a theme switch
// while the user is still looking at the terminal.
const darkModeCacheTTLMs = 10 * 1000

// DarkModeCacheFile is the file name, inside the Claude configuration
// directory, that holds the last detected dark/light preference.
const DarkModeCacheFile = ".dark-mode-cache.json"

// darkModeCacheEntry is what DarkModeCacheFile holds.
type darkModeCacheEntry struct {
	Dark bool  `json:"dark"`
	Ts   int64 `json:"ts"`
}

// IsDarkMode reports whether the desktop is set to a dark theme.
// Check order: HOWMUCHLEFT_DARK env override, then a cache file in the Claude
// configuration directory younger than darkModeCacheTTLMs, then OS-specific
// detection, whose answer refreshes that cache file. Within one process the
// answer is computed once.
func IsDarkMode() bool {
	darkModeOnce.Do(func() {
		darkModeCached = detectDarkMode()
	})
	return darkModeCached
}

func detectDarkMode() bool {
	// Check env override first
	if v := os.Getenv("HOWMUCHLEFT_DARK"); v != "" {
		return v == "1"
	}

	cachePath := filepath.Join(platform.GetClaudeDir(), DarkModeCacheFile)
	if dark, ok := readDarkModeCache(cachePath); ok {
		return dark
	}

	dark := detectDarkModeFromOS()
	writeDarkModeCache(cachePath, dark)
	return dark
}

func detectDarkModeFromOS() bool {
	switch runtime.GOOS {
	case "darwin":
		return detectDarkModeDarwin()
	default:
		return detectDarkModeLinux()
	}
}

// readDarkModeCache returns the cached preference when the cache file exists
// and is younger than darkModeCacheTTLMs. The second return value says whether
// the first one means anything.
func readDarkModeCache(path string) (bool, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, false
	}
	var entry darkModeCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return false, false
	}
	age := time.Now().UnixMilli() - entry.Ts
	if age < 0 || age >= darkModeCacheTTLMs {
		return false, false
	}
	return entry.Dark, true
}

// writeDarkModeCache stores the preference with the time it was detected. A
// failure here only costs the next render another detection, so it is silent.
func writeDarkModeCache(path string, dark bool) {
	data, err := json.Marshal(darkModeCacheEntry{Dark: dark, Ts: time.Now().UnixMilli()})
	if err != nil {
		return
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".tmp-dark-mode-*")
	if err != nil {
		return
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
	}
}

func detectDarkModeDarwin() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err := exec.CommandContext(ctx, "defaults", "read", "-g", "AppleInterfaceStyle").Run()
	return err == nil
}

func detectDarkModeLinux() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gsettings",
		"get", "org.gnome.desktop.interface", "color-scheme").CombinedOutput()
	return parseDarkModeGsettings(string(out), err)
}

// parseDarkModeGsettings interprets gsettings color-scheme output.
// If the command failed and produced no output, defaults to dark.
// Otherwise, dark only if output contains "prefer-dark".
func parseDarkModeGsettings(out string, err error) bool {
	if err != nil && len(strings.TrimSpace(out)) == 0 {
		// Command not found or produced no output: default to dark
		return true
	}
	// If we got output (even with a non-zero exit), check for "prefer-dark".
	// Any other value (e.g. 'default', 'prefer-light') means light mode.
	return strings.Contains(out, "prefer-dark")
}

// RgbTo256 converts RGB to nearest 256-color 6x6x6 cube index (16-231).
func RgbTo256(r, g, b uint8) int {
	ri := int(math.Round(float64(r) / 255.0 * 5.0))
	gi := int(math.Round(float64(g) / 255.0 * 5.0))
	bi := int(math.Round(float64(b) / 255.0 * 5.0))
	return 16 + 36*ri + 6*gi + bi
}

// InterpolateRgb does linear interpolation between gradient stops at position t (0-1).
func InterpolateRgb(stops [][3]uint8, t float64) [3]uint8 {
	if len(stops) == 0 {
		return [3]uint8{0, 0, 0}
	}
	if len(stops) == 1 || t <= 0 {
		return stops[0]
	}
	if t >= 1 {
		return stops[len(stops)-1]
	}

	pos := t * float64(len(stops)-1)
	lo := int(math.Floor(pos))
	hi := lo + 1
	if hi >= len(stops) {
		hi = len(stops) - 1
	}
	frac := pos - float64(lo)

	return [3]uint8{
		uint8(math.Round(float64(stops[lo][0]) + (float64(stops[hi][0])-float64(stops[lo][0]))*frac)),
		uint8(math.Round(float64(stops[lo][1]) + (float64(stops[hi][1])-float64(stops[lo][1]))*frac)),
		uint8(math.Round(float64(stops[lo][2]) + (float64(stops[hi][2])-float64(stops[lo][2]))*frac)),
	}
}

// FormatFg formats RGB as an ANSI foreground escape sequence.
func FormatFg(r, g, b uint8, truecolor bool) string {
	if truecolor {
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
	}
	return fmt.Sprintf("\x1b[38;5;%dm", RgbTo256(r, g, b))
}

// FormatBg formats RGB as an ANSI background escape sequence.
func FormatBg(r, g, b uint8, truecolor bool) string {
	if truecolor {
		return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
	}
	return fmt.Sprintf("\x1b[48;5;%dm", RgbTo256(r, g, b))
}

// BgValue represents a background color: either RGB or a 256-color palette index.
type BgValue struct {
	IsRgb bool
	Rgb   [3]uint8
	Index int
}

// NewBgRgb creates an RGB background value.
func NewBgRgb(r, g, b uint8) BgValue {
	return BgValue{IsRgb: true, Rgb: [3]uint8{r, g, b}}
}

// NewBgIndex creates a 256-color palette index background value.
func NewBgIndex(idx int) BgValue {
	return BgValue{IsRgb: false, Index: idx}
}

// FormatBgFromValue formats a BgValue as an ANSI background escape sequence.
func FormatBgFromValue(bg BgValue, truecolor bool) string {
	if bg.IsRgb {
		return FormatBg(bg.Rgb[0], bg.Rgb[1], bg.Rgb[2], truecolor)
	}
	return fmt.Sprintf("\x1b[48;5;%dm", bg.Index)
}
