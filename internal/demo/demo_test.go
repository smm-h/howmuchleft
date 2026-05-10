package demo

import (
	"math"
	"testing"
)

func TestComputeWaves_AtZero(t *testing.T) {
	w := ComputeWaves(0, false)

	if w.Context != 0 {
		t.Errorf("Context at t=0: got %f, want 0", w.Context)
	}
	if w.FiveHour != 0 {
		t.Errorf("FiveHour at t=0: got %f, want 0 (pow(0, 0.4) = 0)", w.FiveHour)
	}
	if w.Weekly != 0 {
		t.Errorf("Weekly at t=0: got %f, want 0", w.Weekly)
	}
	if w.ExtraUsage != 0 {
		t.Errorf("ExtraUsage at t=0: got %f, want 0", w.ExtraUsage)
	}
	if w.ExtraUsageEnabled {
		t.Error("ExtraUsageEnabled at t=0: got true, want false")
	}
	if w.FiveHourTimePercent != 0 {
		t.Errorf("FiveHourTimePercent at t=0: got %f, want 0", w.FiveHourTimePercent)
	}
	if w.WeeklyTimePercent != 0 {
		t.Errorf("WeeklyTimePercent at t=0: got %f, want 0", w.WeeklyTimePercent)
	}
}

func TestComputeWaves_AtHalf(t *testing.T) {
	w := ComputeWaves(0.5, false)

	// Context: 0.5 * 15 = 7.5, mod 1 = 0.5 -> 50%
	expectCtx := math.Mod(0.5*contextCycles, 1) * 100
	if math.Abs(w.Context-expectCtx) > 0.01 {
		t.Errorf("Context at t=0.5: got %f, want %f", w.Context, expectCtx)
	}

	// Weekly: 0.5 / 0.7 = ~0.714 -> 71.4%
	expectWeekly := math.Min(1, 0.5/weeklyFillT) * 100
	if math.Abs(w.Weekly-expectWeekly) > 0.01 {
		t.Errorf("Weekly at t=0.5: got %f, want %f", w.Weekly, expectWeekly)
	}

	// 5-hour: (0.5 * 8) % 1 = 0, pow(0, 0.4) = 0 -> 0%
	fiveCycleT := math.Mod(0.5*fiveHourCycles, 1)
	expectFive := math.Pow(fiveCycleT, 0.4) * 100
	if math.Abs(w.FiveHour-expectFive) > 0.01 {
		t.Errorf("FiveHour at t=0.5: got %f, want %f", w.FiveHour, expectFive)
	}

	// Extra usage not yet enabled at t=0.5 (weekly < 100%)
	if w.ExtraUsageEnabled {
		t.Error("ExtraUsageEnabled at t=0.5: got true, want false (weekly < 100%)")
	}
}

func TestComputeWaves_IsLast(t *testing.T) {
	w := ComputeWaves(0.99, true)

	if w.Context != 100 {
		t.Errorf("Context isLast: got %f, want 100", w.Context)
	}
	if w.FiveHour != 100 {
		t.Errorf("FiveHour isLast: got %f, want 100", w.FiveHour)
	}
	if w.Weekly != 100 {
		t.Errorf("Weekly isLast: got %f, want 100", w.Weekly)
	}
	if w.ExtraUsage != 100 {
		t.Errorf("ExtraUsage isLast: got %f, want 100", w.ExtraUsage)
	}
	if !w.ExtraUsageEnabled {
		t.Error("ExtraUsageEnabled isLast: got false, want true")
	}
	if w.FiveHourTimePercent != 100 {
		t.Errorf("FiveHourTimePercent isLast: got %f, want 100", w.FiveHourTimePercent)
	}
	if w.WeeklyTimePercent != 100 {
		t.Errorf("WeeklyTimePercent isLast: got %f, want 100", w.WeeklyTimePercent)
	}
	if w.FiveHourResetIn != 0 {
		t.Errorf("FiveHourResetIn isLast: got %d, want 0", w.FiveHourResetIn)
	}
	if w.WeeklyResetIn != 0 {
		t.Errorf("WeeklyResetIn isLast: got %d, want 0", w.WeeklyResetIn)
	}
}

func TestComputeWaves_WeeklyTransition(t *testing.T) {
	// Just before 70%: weekly should be < 100%
	wBefore := ComputeWaves(0.69, false)
	if wBefore.Weekly >= 100 {
		t.Errorf("Weekly at t=0.69: got %f, want < 100", wBefore.Weekly)
	}
	if wBefore.ExtraUsageEnabled {
		t.Error("ExtraUsageEnabled at t=0.69: got true, want false")
	}
	if wBefore.ExtraUsage != 0 {
		t.Errorf("ExtraUsage at t=0.69: got %f, want 0", wBefore.ExtraUsage)
	}

	// At 70%: weekly should be 100%
	wAt := ComputeWaves(0.7, false)
	if math.Abs(wAt.Weekly-100) > 0.01 {
		t.Errorf("Weekly at t=0.7: got %f, want 100", wAt.Weekly)
	}
	if !wAt.ExtraUsageEnabled {
		t.Error("ExtraUsageEnabled at t=0.7: got false, want true")
	}

	// At 85% (midway through extra): extra should be ~50%
	wMid := ComputeWaves(0.85, false)
	if math.Abs(wMid.Weekly-100) > 0.01 {
		t.Errorf("Weekly at t=0.85: got %f, want 100", wMid.Weekly)
	}
	if !wMid.ExtraUsageEnabled {
		t.Error("ExtraUsageEnabled at t=0.85: got false, want true")
	}
	expectedExtra := (0.85 - 0.7) / (1 - 0.7) * 100
	if math.Abs(wMid.ExtraUsage-expectedExtra) > 0.01 {
		t.Errorf("ExtraUsage at t=0.85: got %f, want %f", wMid.ExtraUsage, expectedExtra)
	}
}

func TestComputeWaves_ContextCycleReset(t *testing.T) {
	// Context resets at each cycle boundary: t = 1/15, 2/15, etc.
	// Just after a reset, context should be near 0.
	resetT := 1.0 / contextCycles
	w := ComputeWaves(resetT+0.001, false)
	if w.Context > 5 {
		t.Errorf("Context just after cycle reset: got %f, want near 0", w.Context)
	}
}

func TestComputeWaves_TimePercentLinear(t *testing.T) {
	// Weekly time percent is simply t * 100 (linear)
	w := ComputeWaves(0.3, false)
	if math.Abs(w.WeeklyTimePercent-30) > 0.01 {
		t.Errorf("WeeklyTimePercent at t=0.3: got %f, want 30", w.WeeklyTimePercent)
	}

	// FiveHour time percent is cycleT * 100 within each cycle
	// At t=0.3, cycle = 0.3*8 = 2.4, frac = 0.4 -> 40%
	fiveCycleT := math.Mod(0.3*fiveHourCycles, 1)
	expected := fiveCycleT * 100
	if math.Abs(w.FiveHourTimePercent-expected) > 0.01 {
		t.Errorf("FiveHourTimePercent at t=0.3: got %f, want %f", w.FiveHourTimePercent, expected)
	}
}
