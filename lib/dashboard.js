/**
 * Dashboard mode: multi-profile usage overview.
 *
 * Rendering is a trimmed copy of renderLines() from statusline.js:
 * - No context bar (2 columns instead of 3, or 3 with extra usage)
 * - No git info, cwd, model, or elapsed time
 * - Resets credential cache between profiles
 *
 * Default: single frame. --live: refresh loop.
 */

const fs = require('fs');
const path = require('path');

const {
  discoverProfiles, CONFIG_PATH, progressBar, formatPercent, formatExtraPercent,
  formatTimeRemaining, colors, getUsageData, getAuthInfo, hashToHue, hueToAnsi,
  isDarkMode, getExtraUsageBg, getBarConfig, verticalBarCell,
} = require('./statusline');

const REFRESH_INTERVAL_MS = 30_000;

function profileNameFromDir(dir) {
  const base = path.basename(dir);
  if (base === '.claude') return 'default';
  if (base.startsWith('.claude-')) return base.slice('.claude-'.length) || base;
  return base;
}

function readCredentialsDirect(dir) {
  try {
    return JSON.parse(fs.readFileSync(path.join(dir, '.credentials.json'), 'utf8'));
  } catch {
    return null;
  }
}

/**
 * Dashboard-specific renderer. Trimmed from renderLines():
 * - 2 bar columns (5hr, weekly/extra) instead of 3
 * - Row 0: bars + profile name + tier
 * - Row 1: bars + 5hr percent + reset
 * - Row 2: bars + weekly/extra percent + reset
 */
function renderDashboardLines({ tier, profile, profileColor, fiveHour, weekly, extraUsage, stale }) {
  const config = getBarConfig();
  const lines = [];

  const profileStr = profile ? `${profileColor || colors.dim}${profile}${colors.reset} ` : '';
  const fiveHourPercent = formatPercent(fiveHour.percent, stale);
  const fiveHourReset = fiveHour.resetIn != null && fiveHour.percent > 0
    ? formatTimeRemaining(fiveHour.resetIn) : '';
  const weeklyPercent = formatPercent(weekly.percent, stale);
  const weeklyReset = weekly.resetIn != null ? formatTimeRemaining(weekly.resetIn) : '';
  const showExtraUsage = extraUsage?.enabled && extraUsage?.percent != null;

  if (config.orientation === 'vertical') {
    const warmBg = showExtraUsage ? getExtraUsageBg() : null;
    const thirdPercent = showExtraUsage ? extraUsage.percent : weekly.percent;
    const percents = [fiveHour.percent, thirdPercent];
    for (let row = 0; row < 3; row++) {
      let barStr = '';
      for (let i = 0; i < percents.length; i++) {
        if (i > 0) barStr += `${colors.reset} `;
        const bgOverride = (i === 1 && warmBg) ? warmBg : null;
        barStr += verticalBarCell(percents[i], row, 3, bgOverride);
      }
      barStr += colors.reset;

      if (row === 0) {
        lines.push(
          `${barStr} ${profileStr}${colors.magenta}${tier}${colors.reset}`
        );
      } else if (row === 1) {
        lines.push(
          `${barStr} ${fiveHourPercent}` +
          (fiveHourReset ? ` ${colors.dim}${fiveHourReset}${colors.reset}` : '')
        );
      } else {
        const thirdPct = showExtraUsage
          ? formatExtraPercent(extraUsage.percent, stale)
          : weeklyPercent;
        lines.push(
          `${barStr} ${thirdPct}` +
          (weeklyReset ? ` ${colors.dim}${weeklyReset}${colors.reset}` : '')
        );
      }
    }
  } else {
    // Horizontal: each line has its own progress bar
    const warmBg = showExtraUsage ? getExtraUsageBg() : null;

    const fiveHourBar = progressBar(fiveHour.percent);
    lines.push(
      `${fiveHourBar} ${fiveHourPercent}` +
      (fiveHourReset ? ` ${colors.dim}${fiveHourReset}${colors.reset}` : '') +
      ` ${profileStr}${colors.magenta}${tier}${colors.reset}`
    );

    const thirdPercent = showExtraUsage ? extraUsage.percent : weekly.percent;
    const thirdBar = progressBar(thirdPercent, null, warmBg);
    const thirdPct = showExtraUsage
      ? formatExtraPercent(extraUsage.percent, stale)
      : weeklyPercent;
    lines.push(
      `${thirdBar} ${thirdPct}` +
      (weeklyReset ? ` ${colors.dim}${weeklyReset}${colors.reset}` : '')
    );
  }

  return lines.join('\n');
}

async function fetchAndRender(dir) {
  const name = profileNameFromDir(dir);
  const hue = hashToHue(dir);
  const profileColor = hueToAnsi(hue, isDarkMode());

  const creds = readCredentialsDirect(dir);
  const oauth = creds?.claudeAiOauth || {};
  const { isOAuth, subscriptionName } = getAuthInfo(oauth);

  if (!isOAuth) return null;

  let usage;
  try {
    usage = await getUsageData(dir);
  } catch {
    return null;
  }

  return renderDashboardLines({
    tier: subscriptionName,
    profile: name,
    profileColor,
    fiveHour: usage.fiveHour,
    weekly: usage.weekly,
    extraUsage: usage.extraUsage,
    stale: usage.stale,
  });
}

async function renderDashboard(dirs) {
  const results = await Promise.all(dirs.map(fetchAndRender));
  const rendered = results.filter(Boolean);

  if (rendered.length === 0) {
    return `${colors.gray}No OAuth profiles found.${colors.reset}`;
  }

  return rendered.join('\n\n');
}

async function runOnce(dirs) {
  const output = await renderDashboard(dirs);
  console.log(output);
}

async function runLive(dirs) {
  process.stdout.write('\x1b[?25l');

  async function render() {
    const now = new Date();
    const timeStr = now.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    process.stdout.write('\x1b[2J\x1b[H');
    console.log(`${colors.dim}howmuchleft ${timeStr}  (refreshes every 30s, ctrl+c to exit)${colors.reset}`);
    console.log();
    const output = await renderDashboard(dirs);
    console.log(output);
  }

  await render();

  const timer = setInterval(async () => {
    try {
      await render();
    } catch (err) {
      process.stderr.write(`Refresh error: ${err.message}\n`);
    }
  }, REFRESH_INTERVAL_MS);

  function cleanup() {
    clearInterval(timer);
    process.stdout.write(`\x1b[?25h\n${colors.reset}`);
    process.exit(0);
  }
  process.on('SIGINT', cleanup);
  process.on('SIGTERM', cleanup);
}

async function runDashboard(live = false) {
  const dirs = discoverProfiles();

  if (dirs.length === 0) {
    console.log(`${colors.gray}No Claude Code profiles found.${colors.reset}`);
    console.log(`${colors.gray}Run 'howmuchleft --install' first, or add dirs to "profiles" in ${CONFIG_PATH}${colors.reset}`);
    process.exit(0);
  }

  if (live) {
    await runLive(dirs);
  } else {
    await runOnce(dirs);
  }
}

module.exports = { runDashboard };
