"""selfdoc custom directive: render the statusline comparison table.

Registered in selfdoc.json as ``statusline-comparison``. selfdoc runs this file
through its Python driver and calls ``resolve(attrs, config, body) -> str``; the
returned markdown replaces the ``:-: statusline-comparison`` directive line.

The measurements come from ``.stricttools/docs/statusline-comparison.toml``,
written by
``scripts/compare-statuslines.sh --run``. That file is the single source of
every number on the comparison page, so no measurement is ever typed by hand
into prose.

Any error propagates: selfdoc turns it into a build error naming the directive
and this script, instead of publishing a page with a hole where the table goes.
"""

from __future__ import annotations

import os
import tomllib
from typing import Any

DEFAULT_RESULTS = ".stricttools/docs/statusline-comparison.toml"

# The project whose page this is; its row says so.
THIS_PROJECT = "howmuchleft"

REQUIRED_TOOL_KEYS = (
    "name",
    "version",
    "language",
    "source",
    "shows_five_hour",
    "shows_weekly",
    "reads_transcripts",
    "median_ms",
    "mean_ms",
    "min_ms",
    "max_ms",
    "peak_mb",
    "runs",
)
REQUIRED_RUN_KEYS = ("date", "machine", "cpu", "floor_ms", "howmuchleft_commit")

# How a declared capability reads in the table.
LIMIT_LABELS = {"yes": "yes", "no": "no", "api": "from the API"}


def _repo_root() -> str:
    """Return the repository root: the nearest ancestor holding selfdoc.json.

    Found by marker rather than by a parent count, so this resolves correctly
    wherever this script sits inside the repository.
    """
    directory = os.path.dirname(os.path.abspath(__file__))
    while True:
        if os.path.isfile(os.path.join(directory, "selfdoc.json")):
            return directory
        parent = os.path.dirname(directory)
        if parent == directory:
            raise RuntimeError(f"no selfdoc.json above {__file__}")
        directory = parent


def _load(path: str) -> dict[str, Any]:
    with open(path, "rb") as handle:
        return tomllib.load(handle)


def _limit(value: str, field: str, tool: str) -> str:
    if value not in LIMIT_LABELS:
        raise ValueError(
            f"{tool}: {field} is {value!r}, expected one of {sorted(LIMIT_LABELS)}"
        )
    return LIMIT_LABELS[value]


def _transcript_free(value: str, tool: str) -> str:
    if value not in ("yes", "no"):
        raise ValueError(f"{tool}: reads_transcripts is {value!r}, expected yes or no")
    return "no" if value == "yes" else "yes"


def resolve(attrs: dict[str, str], config: dict[str, Any], body: list[str]) -> str:
    """Render the measured comparison as a markdown table plus a run line.

    ``attrs`` may carry ``path`` to point at a different results file; ``config``
    and ``body`` go unused. Rows are sorted fastest first, so this project leads
    the table only when it measured fastest.
    """
    path = attrs.get("path", DEFAULT_RESULTS)
    if not os.path.isabs(path):
        path = os.path.join(_repo_root(), path)
    data = _load(path)

    run = data.get("run")
    if not run:
        raise ValueError(f"{path} has no [run] table")
    for key in REQUIRED_RUN_KEYS:
        if key not in run:
            raise ValueError(f"{path}: [run] is missing {key}")

    tools = data.get("tool")
    if not tools:
        raise ValueError(f"{path} has no [[tool]] tables")
    for tool in tools:
        for key in REQUIRED_TOOL_KEYS:
            if key not in tool:
                raise ValueError(
                    f"{path}: tool {tool.get('name', '<unnamed>')!r} is missing {key}"
                )

    headers = [
        "Tool",
        "Language",
        "Median ms",
        "Peak MB",
        "5-hour",
        "Weekly",
        "Transcript-free",
    ]
    rows = []
    for tool in sorted(tools, key=lambda t: t["median_ms"]):
        name = tool["name"]
        label = f"**{name}** (this project)" if name == THIS_PROJECT else name
        rows.append(
            [
                label,
                tool["language"],
                f"{tool['median_ms']:.1f}",
                f"{tool['peak_mb']:.1f}",
                _limit(tool["shows_five_hour"], "shows_five_hour", name),
                _limit(tool["shows_weekly"], "shows_weekly", name),
                _transcript_free(tool["reads_transcripts"], name),
            ]
        )

    lines = [
        "| " + " | ".join(headers) + " |",
        "| " + " | ".join("---" for _ in headers) + " |",
    ]
    for row in rows:
        lines.append("| " + " | ".join(cell.replace("|", r"\|") for cell in row) + " |")

    run_counts = sorted({tool["runs"] for tool in tools})
    runs_text = (
        f"{run_counts[0]} timed runs per tool"
        if len(run_counts) == 1
        else "a varying number of timed runs per tool"
    )
    lines.append("")
    lines.append(
        f"Measured on {run['date']} on {run['machine']} ({run['cpu']}), "
        f"{runs_text}, howmuchleft at commit `{run['howmuchleft_commit']}`. "
        f"Spawning `/bin/true` through the same loop medians {run['floor_ms']:.1f} ms, "
        "so that much of every number above is the cost of starting a process at all."
    )
    return "\n".join(lines)
