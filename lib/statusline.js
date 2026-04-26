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

// -- Time window durations for elapsed-time bar computation --
const FIVE_HOUR_MS = 5 * 60 * 60 * 1000;
const SEVEN_DAY_MS = 7 * 24 * 60 * 60 * 1000;

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

// Time bar background: blended toward terminal default from bar bg (subtler than usage bars).
// Dark terminals default to black (0,0,0), light to white (255,255,255).
// blend: 0 = same as bar bg, 1 = fully terminal default. Default 0.25.
function computeTimeBarBg(rawBg, dark, truecolor, blend) {
  const termDefault = dark ? 0 : 255;
  if (blend == null) blend = 0.25;
  if (isRgbStop(rawBg)) {
    const mid = [
      Math.round(rawBg[0] + (termDefault - rawBg[0]) * blend),
      Math.round(rawBg[1] + (termDefault - rawBg[1]) * blend),
      Math.round(rawBg[2] + (termDefault - rawBg[2]) * blend),
    ];
    if (truecolor) return `\x1b[48;2;${mid[0]};${mid[1]};${mid[2]}m`;
    return `\x1b[48;5;${rgbTo256(mid)}m`;
  }
  // 256-color: grayscale indices 232-255 map to 8+10*n brightness.
  // Shift index toward terminal default end of the grayscale ramp.
  const base = Number.isInteger(rawBg) ? rawBg : (dark ? 236 : 252);
  const target = dark ? 232 : 255;
  const mid = Math.round(base + (target - base) * blend);
  return `\x1b[48;5;${mid}m`;
}

// Warm amber background for extra usage bar (visually distinct from standard gray)
function getExtraUsageBg() {
  const config = getBarConfig();
  const dark = isDarkMode();
  if (config.truecolor) {
    return dark ? '\x1b[48;2;80;50;0m' : '\x1b[48;2;255;220;160m';
  }
  return dark ? '\x1b[48;5;94m' : '\x1b[48;5;223m';
}

/**
 * Compute ANSI foreground escape for a time-bar cell based on urgency ratio (0-1).
 * Two-stage gradient: gray -> yellow (0-0.5) -> red (0.5-1.0).
 * Adapts to truecolor/256-color and dark/light mode.
 */
