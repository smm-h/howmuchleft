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
const crypto = require('crypto');
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
  // Allow override for GIF recording without changing system settings
  if (process.env.HOWMUCHLEFT_DARK !== undefined) {
    _isDark = process.env.HOWMUCHLEFT_DARK === '1';
    return _isDark;
  }
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

// -- Progress bar: fractional block characters for sub-cell precision --
// Horizontal (default): left fractional blocks, fill left-to-right
const HORIZONTAL_CHARS = ['\u258F','\u258E','\u258D','\u258C','\u258B','\u258A','\u2589'];
// Vertical: lower fractional blocks, fill bottom-to-top
const VERTICAL_CHARS = ['\u2581','\u2582','\u2583','\u2584','\u2585','\u2586','\u2587'];

// -- JSONC support: strip // and /* */ comments for config file parsing --
function stripJsonComments(text) {
  let result = '';
  let inString = false;
  let escape = false;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (escape) { result += ch; escape = false; continue; }
    if (inString) {
      if (ch === '\\') escape = true;
      else if (ch === '"') inString = false;
      result += ch;
      continue;
    }
    if (ch === '"') { inString = true; result += ch; continue; }
    if (ch === '/' && text[i + 1] === '/') {
      while (i < text.length && text[i] !== '\n') i++;
      result += '\n';
      continue;
    }
    if (ch === '/' && text[i + 1] === '*') {
      i += 2;
      while (i < text.length && !(text[i] === '*' && text[i + 1] === '/')) i++;
      i++; // skip past closing /
      continue;
    }
    result += ch;
  }
  // Strip trailing commas before } or ] (common JSONC foot-gun)
  return result.replace(/,\s*([}\]])/g, '$1');
}

function parseJsonc(text) {
  return JSON.parse(stripJsonComments(text));
}

// -- Truecolor detection --
function isTruecolorSupported() {
  const ct = process.env.COLORTERM;
  return ct === 'truecolor' || ct === '24bit';
}

// -- Built-in gradient defaults for all 4 combos --
// Each entry: optional dark-mode/true-color conditions, gradient stops.
// RGB arrays for truecolor, integer indices for 256-color.
const BUILTIN_COLORS = [
  { 'dark-mode': true, 'true-color': true, bg: [48, 48, 48], gradient: [
    [0,215,0], [95,215,0], [175,215,0], [255,255,0], [255,215,0], [255,175,0], [255,135,0], [255,95,0], [255,55,0], [255,0,0],
  ]},
  { 'dark-mode': false, 'true-color': true, bg: [208, 208, 208], gradient: [
    [0,170,0], [75,170,0], [140,170,0], [200,200,0], [200,170,0], [200,135,0], [200,100,0], [200,65,0], [200,30,0], [190,0,0],
  ]},
  { 'dark-mode': true, 'true-color': false, bg: 236, gradient: [46, 82, 118, 154, 190, 226, 220, 214, 208, 202, 196] },
  { 'dark-mode': false, 'true-color': false, bg: 252, gradient: [40, 76, 112, 148, 184, 178, 172, 166, 160] },
];

// Config is loaded from ~/.config/howmuchleft.json (XDG-style, install-method-agnostic).
// Supports JSONC (comments and trailing commas).
// Cached per-process to avoid redundant fs reads.
const CONFIG_PATH = path.join(os.homedir(), '.config', 'howmuchleft.json');

// Check if a gradient stop is an RGB array [R,G,B]
function isRgbStop(stop) {
  return Array.isArray(stop) && stop.length === 3 &&
    stop.every(v => Number.isInteger(v) && v >= 0 && v <= 255);
}

// Validate a gradient: non-empty array of either all ints or all RGB arrays
function isValidGradient(arr) {
  if (!Array.isArray(arr) || arr.length === 0) return false;
  if (isRgbStop(arr[0])) return arr.every(isRgbStop);
  return arr.every(v => Number.isInteger(v) && v >= 0 && v <= 255);
}

