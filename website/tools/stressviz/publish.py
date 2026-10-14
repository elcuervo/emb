#!/usr/bin/env python3
"""Publish one `emb-stress` run: the documentation surface's measured figure,
the documentation's capture, and the run's record in `BENCHMARK.md`.

One run produces three things, and this writes all of them from that run's own
output, so no number on a surface is typed into it:

* **The documentation surface's figure** (`website/docs/index.html`) is a
  drawing — a `.plate`, from the atoms that surface already uses — of the level
  curve, each endpoint's offered-versus-served share, and the sizing lines
  `emb-top` rendered while the run was driving the fleet.
* **`docs/operations.md`** gets the run's own terminal output rendered to a GIF
  by `agg`: that surface cannot replay a recording, and a picture of a report is
  what a reader there can read.
* **`BENCHMARK.md`** gets the run's record — machine, fleet, method, invocation
  and every value the figure draws.

Every input is read out of the run: `--report` is the run's stdout (the report
`emb-stress` prints) and `--frames` is the `emb-top -frames` sidecar's JSON
lines, which is where the sizing lines come from. Both are kept under the rig's
`runs/` beside the recording. The publisher refuses to write anything when the
run does not look like the scenario's — fewer than two endpoints, an endpoint
that completed no work, a pair reported under `NOT EXERCISED`, a fleet line that
is not the sum of the endpoints, a marker missing from a surface. Failing before
writing is the point: a figure beside a run that did not happen is the defect
the rig exists to prevent.

    python3 publish.py --report runs/stress.txt --frames runs/frames.jsonl \\
        --capture /tmp/stressviz.gif --commit 9334f4f \\
        --machine "Darwin 25.6.0 arm64, 10 CPUs"
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from dataclasses import dataclass, field
from datetime import date
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[3]
PAGE = REPO_ROOT / "website" / "docs" / "index.html"
OPERATIONS = REPO_ROOT / "docs" / "operations.md"
BENCHMARK = REPO_ROOT / "BENCHMARK.md"
DOC_ASSETS = REPO_ROOT / "docs" / "assets"
CONFIG = REPO_ROOT / "website" / "tools" / "topviz" / "models.yaml"

# The region each surface carries for this figure. The body is generated
# wholesale: the drawing, the caption and the figures are one thing, and a
# marker pair is what makes a re-run replace all of it.
BEGIN = "<!-- stressviz:begin -->"
END = "<!-- stressviz:end -->"
CAPTURE_PREFIX = "stressviz-"

SIZING_LABEL = "SIZING"
# The frame's own escapes, which are not the dashboard's text.
ANSI = re.compile(
    r"\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[()][A-Z0-9]"
)

HEADER = re.compile(
    r"^emb-stress (?P<version>\S+) · (?P<target>.+?) → (?P<endpoints>.+?) · "
    r"(?P<models>\d+) model\(s\) · seed (?P<seed>-?\d+) · dwell (?P<dwell>\S+) "
    r"\(warm-up discarded\) · (?P<texts>.+)$"
)
LEVEL = re.compile(
    r"^  level (?P<n>\d+) · workers (?P<workers>\d+) · offered (?P<offered>[\d.]+)/s · "
    r"completed (?P<completed>[\d.]+)/s · p50 (?P<p50>\S+) · p95 (?P<p95>\S+) · "
    r"p99 (?P<p99>\S+) · (?P<errors>\d+) errors$"
)
LATENCY = re.compile(r"^(?P<value>[\d.]+)(?P<unit>ns|µs|us|ms|s)$")
LATENCY_US = {"ns": 1e-3, "µs": 1.0, "us": 1.0, "ms": 1e3, "s": 1e6}
MODEL_ENTRY = re.compile(r"^  (?P<name>[A-Za-z0-9_-]+):")


def escape(text: str) -> str:
    return text.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


@dataclass
class Level:
    """One level's numbers, as the run printed them."""

    workers: int
    offered: float
    completed: float
    p50: str
    p95: str
    p99: str
    errors: int

    def p95_ms(self) -> float:
        found = LATENCY.match(self.p95)
        if not found:
            raise SystemExit(f"publish: cannot read p95 {self.p95!r} as a latency")
        return float(found.group("value")) * LATENCY_US[found.group("unit")] / 1000


