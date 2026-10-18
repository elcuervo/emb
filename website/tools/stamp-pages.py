#!/usr/bin/env python3
"""Emit every repeated fact about the site's pages from one descriptor.

The site has no build step, and `site-deployment` requires the served tree to
render without the deploy tooling. So the repeated facts are still *committed* --
they are just no longer *typed twenty times*. `website/tools/pages.json` is the
descriptor: the site's origin, social image, theme colour, favicon geometry and
palette tokens, plus one entry per page (file, request path, title, description,
social texts, indexable). This tool emits from it:

1. **Each page's metadata fragment**, between `<!-- meta:start -->` and
   `<!-- meta:end -->` immediately inside `<head>`. Everything after the markers
   -- font preloads, stylesheets, scripts -- belongs to the page and is left
   alone.
2. **`website/sitemap.xml`**, one `<loc>` per indexable page at the address that
   page declares, which is why the inventory cannot disagree with the pages.
3. **The palette block in `assets/css/styles.css`**, between `/* tokens:start */`
   and `/* tokens:end */` inside `:root`, so a colour is written once -- and the
   theme colour and favicon emitted into every page follow it.

Run it after changing the descriptor:

    python3 website/tools/stamp-pages.py          # rewrite in place
    python3 website/tools/stamp-pages.py --check  # exit 1 on any drift

`--check` is the drift guard: it is what CI runs, and it is what makes a
hand-edited page fail rather than silently disagree with the sitemap.

It also measures: every `fg`/`bg` pair the descriptor declares has to meet the
WCAG AA contrast it declares (4.5:1 body, 3:1 large). The palette is content, so
the check belongs where the values are, not in a reviewer's head.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
SITE_DIR = REPO_ROOT / "website"
DESCRIPTOR = SITE_DIR / "tools" / "pages.json"
STYLES = SITE_DIR / "assets" / "css" / "styles.css"
SITEMAP = SITE_DIR / "sitemap.xml"

META_START = "<!-- meta:start -->"
META_END = "<!-- meta:end -->"
TOKENS_START = "/* tokens:start */"
TOKENS_END = "/* tokens:end */"

VIEWPORT = "width=device-width, initial-scale=1, viewport-fit=cover"
FAVICON_PREFIX = "data:image/svg+xml,"

# The head of every page, in one order rather than the five it had. Which of
# these a page carries is decided by `indexable`: a page that asks not to be
# indexed gets the `robots` directive and no canonical or social metadata, since
# advertising an address for a page that opted out is a claim about itself the
# page has already contradicted.
HEAD_ORDER = (
    "charset",
    "viewport",
    "title",
    "canonical",
    "robots",
    "description",
    "theme-color",
    "og:type",
    "og:title",
    "og:site_name",
    "og:description",
    "og:url",
    "og:image",
    "og:image:width",
    "og:image:height",
    "twitter:card",
    "twitter:title",
    "twitter:description",
    "twitter:image",
    "icon",
)


class Problem(Exception):
    """A descriptor or tree that cannot be emitted from."""


def load() -> tuple[dict, list[dict]]:
    if not DESCRIPTOR.exists():
        raise Problem(f"no descriptor at {DESCRIPTOR}")
    try:
        document = json.loads(DESCRIPTOR.read_text(encoding="utf-8"))
    except json.JSONDecodeError as err:
        raise Problem(f"{DESCRIPTOR} is not valid JSON: {err}") from err
    site, pages = document.get("site"), document.get("pages")
    if not isinstance(site, dict) or not isinstance(pages, list):
        raise Problem("the descriptor needs a `site` object and a `pages` array")
    for key in ("origin", "site_name", "social_image", "social_image_size",
                "theme_color", "favicon", "tokens", "contrast"):
        if key not in site:
            raise Problem(f"the descriptor's site block has no {key!r}")
    seen: set[str] = set()
    for page in pages:
        for key in ("file", "path", "title", "description", "indexable"):
            if key not in page:
                raise Problem(f"a page entry is missing {key!r}: {page.get('file')}")
        if page["file"] in seen:
            raise Problem(f"{page['file']} appears twice in the descriptor")
        seen.add(page["file"])
        if not (SITE_DIR / page["file"]).exists():
            raise Problem(f"the descriptor names {page['file']}, which is not in the tree")
        if page["indexable"] and not page["path"].startswith("/"):
            raise Problem(f"{page['file']} has a path that is not root-relative")
    return site, pages


def attribute(value: str) -> str:
    """Escape a value for an HTML attribute, in the one place that decides."""
    return (value.replace("&", "&amp;").replace('"', "&quot;")
                 .replace("<", "&lt;").replace(">", "&gt;"))


def text(value: str) -> str:
    return value.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def favicon(site: dict) -> str:
    """The icon, drawn from the geometry and the tokens rather than typed."""
    geometric = site["favicon"]
    tokens = site["tokens"]
    size = geometric["size"]
    x, y, width, height = geometric["bar_box"]

    def paint(role: str) -> str:
        return "%23" + tokens[geometric[role]].lstrip("#")

    svg = (
        f"<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 {size} {size}'>"
        f"<rect width='{size}' height='{size}' fill='{paint('fill')}'/>"
        f"<rect x='0' y='0' width='{size}' height='{size}' fill='none' "
        f"stroke='{paint('stroke')}' stroke-width='2'/>"
        f"<rect x='{x}' y='{y}' width='{width}' height='{height}' fill='{paint('bar')}'/>"
        f"</svg>"
    )
    return FAVICON_PREFIX + svg.replace("<", "%3C").replace(">", "%3E")


def metadata_fragment(site: dict, page: dict) -> str:
    url = site["origin"] + page["path"]
    image = site["origin"] + site["social_image"]
    social_title = page.get("social_title", page["title"])
    social_description = page["social_description"] if page["indexable"] else None
    twitter_description = page.get(
        "twitter_description", social_description or page["description"]
    )
    tags = {
        "charset": '<meta charset="utf-8">',
        "viewport": f'<meta name="viewport" content="{VIEWPORT}">',
        "title": f"<title>{text(page['title'])}</title>",
        "description": f'<meta name="description" content="{attribute(page["description"])}">',
        "theme-color": (
            f'<meta name="theme-color" content="{site["tokens"][site["theme_color"]]}">'
        ),
    }
    if page["indexable"]:
        tags |= {
            "canonical": f'<link rel="canonical" href="{url}">',
            "og:type": '<meta property="og:type" content="website">',
            "og:title": f'<meta property="og:title" content="{attribute(social_title)}">',
            "og:site_name": (
                f'<meta property="og:site_name" content="{attribute(site["site_name"])}">'
            ),
            "og:description": (
                f'<meta property="og:description" content="{attribute(social_description)}">'
            ),
            "og:url": f'<meta property="og:url" content="{url}">',
            "og:image": f'<meta property="og:image" content="{image}">',
            "og:image:width": (
                f'<meta property="og:image:width" content="{site["social_image_size"][0]}">'
            ),
            "og:image:height": (
                f'<meta property="og:image:height" content="{site["social_image_size"][1]}">'
            ),
            "twitter:card": '<meta name="twitter:card" content="summary_large_image">',
            "twitter:title": (
                f'<meta name="twitter:title" content="{attribute(social_title)}">'
            ),
            "twitter:description": (
                f'<meta name="twitter:description" content="{attribute(twitter_description)}">'
            ),
            "twitter:image": f'<meta name="twitter:image" content="{image}">',
        }
    else:
        tags["robots"] = '<meta name="robots" content="noindex">'
    tags["icon"] = f'<link rel="icon" href="{favicon(site)}">'
    # A page that is indexable carries a canonical; one that is not carries the
    # directive that says so. Which of the two it gets is decided here, not by
    # whoever edits the page -- and the order is the one above, not five orders.
    return "\n".join(
        [META_START, *(tags[name] for name in HEAD_ORDER if name in tags), META_END]
    )


def splice_metadata(source: str, fragment: str, where: str) -> str:
    """Replace the marked fragment, or mark it on the first run over a page."""
    start = source.find(META_START)
    if start != -1:
        end = source.find(META_END, start)
        if end == -1:
            raise Problem(f"{where} has {META_START} with no {META_END}")
        return source[:start] + fragment + source[end + len(META_END):]

    # First run: the page predates the markers. The region is the metadata every
    # page already carries -- from its first `<meta>` to the end of its icon
    # line -- because everything a page owns on its own (preloads, stylesheets,
    # scripts) sits after the icon. A page whose head does not have that shape is
    # refused rather than guessed at.
    first = re.search(r"^[ \t]*<meta\b.*$", source, re.M)
    icon = re.search(r'^.*<link rel="icon".*$', source, re.M)
    if first is None or icon is None or icon.start() < first.start():
        raise Problem(
            f"{where} carries neither the markers nor a head this tool can mark "
            f"(a first <meta> followed by an icon link)"
        )
    end = icon.end()
    return source[: first.start()] + fragment + source[end:]


def token_block(site: dict) -> str:
    """The colour declarations, with the contrast each one is measured at."""
    tokens = site["tokens"]
    measured: dict[str, list[str]] = {}
    for pair in site["contrast"]:
        value = contrast_ratio(tokens[pair["fg"]], tokens[pair["bg"]])
        measured.setdefault(pair["fg"], []).append(
            f"{value:.2f}:1 on --{pair['bg']}"
        )
    width = max(len(name) for name in tokens)
    lines = [TOKENS_START]
    for name, value in tokens.items():
        note = f"   /* {', '.join(measured[name])} */" if name in measured else ""
        lines.append(f"  --{name}:{' ' * (width - len(name))} {value};{note}")
    lines.append(TOKENS_END)
    return "\n".join(lines)


def splice_tokens(source: str, block: str, where: str) -> str:
    start = source.find(TOKENS_START)
    if start != -1:
        end = source.find(TOKENS_END, start)
        if end == -1:
            raise Problem(f"{where} has {TOKENS_START} with no {TOKENS_END}")
        return source[:start] + block + source[end + len(TOKENS_END):]
    anchor = source.find(":root{")
    if anchor == -1:
        raise Problem(f"{where} has neither the token markers nor a `:root{{` block")
    insert = anchor + len(":root{")
    return source[:insert] + "\n" + block + source[insert:]


def render_sitemap(site: dict, pages: list[dict]) -> str:
    listed = sorted(
        (page for page in pages if page["indexable"]), key=lambda page: page["path"]
    )
    body = "\n".join(
        f"  <url><loc>{site['origin']}{page['path']}</loc></url>" for page in listed
    )
    return (
        '<?xml version="1.0" encoding="UTF-8"?>\n'
        "<!--\n"
        "  Emitted from website/tools/pages.json by website/tools/stamp-pages.py.\n"
        "  Do not edit: change the descriptor and run `just website-pages`.\n"
        "\n"
        "  Each <loc> is the canonical address the page itself declares, so this\n"
        "  document and the pages cannot disagree. A page that is not indexable is\n"
        "  absent by its own `noindex`.\n"
        "-->\n"
        '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n'
        f"{body}\n"
        "</urlset>\n"
    )


def channel(component: int) -> float:
    value = component / 255
    return value / 12.92 if value <= 0.04045 else ((value + 0.055) / 1.055) ** 2.4


def luminance(colour: str) -> float:
    if not re.fullmatch(r"#[0-9A-Fa-f]{6}", colour):
        raise Problem(f"{colour!r} is not a six-digit hex colour")
    red, green, blue = (int(colour[index:index + 2], 16) for index in (1, 3, 5))
    return 0.2126 * channel(red) + 0.7152 * channel(green) + 0.0722 * channel(blue)


def contrast_ratio(foreground: str, background: str) -> float:
    darker, lighter = sorted((luminance(foreground), luminance(background)))
    return (lighter + 0.05) / (darker + 0.05)


def check_contrast(site: dict) -> list[str]:
    problems = []
    for pair in site["contrast"]:
        value = contrast_ratio(site["tokens"][pair["fg"]], site["tokens"][pair["bg"]])
        if value < pair["min"]:
            problems.append(
                f"low contrast: --{pair['fg']} on --{pair['bg']} is {value:.2f}:1, "
                f"below the {pair['min']}:1 floor"
            )
    return problems


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--check", action="store_true",
                        help="write nothing; exit 1 if any emitted file has drifted")
    args = parser.parse_args()

    try:
        site, pages = load()
    except Problem as problem:
        print(f"stamp-pages: {problem}", file=sys.stderr)
        return 1

    problems = check_contrast(site)

    outputs: list[tuple[Path, str]] = []
    for page in pages:
        path = SITE_DIR / page["file"]
        try:
            outputs.append((
                path,
                splice_metadata(
                    path.read_text(encoding="utf-8"),
                    metadata_fragment(site, page),
                    f"website/{page['file']}",
                ),
            ))
        except Problem as problem:
            problems.append(str(problem))

    styles = STYLES.read_text(encoding="utf-8")
    try:
        outputs.append(
            (STYLES, splice_tokens(styles, token_block(site), "website/assets/css/styles.css"))
        )
    except Problem as problem:
        problems.append(str(problem))

    outputs.append((SITEMAP, render_sitemap(site, pages)))

    drifted = [
        (path, text) for path, text in outputs
        if path.read_text(encoding="utf-8") != text
    ]

    if args.check:
        for path, _ in drifted:
            problems.append(
                f"drifted:      {path.relative_to(REPO_ROOT)} does not match "
                f"website/tools/pages.json"
            )
    else:
        for path, text in drifted:
            path.write_text(text, encoding="utf-8")

    if problems:
        print("stamp-pages: the descriptor and the tree do not agree", file=sys.stderr)
        for problem in problems:
            print(f"  {problem}", file=sys.stderr)
        return 1

    indexable = sum(1 for page in pages if page["indexable"])
    verb = "checked" if args.check else "wrote"
    print(
        f"stamp-pages: {verb} {len(pages)} page fragment(s), {indexable} sitemap "
        f"address(es), the palette block"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