// Find first matching color entry from a colors array (returns full entry)
function findColorMatch(colorsArr, isDark, isTruecolor) {
  for (const entry of colorsArr) {
    if (entry['dark-mode'] !== undefined && entry['dark-mode'] !== isDark) continue;
    if (entry['true-color'] !== undefined && entry['true-color'] !== isTruecolor) continue;
    if (isValidGradient(entry.gradient)) return entry;
  }
  return null;
}

// Format a bg value (integer 0-255 or [R,G,B]) as an ANSI background escape
function formatBgEscape(bg, truecolor) {
  if (isRgbStop(bg)) {
    if (truecolor) return `\x1b[48;2;${bg[0]};${bg[1]};${bg[2]}m`;
    return `\x1b[48;5;${rgbTo256(bg)}m`;
  }
  if (Number.isInteger(bg) && bg >= 0 && bg <= 255) return `\x1b[48;5;${bg}m`;
  return null;
}

// Convert RGB [R,G,B] to nearest 256-color cube index
function rgbTo256(rgb) {
  return 16 + 36 * Math.round(rgb[0] / 255 * 5) + 6 * Math.round(rgb[1] / 255 * 5) + Math.round(rgb[2] / 255 * 5);
}

// Linearly interpolate between RGB gradient stops for smooth truecolor
function interpolateRgb(stops, t) {
  const pos = t * (stops.length - 1);
  const lo = Math.floor(pos);
  const hi = Math.min(lo + 1, stops.length - 1);
  const frac = pos - lo;
  return [
    Math.round(stops[lo][0] + (stops[hi][0] - stops[lo][0]) * frac),
    Math.round(stops[lo][1] + (stops[hi][1] - stops[lo][1]) * frac),
    Math.round(stops[lo][2] + (stops[hi][2] - stops[lo][2]) * frac),
  ];
}

// Terminals known to have broken fractional block character rendering.
// Checked by auto-detection when partialBlocks is "auto" (default).
const PARTIAL_BLOCKS_BLOCKLIST = new Set([
  'Apple_Terminal',   // Terminal.app: block chars often render at wrong height
  'linux',           // Linux console (TERM=linux): limited Unicode support
]);

function shouldUsePartialBlocks() {
  if (PARTIAL_BLOCKS_BLOCKLIST.has(process.env.TERM_PROGRAM)) return false;
  if (PARTIAL_BLOCKS_BLOCKLIST.has(process.env.TERM)) return false;
  return true;
}

let _barConfig = null;
function getBarConfig() {
  if (_barConfig) return _barConfig;

  const dark = isDarkMode();
  let colorMode = 'auto';
  let width = 12;
  let userColors = [];
  let partialBlocks = 'auto';
  let orientation = 'vertical';

  try {
    const raw = fs.readFileSync(CONFIG_PATH, 'utf8');
    const config = parseJsonc(raw);
    const w = Number(config.progressLength);
    if (Number.isInteger(w) && w >= 3 && w <= 40) width = w;
    if (config.colorMode === 'truecolor' || config.colorMode === '256') colorMode = config.colorMode;
    if (Array.isArray(config.colors)) userColors = config.colors;
    if (config.partialBlocks === true || config.partialBlocks === false) partialBlocks = config.partialBlocks;
    if (config.progressBarOrientation === 'horizontal' || config.progressBarOrientation === 'vertical') {
      orientation = config.progressBarOrientation;
    }
  } catch {}

  const truecolor = colorMode === 'truecolor' || (colorMode === 'auto' && isTruecolorSupported());
  const usePartialBlocks = partialBlocks === 'auto' ? shouldUsePartialBlocks() : partialBlocks;
  const userMatch = findColorMatch(userColors, dark, truecolor);
  const builtinMatch = findColorMatch(BUILTIN_COLORS, dark, truecolor);
  const gradient = userMatch?.gradient || builtinMatch.gradient;
  const isRgb = isRgbStop(gradient[0]);

  // bg: user entry wins, then builtin, then hardcoded fallback
  const emptyBg = formatBgEscape(userMatch?.bg, truecolor) ||
                  formatBgEscape(builtinMatch?.bg, truecolor) ||
                  `\x1b[48;5;${dark ? 236 : 252}m`;

  _barConfig = { width, emptyBg, gradient, truecolor, isRgb, partialBlocks: usePartialBlocks, orientation };
  return _barConfig;
}

