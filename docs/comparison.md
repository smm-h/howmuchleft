+++
title = "Comparison with other statuslines"
description = "How howmuchleft measures up against the other Claude Code statuslines: start-up time, peak memory, what each one can show, and how to rerun the benchmark."
nav_order = 10
+++

# Comparison with other statuslines

Claude Code spawns its statusline as a child process on every render, so the
program runs hundreds of times an hour and its start-up cost is paid every
time. That makes a statusline one of the few places where the difference
between a compiled binary and a Node.js script is something you can feel.

This page is the measurement, not an argument. Every number here comes from
`docs/statusline-comparison.toml`, which
[`scripts/compare-statuslines.sh`](https://github.com/smm-h/howmuchleft/blob/main/scripts/compare-statuslines.sh)
writes when it is run, and which a selfdoc directive renders into the table
below.

## What was compared

Every tool in the table is a Claude Code statusline: it reads the status object
Claude Code writes on standard input and prints the status bar. They are the
ones distributed as a ready-to-run release binary or an npm package, which is
what makes a pinned, reproducible install possible; a statusline that only
exists as a shell snippet in somebody's dotfiles cannot be pinned and is not
here.

Each tool gets the same synthetic status object on standard input, the same
throwaway git repository as its working directory, and a scratch `HOME`,
`XDG_CONFIG_HOME` and Claude configuration directory that contain nothing but
the files the harness wrote. Nothing reaches the real Claude Code state, and
the measurement does not depend on how much history the machine happens to have
accumulated.

## Results

:-: statusline-comparison

The `Transcript-free` column answers whether a tool can render its status bar
from the status object alone. A tool that reads the JSONL transcripts under the
Claude configuration directory does more work as a session grows, and it reads
the text of your conversations to do it.

## What each tool shows

- **howmuchleft** renders three lines: a context window bar, a 5-hour limit bar
  and a weekly limit bar, each shaded green to red as it fills, plus the model,
  the subscription tier, the reset times, the git branch, the session's added
  and removed line counts, and the working directory. The limit percentages and
  the line counts come from the status object. The branch name comes from
  reading the repository's `HEAD` file rather than from running `git`, and the
  desktop light/dark preference, which decides the gradient, is detected once
  and kept in a cache file, so a render is a short series of file reads.
- **cship** renders a single configurable line in the style of Starship: model,
  a context bar, session cost and both limit windows. Its directory and git
  segments are Starship passthrough modules, which means they need Starship
  installed as a separate program. The harness does not install it, so cship's
  number covers its own segments and no git work.
- **CCometixLine (ccline)** renders model, directory, git, a context window
  segment and a usage segment. Its usage segment is the one exception to the
  rule that these tools read their limits from the status object: it calls the
  Anthropic API with the OAuth credentials Claude Code has cached, and keeps
  the answer in its own cache. In the scratch configuration directory there are
  no credentials, so the measured runs make no network call. In real use a
  cache miss adds a round trip to the API that no amount of local speed can
  hide.
- **best-claude-hud** shares ccline's configuration format and segment set. It
  renders the 5-hour window, and no weekly window.
- **claude-code-statusline-pro** renders model, directory, git, a context bar,
  cost and both limit windows, and keeps per-session state of its own under the
  Claude configuration directory.
- **ccusage statusline** is the statusline mode of the ccusage cost tracker. It
  reports session, daily and block costs and a block timer rather than the
  limit percentages, computing them from the transcripts, and keeps its own
  cache. A run that refreshes that cache costs several times a run that hits
  it, which is why its slowest run sits far from its median in the results
  file, while the other tools stay within a narrow band.
- **ccstatusline** renders a configurable widget line and supports both limit
  windows. It ships as an npm package that runs on the Bun runtime.
- **claude-powerline** renders a powerline-styled bar with both limit windows,
  cost and context, computed from the transcripts, on Node.js.

## Reproducing the measurement

The harness needs `go`, `npm`, `gh` (authenticated), `/usr/bin/time`, `tar`,
`xz` and `git`. It refuses to start when one of them is missing, and says which
one.

```bash
# print what would be installed and measured, and change nothing
scripts/compare-statuslines.sh --dry-run

# install the tools into a scratch directory, measure, write the results file
scripts/compare-statuslines.sh --run

# more timed runs per tool
scripts/compare-statuslines.sh --run --runs 200
```

`--run` creates a scratch directory with `mktemp -d` and does everything
inside it: it builds howmuchleft from the checkout, downloads the pinned
release asset for each compiled tool with `gh release download`, installs the
npm packages under the scratch directory with `npm install --prefix` (never
globally), and writes the synthetic status object, a synthetic transcript and
the configuration files that cship, ccline and best-claude-hud need. It then
runs each tool a few times to warm the caches, times the timed runs with
`date +%s%N`, measures peak resident memory with `/usr/bin/time -f '%M'`, and
puts `/bin/true` through the same loop to measure what starting a process costs
on the machine. The one file it writes outside the scratch directory is the
results file. The scratch directory is left in place, and its path is printed.

Reading the results file: `[run]` records the date, the machine, the CPU, the
process spawn floor and the howmuchleft commit that was built. Each `[[tool]]`
records the tool's version and where it came from, what it can show, and the
median, mean, minimum and maximum of its timed runs together with its peak
resident memory. The table on this page shows a subset; the file has the rest.

Adding a tool means adding one record to the `TOOLS` array in the harness,
which declares its identity and what it can show, one download or `npm install`
line next to the others, and one entry in the `command_for` function saying how
it is invoked. The results file and this page's table then pick it up on the
next run.

## The honest summary

The compiled tools are all within a few milliseconds of each other, and a good
part of every one of those numbers is the cost of starting a process at all,
which no statusline can avoid: compare any of them against the `/bin/true`
floor in the results file. howmuchleft is the quickest of them in this run, and
the reason is not the language it is written in -- the `Language` column says
what the tools behind it are written in. The reason is that these renders start
no child process at all: the branch comes out of the repository's `HEAD` file,
the limit percentages come out of the status object, and the desktop theme
comes out of a cache file. Between compiled statuslines the choice is still
mostly about what they show and where they get it, but the milliseconds are no
longer an argument against showing more.

The gap to the JavaScript tools is a different matter entirely. Those numbers
are one and two orders of magnitude above the compiled group, and their peak
memory is one to two orders above it too, because each render starts a
JavaScript runtime from scratch. That gap is not a tuning difference, and it is
the one a user notices.
