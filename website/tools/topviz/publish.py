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

With `--fleet` the same run's *fleet* view is published instead: the take, its
still frame, its caption and its figures go into the documentation page's fleet
region (`<!-- fleetviz:begin -->`), its capture into `docs/operations.md`'s
fleet block, and its served-set entry beside the single-node take's. The
landing page carries one plate and is not touched.

    python3 publish.py --fleet --frame cluster-frame.txt --cast cluster-take.cast \\
        --gif cluster.gif --samples cluster-samples.txt \\
        --version 0.4.3.pre5 --models 4 --addr 127.0.0.1:16379,127.0.0.1:16380

What it rewrites, by marker, in `website/index.html` (relative cast URL) and in
`website/docs/index.html` (the same file, spelled `../assets/cast/…` because the
two pages live at different depths):

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
DOC_PAGE = SITE / "docs" / "index.html"

CAPTURE_PREFIX = "emb-top-"
CAST_PREFIX = "emb-top-"
# The fleet view's capture lives beside this take and starts with the same
# prefix, but `website-clusterviz` owns it; deleting it here breaks the
# operations page's image.
CLUSTER_PREFIX = "emb-top-cluster-"
BLOCK_BEGIN = "<!-- topviz:begin -->"
BLOCK_END = "<!-- topviz:end -->"
# The fleet capture's own block in `docs/operations.md`, written by the same
# run that writes its take and its plate, so the reference cannot name a GIF
# the rig no longer serves.
CLUSTER_BLOCK_BEGIN = "<!-- clusterviz:begin -->"
CLUSTER_BLOCK_END = "<!-- clusterviz:end -->"

# The fleet plate's own region on the documentation surface. The landing page
# carries one plate, so the fleet take is stamped only inside this slice -- and
# sliced rather than passed whole because `stamp_text` and `stamp_attr` replace
# the *first* match: with two plates on one page, the single-node plate's
# helpers would rewrite the fleet's. The mount keeps the `data-topviz-*`
# vocabulary, so the page's one script drives both plates.
FLEET_BEGIN = "<!-- fleetviz:begin -->"
FLEET_END = "<!-- fleetviz:end -->"


def escape(text: str) -> str:
    return text.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def stamp_text(html: str, marker: str, text: str, page: Path) -> str:
    """Replace the inner text of the element carrying `marker`."""
    marked = re.compile(
        r"(?P<open><(?P<tag>[a-zA-Z][\w-]*)[^<>]*\s"
        + re.escape(marker)
        + r"(?=[\s>/])[^<>]*>)"
        r"(?P<inner>[^<>]*)"
        r"(?P<close></(?P=tag)>)"
    )
    if not marked.search(html):
        where = page.relative_to(REPO_ROOT)
        raise SystemExit(f"publish: no element carries {marker} in {where}")
    return marked.sub(
        lambda m: m.group("open") + text + m.group("close"), html, count=1
    )


def stamp_block(markdown: str, begin: str, end: str, body: str, page: Path) -> str:
    """Replace everything between the block markers, markers themselves kept."""
    pattern = re.compile(re.escape(begin) + r".*?" + re.escape(end), re.DOTALL)
    if not pattern.search(markdown):
        where = page.relative_to(REPO_ROOT)
        raise SystemExit(f"publish: no {begin} block in {where}")
    return pattern.sub(f"{begin}\n{body}\n{end}", markdown, count=1)


def stamp_attr(html: str, marker: str, value: str, page: Path) -> str:
    """Replace the value of the attribute `marker` on the element carrying it."""
    marked = re.compile(
        r'(?P<open><[a-zA-Z][\w-]*[^<>]*\s'
        + re.escape(marker)
        + r'=")(?P<value>[^"]*)(?P<close>")'
    )
    if not marked.search(html):
        where = page.relative_to(REPO_ROOT)
        raise SystemExit(f"publish: no element carries {marker} in {where}")
    return marked.sub(
        lambda m: m.group("open") + value + m.group("close"), html, count=1
    )


def stamp_plate(page: Path, take: str, frame: str, label: str, figures: str) -> str:
    """Return `page`'s source with its emb-top plate rewritten from this run.

    Both surfaces that show the take are written from the one run, so the page
    and the documentation cannot describe different recordings. `take` is the
    cast URL as *that page* must spell it: the two live in different
    directories, so the same file is referenced two ways.
    """
    html = page.read_text(encoding="utf-8")
    for marker, text in (
        ("data-topviz-frame", escape(frame)),
        ("data-topviz-run", escape(label)),
        ("data-topviz-metrics", escape(figures)),
    ):
        html = stamp_text(html, marker, text, page)
    return stamp_attr(html, "data-topviz-cast", take, page)


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