function getUrgencyColor(urgency) {
  const config = getBarConfig();
  const dark = isDarkMode();
  const u = Math.max(0, Math.min(1, urgency));

  if (config.truecolor) {
    // Truecolor: smooth linear interpolation across 3 stops
    const gray  = dark ? [120, 120, 120] : [160, 160, 160];
    const yellow = dark ? [255, 200, 0]  : [200, 160, 0];
    const red   = dark ? [255, 0, 0]     : [200, 0, 0];
    let rgb;
    if (u <= 0.5) {
      const t = u / 0.5;
      rgb = [
        Math.round(gray[0] + (yellow[0] - gray[0]) * t),
        Math.round(gray[1] + (yellow[1] - gray[1]) * t),
        Math.round(gray[2] + (yellow[2] - gray[2]) * t),
      ];
    } else {
      const t = (u - 0.5) / 0.5;
      rgb = [
        Math.round(yellow[0] + (red[0] - yellow[0]) * t),
        Math.round(yellow[1] + (red[1] - yellow[1]) * t),
        Math.round(yellow[2] + (red[2] - yellow[2]) * t),
      ];
    }
    return { fg: `\x1b[38;2;${rgb[0]};${rgb[1]};${rgb[2]}m` };
  }

  // 256-color: snap to nearest of 3 stops
  if (u < 0.25) {
    return { fg: `\x1b[38;5;${dark ? 245 : 249}m` };
  } else if (u < 0.75) {
    return { fg: `\x1b[38;5;${dark ? 226 : 178}m` };
  } else {
    return { fg: `\x1b[38;5;${dark ? 196 : 160}m` };
  }
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
  let cwdMaxLength = 50;
  let cwdDepth = 3;
  let showTimeBars = true;
  let timeBarDim = 0.25;

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
    const ml = Number(config.cwdMaxLength);
    if (Number.isInteger(ml) && ml >= 10 && ml <= 100) cwdMaxLength = ml;
    const cd = Number(config.cwdDepth);
    if (Number.isInteger(cd) && cd >= 1 && cd <= 10) cwdDepth = cd;
    if (config.showTimeBars === true || config.showTimeBars === false) showTimeBars = config.showTimeBars;
    const td = Number(config.timeBarDim);
    if (isFinite(td) && td >= 0 && td <= 1) timeBarDim = td;
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

  // Time bar bg: blended toward terminal default (subtler than usage bars)
  const rawBg = userMatch?.bg ?? builtinMatch?.bg;
  const timeBarBg = computeTimeBarBg(rawBg, dark, truecolor, timeBarDim);

  _barConfig = { width, emptyBg, timeBarBg, gradient, truecolor, isRgb, partialBlocks: usePartialBlocks, orientation, cwdMaxLength, cwdDepth, showTimeBars };
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

// Cached per-process, keyed by claudeDir. Credentials won't change mid-render.
const _credentialsByDir = new Map();

function resetCredentialsCache() {
  _credentialsByDir.clear();
}

function readCredentialsFile(claudeDir) {
  if (_credentialsByDir.has(claudeDir)) return _credentialsByDir.get(claudeDir);

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
    _credentialsByDir.set(claudeDir, fileData);
    return fileData;
  }

  // File token missing or expired-without-refresh-token: try Keychain
  const keychainData = readKeychainCredentials(claudeDir);
  if (keychainData) {
    // Merge: preserve file data (mcpOAuth etc.) with keychain OAuth
    const merged = { ...(fileData || {}), claudeAiOauth: keychainData.claudeAiOauth };
    _credentialsByDir.set(claudeDir, merged);
    return merged;
  }

  // Fall back to file data even if token is stale (refresh will be attempted)
  _credentialsByDir.set(claudeDir, fileData);
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
        lastSuccessTs: cache.lastSuccessTs ?? null,
        fiveHour: {
          percent: cache.fiveHour?.percent ?? null,
          resetIn: cache.fiveHour?.resetAt != null ? Math.max(0, cache.fiveHour.resetAt - now) : null,
        },
        weekly: {
          percent: cache.weekly?.percent ?? null,
          resetIn: cache.weekly?.resetAt != null ? Math.max(0, cache.weekly.resetAt - now) : null,
        },
        extraUsage: {
          percent: cache.extraUsage?.percent ?? null,
          enabled: cache.extraUsage?.enabled ?? false,
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
        lastSuccessTs: cache?.lastSuccessTs ?? null,
        status: 'error',
        consecutiveErrors: (cache?.consecutiveErrors || 0) + 1,
        fiveHour: { percent: cache?.fiveHour?.percent ?? null, resetAt: null },
        weekly: { percent: cache?.weekly?.percent ?? null, resetAt: null },
        extraUsage: { percent: cache?.extraUsage?.percent ?? null, enabled: cache?.extraUsage?.enabled ?? false },
      }));
    } catch {}

    if (cache?.fiveHour?.percent != null || cache?.weekly?.percent != null) {
      return {
        stale: true,
        lastSuccessTs: cache.lastSuccessTs ?? null,
        fiveHour: {
          percent: cache.fiveHour?.percent ?? null,
          resetIn: cache.fiveHour?.resetAt != null ? Math.max(0, cache.fiveHour.resetAt - now) : null,
        },
        weekly: {
          percent: cache.weekly?.percent ?? null,
          resetIn: cache.weekly?.resetAt != null ? Math.max(0, cache.weekly.resetAt - now) : null,
        },
        extraUsage: {
          percent: cache?.extraUsage?.percent ?? null,
          enabled: cache?.extraUsage?.enabled ?? false,
        },
      };
    }
    return { stale: false, lastSuccessTs: null, fiveHour: { percent: null, resetIn: null }, weekly: { percent: null, resetIn: null }, extraUsage: { percent: null, enabled: false } };
  }

  // Transient failure: cache and fall back to stale data
  if (!apiData) {
    try {
      writeFileAtomic(cacheFile, JSON.stringify({
        timestamp: now,
        lastSuccessTs: cache?.lastSuccessTs ?? null,
        status: 'error',
        consecutiveErrors: (cache?.consecutiveErrors || 0) + 1,
        fiveHour: { percent: cache?.fiveHour?.percent ?? null, resetAt: null },
        weekly: { percent: cache?.weekly?.percent ?? null, resetAt: null },
        extraUsage: { percent: cache?.extraUsage?.percent ?? null, enabled: cache?.extraUsage?.enabled ?? false },
      }));
    } catch {}

    if (cache?.fiveHour?.percent != null || cache?.weekly?.percent != null) {
      return {
        stale: true,
        lastSuccessTs: cache.lastSuccessTs ?? null,
        fiveHour: {
          percent: cache.fiveHour?.percent ?? null,
          resetIn: cache.fiveHour?.resetAt != null ? Math.max(0, cache.fiveHour.resetAt - now) : null,
        },
        weekly: {
          percent: cache.weekly?.percent ?? null,
          resetIn: cache.weekly?.resetAt != null ? Math.max(0, cache.weekly.resetAt - now) : null,
        },
        extraUsage: {
          percent: cache?.extraUsage?.percent ?? null,
          enabled: cache?.extraUsage?.enabled ?? false,
        },
      };
    }
    return { stale: false, lastSuccessTs: null, fiveHour: { percent: null, resetIn: null }, weekly: { percent: null, resetIn: null }, extraUsage: { percent: null, enabled: false } };
  }

  // Success: parse and cache with absolute timestamps
  let usage = {
    stale: false,
    lastSuccessTs: Date.now(),
    fiveHour: { percent: null, resetIn: null },
    weekly: { percent: null, resetIn: null },
    extraUsage: { percent: null, enabled: false },
  };

  let cacheEntry = {
    timestamp: now,
    lastSuccessTs: usage.lastSuccessTs,
    status: 'ok',
    consecutiveErrors: 0,
    fiveHour: { percent: null, resetAt: null },
    weekly: { percent: null, resetAt: null },
    extraUsage: { percent: null, enabled: false },
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

  if (apiData.extra_usage) {
    usage.extraUsage.enabled = !!apiData.extra_usage.is_enabled;
    if (apiData.extra_usage.is_enabled) {
      usage.extraUsage.percent = apiData.extra_usage.utilization ?? 0;
    }
    cacheEntry.extraUsage = { percent: usage.extraUsage.percent, enabled: usage.extraUsage.enabled };
  }

  if (usage.fiveHour.percent != null || usage.weekly.percent != null) {
    try {
      writeFileAtomic(cacheFile, JSON.stringify(cacheEntry));
    } catch {}
  }

  return usage;
}