const OAUTH_CLIENT_ID = '9d1c250a-e61b-44d9-88ed-5944d1962f5e';

// Cache TTL: 60s for success, 5min for failures (avoids hammering a dead API)
const CACHE_TTL_MS = 60_000;
const ERROR_CACHE_TTL_MS = 300_000;
const MAX_ERROR_CACHE_TTL_MS = 3_600_000; // 1 hour cap for error backoff

function resolvePath(p) {
  if (p.startsWith('~')) return path.join(os.homedir(), p.slice(1));
  return path.resolve(p);
}

function getClaudeDir() {
  const arg = process.argv[2];
  if (!arg || arg.startsWith('-')) return process.env.CLAUDE_CONFIG_DIR || path.join(os.homedir(), '.claude');
  return resolvePath(arg);
}

// Extract active profile name from the Claude config directory basename.
// Returns null for the default .claude dir (no profile).
function getProfileName() {
  const dir = getClaudeDir();
  const base = path.basename(dir);
  if (base === '.claude') return null;
  if (base.startsWith('.claude-')) return base.slice('.claude-'.length) || null;
  return base;
}

// Map a string to a hue value 0-359 using the djb2 hash algorithm.
// Used to give each Claude config directory a stable, unique color.
function hashToHue(str) {
  let hash = 5381;
  for (let i = 0; i < str.length; i++) {
    hash = ((hash << 5) + hash + str.charCodeAt(i)) | 0; // hash * 33 + c
  }
  return ((hash % 360) + 360) % 360; // ensure positive
}

// Convert a hue (0-359) to an ANSI foreground escape sequence.
// Uses HSL->RGB with fixed saturation (0.7) and lightness that adapts to
// the terminal background (bright on dark, dark on light).
// Returns truecolor (\x1b[38;2;...m) or 256-color (\x1b[38;5;...m) based
// on terminal capability.
function hueToAnsi(hue, isDark) {
  const s = 0.7;
  const l = isDark ? 0.75 : 0.35;

  // HSL to RGB conversion (standard algorithm)
  const c = (1 - Math.abs(2 * l - 1)) * s;
  const x = c * (1 - Math.abs((hue / 60) % 2 - 1));
  const m = l - c / 2;
  let r1, g1, b1;
  if (hue < 60)       { r1 = c; g1 = x; b1 = 0; }
  else if (hue < 120) { r1 = x; g1 = c; b1 = 0; }
  else if (hue < 180) { r1 = 0; g1 = c; b1 = x; }
  else if (hue < 240) { r1 = 0; g1 = x; b1 = c; }
  else if (hue < 300) { r1 = x; g1 = 0; b1 = c; }
  else                { r1 = c; g1 = 0; b1 = x; }
  const r = Math.round((r1 + m) * 255);
  const g = Math.round((g1 + m) * 255);
  const b = Math.round((b1 + m) * 255);

  if (isTruecolorSupported()) {
    return `\x1b[38;2;${r};${g};${b}m`;
  }
  // Fallback: map RGB to nearest 256-color 6x6x6 cube index (16-231)
  const idx = 16 + 36 * Math.round(r / 255 * 5) + 6 * Math.round(g / 255 * 5) + Math.round(b / 255 * 5);
  return `\x1b[38;5;${idx}m`;
}

/**
 * Get session elapsed time by reading Claude Code's PID session file.
 * Claude writes {pid, sessionId, cwd, startedAt} to <claudeDir>/sessions/<pid>.json.
 * We walk up the process tree via PPID to find the Claude parent process.
 */