@dataclass
class Run:
    """One run's report: the fleet's levels, each endpoint's, and the drift."""

    version: str
    target: str
    endpoints_reported: str
    models: int
    seed: int
    dwell: str
    texts: str
    fleet: list[Level] = field(default_factory=list)
    endpoints: dict[str, list[Level]] = field(default_factory=dict)
    drift: list[str] = field(default_factory=list)

    def levels(self) -> list[int]:
        """The levels the run was given: the per-endpoint worker count.

        The FLEET section's own `workers` is the fleet's *total* concurrency —
        `mergeLevels` multiplies the per-endpoint count by the endpoint count —
        so the requested level is what each endpoint section reports, and every
        endpoint gets the same one, which is also what makes the offered share
        `1/N`.
        """
        first = next(iter(self.endpoints.values()), None)
        if not first:
            raise SystemExit("publish: the report carries no endpoint section")
        return [level.workers for level in first]

    def level_csv(self) -> str:
        return ",".join(str(level) for level in self.levels())


def parse_report(text: str) -> Run:
    """Read the report `emb-stress` prints into the values the figure draws.

    The report is the tool's own prose; a line it does not print is a run this
    publisher does not understand, and it says so rather than drawing the part
    it recognised. The report is not necessarily the first thing in the file:
    the recorder keeps the run's stderr beside its stdout, and the run's own
    progress line comes first.
    """
    lines = text.splitlines()
    found = next(
        ((i, HEADER.match(line)) for i, line in enumerate(lines) if HEADER.match(line)),
        None,
    )
    if not found:
        first = lines[0] if lines else ""
        raise SystemExit(
            f"publish: no emb-stress report header in the run's output (first line: {first!r})"
        )
    start, header = found
    run = Run(
        version=header.group("version"),
        target=header.group("target"),
        endpoints_reported=header.group("endpoints"),
        models=int(header.group("models")),
        seed=int(header.group("seed")),
        dwell=header.group("dwell"),
        texts=header.group("texts"),
    )

    section: list[Level] | None = None
    in_drift = False
    for i in range(start + 1, len(lines)):
        line = lines[i]
        if not line.strip():
            continue
        if line == "FLEET":
            section, in_drift = run.fleet, False
            continue
        if line == "NOT EXERCISED":
            section, in_drift = None, True
            continue
        level = LEVEL.match(line)
        if level:
            if section is None:
                raise SystemExit(f"publish: a level line belongs to no section: {line!r}")
            section.append(
                Level(
                    workers=int(level.group("workers")),
                    offered=float(level.group("offered")),
                    completed=float(level.group("completed")),
                    p50=level.group("p50"),
                    p95=level.group("p95"),
                    p99=level.group("p99"),
                    errors=int(level.group("errors")),
                )
            )
            continue
        if line.startswith("  "):
            if in_drift:
                run.drift.append(line.strip())
                continue
            raise SystemExit(f"publish: unrecognised line in the report: {line!r}")
        # An unindented line starts an endpoint section only when a level line
        # follows it: the report also carries a fixed line of prose about its
        # own units, and a line that stands in front of nothing is that, not an
        # endpoint.
        if i + 1 < len(lines) and LEVEL.match(lines[i + 1]):
            section, in_drift = run.endpoints.setdefault(line.strip(), []), False

    return run


def sizing_lines(frames: Path) -> list[tuple[int, str]]:
    """The distinct sizing lines `emb-top` rendered, in first-seen order.

    One JSON object per poll, each carrying the dashboard's rendered frame. The
    line is the advisory the run pushed the fleet into, quoted as it appeared
    (minus the label the figure draws itself) and stamped with the poll it first
    appeared on.
    """
    seen: dict[str, int] = {}
    for poll, raw in enumerate(frames.read_text(encoding="utf-8").splitlines()):
        try:
            message = json.loads(raw)
        except json.JSONDecodeError:
            continue
        for line in ANSI.sub("", message.get("ansi", "")).splitlines():
            text = re.sub(r"\s+", " ", line).strip()
            if not text.startswith(SIZING_LABEL + " "):
                continue
            text = text[len(SIZING_LABEL) :].strip()
            if text and text not in seen:
                seen[text] = poll
    return [(poll, text) for text, poll in seen.items()]


