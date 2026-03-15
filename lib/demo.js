/**
 * Demo mode: time-lapse animation showing the statusline filling up.
 *
 * Single continuous run (default 60s, configurable):
 *   - Weekly bar:  0→100%, 7d remaining → 0  (1 cycle)
 *   - 5-hour bar:  0→100%, 5h remaining → 0  (8 cycles)
 *   - Context bar: 0→100%                    (15 cycles)
 *
 * Git changes and lines added/removed accumulate on each context reset.
 */

const { progressBar, formatPercent, formatTimeRemaining, colors } = require('./statusline');

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

/**
 * Render one frame: 3 statusline bars.
 * Overwrites previous frame by moving cursor up.
 */
function renderFrame(state, isFirst) {
  if (!isFirst) process.stdout.write(`\x1b[${FRAME_LINES}A`);

  const lines = [];

  // Line 1: context bar
  const ctxBar = progressBar(state.context);
  lines.push(
    `${ctxBar} ${colors.cyan}${Math.round(state.context)}%${colors.reset} ` +
    `${colors.magenta}${TIER}${colors.reset} ` +
    `${colors.white}${MODEL}${colors.reset}`
  );

  // Line 2: 5-hour bar + git info
  const fiveBar = progressBar(state.fiveHour);
  const fivePct = formatPercent(state.fiveHour);
  const fiveReset = formatTimeRemaining(state.fiveHourResetIn);
  lines.push(
    `${fiveBar} ${fivePct} ` +
    `${colors.dim}${fiveReset}${colors.reset} ` +
    `${colors.cyan}${BRANCH}${colors.reset}` +
    (state.changes > 0 ? ` ${colors.yellow}+${state.changes}${colors.reset}` : '') +
    ` ${colors.green}+${state.linesAdded}${colors.reset}/${colors.red}-${state.linesRemoved}${colors.reset}`
  );

  // Line 3: weekly bar + cwd
  const weekBar = progressBar(state.weekly);
  const weekPct = formatPercent(state.weekly);
  const weekReset = formatTimeRemaining(state.weeklyResetIn);
  lines.push(
    `${weekBar} ${weekPct} ` +
    `${colors.dim}${weekReset}${colors.reset} ` +
    `${colors.white}${CWD}${colors.reset}`
  );

  process.stdout.write(lines.map(l => '\x1b[2K' + l).join('\n') + '\n');
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

    // Weekly: single ramp 0→100%, 7d→0
    const weekly = isLast ? 100 : t * 100;
    const weeklyResetIn = isLast ? 0 : (1 - t) * SEVEN_DAY_MS;

    // 5-hour: 8 sawtooth cycles (last frame pinned to 100%)
    const fiveCycleT = (t * FIVE_HOUR_CYCLES) % 1;
    const fiveHour = isLast ? 100 : fiveCycleT * 100;
    const fiveHourResetIn = isLast ? 0 : (1 - fiveCycleT) * FIVE_HOUR_MS;

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
      changes,
      linesAdded,
      linesRemoved,
    }, frame === 0);

    frame++;
  }, FRAME_MS);
}

module.exports = { runDemo };
