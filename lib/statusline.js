/**
 * HowMuchLeft - Claude Code Statusline
 *
 * Displays a 3-line statusline with pixel-perfect progress bars:
 * 1. Context usage bar, subscription tier, model name
 * 2. 5-hour usage bar + time to reset, git branch/changes, lines added/removed
 * 3. Weekly usage bar + time to reset, current directory
 *
 * Claude Code spawns this as a child process, piping JSON to stdin with
 * model info, context window usage, cwd, and cost data.
 *
 * Fetches real usage data from Anthropic's OAuth API (with caching),
 * and auto-refreshes expired OAuth tokens.
 * API key users (no OAuth) see "API" label with no usage bars.
 */

const fs = require('fs');
const path = require('path');
const https = require('https');
const { execFile, execFileSync } = require('child_process');
const { promisify } = require('util');
const os = require('os');

const execFileAsync = promisify(execFile);

// -- OS dark/light mode detection --
// Cached per-process (theme won't change mid-render).
let _isDark = null;
function isDarkMode() {
  if (_isDark !== null) return _isDark;
  try {
    if (process.platform === 'darwin') {
      // Returns "Dark" in dark mode, throws in light mode (key doesn't exist)
      execFileSync('defaults', ['read', '-g', 'AppleInterfaceStyle'], { timeout: 1000 });
      _isDark = true;
    } else {
      const scheme = execFileSync('gsettings', [
        'get', 'org.gnome.desktop.interface', 'color-scheme',
      ], { encoding: 'utf8', timeout: 1000 }).trim();
      _isDark = scheme.includes('prefer-dark');
    }
  } catch {
    // macOS light mode throws (key absent), Linux failure defaults to dark
    _isDark = process.platform === 'darwin' ? false : true;
  }
  return _isDark;
}