def models(config: Path) -> list[str]:
    """The models the fleet was started with, from the configuration itself."""
    names = [
        found.group("name")
        for line in config.read_text(encoding="utf-8").splitlines()
        if (found := MODEL_ENTRY.match(line))
    ]
    if not names:
        raise SystemExit(f"publish: no models declared in {config}")
    return names


def shares(run: Run) -> list[tuple[str, float, float]]:
    """Each endpoint's offered and served share of the fleet.

    *Offered* is the share the run divided among the endpoints: every level
    fixes the **same** worker count per endpoint, so each endpoint's offered
    share is `1/N` by construction — that even split is what makes the two
    comparable at all. *Served* is what each endpoint completed as a share of
    the fleet's completed work, which is *not* `1/N` when one node is slower
    than its peers. The gap between the two is what the figure is for.

    The fleet's own line is the independent check: it sums what the nodes
    served, and a report where it does not is not one run.
    """
    completed = sum(level.completed for level in run.fleet)
    if completed <= 0:
        raise SystemExit("publish: the fleet completed nothing")
    served = sum(level.completed for levels in run.endpoints.values() for level in levels)
    if abs(served - completed) > completed * 0.005:
        raise SystemExit(
            f"publish: the fleet line ({completed:.1f}/s) is not the sum of the "
            f"endpoints ({served:.1f}/s)"
        )

    even = 100 / len(run.endpoints)
    return [
        (label, even, sum(level.completed for level in levels) / completed * 100)
        for label, levels in run.endpoints.items()
    ]


# ---- the drawing ----
#
# The surface's existing figure atoms and nothing else: the ruled graticule
# (`plate__grid`), the axis rule (`plate__axis`), the dashed reference
# (`plate__cap-line`), the faint ink area (`plate__area`), mono labels
# (`plate__label`, `--strong`), and exactly one accent signal
# (`plate__signal` + `plate__point--accent`) on the measured throughput. The
# shares are drawn in ink: their message is the distance between the dashed
# offered mark and the served bar beside it.

VB_W = 720
TOP, BASE = 34, 150
AX0, AX1 = 64, 356
BX0, BX1 = 400, 692
TRACK0, TRACK1 = 240, 600


def panel(
    levels: list[int], values: list[float], x0: int, x1: int, top: float, title: str
) -> tuple[list[str], list[float], list[float]]:
    """One panel's graticule, axis and level positions."""
    step = (x1 - x0) / max(len(values) - 1, 1)
    xs = [x0 + i * step for i in range(len(values))]
    ys = [BASE - (value / top) * (BASE - TOP) for value in values]

    out = [
        f'<text class="plate__label plate__label--strong" x="{x0}" y="20">{escape(title)}</text>'
    ]
    for tick in (0.0, top / 2, top):
        y = BASE - (tick / top) * (BASE - TOP)
        out.append(f'<line class="plate__grid" x1="{x0}" y1="{y:.1f}" x2="{x1}" y2="{y:.1f}"/>')
        out.append(
            f'<text class="plate__label" x="{x0 - 8}" y="{y + 4:.1f}" '
            f'text-anchor="end">{tick:.0f}</text>'
        )
    out.append(f'<line class="plate__axis" x1="{x0}" y1="{TOP}" x2="{x0}" y2="{BASE}"/>')
    out.append(f'<line class="plate__axis" x1="{x0}" y1="{BASE}" x2="{x1}" y2="{BASE}"/>')
    return out, xs, ys


