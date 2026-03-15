#!/usr/bin/env node
/**
 * HowMuchLeft CLI
 *
 * When invoked with no flags (or just a config dir arg), runs the statusline.
 * Flags: --install, --uninstall, --config, --help, --version
 */

const fs = require('fs');
const path = require('path');
const os = require('os');
const { main, CONFIG_PATH } = require('../lib/statusline');

const VERSION = require('../package.json').version;

// --- Helpers ---

function resolveClaudeDir(args) {
  // Find the first arg that isn't a flag (the Claude config dir)
  const dir = args.find(a => !a.startsWith('-'));
  if (dir) {
    if (dir.startsWith('~')) return path.join(os.homedir(), dir.slice(1));
    return path.resolve(dir);
  }
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
  // Ensure directory exists
  fs.mkdirSync(claudeDir, { recursive: true });
  fs.writeFileSync(settingsPath, JSON.stringify(settings, null, 2) + '\n');
}

// --- Commands ---

function showHelp() {
  console.log(`howmuchleft v${VERSION}
Pixel-perfect progress bars for your Claude Code statusline.

Usage:
  howmuchleft [claude-config-dir]       Run the statusline (called by Claude Code)
  howmuchleft --install [config-dir]    Add howmuchleft to your Claude Code settings
  howmuchleft --uninstall [config-dir]  Remove howmuchleft from your Claude Code settings
  howmuchleft --config                  Show config file path and current settings
  howmuchleft --version                 Show version

Config file: ${CONFIG_PATH}
  {
    "progressLength": 12,       Bar width in characters (3-40, default 12)
    "emptyBgDark": 236,         256-color index for empty bar in dark mode
    "emptyBgLight": 252         256-color index for empty bar in light mode
  }

Examples:
  howmuchleft --install
  howmuchleft --install ~/.claude-work`);
}

function showVersion() {
  console.log(VERSION);
}

function install(args) {
  const claudeDir = resolveClaudeDir(args.filter(a => a !== '--install'));
  const settingsPath = path.join(claudeDir, 'settings.json');
  const settings = readSettingsJson(claudeDir);
  const command = getStatuslineCommand(claudeDir);

  if (settings.statusLine) {
    console.log(`Current statusLine in ${settingsPath}:`);
    console.log(`  ${JSON.stringify(settings.statusLine)}`);
    console.log();

    // Check if it's already howmuchleft
    if (settings.statusLine.command && settings.statusLine.command.includes('howmuchleft')) {
      console.log('howmuchleft is already installed. To update the command:');
      console.log('  howmuchleft --uninstall && howmuchleft --install');
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
  console.log(`Installed. Added to ${settingsPath}:`);
  console.log(`  ${JSON.stringify(settings.statusLine, null, 2).replace(/\n/g, '\n  ')}`);
  console.log();
  console.log('Restart Claude Code to see the statusline.');
}

function uninstall(args) {
  const claudeDir = resolveClaudeDir(args.filter(a => a !== '--uninstall'));
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
  console.log(`Removed statusLine from ${settingsPath}.`);
  console.log('Restart Claude Code to apply.');
}

function showConfig() {
  console.log(`Config file: ${CONFIG_PATH}`);
  console.log();
  try {
    const config = JSON.parse(fs.readFileSync(CONFIG_PATH, 'utf8'));
    console.log('Current settings:');
    console.log(`  progressLength: ${config.progressLength ?? '(default: 12)'}`);
    console.log(`  emptyBgDark:    ${config.emptyBgDark ?? '(default: 236)'}`);
    console.log(`  emptyBgLight:   ${config.emptyBgLight ?? '(default: 252)'}`);
  } catch {
    console.log('No config file found. Using defaults:');
    console.log('  progressLength: 12');
    console.log('  emptyBgDark:    236');
    console.log('  emptyBgLight:   252');
    console.log();
    console.log(`Create one with: cp ${path.resolve(__dirname, '..', 'config.example.json')} ${CONFIG_PATH}`);
  }
}

// --- Main ---

const args = process.argv.slice(2);

if (args.includes('--help') || args.includes('-h')) {
  showHelp();
} else if (args.includes('--version') || args.includes('-v')) {
  showVersion();
} else if (args.includes('--install')) {
  install(args);
} else if (args.includes('--uninstall')) {
  uninstall(args);
} else if (args.includes('--config')) {
  showConfig();
} else if (process.stdin.isTTY) {
  // Running from a terminal, not piped by Claude Code
  console.log('This command is meant to be called by Claude Code, not run directly.');
  console.log('Try: howmuchleft --help');
  process.exit(0);
} else {
  // Default: run the statusline (stdin is piped by Claude Code)
  main().catch(err => {
    console.error('Statusline error:', err.message);
    process.exit(1);
  });
}