function getSessionElapsed(claudeDir) {
  try {
    // Walk up process tree to find a PID that has a session file
    let pid = process.ppid;
    for (let i = 0; i < 5 && pid > 1; i++) {
      const sessionFile = path.join(claudeDir, 'sessions', `${pid}.json`);
      try {
        const data = JSON.parse(fs.readFileSync(sessionFile, 'utf8'));
        if (data.startedAt) return Date.now() - data.startedAt;
      } catch {}
      // Read PPID from /proc/<pid>/stat (field 4)
      try {
        const stat = fs.readFileSync(`/proc/${pid}/stat`, 'utf8');
        pid = Number(stat.split(' ')[3]);
      } catch { break; }
    }
  } catch {}
  return null;
}

/**
 * Read credentials from the macOS Keychain.
 * Claude Code stores OAuth tokens in the Keychain on macOS instead of
 * (or in addition to) the .credentials.json file. The service name includes
 * a hash of the config dir path to support multiple accounts.
 */
function readKeychainCredentials(claudeDir) {
  if (process.platform !== 'darwin') return null;
  const hash = crypto.createHash('sha256').update(claudeDir).digest('hex').slice(0, 8);
  // Try current hashed service name, then legacy unhashed name
  for (const svc of [`Claude Code-credentials-${hash}`, 'Claude Code-credentials']) {
    try {
      const raw = execFileSync('security', [
        'find-generic-password', '-s', svc, '-w',
      ], { encoding: 'utf8', timeout: 5000, stdio: ['pipe', 'pipe', 'pipe'] });
      const parsed = JSON.parse(raw.trim());
      if (parsed?.claudeAiOauth?.accessToken) return parsed;
    } catch {}
  }
  return null;
}

// Cached per-process: credentials won't change during a single render cycle.
let _credentialsRead = false;
let _credentialsCache = null;

function readCredentialsFile(claudeDir) {
  if (_credentialsRead) return _credentialsCache;
  _credentialsRead = true;

  // Try file first (fast, no system prompt)
  let fileData = null;
  try {
    fileData = JSON.parse(fs.readFileSync(path.join(claudeDir, '.credentials.json'), 'utf8'));
  } catch {}

  const fileOAuth = fileData?.claudeAiOauth;

  // File token is usable if: (a) not expired, or (b) has a refresh token to renew it
  const fileTokenUsable = fileOAuth?.accessToken && (
    (fileOAuth.expiresAt && Date.now() < fileOAuth.expiresAt - 60_000) ||
    fileOAuth.refreshToken
  );

  if (fileTokenUsable) {
    _credentialsCache = fileData;
    return fileData;
  }

  // File token missing or expired-without-refresh-token: try Keychain
  const keychainData = readKeychainCredentials(claudeDir);
  if (keychainData) {
    // Merge: preserve file data (mcpOAuth etc.) with keychain OAuth
    const merged = { ...(fileData || {}), claudeAiOauth: keychainData.claudeAiOauth };
    _credentialsCache = merged;
    return merged;
  }

  // Fall back to file data even if token is stale (refresh will be attempted)
  _credentialsCache = fileData;
  return fileData;
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

  const refreshed = await refreshToken(claudeDir, credFile, oauth);
  if (refreshed) return refreshed;

  // Refresh failed: try Keychain directly (Claude Code may have updated it)
  const keychainData = readKeychainCredentials(claudeDir);
  const keychainOAuth = keychainData?.claudeAiOauth;
  if (keychainOAuth?.accessToken && keychainOAuth.expiresAt && Date.now() < keychainOAuth.expiresAt - 60_000) {
    return keychainOAuth.accessToken;
  }

  return null;
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
      hostname: 'platform.claude.com',
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
    const ttl = cache.status === 'error'
      ? Math.min(ERROR_CACHE_TTL_MS * Math.pow(2, (cache.consecutiveErrors || 1) - 1), MAX_ERROR_CACHE_TTL_MS)
      : CACHE_TTL_MS;

    // Force refresh if a quota reset has passed (cached percent is definitely wrong)
    const fiveHourResetPassed = cache.fiveHour?.resetAt != null && now >= cache.fiveHour.resetAt;
    const weeklyResetPassed = cache.weekly?.resetAt != null && now >= cache.weekly.resetAt;
    const resetPassed = fiveHourResetPassed || weeklyResetPassed;

    const shouldBypassTtl = resetPassed && cache.status !== 'error';
    if (age < ttl && !shouldBypassTtl) {
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
        consecutiveErrors: (cache?.consecutiveErrors || 0) + 1,
        fiveHour: { percent: cache?.fiveHour?.percent ?? null, resetAt: null },
        weekly: { percent: cache?.weekly?.percent ?? null, resetAt: null },
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
        consecutiveErrors: (cache?.consecutiveErrors || 0) + 1,
        fiveHour: { percent: cache?.fiveHour?.percent ?? null, resetAt: null },
        weekly: { percent: cache?.weekly?.percent ?? null, resetAt: null },
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
    consecutiveErrors: 0,
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
      try {
        resolve(data.trim() ? JSON.parse(data) : {});
      } catch (err) {
        console.warn(`howmuchleft: failed to parse stdin JSON: ${err.message}`);
        resolve({});
      }
    });
  });
}