def drawing(run: Run, rows: list[tuple[str, float, float]], sizing: list[tuple[int, str]]) -> str:
    rates = [level.completed for level in run.fleet]
    p95s = [level.p95_ms() for level in run.fleet]

    out: list[str] = []

    # Throughput: the figure's one accent signal, labelled with the run's own
    # numbers rather than a rounded redrawing of them.
    head, xs, ys = panel(run.levels(), rates, AX0, AX1, max(rates) or 1.0, "completed req/s · fleet")
    out += head
    out.append(
        '<polyline class="plate__signal" points="'
        + " ".join(f"{x:.1f},{y:.1f}" for x, y in zip(xs, ys))
        + '"/>'
    )
    for x, y, level in zip(xs, ys, run.fleet):
        out.append(f'<circle class="plate__point plate__point--accent" cx="{x:.1f}" cy="{y:.1f}" r="3"/>')
        out.append(
            f'<text class="plate__label plate__label--strong" x="{x:.1f}" y="{y - 8:.1f}" '
            f'text-anchor="middle">{level.completed:.1f}</text>'
        )

    # Latency: the faint ink area, beside the curve rather than on a second axis.
    head, pxs, pys = panel(run.levels(), p95s, BX0, BX1, max(p95s) or 1.0, "p95 latency · fleet (ms)")
    out += head
    out.append(
        '<polygon class="plate__area" points="'
        + f"{BX0},{BASE} "
        + " ".join(f"{x:.1f},{y:.1f}" for x, y in zip(pxs, pys))
        + f" {BX1},{BASE}/>"
    )
    for x, y, level in zip(pxs, pys, run.fleet):
        out.append(f'<circle class="plate__point" cx="{x:.1f}" cy="{y:.1f}" r="2.5"/>')
        out.append(
            f'<text class="plate__label plate__label--strong" x="{x:.1f}" y="{y - 8:.1f}" '
            f'text-anchor="middle">{escape(level.p95)}</text>'
        )

    for x, workers in zip(xs, run.levels()):
        out.append(
            f'<text class="plate__label" x="{x:.1f}" y="{BASE + 16}" '
            f'text-anchor="middle">{workers}</text>'
        )
    out.append(
        f'<text class="plate__label" x="{AX0}" y="{BASE + 34}">levels · workers per endpoint</text>'
    )

    # Offered versus served, per endpoint.
    shares_y = BASE + 60
    out.append(
        f'<text class="plate__label plate__label--strong" x="{AX0}" y="{shares_y}">'
        "share of the fleet · served, beside the offered mark</text>"
    )
    row_y = shares_y + 26
    for i, (label, offered, served) in enumerate(rows):
        y = row_y + i * 40
        out.append(
            f'<text class="plate__label plate__label--strong" x="{AX0}" y="{y + 4}">'
            f"{escape(label)}</text>"
        )
        out.append(f'<line class="plate__grid" x1="{TRACK0}" y1="{y}" x2="{TRACK1}" y2="{y}"/>')
        out.append(
            f'<rect class="plate__area" x="{TRACK0}" y="{y - 9}" '
            f'width="{(TRACK1 - TRACK0) * served / 100:.1f}" height="18"/>'
        )
        mark = TRACK0 + (TRACK1 - TRACK0) * offered / 100
        out.append(
            f'<line class="plate__cap-line" x1="{mark:.1f}" y1="{y - 15}" '
            f'x2="{mark:.1f}" y2="{y + 15}"/>'
        )
        out.append(
            f'<text class="plate__label" x="{TRACK1 + 10}" y="{y + 4}">'
            f"{offered:.0f}% → {served:.0f}%</text>"
        )

    # The fleet's own advisory, as it rendered during the run.
    sizing_y = row_y + len(rows) * 40 + 10
    out.append(
        f'<text class="plate__label plate__label--strong" x="{AX0}" y="{sizing_y}">'
        "emb-top's sizing lines during the run</text>"
    )
    if not sizing:
        out.append(
            f'<text class="plate__label" x="{AX0}" y="{sizing_y + 20}">'
            "none · nothing was near a threshold</text>"
        )
        drawn = 1
    else:
        for i, (poll, text) in enumerate(sizing):
            y = sizing_y + 20 + i * 18
            out.append(f'<text class="plate__label" x="{AX0}" y="{y}">poll {poll}</text>')
            out.append(
                f'<text class="plate__label plate__label--strong" x="{AX0 + 66}" y="{y}">'
                f"{escape(text)}</text>"
            )
        drawn = len(sizing)

    height = sizing_y + 20 + drawn * 18 + 10
    return (
        f'<svg class="plate__svg" viewBox="0 0 {VB_W} {height:.0f}" role="img" '
        f'aria-labelledby="stressviz-t">\n'
        f'          <title id="stressviz-t">{escape(title_text(run, rows, sizing))}</title>\n'
        "          " + "\n          ".join(out) + "\n        </svg>"
    )


# ---- what the figure says, in words ----
#
# The caption states every value the drawing presents, so the surface's claims
# survive the drawing being removed (an SVG that will not render, a reader with
# images off). It is generated from the same values as the drawing, so the two
# cannot drift.


