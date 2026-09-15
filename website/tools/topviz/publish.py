#!/usr/bin/env python3
"""Publish one `emb-top` take into the site: name it, place it, stamp it.

The site has no build step, so a take's asset names and its provenance line are
hand-typed copies of a run that happened elsewhere and drift silently. This makes
them derived instead: the assets are named from their own bytes (so the existing
`/assets/img/*` immutable cache rule stays honest, and `_headers` needs no
exception), the previous take is removed, and the values are written into the one
place the page consumes them.

    python3 publish.py --anim take.webp --poster take.png \\
        --version 0.4.0.pre5 --models 4 --addr 127.0.0.1:16379

What it rewrites, by marker, in `website/index.html`:

    <source ... srcset="assets/img/emb-top-<sha8>.webp" data-topviz-anim>
    <img ... src="assets/img/emb-top-<sha8>.png" data-topviz-poster>
    <span data-topviz-run>…</span>
    <p data-topviz-metrics>…</p>

From the same run's headless sample log it also writes the dashboard's figures as
text, so the plate's numbers are not image-only: the animated capture is
decorative to a screen reader, the sentence is not. The figures come from the
run's busiest poll, and are labelled as that rather than as the whole run.

and the two `emb-top-*` lines in `published-tree.py`'s served set, which is a
frozenset of exact names: a re-record changes them, and the check would fail on
an orphaned take. Both files must agree on the names, so one tool owns both
writes rather than a `sed` in the recipe.

Fails — rather than writing half of it — when a marker is missing, when the
animation is empty, or when the two artifacts disagree on their take.
"""

from __future__ import annotations

import argparse
import hashlib
import re
import sys
from datetime import date
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[3]
SITE = REPO_ROOT / "website"
IMG_DIR = SITE / "assets" / "img"
INDEX = SITE / "index.html"
SERVED_SET = SITE / "tools" / "published-tree.py"

TAKE_PREFIX = "emb-top-"

# One element carrying a marker attribute, with room for the attribute's value.
MARKED_TAG = r"<(?P<tag>[a-zA-Z][\w-]*)[^<>]*\s{marker}(?=[\s>/])[^<>]*>"


def mark(marker: str) -> re.Pattern[str]:
    return re.compile(MARKED_TAG.format(marker=re.escape(marker)))


def set_attribute(tag: str, attribute: str, value: str) -> str:
    """Replace `attribute="…"` inside one tag, which must already carry it."""
    pattern = re.compile(rf'(\b{re.escape(attribute)}\s*=\s*")([^"]*)(")')
    if not pattern.search(tag):
        raise SystemExit(f"publish: marked element has no {attribute}= to set: {tag}")
    return pattern.sub(lambda m: m.group(1) + value + m.group(3), tag, count=1)


def stamp_assets(html: str, anim: str, poster: str) -> str:
    anim_tag = mark("data-topviz-anim")
    poster_tag = mark("data-topviz-poster")
    if not anim_tag.search(html):
        raise SystemExit("publish: no element carries data-topviz-anim in index.html")
    if not poster_tag.search(html):
        raise SystemExit("publish: no element carries data-topviz-poster in index.html")

    html = anim_tag.sub(lambda m: set_attribute(m.group(0), "srcset", anim), html, count=1)
    return poster_tag.sub(lambda m: set_attribute(m.group(0), "src", poster), html, count=1)


def stamp_run(html: str, label: str) -> str:
    """Rewrite the text of the element carrying data-topviz-run."""
    return stamp_text(html, "data-topviz-run", label)


def stamp_text(html: str, marker: str, text: str) -> str:
    marked = re.compile(
        r"(?P<open><(?P<tag>[a-zA-Z][\w-]*)[^<>]*\s" + re.escape(marker) + r"(?=[\s>/])[^<>]*>)"
        r"(?P<inner>[^<>]*)"
        r"(?P<close></(?P=tag)>)"
    )
    if not marked.search(html):
        raise SystemExit(f"publish: no element carries {marker} in index.html")
    return marked.sub(
        lambda m: m.group("open") + text + m.group("close"), html, count=1
    )


# ---- the run's figures, as text ----
#
# One line per poll, e.g.
#   t=… total_requests=3799 req_rate=138.0 lat_p95_us=148300 cache_hit_rate=59.1
#     model:minilm dim=384 reqs=1285 req_rate=43.0 pooled… quant=fp32
# See internal/embtop/once.go and docs/operations.md. Keys carry digits
# (`lat_p95_us`), so the key pattern is not `[a-z_]+` — which silently skips
# them and leaves the sentence below missing its latency.
PAIR = re.compile(r"\b([a-z][a-z0-9_]*)=([^\s]+)")
MODEL = re.compile(r"model:(\S+)((?:\s+[a-z][a-z0-9_]*=\S+)+)")


def ms(us: str) -> str:
    return f"{float(us) / 1000:.1f}ms"