/**
 * Get foreground and background ANSI escape codes for a given percent.
 * Uses smooth RGB interpolation for truecolor, nearest-stop for 256-color.
 */
function getGradientStop(percent) {
  const { gradient, truecolor, isRgb } = getBarConfig();
  const t = Math.max(0, Math.min(1, percent / 100));

  if (isRgb) {
    const rgb = interpolateRgb(gradient, t);
    if (truecolor) {
      return {
        fg: `\x1b[38;2;${rgb[0]};${rgb[1]};${rgb[2]}m`,
        bg: `\x1b[48;2;${rgb[0]};${rgb[1]};${rgb[2]}m`,
      };
    }
    // RGB gradient on 256-color terminal: convert to nearest index
    const idx = rgbTo256(rgb);
    return { fg: `\x1b[38;5;${idx}m`, bg: `\x1b[48;5;${idx}m` };
  }

  // 256-color gradient: snap to nearest stop
  const idx = Math.round(t * (gradient.length - 1));
  const colorIdx = gradient[idx];
  return { fg: `\x1b[38;5;${colorIdx}m`, bg: `\x1b[48;5;${colorIdx}m` };
}

/**
 * Render an hblock progress bar with sub-cell precision.
 * Filled cells: bg-colored spaces. Fractional cell: fg-colored left block char.
 * Empty cells: configurable gray background.
 */
