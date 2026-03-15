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

// Total lines rendered per frame (wave label + 3 bars)
const FRAME_LINES = 4;

/**
 * Render one frame: wave label + 3 statusline bars.
 * Always renders exactly FRAME_LINES lines and moves cursor back up to overwrite.
 */
function renderFrame(state, waveLabel, isFirst) {
  if (!isFirst) process.stdout.write(`\x1b[${FRAME_LINES}A`);

  const lines = [];

  // Line 0: wave label
  lines.push(`${colors.dim}-- ${waveLabel} --${colors.reset}`);

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

  process.stdout.write(lines.map(l => '\x1b[2K' + l).join('\n') + '\n');
}

/**
 * Run the demo animation.
 *
 * Usage growth rates are tuned so each wave showcases its target bar
 * reaching high percentages (60-90%+), making the gradient visible.
 */
function runDemo() {
  // Hide cursor during animation
  process.stdout.write('\x1b[?25l');

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
  let contextTarget = 30 + Math.random() * 70;
  let contextDir = 1; // 1 = filling, -1 = resetting
  let fiveHourUsed = 5;
  let weeklyUsed = 2;
  let fiveHourResetAt = FIVE_HOUR_MS;
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
    simTime += simDelta;

    // -- Context: fill up then reset, cycling --
    if (contextDir === 1) {
      context += simDelta / (60 * 1000) * 3;
      if (context >= contextTarget) {
        contextDir = -1;
        contextTarget = Math.random() * 20;
      }
    } else {
      context -= simDelta / (60 * 1000) * 30;
      if (context <= contextTarget) {
        context = contextTarget;
        contextDir = 1;
        contextTarget = 30 + Math.random() * 70;
        changes += Math.floor(Math.random() * 3) + 1;
        linesAdded += Math.floor(Math.random() * 50) + 10;
        linesRemoved += Math.floor(Math.random() * 20);
      }
    }
    context = Math.max(0, Math.min(100, context));

    // -- 5-hour usage: grows aggressively so it reaches 60-90% during wave 2 --
    if (contextDir === 1) {
      fiveHourUsed += simDelta / FIVE_HOUR_MS * 80;
    }
    fiveHourResetAt -= simDelta;
    if (fiveHourResetAt <= 0) {
      fiveHourUsed = Math.max(0, fiveHourUsed * 0.3);
      fiveHourResetAt = FIVE_HOUR_MS;
    }
    fiveHourUsed = Math.min(100, fiveHourUsed);

    // -- 7-day usage: grows steadily so it reaches 50-80% during wave 3 --
    weeklyUsed += simDelta / SEVEN_DAY_MS * 50;
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
    }, wave.label, isFirst);

    isFirst = false;
    frameInWave++;

    if (frameInWave >= wave.frames) {
      waveIdx++;
      frameInWave = 0;
    }
  }, FRAME_MS);
}

module.exports = { runDemo };