def title_text(run: Run, rows: list[tuple[str, float, float]], sizing: list[tuple[int, str]]) -> str:
    levels = ", ".join(str(level) for level in run.levels())
    served = "; ".join(f"{label} served {observed:.0f}% of the fleet" for label, _, observed in rows)
    return (
        f"One emb-stress run against a {len(rows)}-node fleet: the fleet's completed "
        f"requests per second and p95 latency at levels {levels}, each endpoint's "
        f"served share beside its offered share ({served}), and the sizing lines "
        f"emb-top rendered during the run ("
        + (f"{len(sizing)} line(s)" if sizing else "none")
        + ")"
    )


def fleet_size(run: Run) -> str:
    """The fleet size, in prose: the run prints `2 endpoint(s)`."""
    return run.endpoints_reported.replace("endpoint(s)", "endpoints")


def run_label(run: Run, commit: str, day: str) -> str:
    """The one-line provenance each surface's caption carries."""
    parts = [day, f"emb-stress v{run.version}"]
    if commit:
        parts.append(commit)
    parts += [fleet_size(run), f"{run.models} models", run.target]
    return " · ".join(parts)


def invocation(run: Run, warmup: str) -> str:
    return (
        f"./bin/emb-stress -config website/tools/topviz/models.yaml -target {run.target} "
        f"-levels {run.level_csv()} -dwell {run.dwell} -warmup {warmup} -seed {run.seed}"
    )


def caption(
    run: Run, rows: list[tuple[str, float, float]], sizing: list[tuple[int, str]],
    commit: str, day: str, warmup: str,
) -> str:
    per_level = "; ".join(
        f"level {requested} {level.completed:.1f} completed req/s, p50 {level.p50}, "
        f"p95 {level.p95}, p99 {level.p99}, {level.errors} errors"
        for requested, level in zip(run.levels(), run.fleet)
    )
    served = "; ".join(
        f"{label} offered {offered:.0f}%, served {observed:.0f}%"
        for label, offered, observed in rows
    )
    if sizing:
        sizing_text = "; ".join(f"poll {poll}: {text}" for poll, text in sizing)
    else:
        sizing_text = (
            "none — the recommendation is reported only when something is near a "
            "threshold"
        )
    return (
        f"<strong>Drawn from the run's own output</strong> — one "
        f"<code>emb-stress</code> run against {fleet_size(run)} "
        f"({', '.join(row[0] for row in rows)}) of {run.models} models, "
        f"seed {run.seed}, {run.dwell} held per level with the leading {warmup} "
        f"discarded, watched by <code>emb-top -node {run.target} -frames</code>. "
        f"Fleet: {per_level}. Shares: {served}. "
        f"<code>emb-top</code>'s sizing lines during the run: {sizing_text}. "
        f"<span data-stressviz-run>{run_label(run, commit, day)}</span>. "
        f"Reproduce with <code>just website-stressviz</code> — "
        f"<code>{invocation(run, warmup)}</code>."
    )


def figure(
    run: Run, rows: list[tuple[str, float, float]], sizing: list[tuple[int, str]],
    commit: str, day: str, warmup: str,
) -> str:
    return (
        '      <figure class="plate" id="plate-stressviz">\n'
        '        <div class="plate__wrap">\n'
        f"        {drawing(run, rows, sizing)}\n"
        "        </div>\n"
        f'        <figcaption class="plate__cap">'
        f"{caption(run, rows, sizing, commit, day, warmup)}</figcaption>\n"
        '        <div class="plate__legend">\n'
        '          <span><i class="plate__swatch--area" aria-hidden="true"></i>'
        "<b>served</b> — the endpoint's completed share of the fleet</span>\n"
        '          <span><i class="plate__swatch--cap" aria-hidden="true"></i>'
        "<b>offered</b> — its even share of the run's workers</span>\n"
        "        </div>\n"
        "      </figure>"
    )


# ---- the two markdown surfaces ----

