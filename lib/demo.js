/**
 * Demo mode: time-lapse animation showing the statusline filling up.
 *
 * Single continuous run (default 60s, configurable):
 *   - Weekly bar:      0→100% over first 70% of duration, then stays at 100%
 *   - Extra usage bar: kicks in once weekly hits 100%, ramps 0→100% over remaining 30%
 *   - 5-hour bar:      0→100%, 5h remaining → 0  (8 cycles)
 *   - Context bar:     0→100%                     (15 cycles)
 *
 * Git changes and lines added/removed accumulate on each context reset.
 */

const { renderLines } = require('./statusline');

const FRAME_MS = 100;

const FIVE_HOUR_MS = 5 * 60 * 60 * 1000;
const SEVEN_DAY_MS = 7 * 24 * 60 * 60 * 1000;

const MODEL = 'Opus 4.5';
const TIER = 'Max 5x';
const BRANCH = 'feature/auth';
const CWD = '~/Projects/myapp';

const FRAME_LINES = 3;

const CONTEXT_CYCLES = 15;
const FIVE_HOUR_CYCLES = 8;
const WEEKLY_FILL_T = 0.7; // weekly hits 100% at this fraction of total time

/**
 * Render one frame: 3 statusline bars.
 * Overwrites previous frame by moving cursor up.
 */
function renderFrame(state, isFirst) {
  if (!isFirst) process.stdout.write(`\x1b[${FRAME_LINES}A`);

  const output = renderLines({
    context: state.context,
    model: MODEL,
    tier: TIER,
    elapsed: null,
    fiveHour: { percent: state.fiveHour, resetIn: state.fiveHourResetIn },
    weekly: { percent: state.weekly, resetIn: state.weeklyResetIn },
    stale: false,
    git: { hasGit: true, branch: BRANCH, changes: state.changes },
    lines: { added: state.linesAdded, removed: state.linesRemoved },
    cwd: CWD,
    extraUsage: { percent: state.extraUsage, enabled: state.weekly >= 100 },
  });

  // Clear each line before writing (animation overwrites previous frame)
  process.stdout.write(output.split('\n').map(l => '\x1b[2K' + l).join('\n') + '\n');
}

/**
 * Run the demo animation.
 * @param {number} [durationSec=60] Total duration in seconds.
 */
function runDemo(durationSec = 60) {
  process.stdout.write('\x1b[?25l');

  function cleanup() {
    process.stdout.write('\x1b[?25h');
    process.exit(0);
  }
  process.on('SIGINT', cleanup);
  process.on('SIGTERM', cleanup);

  const totalFrames = durationSec * (1000 / FRAME_MS);
  let frame = 0;
  let changes = 0;
  let linesAdded = 0;
  let linesRemoved = 0;
  let prevCtxCycle = 0;

  const interval = setInterval(() => {
    if (frame >= totalFrames) {
      clearInterval(interval);
      cleanup();
      return;
    }

    const t = frame / totalFrames; // 0→1 over full duration
    const isLast = frame === totalFrames - 1;

    // Weekly: ramps to 100% at 70% duration, then stays at 100%
    const weeklyT = Math.min(1, t / WEEKLY_FILL_T);
    const weekly = isLast ? 100 : weeklyT * 100;
    const weeklyResetIn = isLast ? 0 : (1 - weeklyT) * SEVEN_DAY_MS;

    // 5-hour: 8 sawtooth cycles (last frame pinned to 100%)
    const fiveCycleT = (t * FIVE_HOUR_CYCLES) % 1;
    const fiveHour = isLast ? 100 : fiveCycleT * 100;
    const fiveHourResetIn = isLast ? 0 : (1 - fiveCycleT) * FIVE_HOUR_MS;

    // Extra usage: kicks in after weekly hits 100%, ramps 0→100%
    let extraUsage = 0;
    if (t >= WEEKLY_FILL_T) {
      const extraT = (t - WEEKLY_FILL_T) / (1 - WEEKLY_FILL_T);
      extraUsage = isLast ? 100 : extraT * 100;
    }

    // Context: 15 sawtooth cycles (last frame pinned to 100%)
    const ctxCycleT = (t * CONTEXT_CYCLES) % 1;
    const context = isLast ? 100 : ctxCycleT * 100;

    // Accumulate git stats on each context reset
    const currCtxCycle = Math.floor(t * CONTEXT_CYCLES);
    if (frame > 0 && currCtxCycle > prevCtxCycle) {
      changes += Math.floor(Math.random() * 3) + 1;
      linesAdded += Math.floor(Math.random() * 50) + 10;
      linesRemoved += Math.floor(Math.random() * 20);
    }
    prevCtxCycle = currCtxCycle;

    renderFrame({
      context,
      fiveHour,
      fiveHourResetIn,
      weekly,
      weeklyResetIn,
      extraUsage,
      changes,
      linesAdded,
      linesRemoved,
    }, frame === 0);

    frame++;
  }, FRAME_MS);
}

module.exports = { runDemo };
