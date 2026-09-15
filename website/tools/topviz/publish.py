#!/usr/bin/env python3
"""Publish one `emb-top` take: the page's plate, the documentation's capture.

One run produces the take and both of its renderings, and this writes them so
they cannot disagree:

* **The landing page's plate** *plays* the take. The trimmed recording is copied
  into the served tree under a name made of its own bytes, and the plate points
  at it; the page replays it with the vendored asciinema player. The page's
  script is what starts it, so the plate also carries the dashboard's own text
  frame from the middle of the run — that is what a reader with scripting off,
  or with a reduced-motion preference, gets.
* **`docs/operations.md`** carries the animated capture of the same run, in
  colour, at a size where it can be read: the one place the dashboard's motion
  and its palette can be shown without a script.

    python3 publish.py --frame frame.txt --cast take.cast --gif take.gif \\
        --samples samples.txt \\
        --version 0.4.0.pre5 --models 4 --addr 127.0.0.1:16379

What it rewrites, by marker, in `website/index.html`:

    <pre … data-topviz-frame>…</pre>
    <div … data-topviz-cast="assets/cast/emb-top-<sha8>.cast">
    <p …><span data-topviz-run>…</span></p>
    <p … data-topviz-metrics>…</p>

and the block between the two HTML comments in `docs/operations.md` (the only
marker form markdown has):

    <!-- topviz:begin -->
    <!-- topviz:end -->

Both artifacts are named from their own bytes, and both references — the page's
attribute and `published-tree.py`'s served set — are written here, so a re-record
replaces them and deletes the previous files rather than leaving a second take
behind. This also removes the site assets the plate's image revision wrote, so
that revision cannot leave bytes in the served tree.

Fails — rather than writing half of it — when a marker is missing, when the
frame is empty, or when the figures cannot be read out of the sample log.
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
CAST_DIR = SITE / "assets" / "cast"
INDEX = SITE / "index.html"
TREE = SITE / "tools" / "published-tree.py"
OPERATIONS = REPO_ROOT / "docs" / "operations.md"
DOC_ASSETS = REPO_ROOT / "docs" / "assets"

CAPTURE_PREFIX = "emb-top-"
CAST_PREFIX = "emb-top-"
BLOCK_BEGIN = "<!-- topviz:begin -->"
BLOCK_END = "<!-- topviz:end -->"


def escape(text: str) -> str:
    return text.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def stamp_text(html: str, marker: str, text: str) -> str:
    """Replace the inner text of the element carrying `marker`."""
    marked = re.compile(
        r"(?P<open><(?P<tag>[a-zA-Z][\w-]*)[^<>]*\s"
        + re.escape(marker)
        + r"(?=[\s>/])[^<>]*>)"
        r"(?P<inner>[^<>]*)"
        r"(?P<close></(?P=tag)>)"
    )
    if not marked.search(html):
        raise SystemExit(f"publish: no element carries {marker} in index.html")
    return marked.sub(
        lambda m: m.group("open") + text + m.group("close"), html, count=1
    )


def stamp_block(markdown: str, body: str) -> str:
    """Replace everything between the block markers, markers themselves kept."""
    pattern = re.compile(
        re.escape(BLOCK_BEGIN) + r".*?" + re.escape(BLOCK_END), re.DOTALL
    )
    if not pattern.search(markdown):
        raise SystemExit(f"publish: no {BLOCK_BEGIN} block in docs/operations.md")
    return pattern.sub(f"{BLOCK_BEGIN}\n{body}\n{BLOCK_END}", markdown, count=1)


def stamp_attr(html: str, marker: str, value: str) -> str:
    """Replace the value of the attribute `marker` on the element carrying it."""
    marked = re.compile(
        r'(?P<open><[a-zA-Z][\w-]*[^<>]*\s'
        + re.escape(marker)
        + r'=")(?P<value>[^"]*)(?P<close>")'
    )
    if not marked.search(html):
        raise SystemExit(f"publish: no element carries {marker} in index.html")
    return marked.sub(
        lambda m: m.group("open") + value + m.group("close"), html, count=1
    )


def stamp_served(source: str, name: str) -> str:
    """Point `published-tree.py`'s served set at the take this run wrote.

    The take's name is content, so it is written rather than typed — the two
    places that must agree on it are the page and this check, and a half-write
    of either is the class of defect the check exists to catch. First runs
    insert the line (there is no previous one to replace).
    """
    line = f'        "assets/cast/{name}",'
    previous = re.compile(r'^\s*"assets/cast/emb-top-[0-9a-f]+\.cast",$', re.MULTILINE)
    if previous.search(source):
        return previous.sub(line, source, count=1)
    anchor = '        "assets/js/main.js",\n'
    if anchor not in source:
        raise SystemExit(
            "publish: published-tree.py has no assets/js/main.js entry to anchor the cast to"
        )
    return source.replace(anchor, anchor + line + "\n", 1)


# ---- the run's figures, as text ----
#
# One line per poll, e.g.
#   t=… total_requests=3799 req_rate=138.0 lat_p95_us=148300 cache_hit_rate=59.1
#     model:minilm dim=384 reqs=1285 req_rate=43.0 pooling=mean quant=fp32
# See internal/embtop/once.go and docs/operations.md. Keys carry digits
# (`lat_p95_us`), so the key pattern is not `[a-z_]+` — which silently skips
# them and leaves the sentence below missing its latency.
PAIR = re.compile(r"\b([a-z][a-z0-9_]*)=([^\s]+)")
MODEL = re.compile(r"model:(\S+)((?:\s+[a-z][a-z0-9_]*=\S+)+)")


def ms(us: str) -> str:
    return f"{float(us) / 1000:.1f}ms"


def busiest(samples: Path) -> str:
    """Describe the figures of the run's busiest poll, as one sentence.

    "Busiest" is measured as *models active first*, then requests: the aggregate
    peak lands while the last model is still ramping, so ranking on the aggregate
    alone would describe a poll in which half the node reads zero.
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


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--frame", type=Path, required=True, help="page's text frame")
    parser.add_argument("--cast", type=Path, required=True, help="take the page plays")
    parser.add_argument("--gif", type=Path, required=True, help="animated capture")
    parser.add_argument("--samples", type=Path, required=True, help="sample log")
    parser.add_argument("--version", required=True, help="captured emb-top version")
    parser.add_argument("--models", required=True, help="model count of the node")
    parser.add_argument("--addr", required=True, help="node the take came from")
    parser.add_argument("--date", default=date.today().isoformat())
    args = parser.parse_args()

    for path in (args.frame, args.cast, args.gif, args.samples):
        if not path.is_file() or path.stat().st_size == 0:
            sys.exit(f"publish: missing or empty artifact {path}")

    frame = args.frame.read_text(encoding="utf-8").strip("\n")
    if not frame:
        sys.exit(f"publish: {args.frame} has no frame in it")

    digest = hashlib.sha256(args.gif.read_bytes()).hexdigest()[:8]
    capture_name = f"{CAPTURE_PREFIX}{digest}.gif"
    cast_name = f"{CAST_PREFIX}{hashlib.sha256(args.cast.read_bytes()).hexdigest()[:8]}.cast"

    DOC_ASSETS.mkdir(parents=True, exist_ok=True)
    for previous in DOC_ASSETS.glob(f"{CAPTURE_PREFIX}*.gif"):
        previous.unlink()
    (DOC_ASSETS / capture_name).write_bytes(args.gif.read_bytes())

    CAST_DIR.mkdir(parents=True, exist_ok=True)
    for previous in CAST_DIR.glob(f"{CAST_PREFIX}*.cast"):
        previous.unlink()
    (CAST_DIR / cast_name).write_bytes(args.cast.read_bytes())

    # The plate's image revision shipped its take in the served tree. Nothing
    # references those files now, and leaving them would fail published-tree.py.
    for orphan in IMG_DIR.glob(f"{CAPTURE_PREFIX}*"):
        orphan.unlink()

    label = f"{args.date} · emb-top v{args.version} · {args.models} models · {args.addr}"
    figures = busiest(args.samples)

    html = INDEX.read_text(encoding="utf-8")
    for marker, text in (
        ("data-topviz-frame", escape(frame)),
        ("data-topviz-run", escape(label)),
        ("data-topviz-metrics", escape(figures)),
    ):
        html = stamp_text(html, marker, text)
    html = stamp_attr(html, "data-topviz-cast", f"assets/cast/{cast_name}")
    INDEX.write_text(html, encoding="utf-8")

    TREE.write_text(
        stamp_served(TREE.read_text(encoding="utf-8"), cast_name), encoding="utf-8"
    )

    # One line for the image: markdown allows a wrapped link text, but a renderer
    # that does not is a broken image in the repository's own documentation.
    body = (
        f"![The emb-top dashboard under load: four models with their request, token"
        f" and latency rates, an activity heatmap, request-rate and p95-latency"
        f" streams, and cache, CPU and memory gauges](assets/{capture_name})\n"
        f"\n"
        f"*Captured {label}.*"
    )
    OPERATIONS.write_text(
        stamp_block(OPERATIONS.read_text(encoding="utf-8"), body), encoding="utf-8"
    )

    print(f"publish: {len(frame.splitlines())}-line frame into the page")
    print(
        f"publish: website/assets/cast/{cast_name}"
        f" ({args.cast.stat().st_size / 1024:.0f} KiB)"
    )
    print(f"publish: docs/assets/{capture_name} ({args.gif.stat().st_size / 1024:.0f} KiB)")
    print(f"publish: {label}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
