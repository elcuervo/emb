#!/usr/bin/env python3
"""Cut a raw emb-top recording down to the dashboard's alt-screen session.

`asciinema record` captures the whole session, and for a Bubble Tea program that
includes two things the plate must never show:

1. **The startup wait.** Bubble Tea's package init calls
   `lipgloss.HasDarkBackground()`, which asks the terminal for its background
   colour and waits out `termenv.OSCTimeout` — a hard 5 seconds — when nothing
   answers. A recording pty never answers, while a real terminal replies in
   microseconds, so every take begins with five seconds of blank screen.
2. **The exit sequence.** The take is ended by a watchdog (a scripted recording
   cannot press `q`), and quitting restores the screen the dashboard took over.

This keeps the events between the first alternate-screen entry and the first
exit after it, and rebases their timestamps so the first painted frame is t=0.
The result is the take: agg's job is then only to theme, speed and hold it.

Timing is asciicast v3's, which is *relative*: an event's time is the interval
since the previous one. So the events are accumulated to absolute offsets to
find the window, then re-deltaed on the way out — reading v3 times as if they
were v2's absolute ones yields a take that starts in the past.

    python3 trim.py raw.cast take.cast

Exits 1 when the raw recording contains no alternate-screen session at all,
which means nothing was captured or the dashboard never started — a failure the
rig must not paper over with an empty animation.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

ENTER_ALT_SCREEN = "\x1b[?1049h"
EXIT_ALT_SCREEN = "\x1b[?1049l"


def read_cast(path: Path) -> tuple[dict, list[list]]:
    with path.open(encoding="utf-8") as fh:
        header = json.loads(fh.readline())
        events = [json.loads(line) for line in fh if line.strip()]
    return header, events


def absolute(events: list[list]) -> list[list]:
    """v3 event times are deltas; return events carrying absolute offsets."""
    elapsed = 0.0
    out = []
    for event in events:
        elapsed += event[0]
        out.append([elapsed, *event[1:]])
    return out


def trim(
    header: dict, events: list[list], until_pct: float | None = None
) -> list[list]:
    events = absolute(events)
    start = next(
        (
            i
            for i, event in enumerate(events)
            if len(event) > 2 and ENTER_ALT_SCREEN in event[2]
        ),
        None,
    )
    if start is None:
        raise SystemExit(
            "trim: no alternate-screen session in the recording — nothing was captured"
        )

    end = next(
        (
            i
            for i, event in enumerate(events[start:], start)
            if len(event) > 2 and EXIT_ALT_SCREEN in event[2]
        ),
        len(events),
    )

    keep = events[start:end]
    base = keep[0][0]
    rebased = [[event[0] - base, *event[1:]] for event in keep]

    # A frame for the page is taken from the middle of the run, not its end: the
    # last frame of a take is the drain, where every rate reads zero. `--until-pct`
    # cuts the take at a fraction of its own length, and the cast is then replayed
    # to that moment to dump the screen as text (see the rig's README).
    if until_pct is not None:
        horizon = rebased[-1][0] * until_pct / 100
        rebased = [event for event in rebased if event[0] <= horizon]
        if not rebased:
            raise SystemExit(f"trim: --until-pct {until_pct} kept no events")

    # Back to v3's relative timing.
    out = []
    previous = 0.0
    for event in rebased:
        out.append([round(event[0] - previous, 6), *event[1:]])
        previous = event[0]
    return out


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("raw", type=Path, help="recording as asciinema left it")
    parser.add_argument("take", type=Path, help="trimmed recording to write")
    parser.add_argument(
        "--until-pct",
        type=float,
        default=None,
        metavar="PERCENT",
        help="keep only the take's first PERCENT percent (a frame for the page)",
    )
    args = parser.parse_args()

    if args.until_pct is not None and not 0 < args.until_pct <= 100:
        sys.exit("trim: --until-pct must be above 0 and at most 100")

    if not args.raw.exists():
        sys.exit(f"trim: no recording at {args.raw}")

    header, events = read_cast(args.raw)
    kept = trim(header, events, args.until_pct)

    with args.take.open("w", encoding="utf-8") as fh:
        fh.write(json.dumps(header) + "\n")
        for event in kept:
            fh.write(json.dumps(event) + "\n")

    duration = sum(event[0] for event in kept)
    note = (
        f"cut to {args.until_pct:g}%"
        if args.until_pct is not None
        else "from the first painted frame"
    )
    print(f"trim: {len(events)} -> {len(kept)} events {note} ({duration:.1f}s) -> {args.take}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
