#!/usr/bin/env node
/**
 * HowMuchLeft CLI
 *
 * Subcommands: profile, demo, config, colors
 * No subcommand + piped stdin = statusline mode (called by Claude Code)
 */

const fs = require('fs');
const path = require('path');
const os = require('os');
const { main, CONFIG_PATH, testColors, parseJsonc, writeFileAtomic, discoverProfiles } = require('../lib/statusline');

const VERSION = require('../package.json').version;

// --- Helpers ---

function resolvePath(p) {
  if (p.startsWith('~')) return path.join(os.homedir(), p.slice(1));
  return path.resolve(p);
}

function resolveClaudeDir(args) {
  const dir = args.find(a => !a.startsWith('-'));
  if (dir) return resolvePath(dir);
  return process.env.CLAUDE_CONFIG_DIR || path.join(os.homedir(), '.claude');
}

function getStatuslineCommand(claudeDir) {
  return `howmuchleft ${claudeDir.replace(os.homedir(), '~')}`;
}

function readSettingsJson(claudeDir) {
  const settingsPath = path.join(claudeDir, 'settings.json');
  try {
    return JSON.parse(fs.readFileSync(settingsPath, 'utf8'));
  } catch {
    return {};
  }
}

function writeSettingsJson(claudeDir, settings) {
  const settingsPath = path.join(claudeDir, 'settings.json');
  fs.mkdirSync(claudeDir, { recursive: true });
  fs.writeFileSync(settingsPath, JSON.stringify(settings, null, 2) + '\n');
}

function registerProfile(claudeDir) {
  const absDir = path.resolve(claudeDir);
  let config = {};
  try {
    config = parseJsonc(fs.readFileSync(CONFIG_PATH, 'utf8'));
  } catch {}
  if (!Array.isArray(config.profiles)) config.profiles = [];
  if (!config.profiles.includes(absDir)) {
    config.profiles.push(absDir);
  }
  fs.mkdirSync(path.dirname(CONFIG_PATH), { recursive: true });
  writeFileAtomic(CONFIG_PATH, JSON.stringify(config, null, 2) + '\n');
}

function unregisterProfile(claudeDir) {
  const absDir = path.resolve(claudeDir);
  let config = {};
  try {
    config = parseJsonc(fs.readFileSync(CONFIG_PATH, 'utf8'));
  } catch { return; }
  if (!Array.isArray(config.profiles)) return;
  config.profiles = config.profiles.filter(p => p !== absDir);
  writeFileAtomic(CONFIG_PATH, JSON.stringify(config, null, 2) + '\n');
}

// --- Profile subcommands ---

function profileList(args) {
  const { runDashboard } = require('../lib/dashboard');
  runDashboard(args.includes('--live'));
}

function profileInstall(args) {
  const remaining = args.filter(a => a !== 'profile' && a !== 'install');
  const claudeDir = resolveClaudeDir(remaining);
  const settingsPath = path.join(claudeDir, 'settings.json');
  const settings = readSettingsJson(claudeDir);
  const command = getStatuslineCommand(claudeDir);

  if (settings.statusLine) {
    console.log(`Current statusLine in ${settingsPath}:`);
    console.log(`  ${JSON.stringify(settings.statusLine)}`);
    console.log();

    if (settings.statusLine.command && settings.statusLine.command.includes('howmuchleft')) {
      console.log('howmuchleft is already installed. To update:');
      console.log('  howmuchleft profile uninstall && howmuchleft profile install');
      return;
    }

    console.log('A statusLine is already configured. Overwrite? (y/N)');
    process.stdin.setEncoding('utf8');
    process.stdin.once('data', (answer) => {
      if (answer.trim().toLowerCase() !== 'y') {
        console.log('Aborted.');
        process.exit(0);
      }
      doInstall(claudeDir, settings, command, settingsPath);
    });
    return;
  }

  doInstall(claudeDir, settings, command, settingsPath);
}

function doInstall(claudeDir, settings, command, settingsPath) {
  settings.statusLine = {
    type: 'command',
    command,
    padding: 0,
  };

  writeSettingsJson(claudeDir, settings);
  registerProfile(claudeDir);
  console.log(`Installed. Added to ${settingsPath}:`);
  console.log(`  ${JSON.stringify(settings.statusLine, null, 2).replace(/\n/g, '\n  ')}`);
  console.log();
  console.log('Restart Claude Code to see the statusline.');
}