// -- Active GitHub user resolver (gated by GH_TOKEN) --
// Maps process.env.GH_TOKEN -> GitHub username via `gh api user`.
// The token is inherited via env (NEVER placed in argv, NEVER logged).
// Cached by sha256(token)[:8] in a separate 0600-mode file to keep the
// usage cache schema clean and to harden against accidental leakage.

const GH_NEGATIVE_TTL_MS = 5 * 60 * 1000;

function ghTokenHash(token) {
  return crypto.createHash('sha256').update(token).digest('hex').slice(0, 8);
}

let _ghUserCache = null;
function readGhUserCache(claudeDir) {
  if (_ghUserCache !== null) return _ghUserCache;
  try {
    const raw = fs.readFileSync(path.join(claudeDir, '.gh-users-cache.json'), 'utf8');
    const parsed = JSON.parse(raw);
    _ghUserCache = (parsed && typeof parsed === 'object') ? parsed : {};
  } catch {
    _ghUserCache = {};
  }
  return _ghUserCache;
}

function writeGhUserCache(claudeDir, obj) {
  const filePath = path.join(claudeDir, '.gh-users-cache.json');
  // Atomic write + chmod 0600 to match the security note in the spec.
  // writeFileAtomic uses tmpfile + rename(2), so we chmod the final path
  // after the rename succeeds.
  try {
    writeFileAtomic(filePath, JSON.stringify(obj));
    try { fs.chmodSync(filePath, 0o600); } catch {}
    _ghUserCache = obj;
  } catch {}
}