// -- ANSI color codes --
const colors = {
  reset: '\x1b[0m',
  bold: '\x1b[1m',
  dim: '\x1b[2m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  orange: '\x1b[38;5;208m',
  red: '\x1b[31m',
  cyan: '\x1b[36m',
  magenta: '\x1b[35m',
  white: '\x1b[37m',
  gray: '\x1b[90m',
};

// -- Subscription tier display names --
const TIER_NAMES = {
  'default_claude_pro': 'Pro',
  'default_claude_max_5x': 'Max 5x',
  'default_claude_max_20x': 'Max 20x',
};

// -- Progress bar: left fractional block characters for sub-cell precision --
const FRACTIONAL_CHARS = ['\u258F','\u258E','\u258D','\u258C','\u258B','\u258A','\u2589'];

// Default gradient: green -> yellow-green -> yellow -> orange -> red (256-color indices)
const DEFAULT_GRADIENT = [46, 82, 118, 154, 190, 226, 220, 214, 208, 202, 196];

// Config is loaded from ~/.config/howmuchleft.json (XDG-style, install-method-agnostic).
// Cached per-process to avoid redundant fs reads (called once per progressBar, 3x per render).
const CONFIG_PATH = path.join(os.homedir(), '.config', 'howmuchleft.json');

// Validate a gradient array: must be non-empty array of integers 0-255
function isValidGradient(arr) {
  return Array.isArray(arr) && arr.length > 0 &&
    arr.every(v => Number.isInteger(v) && v >= 0 && v <= 255);
}

let _barConfig = null;
function getBarConfig() {
  if (_barConfig) return _barConfig;
  try {
    const config = JSON.parse(fs.readFileSync(CONFIG_PATH, 'utf8'));
    const w = Number(config.progressLength);
    const dark = isDarkMode();
    const rawBg = Number(dark ? config.emptyBgDark : config.emptyBgLight);
    _barConfig = {
      width: (Number.isInteger(w) && w >= 3 && w <= 40) ? w : 12,
      emptyBg: (Number.isInteger(rawBg) && rawBg >= 0 && rawBg <= 255) ? rawBg : (dark ? 236 : 252),
      gradient: isValidGradient(config.gradient) ? config.gradient : DEFAULT_GRADIENT,
    };
  } catch {
    _barConfig = { width: 12, emptyBg: isDarkMode() ? 236 : 252, gradient: DEFAULT_GRADIENT };
  }
  return _barConfig;
}

const OAUTH_CLIENT_ID = '9d1c250a-e61b-44d9-88ed-5944d1962f5e';

// Cache TTL: 60s for success, 5min for failures (avoids hammering a dead API)
const CACHE_TTL_MS = 60_000;
const ERROR_CACHE_TTL_MS = 300_000;

function resolvePath(p) {
  if (p.startsWith('~')) return path.join(os.homedir(), p.slice(1));
  return path.resolve(p);
}

function getClaudeDir() {
  const arg = process.argv[2];
  if (!arg || arg.startsWith('-')) return process.env.CLAUDE_CONFIG_DIR || path.join(os.homedir(), '.claude');
  return resolvePath(arg);
}

function readCredentialsFile(claudeDir) {
  try {
    return JSON.parse(fs.readFileSync(path.join(claudeDir, '.credentials.json'), 'utf8'));
  } catch {
    return null;
  }
}

/**
 * Determine auth type and subscription display name.
 * OAuth users: "Pro", "Max 5x", "Max 20x", "Team Pro", etc.
 * API key users: "API" (no usage API available).
 */
function getAuthInfo(oauth) {
  if (!oauth.accessToken) {
    return { isOAuth: false, subscriptionName: 'API' };
  }
  const baseName = TIER_NAMES[oauth.rateLimitTier] || 'Pro';
  const subscriptionName = (oauth.subscriptionType === 'team' && !baseName.startsWith('Team'))
    ? `Team ${baseName}`
    : baseName;
  return { isOAuth: true, subscriptionName };
}

/**
 * Atomic file write using tmpfile + rename(2).
 * rename(2) is atomic on POSIX for same-filesystem operations,
 * so readers always see either the complete old or complete new file.
 */
function writeFileAtomic(filePath, data) {
  const tmpPath = filePath + '.tmp.' + process.pid;
  try {
    fs.writeFileSync(tmpPath, data);
    fs.renameSync(tmpPath, filePath);
  } catch (err) {
    try { fs.unlinkSync(tmpPath); } catch {}
    throw err;
  }
}

/**
 * Refresh an expired OAuth token.
 *
 * Validates response before writing back:
 * - Non-2xx = failure
 * - expires_in validated to prevent NaN poisoning (NaN -> JSON null -> perpetual refresh loop)
 * - Atomic write to prevent corruption
 *
 * Known limitation: two Claude sessions refreshing simultaneously can race.
 * OAuth refresh tokens are single-use, so the loser's token is invalidated.
 */
function refreshToken(claudeDir, credFile, oauth) {
  return new Promise((resolve) => {
    if (!oauth.refreshToken) { resolve(null); return; }

    const postData = `grant_type=refresh_token&refresh_token=${encodeURIComponent(oauth.refreshToken)}&client_id=${OAUTH_CLIENT_ID}`;
    const req = https.request({
      hostname: 'console.anthropic.com',
      path: '/v1/oauth/token',
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'Content-Length': Buffer.byteLength(postData),
      },
    }, (res) => {
      let data = '';
      res.on('data', chunk => data += chunk);
      res.on('end', () => {
        try {
          if (res.statusCode < 200 || res.statusCode >= 300) {
            resolve(null);
            return;
          }
          const tokens = JSON.parse(data);
          if (!tokens.access_token || !tokens.refresh_token) { resolve(null); return; }

          // Validate expires_in: NaN would propagate as JSON null, causing perpetual refresh
          const rawTTL = Number(tokens.expires_in);
          const ttl = (isFinite(rawTTL) && rawTTL > 0)
            ? Math.min(rawTTL, 86400) // cap at 24h
            : 28800;                    // default 8h if invalid

          credFile.claudeAiOauth.accessToken = tokens.access_token;
          credFile.claudeAiOauth.refreshToken = tokens.refresh_token;
          credFile.claudeAiOauth.expiresAt = Date.now() + (ttl * 1000);

          const credPath = path.join(claudeDir, '.credentials.json');
          writeFileAtomic(credPath, JSON.stringify(credFile, null, 2));
          resolve(tokens.access_token);
        } catch { resolve(null); }
      });
    });
    req.on('error', () => resolve(null));
    req.setTimeout(5000, () => { req.destroy(); resolve(null); });
    req.write(postData);
    req.end();
  });
}

