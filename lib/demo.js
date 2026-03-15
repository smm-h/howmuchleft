/**
 * Demo mode: time-lapse animation showing the statusline in action.
 *
 * Three waves at increasing time scales:
 *   Wave 1: 1 minute = 1 second  (~15s, context bar fills up)
 *   Wave 2: 1 hour   = 1 second  (~15s, 5-hour bar moves)
 *   Wave 3: 6 hours  = 1 second  (~15s, 7-day bar moves)
 *
 * Context cycles between 0-20% and 30-100% to simulate conversations.
 * 5-hour and 7-day windows tick down and reset realistically.
 */

const { progressBar, formatPercent, formatTimeRemaining, colors } = require('./statusline');

const FRAME_MS = 100;

// Simulated constants
const FIVE_HOUR_MS = 5 * 60 * 60 * 1000;
const SEVEN_DAY_MS = 7 * 24 * 60 * 60 * 1000;

// Fake metadata
const MODEL = 'Claude Sonnet 4';
const TIER = 'Max 5x';
const BRANCH = 'feature/auth';
const CWD = '~/Projects/myapp';

/**
 * Render one frame of the statusline with given state.
 * Uses ANSI cursor movement to overwrite in place.
 */
function renderFrame(state, isFirst) {
  // Move cursor up 3 lines to overwrite (skip on first frame)
  if (!isFirst) process.stdout.write('\x1b[3A');

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
  const changes = state.changes;
  lines.push(
    `${fiveBar} ${fivePct} ` +
    `${colors.dim}${fiveReset}${colors.reset} ` +
    `${colors.cyan}${BRANCH}${colors.reset}` +
    (changes > 0 ? ` ${colors.yellow}+${changes}${colors.reset}` : '') +
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

  // Clear each line before writing (handles variable-length content)
  process.stdout.write(lines.map(l => '\x1b[2K' + l).join('\n') + '\n');
}

/**
 * Run the demo animation.
 *
 * Each wave simulates a different time scale. simTimePerFrameMs controls
 * how much simulated time passes per 100ms real frame.
 */
function runDemo() {
  // Hide cursor during animation
  process.stdout.write('\x1b[?25l');

  // Restore cursor on exit
  function cleanup() {
    process.stdout.write('\x1b[?25h');
    process.exit(0);
  }
  process.on('SIGINT', cleanup);
  process.on('SIGTERM', cleanup);

  const waves = [
    { label: '1 min = 1 sec', simPerFrame: 60 * 1000 / (1000 / FRAME_MS), frames: 150 },
    { label: '1 hour = 1 sec', simPerFrame: 3600 * 1000 / (1000 / FRAME_MS), frames: 150 },
    { label: '6 hours = 1 sec', simPerFrame: 6 * 3600 * 1000 / (1000 / FRAME_MS), frames: 150 },
  ];

  // State
  let simTime = 0;
  let context = 0;
  let contextTarget = 30 + Math.random() * 70; // first fill target
  let contextDir = 1; // 1 = filling, -1 = resetting
  let fiveHourUsed = 5; // percent used in 5-hour window
  let weeklyUsed = 2; // percent used in 7-day window
  let fiveHourResetAt = FIVE_HOUR_MS; // simulated time until reset
  let weeklyResetAt = SEVEN_DAY_MS;
  let changes = 0;
  let linesAdded = 0;
  let linesRemoved = 0;
  let isFirst = true;
  let waveIdx = 0;
  let frameInWave = 0;

  const interval = setInterval(() => {
    if (waveIdx >= waves.length) {
      clearInterval(interval);
      cleanup();
      return;
    }

    const wave = waves[waveIdx];
    const simDelta = wave.simPerFrame;

    // Show wave label at the start of each wave
    if (frameInWave === 0) {
      if (!isFirst) {
        // Pause briefly between waves (label is shown via the frame)
        process.stdout.write('\x1b[2K');
      }
      const labelLine = `${colors.dim}-- ${wave.label} --${colors.reset}`;
      process.stdout.write('\x1b[2K' + labelLine + '\n');
    }

    simTime += simDelta;

    // Context: fill up then reset, cycling
    if (contextDir === 1) {
      // Filling rate scales with wave speed (faster waves = faster context cycles)
      context += simDelta / (60 * 1000) * 3; // ~3% per simulated minute
      if (context >= contextTarget) {
        contextDir = -1;
        contextTarget = Math.random() * 20; // reset target: 0-20%
      }
    } else {
      // Reset is quick (new conversation)
      context -= simDelta / (60 * 1000) * 30;
      if (context <= contextTarget) {
        context = contextTarget;
        contextDir = 1;
        contextTarget = 30 + Math.random() * 70; // new fill target: 30-100%
        // New conversation: bump git stats
        changes += Math.floor(Math.random() * 3) + 1;
        linesAdded += Math.floor(Math.random() * 50) + 10;
        linesRemoved += Math.floor(Math.random() * 20);
      }
    }
    context = Math.max(0, Math.min(100, context));

    // 5-hour usage: climbs with context activity, resets at 5-hour marks
    if (contextDir === 1) {
      fiveHourUsed += simDelta / FIVE_HOUR_MS * 15; // usage grows while active
    }
    fiveHourResetAt -= simDelta;
    if (fiveHourResetAt <= 0) {
      // Window resets: usage drops significantly but not to zero
      fiveHourUsed = Math.max(0, fiveHourUsed * 0.3);
      fiveHourResetAt = FIVE_HOUR_MS;
    }
    fiveHourUsed = Math.min(100, fiveHourUsed);

    // 7-day usage: climbs slowly, resets at 7-day marks
    weeklyUsed += simDelta / SEVEN_DAY_MS * 8;
    weeklyResetAt -= simDelta;
    if (weeklyResetAt <= 0) {
      weeklyUsed = Math.max(0, weeklyUsed * 0.2);
      weeklyResetAt = SEVEN_DAY_MS;
    }
    weeklyUsed = Math.min(100, weeklyUsed);

    renderFrame({
      context,
      fiveHour: fiveHourUsed,
      fiveHourResetIn: Math.max(0, fiveHourResetAt),
      weekly: weeklyUsed,
      weeklyResetIn: Math.max(0, weeklyResetAt),
      changes,
      linesAdded,
      linesRemoved,
    }, isFirst);

    isFirst = false;
    frameInWave++;

    if (frameInWave >= wave.frames) {
      waveIdx++;
      frameInWave = 0;
    }
  }, FRAME_MS);
}

module.exports = { runDemo };