async function getActiveGhUser(claudeDir) {
  const token = process.env.GH_TOKEN;
  if (!token) return null;

  const hash = ghTokenHash(token);
  const cache = readGhUserCache(claudeDir);
  const entry = cache[hash];

  if (entry) {
    if (entry.login) return entry.login;
    // Negative entry: honor TTL before retrying
    const ttl = entry.ttl || GH_NEGATIVE_TTL_MS;
    if (entry.ts && Date.now() - entry.ts < ttl) return null;
  }

  // Cache miss or stale negative entry: ask `gh`. Token inherits via env --
  // do NOT pass it in argv (visible in `ps`).
  let login = null;
  try {
    const { stdout } = await execFileAsync('gh', ['api', 'user', '--jq', '.login'], {
      timeout: 3000,
      env: process.env,
    });
    const trimmed = (stdout || '').trim();
    if (trimmed) login = trimmed;
  } catch {
    // Intentionally do not log the error object: it may contain stderr that
    // includes URL params or response bodies. Brief warning only.
    try { process.stderr.write('howmuchleft: gh user lookup failed\n'); } catch {}
  }

  const next = { ...cache };
  if (login) {
    next[hash] = { login, ts: Date.now() };
  } else {
    next[hash] = { login: null, ts: Date.now(), ttl: GH_NEGATIVE_TTL_MS };
  }
  writeGhUserCache(claudeDir, next);
  return login;
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
function progressBar(percent, width, emptyBgOverride) {
  const config = getBarConfig();
  if (width == null) width = config.width;
  const emptyBg = emptyBgOverride || config.emptyBg;

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

// Format extra usage percentage with warm background highlight
function formatExtraPercent(percent, stale = false) {
  if (percent === null || percent === undefined) {
    return `${colors.gray}?%${colors.reset}`;
  }
  const dark = isDarkMode();
  const config = getBarConfig();
  // Warm background on the percentage text
  let warmBg;
  if (config.truecolor) {
    warmBg = dark ? '\x1b[48;2;100;60;0m' : '\x1b[48;2;255;210;140m';
  } else {
    warmBg = dark ? '\x1b[48;5;94m' : '\x1b[48;5;223m';
  }
  const val = stale ? `~${Math.round(percent)}%` : `${Math.round(percent)}%`;
  return `${warmBg}${colors.white}${val}${colors.reset}`;
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
      '--no-optional-locks', 'status', '--porcelain=v2', '--branch', '-unormal', '--no-renames',
    ], { cwd, timeout: 3000 });

    let branch = null;
    let changes = 0;
    let ahead = 0;
    let behind = 0;

    for (const line of stdout.split('\n')) {
      if (line.startsWith('# branch.head ')) {
        branch = line.slice('# branch.head '.length);
        if (branch === '(detached)') branch = 'detached';
      } else if (line.startsWith('# branch.ab ')) {
        const parts = line.slice('# branch.ab '.length).split(' ');
        const a = parseInt(parts[0], 10);
        const b = parseInt(parts[1], 10);
        if (!Number.isNaN(a)) ahead = a;
        if (!Number.isNaN(b)) behind = Math.abs(b);
      } else if (/^([12u] |\? )/.test(line)) {
        changes++;
      }
    }

    return { branch: branch || 'detached', changes, ahead, behind, hasGit: true };
  } catch {
    return { branch: null, changes: 0, ahead: 0, behind: 0, hasGit: false };
  }
}

function formatAge(ms) {
  if (ms < 60_000) return `${Math.floor(ms / 1000)}s`;
  if (ms < 3_600_000) return `${Math.floor(ms / 60_000)}m`;
  return `${Math.floor(ms / 3_600_000)}h`;
}