def operations_body(
    run: Run, rows: list[tuple[str, float, float]], sizing: list[tuple[int, str]],
    capture: str, commit: str, day: str, warmup: str,
) -> str:
    if sizing:
        sizing_text = "the fleet's own sizing advice during the run: " + "; ".join(
            f"`{text}`" for _, text in sizing
        )
    else:
        sizing_text = (
            "no sizing line — the recommendation is reported only when something is "
            "near a threshold"
        )
    numbers = "; ".join(
        f"level {requested} {level.completed:.1f} req/s · p95 {level.p95}"
        for requested, level in zip(run.levels(), run.fleet)
    )
    served = "; ".join(
        f"{label} offered {offered:.0f}%, served {observed:.0f}%"
        for label, offered, observed in rows
    )
    return (
        f"![The emb-stress report of a {len(rows)}-node fleet: the fleet and per-endpoint "
        f"sections, one level line each with offered, completed, p50/p95/p99 and errors]"
        f"(assets/{capture})\n"
        f"\n"
        f"*Measured run · {run_label(run, commit, day)}. `emb-stress` offered the load to "
        f"{fleet_size(run)} at levels {run.level_csv()} ({run.dwell} per level, leading "
        f"{warmup} discarded, seed {run.seed}) and reported per level and per endpoint — "
        f"{numbers} — while `emb-top` observed the same fleet: {served}; {sizing_text}. "
        f"Re-record with `just website-stressviz`: `{invocation(run, warmup)}`.*"
    )


def benchmark_body(
    run: Run, rows: list[tuple[str, float, float]], sizing: list[tuple[int, str]],
    models_list: list[str], capture: str, commit: str, machine: str, day: str, warmup: str,
) -> str:
    per_level = "\n".join(
        f"| {requested} | {level.offered:.1f} | {level.completed:.1f} | "
        f"{level.p50} | {level.p95} | {level.p99} | {level.errors} |"
        for requested, level in zip(run.levels(), run.fleet)
    )
    endpoints = "\n".join(
        f"\n**{label}** — " + "; ".join(
            f"level {requested} {level.completed:.1f} req/s, p95 {level.p95}"
            for requested, level in zip(run.levels(), run.endpoints[label])
        )
        for label, _, _ in rows
    )
    served = "; ".join(
        f"{label} offered {offered:.0f}%, served {observed:.0f}%"
        for label, offered, observed in rows
    )
    if sizing:
        sizing_text = "\n".join(f"- poll {poll}: `{text}`" for poll, text in sizing)
    else:
        sizing_text = (
            "- none — the sizing line is advisory and appears only when a rule fires"
        )
    return (
        f"**Machine.** {machine}, recorded {day}.\n"
        f"\n"
        f"**Fleet.** {fleet_size(run)} from `website/tools/topviz/models.yaml` "
        f"({', '.join(models_list)}), both loopback, one per address family of "
        f"`{run.target}`, each preloaded with `cache: 256MB`.\n"
        f"\n"
        f"**Method.** One `emb-stress` run: closed-loop levels, {run.dwell} held per level "
        f"with the leading {warmup} discarded, seed {run.seed}, {run.texts}. An "
        f"`emb-top -node {run.target} -frames -interval 1s` sidecar watched the same fleet "
        f"for the run's duration; the sizing lines below are its own.\n"
        f"\n"
        f"**Invocation.** `{invocation(run, warmup)}` (`just website-stressviz`), recorded "
        f"{commit or 'uncommitted'}.\n"
        f"\n"
        f"**Fleet, per level** — the table the figure draws:\n"
        f"\n"
        f"| level (workers per endpoint) | offered req/s | completed req/s | p50 | p95 | p99 | errors |\n"
        f"|---|---|---|---|---|---|---|\n"
        f"{per_level}\n"
        f"{endpoints}\n"
        f"\n"
        f"**Shares.** {served}. The offered share is the run's own split (the endpoints "
        f"are divided evenly, which is the point of the comparison); the served share is "
        f"each endpoint's completed work of the fleet's.\n"
        f"\n"
        f"**`emb-top`'s sizing lines during the run.**\n"
        f"{sizing_text}\n"
        f"\n"
        f"**Capture.** `docs/assets/{capture}` — the run's own output, rendered."
    )