// Get a valid access token, refreshing if expired (with 60s buffer)
async function getValidToken(claudeDir) {
  const credFile = readCredentialsFile(claudeDir);
  if (!credFile) return null;
  const oauth = credFile.claudeAiOauth || {};
  if (!oauth.accessToken) return null;

  const expiresAt = oauth.expiresAt || 0;
  if (Date.now() < expiresAt - 60_000) return oauth.accessToken;
  return await refreshToken(claudeDir, credFile, oauth);
}

/**
 * Fetch usage data from Anthropic's OAuth usage API.
 * - 2xx: parse and return
 * - 401/403: return { error: 'auth' } (permanent, cache and stop retrying)
 * - 429/5xx: return null (transient, retry next cycle)
 */
function fetchUsageFromAPI(accessToken) {
  return new Promise((resolve) => {
    if (!accessToken) { resolve(null); return; }

    const req = https.request({
      hostname: 'api.anthropic.com',
      path: '/api/oauth/usage',
      method: 'GET',
      headers: {
        'Accept': 'application/json',
        'Content-Type': 'application/json',
        'User-Agent': 'howmuchleft',
        'Authorization': `Bearer ${accessToken}`,
        'anthropic-beta': 'oauth-2025-04-20',
      },
    }, (res) => {
      let data = '';
      res.on('data', chunk => data += chunk);
      res.on('end', () => {
        try {
          if (res.statusCode === 401 || res.statusCode === 403) {
            resolve({ error: 'auth' });
            return;
          }
          if (res.statusCode < 200 || res.statusCode >= 300) {
            resolve(null);
            return;
          }
          const parsed = JSON.parse(data);
          if (parsed.type === 'error') {
            resolve(null);
            return;
          }
          resolve(parsed);
        } catch { resolve(null); }
      });
    });
    req.on('error', () => resolve(null));
    req.setTimeout(5000, () => { req.destroy(); resolve(null); });
    req.end();
  });
}

/**
 * Get usage data with caching and stale-data fallback.
 *
 * Cache uses absolute timestamps (resetAt) to avoid drift.
 * When a quota reset time has passed, forces refresh regardless of TTL.
 * On API failure, falls back to last-known-good data (marked stale).
 */