def stamp_served_fleet(source: str, name: str) -> str:
    """Point the served set at the fleet take, beside the single-node entry.

    Its own anchored stamp rather than the single-node pattern: that one is
    deliberately narrow (`emb-top-[0-9a-f]+`), so it can never rewrite the
    fleet's line — which is why the fleet take needs a writer of its own.
    """
    line = f'        "assets/cast/{name}",'
    previous = re.compile(
        r'^\s*"assets/cast/emb-top-cluster-[0-9a-f]+\.cast",$', re.MULTILINE
    )
    if previous.search(source):
        return previous.sub(line, source, count=1)
    single = re.compile(r'^\s*"assets/cast/emb-top-[0-9a-f]+\.cast",$', re.MULTILINE)
    if not single.search(source):
        raise SystemExit(
            "publish: published-tree.py has no single-node cast entry to anchor the fleet take to"
        )
    return single.sub(lambda m: f"{m.group(0)}\n{line}", source, count=1)


def stamp_region(
    page: Path, begin: str, end: str, take: str, frame: str, label: str, figures: str
) -> str:
    """Return `page` with the plate inside its marked region rewritten.

    Everything outside the region is returned untouched, so the page's other
    plate keeps its own run's frame, cast, caption and figures.
    """
    html = page.read_text(encoding="utf-8")
    region = re.compile(re.escape(begin) + r"(?P<body>.*?)" + re.escape(end), re.DOTALL)
    found = region.search(html)
    if not found:
        where = page.relative_to(REPO_ROOT)
        raise SystemExit(f"publish: no {begin} … {end} region in {where}")
    body = found.group("body")
    for marker, text in (
        ("data-topviz-frame", escape(frame)),
        ("data-topviz-run", escape(label)),
        ("data-topviz-metrics", escape(figures)),
    ):
        body = stamp_text(body, marker, text, page)
    body = stamp_attr(body, "data-topviz-cast", take, page)
    return html[: found.start("body")] + body + html[found.end("body") :]


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