function shortenPath(p, maxLen = 50, depth = 3) {
  if (!p || p.length <= maxLen) return p || '~';
  const home = os.homedir();
  if (p.startsWith(home)) p = '~' + p.slice(home.length);
  if (p.length <= maxLen) return p;
  const parts = p.split('/');
  if (parts.length <= 2) return '...' + p.slice(-maxLen + 3);
  return parts[0] + '/.../' + parts.slice(-depth).join('/');
}

/**
 * Render one cell of a vertical bar.
 * Each bar is 3 rows tall (top=0, bottom=2), 8 states per row = 24 levels.
 * Fills bottom-to-top: row 2 fills first, then row 1, then row 0.
 */
function verticalBarCell(percent, rowIdx, totalRows, emptyBgOverride) {
  if (totalRows == null) totalRows = 3;
  const config = getBarConfig();
  const emptyBg = emptyBgOverride || config.emptyBg;
  const clamped = (percent == null) ? 0 : Math.max(0, Math.min(100, percent));
  const maxLevel = totalRows * 8;
  const level = Math.round(clamped / 100 * maxLevel); // 0–maxLevel
  const { fg } = getGradientStop(clamped);

  // Each row covers 8 levels; bottom row fills first, top row last
  const rowBase = (totalRows - 1 - rowIdx) * 8;
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
 * Render one cell of a vertical time-elapsed bar with urgency coloring.
 * Same fill logic as verticalBarCell but colors by urgency (how far usage
 * outpaces elapsed time) instead of the standard usage gradient.
 */
function timeBarCell(timePercent, usagePercent, rowIdx, totalRows) {
  if (totalRows == null) totalRows = 3;
  const config = getBarConfig();
  const emptyBg = config.timeBarBg;
  const clamped = (timePercent == null) ? 0 : Math.max(0, Math.min(100, timePercent));
  const maxLevel = totalRows * 8;
  const level = Math.round(clamped / 100 * maxLevel);

  // Urgency: how much usage outpaces elapsed time (0 = fine, 1 = critical)
  const urgency = (timePercent >= 100) ? 0
    : Math.max(0, Math.min(1, (usagePercent - timePercent) / (100 - timePercent)));
  const { fg } = getUrgencyColor(urgency);

  const rowBase = (totalRows - 1 - rowIdx) * 8;
  const rowLevel = level - rowBase;

  if (rowLevel >= 8) {
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
 *
 * When weekly usage >= 100% and extra usage is enabled, the 3rd bar
 * contextually swaps from weekly to extra usage with a warm amber background.
 * The weekly reset countdown always remains visible.
 */
function renderLines({ context, model, tier, elapsed, profile, profileColor, ghUser, fiveHour, weekly, extraUsage, stale, lastSuccessTs, git, lines: lineChanges, cwd, fiveHourTimePercent, weeklyTimePercent }) {
  const config = getBarConfig();
  const lines = [];

  // Pre-compute labels shared by both modes
  const elapsedStr = elapsed != null ? `${colors.dim}${formatTimeRemaining(elapsed)}${colors.reset} ` : '';
  const profileStr = profile ? `${profileColor || colors.dim}${profile}${colors.reset} ` : '';
  // GitHub username adjacent to the profile name (both are identity signals).
  // Subtle gray + dim to keep visual weight low; only present when GH_TOKEN
  // resolved to a real account.
  const ghUserStr = ghUser ? `${colors.dim}${colors.gray}@${ghUser}${colors.reset} ` : '';
  const ageSuffix = (stale && lastSuccessTs)
    ? ` ${colors.dim}(${formatAge(Date.now() - lastSuccessTs)})${colors.reset}`
    : '';
  const fiveHourPercent = formatPercent(fiveHour.percent, stale) + ageSuffix;
  const fiveHourReset = formatTimeRemaining(fiveHour.resetIn);
  const gitStr = git.hasGit
    ? `${colors.cyan}${git.branch}${colors.reset}` +
      (git.changes > 0 ? ` ${colors.yellow}+${git.changes}${colors.reset}` : '') +
      (git.ahead > 0 ? ` ${colors.yellow}\u2191${git.ahead}${colors.reset}` : '') +
      (git.behind > 0 ? ` ${colors.yellow}\u2193${git.behind}${colors.reset}` : '')
    : `${colors.gray}no .git${colors.reset}`;
  const added = lineChanges.added;
  const removed = lineChanges.removed;
  const linesStr = (added || removed)
    ? `${colors.green}+${added ?? 0}${colors.reset}/${colors.red}-${removed ?? 0}${colors.reset}`
    : '';
  const weeklyPercent = formatPercent(weekly.percent, stale) + ageSuffix;
  const weeklyReset = formatTimeRemaining(weekly.resetIn);

  // Determine if the 3rd bar should show extra usage instead of weekly
  const showExtraUsage = weekly.percent >= 100 && extraUsage?.enabled;

  if (config.orientation === 'vertical') {
    const warmBg = showExtraUsage ? getExtraUsageBg() : null;
    // 3rd bar is either weekly or extra usage
    const thirdPercent = showExtraUsage ? extraUsage.percent : weekly.percent;
    const percents = [context, fiveHour.percent, thirdPercent];
    // Show time bars when enabled and time data is available
    const showTime = config.showTimeBars && fiveHourTimePercent != null;
    for (let row = 0; row < 3; row++) {
      let barStr = '';
      for (let i = 0; i < percents.length; i++) {
        if (i > 0) barStr += `${colors.reset} `;
        // Use warm bg for 3rd bar when showing extra usage
        const bgOverride = (i === 2 && warmBg) ? warmBg : null;
        barStr += verticalBarCell(percents[i], row, 3, bgOverride);
        // Time bar immediately after 5hr (i=1) and weekly (i=2), no space separator
        if (showTime && (i === 1 || i === 2)) {
          barStr += timeBarCell(
            i === 1 ? fiveHourTimePercent : weeklyTimePercent,
            i === 1 ? fiveHour.percent : (showExtraUsage ? extraUsage.percent : weekly.percent),
            row, 3
          );
        }
      }
      barStr += colors.reset;

      if (row === 0) {
        lines.push(
          `${barStr} ${colors.cyan}${Math.round(context)}%${colors.reset} ` +
          `${elapsedStr}` +
          `${profileStr}` +
          `${ghUserStr}` +
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
        // 3rd line: show extra usage % or weekly %, always show weekly reset
        const thirdPct = showExtraUsage
          ? formatExtraPercent(extraUsage.percent, stale) + ageSuffix
          : weeklyPercent;
        lines.push(
          `${barStr} ${thirdPct} ` +
          `${colors.dim}${weeklyReset}${colors.reset} ` +
          `${colors.white}${cwd}${colors.reset}`
        );
      }
    }
  } else {
    // Horizontal: each line has its own progress bar
    const warmBg = showExtraUsage ? getExtraUsageBg() : null;

    const contextBar = progressBar(context);
    lines.push(
      `${contextBar} ${colors.cyan}${Math.round(context)}%${colors.reset} ` +
      `${elapsedStr}` +
      `${profileStr}` +
      `${ghUserStr}` +
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

    // 3rd bar: extra usage replaces weekly when extra usage is active
    const thirdPercent = showExtraUsage ? extraUsage.percent : weekly.percent;
    const thirdBar = progressBar(thirdPercent, null, warmBg);
    const thirdPct = showExtraUsage
      ? formatExtraPercent(extraUsage.percent, stale) + ageSuffix
      : weeklyPercent;
    lines.push(
      `${thirdBar} ${thirdPct} ` +
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

  const [usage, gitInfo, ghUser] = await Promise.all([
    isOAuth
      ? getUsageData(claudeDir, isSessionStart || credentialsChanged)
      : { stale: false, fiveHour: { percent: null, resetIn: null }, weekly: { percent: null, resetIn: null }, extraUsage: { percent: null, enabled: false } },
    getGitInfo(cwd),
    getActiveGhUser(claudeDir),
  ]);

  const elapsed = getSessionElapsed(claudeDir);
  const profile = getProfileName();
  const profileColor = profile ? hueToAnsi(hashToHue(claudeDir), isDarkMode()) : '';

  // Time elapsed as percentage of total window (null when resetIn unavailable)
  const fiveHourTimePercent = usage.fiveHour.resetIn != null
    ? Math.max(0, Math.min(100, (1 - usage.fiveHour.resetIn / FIVE_HOUR_MS) * 100))
    : null;
  const weeklyTimePercent = usage.weekly.resetIn != null
    ? Math.max(0, Math.min(100, (1 - usage.weekly.resetIn / SEVEN_DAY_MS) * 100))
    : null;

  console.log(renderLines({
    context: contextUsedPercent,
    model,
    tier: subscriptionName,
    elapsed,
    profile,
    profileColor,
    ghUser,
    fiveHour: usage.fiveHour,
    weekly: usage.weekly,
    extraUsage: usage.extraUsage,
    stale: usage.stale,
    lastSuccessTs: usage.lastSuccessTs,
    git: gitInfo,
    lines: { added: cost.total_lines_added, removed: cost.total_lines_removed },
    cwd: shortenPath(cwd, getBarConfig().cwdMaxLength, getBarConfig().cwdDepth),
    fiveHourTimePercent,
    weeklyTimePercent,
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

  // Sample bars at key percentages, with vertical columns on all 7 rows
  const pcts = [5, 20, 35, 50, 65, 80, 95];
  for (let i = 0; i < pcts.length; i++) {
    const bar = progressBar(pcts[i], 13);
    const label = String(pcts[i]).padStart(2);
    let line = `${bar} ${label}%  `;
    for (const vp of pcts) {
      const cell = verticalBarCell(vp, i, pcts.length);
      line += cell + cell;
    }
    line += colors.reset;
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
  console.log(strip + colors.reset);
}

/**
 * Discover Claude Code profile directories.
 *
 * Resolution order:
 *   1. Read the "profiles" array from the howmuchleft config file.
 *   2. If empty or missing, fall back to scanning the home directory for
 *      ~/.claude and ~/.claude-* directories that contain .credentials.json.
 *
 * Returns an array of absolute directory paths.
 */
function discoverProfiles() {
  const homeDir = os.homedir();

  // Try config file first
  try {
    const raw = fs.readFileSync(CONFIG_PATH, 'utf8');
    const config = parseJsonc(raw);
    if (Array.isArray(config.profiles) && config.profiles.length > 0) {
      // Only return dirs that actually exist
      return config.profiles.filter(p => {
        try { return fs.statSync(p).isDirectory(); } catch { return false; }
      });
    }
  } catch {}

  // Fall back to scanning home directory
  const found = [];

  // Check default ~/.claude
  const defaultDir = path.join(homeDir, '.claude');
  try {
    if (fs.statSync(path.join(defaultDir, '.credentials.json')).isFile()) {
      found.push(defaultDir);
    }
  } catch {}

  // Check ~/.claude-* profile directories
  try {
    const entries = fs.readdirSync(homeDir);
    for (const entry of entries) {
      if (entry.startsWith('.claude-') && entry !== '.claude') {
        const fullPath = path.join(homeDir, entry);
        try {
          if (fs.statSync(fullPath).isDirectory() &&
              fs.statSync(path.join(fullPath, '.credentials.json')).isFile()) {
            found.push(fullPath);
          }
        } catch {}
      }
    }
  } catch {}

  // Sort by directory creation time (oldest first)
  found.sort((a, b) => {
    try {
      return fs.statSync(a).birthtimeMs - fs.statSync(b).birthtimeMs;
    } catch { return 0; }
  });

  return found;
}

module.exports = {
  main, CONFIG_PATH, progressBar, formatPercent, formatExtraPercent,
  formatTimeRemaining, colors, renderLines, testColors,
  parseJsonc, writeFileAtomic, discoverProfiles,
  getUsageData, getAuthInfo, hashToHue, hueToAnsi, isDarkMode,
  getExtraUsageBg, getBarConfig, verticalBarCell, resetCredentialsCache,
  timeBarCell, getUrgencyColor,
};

// Run directly when invoked as a script (not when required by cli.js)
if (require.main === module) {
  main().catch(err => {
    console.error('Statusline error:', err.message);
    process.exit(1);
  });
}