async function getUsageData(claudeDir, forceRefresh = false) {
  const now = Date.now();
  const cacheFile = path.join(claudeDir, '.statusline-cache.json');

  let cache = null;
  try {
    if (fs.existsSync(cacheFile)) {
      cache = JSON.parse(fs.readFileSync(cacheFile, 'utf8'));
    }
  } catch {}

  if (!forceRefresh && cache && cache.timestamp) {
    const age = now - cache.timestamp;
    const ttl = cache.status === 'error' ? ERROR_CACHE_TTL_MS : CACHE_TTL_MS;

    // Force refresh if a quota reset has passed (cached percent is definitely wrong)
    const fiveHourResetPassed = cache.fiveHour?.resetAt != null && now >= cache.fiveHour.resetAt;
    const weeklyResetPassed = cache.weekly?.resetAt != null && now >= cache.weekly.resetAt;
    const resetPassed = fiveHourResetPassed || weeklyResetPassed;

    if (age < ttl && !resetPassed) {
      return {
        stale: false,
        fiveHour: {
          percent: cache.fiveHour?.percent ?? null,
          resetIn: cache.fiveHour?.resetAt != null ? Math.max(0, cache.fiveHour.resetAt - now) : null,
        },
        weekly: {
          percent: cache.weekly?.percent ?? null,
          resetIn: cache.weekly?.resetAt != null ? Math.max(0, cache.weekly.resetAt - now) : null,
        },
      };
    }
  }

  const accessToken = await getValidToken(claudeDir);
  const apiData = await fetchUsageFromAPI(accessToken);

  // Auth error: cache failure for ERROR_CACHE_TTL_MS, fall back to last-known-good
  if (apiData && apiData.error === 'auth') {
    try {
      writeFileAtomic(cacheFile, JSON.stringify({
        timestamp: now,
        status: 'error',
        fiveHour: cache?.fiveHour || { percent: null, resetAt: null },
        weekly: cache?.weekly || { percent: null, resetAt: null },
      }));
    } catch {}

    if (cache?.fiveHour?.percent != null || cache?.weekly?.percent != null) {
      return {
        stale: true,
        fiveHour: {
          percent: cache.fiveHour?.percent ?? null,
          resetIn: cache.fiveHour?.resetAt != null ? Math.max(0, cache.fiveHour.resetAt - now) : null,
        },
        weekly: {
          percent: cache.weekly?.percent ?? null,
          resetIn: cache.weekly?.resetAt != null ? Math.max(0, cache.weekly.resetAt - now) : null,
        },
      };
    }
    return { stale: false, fiveHour: { percent: null, resetIn: null }, weekly: { percent: null, resetIn: null } };
  }

  // Transient failure: cache and fall back to stale data
  if (!apiData) {
    try {
      writeFileAtomic(cacheFile, JSON.stringify({
        timestamp: now,
        status: 'error',
        fiveHour: cache?.fiveHour || { percent: null, resetAt: null },
        weekly: cache?.weekly || { percent: null, resetAt: null },
      }));
    } catch {}

    if (cache?.fiveHour?.percent != null || cache?.weekly?.percent != null) {
      return {
        stale: true,
        fiveHour: {
          percent: cache.fiveHour?.percent ?? null,
          resetIn: cache.fiveHour?.resetAt != null ? Math.max(0, cache.fiveHour.resetAt - now) : null,
        },
        weekly: {
          percent: cache.weekly?.percent ?? null,
          resetIn: cache.weekly?.resetAt != null ? Math.max(0, cache.weekly.resetAt - now) : null,
        },
      };
    }
    return { stale: false, fiveHour: { percent: null, resetIn: null }, weekly: { percent: null, resetIn: null } };
  }

  // Success: parse and cache with absolute timestamps
  let usage = {
    stale: false,
    fiveHour: { percent: null, resetIn: null },
    weekly: { percent: null, resetIn: null },
  };

  let cacheEntry = {
    timestamp: now,
    status: 'ok',
    fiveHour: { percent: null, resetAt: null },
    weekly: { percent: null, resetAt: null },
  };

  if (apiData.five_hour) {
    usage.fiveHour.percent = apiData.five_hour.utilization ?? 0;
    cacheEntry.fiveHour.percent = usage.fiveHour.percent;
    if (apiData.five_hour.resets_at) {
      const resetAt = new Date(apiData.five_hour.resets_at).getTime();
      cacheEntry.fiveHour.resetAt = resetAt;
      usage.fiveHour.resetIn = Math.max(0, resetAt - now);
    }
  }

  if (apiData.seven_day) {
    usage.weekly.percent = apiData.seven_day.utilization ?? 0;
    cacheEntry.weekly.percent = usage.weekly.percent;
    if (apiData.seven_day.resets_at) {
      const resetAt = new Date(apiData.seven_day.resets_at).getTime();
      cacheEntry.weekly.resetAt = resetAt;
      usage.weekly.resetIn = Math.max(0, resetAt - now);
    }
  }

  if (usage.fiveHour.percent != null || usage.weekly.percent != null) {
    try {
      writeFileAtomic(cacheFile, JSON.stringify(cacheEntry));
    } catch {}
  }

  return usage;
}

/**
 * Read JSON from stdin (piped by Claude Code).
 * No timeout needed -- Claude Code kills hung scripts when a new render triggers.
 */