def fleet_figures(samples: Path) -> tuple[str, int]:
    """Describe the busiest fleet poll: the aggregate, then each node's share.

    `emb-top -once` with several nodes prints the summed fleet aggregate
    followed by one `node:<label>` section per node (see `writeFleetOnceLine`),
    so the aggregate fields live before the first `node:` and a node's own
    fields before its first `model:`. Returns the sentence and the node count
    the log itself reports, so neither the caption nor the figures are typed
    into the page.
    """
    peak, best = None, (0, -1.0)
    for line in samples.read_text(encoding="utf-8").splitlines():
        if " node:" not in line:
            continue
        score = (len(line.split(" node:")) - 1, float(dict(PAIR.findall(line.split(" node:", 1)[0]))["req_rate"]))
        if score > best:
            best, peak = score, line
    if peak is None:
        raise SystemExit(f"publish: no fleet poll in {samples}")

    aggregate = dict(PAIR.findall(peak.split(" node:", 1)[0]))
    total = float(aggregate["req_rate"])

    nodes = []
    for chunk in peak.split(" node:")[1:]:
        label, _, fields = chunk.partition(" ")
        node = dict(PAIR.findall(fields.split(" model:", 1)[0]))
        rate = float(node["req_rate"])
        share = rate / total * 100 if total > 0 else 0.0
        nodes.append(
            f"{label} {rate:.0f}/s ({share:.0f}% of the fleet, "
            f"p95 {ms(node['lat_p95_us'])}, CPU {float(node['cpu_pct']):.0f}%)"
        )

    head = (
        f"Busiest poll of the recorded fleet run: {total:.0f} requests/s, "
        f"{float(aggregate['tok_rate']) / 1000:.1f}k tokens/s, "
        f"cache hit rate {float(aggregate['cache_hit_rate']):.0f}%, "
        f"CPU {float(aggregate['cpu_pct']):.0f}%, {aggregate['mem_mb']}MB resident "
        f"across {len(nodes)} nodes."
    )
    return head + " Per node: " + "; ".join(nodes) + ".", len(nodes)


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
    parser.add_argument("--commit", default="", help="commit the take was recorded at")
    parser.add_argument(
        "--fleet",
        action="store_true",
        help="publish the fleet take's plate and capture instead of the single node's",
    )
    args = parser.parse_args()

    for path in (args.frame, args.cast, args.gif, args.samples):
        if not path.is_file() or path.stat().st_size == 0:
            sys.exit(f"publish: missing or empty artifact {path}")

    # The published artifacts are deleted before the new ones are written, so
    # naming one of them as the source deletes it and then fails to read it.
    for source, published in ((args.gif, DOC_ASSETS), (args.cast, CAST_DIR)):
        if source.resolve().parent == published.resolve():
            sys.exit(f"publish: {source} is a published artifact, not a run's input")

    frame = args.frame.read_text(encoding="utf-8").strip("\n")
    if not frame:
        sys.exit(f"publish: {args.frame} has no frame in it")

    digest = hashlib.sha256(args.gif.read_bytes()).hexdigest()[:8]
    # The fleet's names replace the single node's prefix: `emb-top-cluster-<sha8>`
    # is the pair `website-clusterviz` already established for its GIF.
    capture_name = f"{CLUSTER_PREFIX if args.fleet else CAPTURE_PREFIX}{digest}.gif"
    cast_name = (
        f"{CLUSTER_PREFIX if args.fleet else CAST_PREFIX}"
        f"{hashlib.sha256(args.cast.read_bytes()).hexdigest()[:8]}.cast"
    )

    if args.fleet:
        figures, nodes = fleet_figures(args.samples)
        scope = f"{nodes} nodes · {args.models} models"
    else:
        figures, scope = busiest(args.samples), f"{args.models} models"

    label = f"{args.date} · emb-top v{args.version} · {scope} · {args.addr}"
    if args.commit:
        label = (
            f"{args.date} · emb-top v{args.version} · {args.commit} · {scope} · {args.addr}"
        )

    operations = OPERATIONS.read_text(encoding="utf-8")
    tree = TREE.read_text(encoding="utf-8")
    if args.fleet:
        # The landing page carries one plate, so the fleet take is stamped
        # inside the documentation page's fleet region and nowhere else. Both
        # regions are stamped before anything is written, so a missing marker
        # fails the run rather than leaving a new cast beside an old plate.
        documentation = stamp_region(
            DOC_PAGE,
            FLEET_BEGIN,
            FLEET_END,
            f"../assets/cast/{cast_name}",
            frame,
            label,
            figures,
        )
        tree = stamp_served_fleet(tree, cast_name)
        body = (
            f"![The emb-top cluster view: a live traffic chart of inbound req/s per"
            f" node above a fleet banner, totals, load-distribution bar, one ingress"
            f" row per node — inbound req/s, share of the fleet, a proportional bar"
            f" and a trend sparkline — and a cross-node imbalance panel]"
            f"(assets/{capture_name})\n"
            f"\n"
            f"*Measured fleet run · {label}. The traffic chart streams each node's"
            f" inbound rate in its row's colour; the load bar and ingress rows split"
            f" the fleet's inbound traffic, and the imbalance panel checks it across"
            f" nodes. Re-record with `just website-clusterviz`.*"
        )
        markdown = stamp_block(
            operations, CLUSTER_BLOCK_BEGIN, CLUSTER_BLOCK_END, body, OPERATIONS
        )
        landing = None
    else:
        # Both pages that show the single-node take are stamped from the one
        # run, so the landing page and the documentation cannot describe
        # different recordings.
        landing = stamp_plate(INDEX, f"assets/cast/{cast_name}", frame, label, figures)
        documentation = stamp_plate(
            DOC_PAGE, f"../assets/cast/{cast_name}", frame, label, figures
        )
        tree = stamp_served(tree, cast_name)
        # One line for the image: markdown allows a wrapped link text, but a
        # renderer that does not is a broken image in the repository's own
        # documentation.
        body = (
            f"![The emb-top dashboard under load: one row per model with its request,"
            f" token and latency rates beside an activity strip, request-rate and p95-"
            f"latency streams, and cache, CPU and memory gauges](assets/{capture_name})\n"
            f"\n"
            f"*Captured {label}.*"
        )
        markdown = stamp_block(
            operations, BLOCK_BEGIN, BLOCK_END, body, OPERATIONS
        )

    # One run owns one pair of files: a publish replaces its own take and
    # capture and never the other rig's, whose names start with the cluster
    # prefix.
    DOC_ASSETS.mkdir(parents=True, exist_ok=True)
    for previous in DOC_ASSETS.glob(f"{CAPTURE_PREFIX}*.gif"):
        if previous.name.startswith(CLUSTER_PREFIX) == args.fleet:
            previous.unlink()
    (DOC_ASSETS / capture_name).write_bytes(args.gif.read_bytes())

    CAST_DIR.mkdir(parents=True, exist_ok=True)
    for previous in CAST_DIR.glob(f"{CAST_PREFIX}*.cast"):
        if previous.name.startswith(CLUSTER_PREFIX) == args.fleet:
            previous.unlink()
    (CAST_DIR / cast_name).write_bytes(args.cast.read_bytes())

    if not args.fleet:
        # The plate's image revision shipped its take in the served tree.
        # Nothing references those files now, and leaving them would fail
        # published-tree.py.
        for orphan in IMG_DIR.glob(f"{CAPTURE_PREFIX}*"):
            if not orphan.name.startswith(CLUSTER_PREFIX):
                orphan.unlink()

    if landing is not None:
        INDEX.write_text(landing, encoding="utf-8")
    DOC_PAGE.write_text(documentation, encoding="utf-8")
    TREE.write_text(tree, encoding="utf-8")
    OPERATIONS.write_text(markdown, encoding="utf-8")

    where = "documentation plate" if args.fleet else "landing page"
    print(f"publish: {len(frame.splitlines())}-line frame into the {where}")
    print(f"publish: the same frame into {DOC_PAGE.relative_to(REPO_ROOT)}")
    print(
        f"publish: website/assets/cast/{cast_name}"
        f" ({args.cast.stat().st_size / 1024:.0f} KiB)"
    )
    print(f"publish: docs/assets/{capture_name} ({args.gif.stat().st_size / 1024:.0f} KiB)")
    print(f"publish: {label}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