function progressBar(percent, width) {
  const config = getBarConfig();
  if (width == null) width = config.width;
  const emptyBg = config.emptyBg;

  if (percent === null || percent === undefined) {
    return `${emptyBg}${' '.repeat(width)}${colors.reset}`;
  }
  const clamped = Math.max(0, Math.min(100, percent));
  const { fg, bg } = getGradientStop(clamped);
  const fillFrac = clamped / 100;
  let out = '';
  for (let i = 0; i < width; i++) {
    const cellStart = i / width;
    const cellEnd = (i + 1) / width;
    if (cellEnd <= fillFrac) {
      out += bg + ' ';
    } else if (cellStart >= fillFrac) {
      out += emptyBg + ' ';
    } else if (config.partialBlocks) {
      const cellFill = (fillFrac - cellStart) / (1 / width);
      const chars = HORIZONTAL_CHARS;
      const idx = Math.max(0, Math.min(chars.length - 1, Math.floor(cellFill * chars.length)));
      out += emptyBg + fg + chars[idx];
    } else {
      // No partial blocks: round to nearest full cell
      const cellMid = (cellStart + cellEnd) / 2;
      out += (fillFrac >= cellMid) ? bg + ' ' : emptyBg + ' ';
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

/**
 * Render one cell of a vertical bar.
 * Each bar is 3 rows tall (top=0, bottom=2), 8 states per row = 24 levels.
 * Fills bottom-to-top: row 2 fills first, then row 1, then row 0.
 */
function verticalBarCell(percent, rowIdx) {
  const config = getBarConfig();
  const emptyBg = config.emptyBg;
  const clamped = (percent == null) ? 0 : Math.max(0, Math.min(100, percent));
  const level = Math.round(clamped / 100 * 24); // 0–24
  const { fg } = getGradientStop(clamped);

  // Each row covers 8 levels; bottom row (2) = levels 0–8, mid (1) = 8–16, top (0) = 16–24
  const rowBase = (2 - rowIdx) * 8;
  const rowLevel = level - rowBase;

  if (rowLevel >= 8) {
    // Full cell: use fg full block on empty bg (matches vertical block aesthetic)
    return `${emptyBg}${fg}\u2588`;
  } else if (rowLevel > 0 && config.partialBlocks) {
    return `${emptyBg}${fg}${VERTICAL_CHARS[rowLevel - 1]}`;
  } else {
    return `${emptyBg} `;
  }
}

/**
 * Compose the 3-line statusline output from normalized data.
 * Shared by main() and demo mode.
 *
 * Horizontal: each line has its own progress bar.
 * Vertical: 3 bars are columns spanning all 3 lines (context, 5hr, weekly),
 *   filling bottom-to-top with 8 states per cell = 24 levels each.
 */
function renderLines({ context, model, tier, elapsed, profile, profileColor, fiveHour, weekly, stale, git, lines: lineChanges, cwd }) {
  const config = getBarConfig();
  const lines = [];

  // Pre-compute labels shared by both modes
  const elapsedStr = elapsed != null ? `${colors.dim}${formatTimeRemaining(elapsed)}${colors.reset} ` : '';
  const profileStr = profile ? `${profileColor || colors.dim}${profile}${colors.reset} ` : '';
  const fiveHourPercent = formatPercent(fiveHour.percent, stale);
  const fiveHourReset = formatTimeRemaining(fiveHour.resetIn);
  const gitStr = git.hasGit
    ? `${colors.cyan}${git.branch}${colors.reset}` +
      (git.changes > 0 ? ` ${colors.yellow}+${git.changes}${colors.reset}` : '')
    : `${colors.gray}no .git${colors.reset}`;
  const added = lineChanges.added;
  const removed = lineChanges.removed;
  const linesStr = (added || removed)
    ? `${colors.green}+${added ?? 0}${colors.reset}/${colors.red}-${removed ?? 0}${colors.reset}`
    : '';
  const weeklyPercent = formatPercent(weekly.percent, stale);
  const weeklyReset = formatTimeRemaining(weekly.resetIn);

  if (config.orientation === 'vertical') {
    // 3 vertical bar columns (context, 5hr, weekly) spanning all 3 rows
    const percents = [context, fiveHour.percent, weekly.percent];
    for (let row = 0; row < 3; row++) {
      let barStr = '';
      for (let i = 0; i < percents.length; i++) {
        if (i > 0) barStr += `${colors.reset} `;
        barStr += verticalBarCell(percents[i], row);
      }
      barStr += colors.reset;

      if (row === 0) {
        lines.push(
          `${barStr} ${colors.cyan}${Math.round(context)}%${colors.reset} ` +
          `${elapsedStr}` +
          `${profileStr}` +
          `${colors.magenta}${tier}${colors.reset} ` +
          `${colors.white}${model}${colors.reset}`
        );
      } else if (row === 1) {
        lines.push(
          `${barStr} ${fiveHourPercent} ` +
          `${colors.dim}${fiveHourReset}${colors.reset} ` +
          `${gitStr}` +
          (linesStr ? ` ${linesStr}` : '')
        );
      } else {
        lines.push(
          `${barStr} ${weeklyPercent} ` +
          `${colors.dim}${weeklyReset}${colors.reset} ` +
          `${colors.white}${cwd}${colors.reset}`
        );
      }
    }
  } else {
    // Horizontal: each line has its own progress bar
    const contextBar = progressBar(context);
    lines.push(
      `${contextBar} ${colors.cyan}${Math.round(context)}%${colors.reset} ` +
      `${elapsedStr}` +
      `${profileStr}` +
      `${colors.magenta}${tier}${colors.reset} ` +
      `${colors.white}${model}${colors.reset}`
    );

    const fiveHourBar = progressBar(fiveHour.percent);
    lines.push(
      `${fiveHourBar} ${fiveHourPercent} ` +
      `${colors.dim}${fiveHourReset}${colors.reset} ` +
      `${gitStr}` +
      (linesStr ? ` ${linesStr}` : '')
    );

    const weeklyBar = progressBar(weekly.percent);
    lines.push(
      `${weeklyBar} ${weeklyPercent} ` +
      `${colors.dim}${weeklyReset}${colors.reset} ` +
      `${colors.white}${cwd}${colors.reset}`
    );
  }

  return lines.join('\n');
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

  // Detect if Claude Code has written fresh credentials since last cache update
  let credentialsChanged = false;
  try {
    const credPath = path.join(claudeDir, '.credentials.json');
    const cachePath = path.join(claudeDir, '.statusline-cache.json');
    const credMtime = fs.statSync(credPath).mtimeMs;
    const cacheStat = fs.statSync(cachePath);
    credentialsChanged = credMtime > cacheStat.mtimeMs;
  } catch {}

  const [usage, gitInfo] = await Promise.all([
    isOAuth
      ? getUsageData(claudeDir, isSessionStart || credentialsChanged)
      : { stale: false, fiveHour: { percent: null, resetIn: null }, weekly: { percent: null, resetIn: null } },
    getGitInfo(cwd),
  ]);

  const elapsed = getSessionElapsed(claudeDir);
  const profile = getProfileName();
  const profileColor = profile ? hueToAnsi(hashToHue(claudeDir), isDarkMode()) : '';

  console.log(renderLines({
    context: contextUsedPercent,
    model,
    tier: subscriptionName,
    elapsed,
    profile,
    profileColor,
    fiveHour: usage.fiveHour,
    weekly: usage.weekly,
    stale: usage.stale,
    git: gitInfo,
    lines: { added: cost.total_lines_added, removed: cost.total_lines_removed },
    cwd: shortenPath(cwd, 25),
  }));
}

/**
 * Render a gradient swatch so users can preview their color config.
 * Shows bars at 0%, 25%, 50%, 75%, 100% plus a continuous gradient strip.
 */
function testColors() {
  const config = getBarConfig();
  const dark = isDarkMode();
  const tc = config.truecolor;

  const orient = config.orientation;
  console.log(`Mode: ${dark ? 'dark' : 'light'}, Color: ${tc ? 'truecolor' : '256-color'}, Width: ${config.width}, Partial blocks: ${config.partialBlocks ? 'on' : 'off'}, Orientation: ${orient}`);
  console.log();

  // Sample bars at key percentages, with vertical columns on the first 3 rows
  const pcts = [5, 20, 35, 50, 65, 80, 95];
  for (let i = 0; i < pcts.length; i++) {
    const bar = progressBar(pcts[i]);
    let line = `${bar} ${pcts[i]}%`;
    // Append vertical bar columns on the first 3 rows
    if (i < 3) {
      line += '  ';
      for (const vp of pcts) {
        line += verticalBarCell(vp, i);
      }
      line += colors.reset;
    }
    console.log(line);
  }

  console.log();

  // Continuous gradient strip (40 cells, each colored by position)
  const stripWidth = 40;
  let strip = '';
  for (let i = 0; i < stripWidth; i++) {
    const pct = (i / (stripWidth - 1)) * 100;
    const { bg } = getGradientStop(pct);
    strip += bg + ' ';
  }
  console.log(strip + colors.reset + '  gradient');
}

module.exports = { main, CONFIG_PATH, progressBar, formatPercent, formatTimeRemaining, colors, renderLines, testColors };

// Run directly when invoked as a script (not when required by cli.js)
if (require.main === module) {
  main().catch(err => {
    console.error('Statusline error:', err.message);
    process.exit(1);
  });
}