async function readStdin() {
  return new Promise((resolve) => {
    let data = '';
    process.stdin.setEncoding('utf8');
    process.stdin.on('data', (chunk) => { data += chunk; });
    process.stdin.on('end', () => {
      try { resolve(JSON.parse(data)); }
      catch { resolve({}); }
    });
  });
}

// Map percent (0-100) to a 256-color index from the configured gradient array
function getGradientIndex(percent) {
  const { gradient } = getBarConfig();
  const clamped = Math.max(0, Math.min(100, percent));
  const idx = Math.round(clamped / 100 * (gradient.length - 1));
  return gradient[idx];
}

// Foreground color from gradient
function getProgressColor(percent) {
  return `\x1b[38;5;${getGradientIndex(percent)}m`;
}

// Background color from gradient
function getProgressBgColor(percent) {
  return `\x1b[48;5;${getGradientIndex(percent)}m`;
}

/**
 * Render an hblock progress bar with sub-cell precision.
 * Filled cells: bg-colored spaces. Fractional cell: fg-colored left block char.
 * Empty cells: configurable gray background.
 */
function progressBar(percent, width) {
  const config = getBarConfig();
  if (width == null) width = config.width;
  const emptyBg = `\x1b[48;5;${config.emptyBg}m`;

  if (percent === null || percent === undefined) {
    return `${emptyBg}${' '.repeat(width)}${colors.reset}`;
  }
  const clamped = Math.max(0, Math.min(100, percent));
  const color = getProgressColor(clamped);
  const bgColor = getProgressBgColor(clamped);
  const fillFrac = clamped / 100;
  let out = '';
  for (let i = 0; i < width; i++) {
    const cellStart = i / width;
    const cellEnd = (i + 1) / width;
    if (cellEnd <= fillFrac) {
      out += bgColor + ' ';
    } else if (cellStart >= fillFrac) {
      out += emptyBg + ' ';
    } else {
      const cellFill = (fillFrac - cellStart) / (1 / width);
      const idx = Math.max(0, Math.min(FRACTIONAL_CHARS.length - 1, Math.floor(cellFill * FRACTIONAL_CHARS.length)));
      out += emptyBg + color + FRACTIONAL_CHARS[idx];
    }
  }
  return out + colors.reset;
}

/**
 * Format a usage percentage.
 * stale=true: prefix with ~ (approximate). null: show "?%" (no data).
 */
function formatPercent(percent, stale = false) {
  if (percent === null || percent === undefined) {
    return `${colors.gray}?%${colors.reset}`;
  }
  if (stale) {
    return `${colors.dim}~${Math.round(percent)}%${colors.reset}`;
  }
  return `${colors.cyan}${Math.round(percent)}%${colors.reset}`;
}

function formatTimeRemaining(ms) {
  if (ms === null || ms === undefined) return '?';
  if (ms <= 0) return 'now';
  const days = Math.floor(ms / (1000 * 60 * 60 * 24));
  const hours = Math.floor((ms % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60));
  const minutes = Math.floor((ms % (1000 * 60 * 60)) / (1000 * 60));
  if (days > 0) return `${days}d${hours}h`;
  if (hours > 0) return `${hours}h${minutes}m`;
  return `${minutes}m`;
}

/**
 * Get git branch and change count with a single async call.
 * Uses --porcelain=v2 --branch -uno --no-renames --no-optional-locks
 * for machine-readable output without blocking concurrent git ops.
 */
async function getGitInfo(cwd) {
  try {
    const { stdout } = await execFileAsync('git', [
      '--no-optional-locks', 'status', '--porcelain=v2', '--branch', '-uno', '--no-renames',
    ], { cwd, timeout: 3000 });

    let branch = null;
    let changes = 0;

    for (const line of stdout.split('\n')) {
      if (line.startsWith('# branch.head ')) {
        branch = line.slice('# branch.head '.length);
        if (branch === '(detached)') branch = 'detached';
      } else if (/^[12u] /.test(line)) {
        changes++;
      }
    }

    return { branch: branch || 'detached', changes, hasGit: true };
  } catch {
    return { branch: null, changes: 0, hasGit: false };
  }
}

