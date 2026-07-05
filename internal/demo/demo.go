package demo

import (
	"math"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/smm-h/howmuchleft/internal/config"
	"github.com/smm-h/howmuchleft/internal/render"
)

const (
	frameMs        = 100
	contextCycles  = 15
	fiveHourCycles = 8
	weeklyFillT    = 0.7 // weekly hits 100% at this fraction of total time

	model   = "claude-sonnet-4-6-20250514"
	tier    = "Max 5x"
	branch  = "feature/auth"
	cwd     = "~/Projects/myapp"
	profile = "work"

	ccVersion = "v1.0.50"
)

// WaveState holds the computed wave values for a single frame.
type WaveState struct {
	Context              float64
	FiveHour             float64
	FiveHourResetIn      int64
	FiveHourTimePercent  float64
	Weekly               float64
	WeeklyResetIn        int64
	WeeklyTimePercent    float64
	ExtraUsage           float64
	ExtraUsageEnabled    bool
}

// ComputeWaves calculates the sawtooth wave values at time fraction t (0-1).
// isLast pins all values to 100%.
func ComputeWaves(t float64, isLast bool) WaveState {
	const fiveHourMs = 5 * 60 * 60 * 1000
	const sevenDayMs = 7 * 24 * 60 * 60 * 1000

	if isLast {
		return WaveState{
			Context:             100,
			FiveHour:            100,
			FiveHourResetIn:     0,
			FiveHourTimePercent: 100,
			Weekly:              100,
			WeeklyResetIn:       0,
			WeeklyTimePercent:   100,
			ExtraUsage:          100,
			ExtraUsageEnabled:   true,
		}
	}

	// Weekly: ramps to 100% at 70% duration, then stays at 100%
	weeklyT := math.Min(1, t/weeklyFillT)
	weekly := weeklyT * 100
	weeklyResetIn := int64((1 - weeklyT) * sevenDayMs)
	weeklyTimePercent := t * 100

	// 5-hour: 8 sawtooth cycles with front-loaded ramp
	fiveCycleT := math.Mod(t*fiveHourCycles, 1)
	fiveHour := math.Pow(fiveCycleT, 0.4) * 100
	fiveHourResetIn := int64((1 - fiveCycleT) * fiveHourMs)
	fiveHourTimePercent := fiveCycleT * 100

	// Extra usage: kicks in after weekly hits 100%, ramps 0->100%
	var extraUsage float64
	extraEnabled := weekly >= 100
	if t >= weeklyFillT {
		extraT := (t - weeklyFillT) / (1 - weeklyFillT)
		extraUsage = extraT * 100
	}

	// Context: 15 sawtooth cycles
	ctxCycleT := math.Mod(t*contextCycles, 1)
	context := ctxCycleT * 100

	return WaveState{
		Context:             context,
		FiveHour:            fiveHour,
		FiveHourResetIn:     fiveHourResetIn,
		FiveHourTimePercent: fiveHourTimePercent,
		Weekly:              weekly,
		WeeklyResetIn:       weeklyResetIn,
		WeeklyTimePercent:   weeklyTimePercent,
		ExtraUsage:          extraUsage,
		ExtraUsageEnabled:   extraEnabled,
	}
}

// Run executes the demo animation for the given duration in seconds.
func Run(durationSec int) error {
	if durationSec <= 0 {
		durationSec = 60
	}

	// Load config for gradient colors, orientation, etc.
	cfg := config.Get()

	lineElements := cfg.Lines
	if lineElements == nil {
		lineElements = config.DefaultLines()
	}

	barCfg := render.BuildBarConfig(cfg)

	// Profile color: fixed hue from "work" profile path
	profileColor := render.HueToAnsi(render.HashToHue("/home/user/.claude"), render.IsDarkMode())

	// Hide cursor
	os.Stdout.Write([]byte("\x1b[?25l"))

	// Signal handling for clean exit
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	doneCh := make(chan struct{})

	cleanup := func() {
		os.Stdout.Write([]byte("\x1b[?25h"))
	}

	totalFrames := durationSec * (1000 / frameMs)
	ticker := time.NewTicker(time.Duration(frameMs) * time.Millisecond)
	defer ticker.Stop()

	frame := 0
	changes := 0
	linesAdded := 0
	linesRemoved := 0
	prevCtxCycle := 0
	startTime := time.Now()

	go func() {
		select {
		case <-sigCh:
			cleanup()
			os.Exit(0)
		case <-doneCh:
		}
	}()

	for frame < totalFrames {
		<-ticker.C

		t := float64(frame) / float64(totalFrames)
		isLast := frame == totalFrames-1

		waves := ComputeWaves(t, isLast)

		// Accumulate git stats on each context cycle reset
		currCtxCycle := int(math.Floor(t * contextCycles))
		if frame > 0 && currCtxCycle > prevCtxCycle {
			changes += rand.Intn(3) + 1
			linesAdded += rand.Intn(50) + 10
			linesRemoved += rand.Intn(20)
		}
		prevCtxCycle = currCtxCycle

		// Elapsed time: real elapsed since start
		elapsed := time.Since(startTime).Milliseconds()

		// Build RenderData
		fiveHourPct := waves.FiveHour
		weeklyPct := waves.Weekly
		fiveHourTimePct := waves.FiveHourTimePercent
		weeklyTimePct := waves.WeeklyTimePercent

		var extraUsage *render.ExtraUsageData
		if waves.ExtraUsageEnabled {
			extraUsage = &render.ExtraUsageData{
				Percent: waves.ExtraUsage,
				Enabled: true,
			}
		}

		added := linesAdded
		removed := linesRemoved

		renderData := &render.RenderData{
			Context:             waves.Context,
			Model:               model,
			Tier:                tier,
			Elapsed:             &elapsed,
			Profile:             profile,
			ProfileColor:        profileColor,
			FiveHour:            render.UsageData{Percent: &fiveHourPct, ResetIn: waves.FiveHourResetIn},
			Weekly:              render.UsageData{Percent: &weeklyPct, ResetIn: waves.WeeklyResetIn},
			ExtraUsage:          extraUsage,
			Stale:               false,
			Git:                 render.GitInfo{HasGit: true, Branch: branch, Changes: changes},
			LineChanges:         render.LineChangeInfo{Added: &added, Removed: &removed},
			Cwd:                 cwd,
			FiveHourTimePercent: &fiveHourTimePct,
			WeeklyTimePercent:   &weeklyTimePct,
			CcVersion:           ccVersion,
		}

		output := render.RenderLines(renderData, barCfg, lineElements)

		// Frame rendering: first frame prints, subsequent frames overwrite
		if frame == 0 {
			os.Stdout.Write([]byte(output + "\n"))
		} else {
			// Move up 3 lines, clear each and rewrite
			os.Stdout.Write([]byte("\x1b[3A"))
			lines := splitLines(output)
			for i, line := range lines {
				os.Stdout.Write([]byte("\x1b[2K" + line))
				if i < len(lines)-1 {
					os.Stdout.Write([]byte("\n"))
				}
			}
			os.Stdout.Write([]byte("\n"))
		}

		frame++
	}

	close(doneCh)
	cleanup()
	return nil
}

// splitLines splits output into individual lines (up to 3).
func splitLines(s string) []string {
	result := []string{"", "", ""}
	idx := 0
	start := 0
	for i := 0; i < len(s) && idx < 3; i++ {
		if s[i] == '\n' {
			result[idx] = s[start:i]
			idx++
			start = i + 1
		}
	}
	if idx < 3 && start <= len(s) {
		result[idx] = s[start:]
	}
	return result
}