def stamp_region(text: str, body: str, where: Path) -> str:
    """Replace everything between the region's markers, markers themselves kept."""
    pattern = re.compile(re.escape(BEGIN) + r".*?" + re.escape(END), re.DOTALL)
    if not pattern.search(text):
        raise SystemExit(
            f"publish: no {BEGIN} … {END} region in {where.relative_to(REPO_ROOT)}"
        )
    return pattern.sub(f"{BEGIN}\n{body}\n{END}", text, count=1)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--report", type=Path, required=True, help="the run's own stdout")
    parser.add_argument("--frames", type=Path, required=True, help="emb-top -frames JSON lines")
    parser.add_argument("--capture", type=Path, required=True, help="the run's rendered output")
    parser.add_argument("--machine", required=True, help="host the run was recorded on")
    parser.add_argument("--commit", default="", help="commit the run was recorded at")
    parser.add_argument("--date", default=date.today().isoformat())
    parser.add_argument(
        "--warmup",
        required=True,
        help="the warm-up the run was given: the report prints the dwell but not this",
    )
    parser.add_argument("--config", type=Path, default=CONFIG, help="the fleet's configuration")
    args = parser.parse_args()

    for path in (args.report, args.frames, args.capture, args.config):
        if not path.is_file() or path.stat().st_size == 0:
            raise SystemExit(f"publish: missing or empty input {path}")

    # The previous capture is deleted before the new one is written, so naming
    # the published file as this run's input deletes it and then fails to read
    # it — a mistake worth naming rather than half-doing.
    if args.capture.resolve().parent == DOC_ASSETS.resolve():
        raise SystemExit(f"publish: {args.capture} is a published artifact, not a run's input")

    run = parse_report(args.report.read_text(encoding="utf-8"))
    sizing = sizing_lines(args.frames)
    rows = shares(run)
    fleet_models = models(args.config)

    # The scenario starts two nodes, one per loopback family, and the run has to
    # show them both: a name that resolved to one address would make the whole
    # offered-versus-served comparison a single row.
    if len(rows) < 2:
        raise SystemExit(
            f"publish: the run reports {len(rows)} endpoint(s) — the scenario starts two "
            f"({run.endpoints_reported} from {run.target!r}); check that the name resolves "
            f"to both address families on this host"
        )
    if run.drift:
        raise SystemExit(
            "publish: the run left pairs unexercised — " + "; ".join(run.drift)
        )
    for label_, levels in run.endpoints.items():
        if len(levels) != len(run.fleet):
            raise SystemExit(
                f"publish: {label_} reports {len(levels)} level(s) against the fleet's "
                f"{len(run.fleet)} — the report is not one run's"
            )
        idle = [level.workers for level in levels if level.completed <= 0]
        if idle:
            raise SystemExit(
                f"publish: {label_} completed no work at level(s) {idle} — the run did not "
                f"exercise it"
            )
    if run.models != len(fleet_models):
        raise SystemExit(
            f"publish: the run exercised {run.models} model(s), but {args.config} declares "
            f"{len(fleet_models)}"
        )

    capture_name = f"{CAPTURE_PREFIX}{hashlib.sha256(args.capture.read_bytes()).hexdigest()[:8]}.gif"
    page = stamp_region(
        PAGE.read_text(encoding="utf-8"),
        figure(run, rows, sizing, args.commit, args.date, args.warmup),
        PAGE,
    )
    operations = stamp_region(
        OPERATIONS.read_text(encoding="utf-8"),
        operations_body(
            run, rows, sizing, capture_name, args.commit, args.date, args.warmup
        ),
        OPERATIONS,
    )
    benchmark = stamp_region(
        BENCHMARK.read_text(encoding="utf-8"),
        benchmark_body(
            run, rows, sizing, fleet_models, capture_name, args.commit, args.machine,
            args.date, args.warmup,
        ),
        BENCHMARK,
    )

    # Everything is stamped before anything is written, so a missing marker
    # fails the run rather than leaving a plate beside half a record.
    DOC_ASSETS.mkdir(parents=True, exist_ok=True)
    for previous in DOC_ASSETS.glob(f"{CAPTURE_PREFIX}*.gif"):
        previous.unlink()
    (DOC_ASSETS / capture_name).write_bytes(args.capture.read_bytes())
    PAGE.write_text(page, encoding="utf-8")
    OPERATIONS.write_text(operations, encoding="utf-8")
    BENCHMARK.write_text(benchmark, encoding="utf-8")

    print(f"publish: docs/assets/{capture_name} ({args.capture.stat().st_size / 1024:.0f} KiB)")
    print(f"publish: the figure into website/docs/index.html")
    print(
        f"publish: docs/operations.md and BENCHMARK.md record "
        f"{run_label(run, args.commit, args.date)}"
    )
    print(f"publish: {len(sizing)} sizing line(s) from the frames; {run.texts}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