function shortenPath(p, maxLen = 25) {
  if (!p || p.length <= maxLen) return p || '~';
  const home = os.homedir();
  if (p.startsWith(home)) p = '~' + p.slice(home.length);
  if (p.length <= maxLen) return p;
  const parts = p.split('/');
  if (parts.length <= 2) return '...' + p.slice(-maxLen + 3);
  return parts[0] + '/.../' + parts.slice(-2).join('/');
}

async function main() {
  const claudeDir = getClaudeDir();
  const credFile = readCredentialsFile(claudeDir);
  const oauth = credFile?.claudeAiOauth || {};
  const { isOAuth, subscriptionName } = getAuthInfo(oauth);
  const stdinData = await readStdin();

  const model = stdinData.model?.display_name || stdinData.model?.id || '?';
  const contextWindow = stdinData.context_window || {};
  const contextUsedPercent = contextWindow.used_percentage || 0;
  const cwd = stdinData.cwd || stdinData.workspace?.current_dir || process.cwd();
  const cost = stdinData.cost || {};

  // Fetch usage and git info in parallel (independent operations)
  const isSessionStart = contextUsedPercent === 0;
  const [usage, gitInfo] = await Promise.all([
    isOAuth
      ? getUsageData(claudeDir, isSessionStart)
      : { stale: false, fiveHour: { percent: null, resetIn: null }, weekly: { percent: null, resetIn: null } },
    getGitInfo(cwd),
  ]);

  const lines = [];

  // Line 1: context bar, subscription tier, model
  const contextBar = progressBar(contextUsedPercent);
  lines.push(
    `${contextBar} ${colors.cyan}${Math.round(contextUsedPercent)}%${colors.reset} ` +
    `${colors.magenta}${subscriptionName}${colors.reset} ` +
    `${colors.white}${model}${colors.reset}`
  );

  // Line 2: 5hr usage bar + reset time, git branch/changes, lines added/removed
  const fiveHourBar = progressBar(usage.fiveHour.percent);
  const fiveHourPercent = formatPercent(usage.fiveHour.percent, usage.stale);
  const fiveHourReset = formatTimeRemaining(usage.fiveHour.resetIn);
  const gitStr = gitInfo.hasGit
    ? `${colors.cyan}${gitInfo.branch}${colors.reset}` +
      (gitInfo.changes > 0 ? ` ${colors.yellow}+${gitInfo.changes}${colors.reset}` : '')
    : `${colors.gray}no .git${colors.reset}`;
  const added = cost.total_lines_added;
  const removed = cost.total_lines_removed;
  const linesStr = (added || removed)
    ? `${colors.green}+${added ?? 0}${colors.reset}/${colors.red}-${removed ?? 0}${colors.reset}`
    : '';
  lines.push(
    `${fiveHourBar} ${fiveHourPercent} ` +
    `${colors.dim}${fiveHourReset}${colors.reset} ` +
    `${gitStr}` +
    (linesStr ? ` ${linesStr}` : '')
  );

  // Line 3: weekly usage bar + reset time, current directory
  const weeklyBar = progressBar(usage.weekly.percent);
  const weeklyPercent = formatPercent(usage.weekly.percent, usage.stale);
  const weeklyReset = formatTimeRemaining(usage.weekly.resetIn);
  const shortCwd = shortenPath(cwd, 25);
  lines.push(
    `${weeklyBar} ${weeklyPercent} ` +
    `${colors.dim}${weeklyReset}${colors.reset} ` +
    `${colors.white}${shortCwd}${colors.reset}`
  );

  console.log(lines.join('\n'));
}

module.exports = { main, CONFIG_PATH };

// Run directly when invoked as a script (not when required by cli.js)
if (require.main === module) {
  main().catch(err => {
    console.error('Statusline error:', err.message);
    process.exit(1);
  });
}