function profileUninstall(args) {
  const remaining = args.filter(a => a !== 'profile' && a !== 'uninstall');
  const claudeDir = resolveClaudeDir(remaining);
  const settingsPath = path.join(claudeDir, 'settings.json');
  const settings = readSettingsJson(claudeDir);

  if (!settings.statusLine) {
    console.log(`No statusLine configured in ${settingsPath}.`);
    return;
  }

  if (settings.statusLine.command && !settings.statusLine.command.includes('howmuchleft')) {
    console.log(`statusLine in ${settingsPath} is not howmuchleft:`);
    console.log(`  ${JSON.stringify(settings.statusLine)}`);
    console.log('Not removing. Edit settings.json manually to remove.');
    return;
  }

  delete settings.statusLine;
  writeSettingsJson(claudeDir, settings);
  unregisterProfile(claudeDir);
  console.log(`Removed statusLine from ${settingsPath}.`);
  console.log('Restart Claude Code to apply.');
}

// --- Other commands ---

function showConfig() {
  console.log(`Config file: ${CONFIG_PATH}`);
  console.log();
  try {
    const config = parseJsonc(fs.readFileSync(CONFIG_PATH, 'utf8'));
    console.log('Current settings:');
    console.log(`  progressLength: ${config.progressLength ?? '(default: 12)'}`);
    console.log(`  colorMode:      ${config.colorMode ?? '(default: auto)'}`);
    if (Array.isArray(config.colors)) {
      console.log(`  colors:         ${config.colors.length} entries`);
    } else {
      console.log('  colors:         (using built-in defaults)');
    }
    console.log();
    if (Array.isArray(config.profiles) && config.profiles.length > 0) {
      console.log(`Registered profiles (${config.profiles.length}):`);
      for (const p of config.profiles) {
        console.log(`  ${p}`);
      }
    } else {
      console.log('Profiles: (none registered -- run howmuchleft profile install)');
    }
  } catch {
    console.log('No config file found. Using defaults:');
    console.log('  progressLength: 12');
    console.log('  colorMode:      auto');
    console.log('  colors:         built-in gradients');
    console.log('  profiles:       (none)');
    console.log();
    console.log(`Create one with: cp ${path.resolve(__dirname, '..', 'config.example.json')} ${CONFIG_PATH}`);
  }
}

function showHelp() {
  console.log(`howmuchleft v${VERSION}
Pixel-perfect progress bars for your Claude Code statusline.

Usage:
  howmuchleft                            Run the statusline (called by Claude Code)
  howmuchleft profile list [--live]      Show all profiles' usage
  howmuchleft profile install [dir]      Add howmuchleft to a Claude Code profile
  howmuchleft profile uninstall [dir]    Remove howmuchleft from a profile
  howmuchleft config                     Show config file and current settings
  howmuchleft demo [seconds]             Run a time-lapse demo (default 60s)
  howmuchleft colors                     Preview gradient colors for your terminal
  howmuchleft version                    Show version

Config file: ${CONFIG_PATH}

Examples:
  howmuchleft profile install
  howmuchleft profile install ~/.claude-work
  howmuchleft profile list --live`);
}

// --- Main ---

const args = process.argv.slice(2);
const cmd = args[0];

if (!cmd && process.stdin.isTTY) {
  // No args, running from terminal
  showHelp();
} else if (!cmd || (cmd.startsWith('-') && !process.stdin.isTTY)) {
  // No subcommand, stdin piped = statusline mode
  main().catch(err => {
    console.error('Statusline error:', err.message);
    process.exit(1);
  });
} else if (cmd === 'help' || cmd === '--help' || cmd === '-h') {
  showHelp();
} else if (cmd === 'version' || cmd === '--version' || cmd === '-v') {
  console.log(VERSION);
} else if (cmd === 'profile') {
  const sub = args[1];
  if (sub === 'list' || sub === 'ls') {
    profileList(args.slice(2));
  } else if (sub === 'install') {
    profileInstall(args);
  } else if (sub === 'uninstall') {
    profileUninstall(args);
  } else {
    // bare "howmuchleft profile" = list
    profileList(args.slice(1));
  }
} else if (cmd === 'config') {
  showConfig();
} else if (cmd === 'demo') {
  const { runDemo } = require('../lib/demo');
  const duration = parseInt(args[1], 10);
  runDemo(duration > 0 ? duration : undefined);
} else if (cmd === 'colors') {
  testColors();
} else {
  // Unknown arg — might be a config dir path for statusline mode
  if (!process.stdin.isTTY) {
    main().catch(err => {
      console.error('Statusline error:', err.message);
      process.exit(1);
    });
  } else {
    console.log(`Unknown command: ${cmd}`);
    console.log('Run howmuchleft help for usage.');
    process.exit(1);
  }
}