def busiest(samples: Path) -> str:
    """Describe the figures of the run's busiest poll, as one sentence.

    "Busiest" is measured as *models active first*, then requests: the
    aggregate peak lands while the last model is still ramping, so ranking on
    the aggregate alone would caption the plate with a poll in which half the
    node reads zero.
    """
    peak, best = None, (0, -1.0)
    for line in samples.read_text(encoding="utf-8").splitlines():
        found = re.search(r"\breq_rate=([\d.]+)", line)
        if not found:
            continue
        active = sum(
            1
            for _, fields in MODEL.findall(line)
            if float(dict(PAIR.findall(fields))["req_rate"]) > 0
        )
        score = (active, float(found.group(1)))
        if score > best:
            best, peak = score, line
    if peak is None:
        raise SystemExit(f"publish: no req_rate in {samples}")
    # The aggregate fields live before the first per-model group; every key worth
    # reading is repeated inside it, and a dict built from the whole line would
    # quietly take the last model's numbers for the run's.
    values = dict(PAIR.findall(peak.split(" model:", 1)[0]))

    head = (
        f"Busiest poll of the recorded run: {float(values['req_rate']):.0f} requests/s, "
        f"{float(values['tok_rate']) / 1000:.1f}k tokens/s, "
        f"p95 latency {ms(values['lat_p95_us'])}, "
        f"cache hit rate {float(values['cache_hit_rate']):.0f}%, "
        f"CPU {float(values['cpu_pct']):.0f}%, {values['mem_mb']}MB resident."
    )

    models = []
    for name, fields in MODEL.findall(peak):
        model = dict(PAIR.findall(fields))
        models.append(
            f"{name} {float(model['req_rate']):.0f}/s "
            f"(avg {ms(model['avg_latency_us'])}, dim {model['dim']}, "
            f"{model['pooling']}, {model['quant']})"
        )
    return head + (" Per model: " + "; ".join(models) + "." if models else "")


def stamp_served_set(source: str, names: list[str]) -> str:
    """Swap the take's two lines in published-tree.py's served set.

    Tolerant of a first publish, when there is nothing yet to replace: the
    anchor (the asset the take's lines belong beside) is what must be present.
    """
    # The needle is the *path*, not the name: the served set quotes
    # `"assets/img/emb-top-…"`, so a leading quote never matches and every take
    # would pile up instead of being replaced.
    needle = f"assets/img/{TAKE_PREFIX}"
    kept = [
        line for line in source.splitlines(keepends=True) if needle not in line
    ]

    anchor = next(
        (i for i, line in enumerate(kept) if "assets/img/speckle.svg" in line),
        None,
    )
    if anchor is None:
        raise SystemExit(
            "publish: cannot find where the take's served-set lines belong"
        )

    kept[anchor + 1 : anchor + 1] = [f'        "assets/img/{name}",\n' for name in names]
    return "".join(kept)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--anim", type=Path, required=True, help="animated WebP")
    parser.add_argument("--poster", type=Path, required=True, help="still frame PNG")
    parser.add_argument("--samples", type=Path, required=True, help="headless sample log")
    parser.add_argument("--version", required=True, help="captured emb-top version")
    parser.add_argument("--models", required=True, help="model count of the node")
    parser.add_argument("--addr", required=True, help="node the take was recorded against")
    parser.add_argument("--date", default=date.today().isoformat())
    args = parser.parse_args()

    for path in (args.anim, args.poster, args.samples):
        if not path.is_file() or path.stat().st_size == 0:
            sys.exit(f"publish: missing or empty artifact {path}")

    digest = hashlib.sha256(args.anim.read_bytes()).hexdigest()[:8]
    anim_name = f"{TAKE_PREFIX}{digest}.webp"
    poster_name = f"{TAKE_PREFIX}{digest}.png"

    IMG_DIR.mkdir(parents=True, exist_ok=True)
    previous = sorted(IMG_DIR.glob(f"{TAKE_PREFIX}*"))
    for old in previous:
        old.unlink()

    (IMG_DIR / anim_name).write_bytes(args.anim.read_bytes())
    (IMG_DIR / poster_name).write_bytes(args.poster.read_bytes())

    label = f"{args.date} · emb-top v{args.version} · {args.models} models · {args.addr}"
    html = stamp_run(
        stamp_assets(
            INDEX.read_text(encoding="utf-8"),
            f"assets/img/{anim_name}",
            f"assets/img/{poster_name}",
        ),
        label,
    )
    html = stamp_text(html, "data-topviz-metrics", busiest(args.samples))
    INDEX.write_text(html, encoding="utf-8")

    SERVED_SET.write_text(
        stamp_served_set(
            SERVED_SET.read_text(encoding="utf-8"), [anim_name, poster_name]
        ),
        encoding="utf-8",
    )

    for name in (anim_name, poster_name):
        if name not in html and name not in SERVED_SET.read_text(encoding="utf-8"):
            sys.exit(f"publish: {name} was written but not referenced")

    print(f"publish: {anim_name} ({args.anim.stat().st_size / 1024:.0f} KiB), {poster_name}")
    print(f"publish: {label}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
