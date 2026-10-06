#!/usr/bin/env python3
"""Stamp the zone's shipped examples into the plate, derived from `dns/examples.json`.

The plate offers one control per shipped example, and the examples are measured
data: `just verify-emoji` fails when the zone stops answering them the way they
are recorded. Copying them into the page by hand makes a second source that
drifts the moment an example is re-pinned or a joke is added, so this tool makes
the controls derived instead of typed. The example set is the plate's starter
set, and adding a joke to `dns/examples.json` adds a control without an edit to
the page.

    python3 website/tools/stamp-examples.py          # rewrite in place
    python3 website/tools/stamp-examples.py --check  # exit 1 if anything drifted

What it rewrites, all derived from the file:

- the controls between the `<!-- examples:start -->` and `<!-- examples:end -->`
  markers, ordered family by family, each carrying the query, its mode, and — for
  an example whose intended answer is unreachable — the note that says so;
- the default query in the plate's input, which is the first sentence example, so
  the page opens on the shape the reply is explained with;
- which control starts pressed, which is that same example.

`--check` is the drift guard: it is what CI runs.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from html import escape
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
EXAMPLES = REPO_ROOT / "dns" / "examples.json"
TARGET = REPO_ROOT / "website" / "demos" / "dns.html"

START = "<!-- examples:start -->"
END = "<!-- examples:end -->"
# The default query is the shape the reply is explained with, and the input is
# the page's own source for it: the button that starts pressed is the one whose
# query this is.
DEFAULT_MODE = "sentence"

# Narrow: only the input's own `value`, never a neighbouring attribute.
INPUT_VALUE = re.compile(r'(id="dns-q"[\s\S]*?value=")([^"]*)(")')


def label(example: dict) -> str:
    """A control's text: a dotted name reads as the sentence it is."""
    name = example["name"]
    return name.replace(".", " ") if "." in name else name


def controls(examples: list[dict]) -> tuple[str, str]:
    """The control block, and the query the plate opens on."""
    default = next((e["name"] for e in examples if e["mode"] == DEFAULT_MODE), None)
    if default is None:
        sys.exit(f"stamp-examples: no {DEFAULT_MODE!r} example to open on")

    # Family by family, in the order the file introduces them, so the block is
    # deterministic and the jokes sit together.
    order: list[str] = []
    for example in examples:
        if example["family"] not in order:
            order.append(example["family"])

    lines = []
    for family in order:
        lines.append(f'            <span class="rig__pick__fam">{escape(family)}</span>')
        for example in examples:
            if example["family"] != family:
                continue
            attrs = {
                "type": "button",
                "data-example": example["name"],
                "data-mode": example["mode"],
                "data-family": family,
                "aria-pressed": "true" if example["name"] == default else "false",
            }
            if example.get("unreachable"):
                attrs["data-note"] = example["claim"]
            rendered = " ".join(f'{name}="{escape(value, quote=True)}"' for name, value in attrs.items())
            lines.append(f'            <button {rendered}>{escape(label(example))}</button>')
    return "\n".join(lines), default


def stamp(text: str, block: str, default: str) -> tuple[str, list[str]]:
    if START not in text or END not in text:
        sys.exit(f"stamp-examples: {TARGET} has no {START} / {END} markers")
    head, rest = text.split(START, 1)
    _, tail = rest.split(END, 1)
    stamped = f"{head}{START}\n{block}\n            {END}{tail}"

    stamped, count = INPUT_VALUE.subn(
        lambda m: f"{m.group(1)}{escape(default, quote=True)}{m.group(3)}", stamped, count=1
    )
    if count != 1:
        sys.exit(f"stamp-examples: {TARGET} has no dns-q input to point at {default!r}")

    changed = []
    if stamped != text:
        changed.append(f"controls and default query -> {default}")
    return stamped, changed


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument(
        "--check",
        action="store_true",
        help="do not write; exit 1 if the plate's controls differ from the shipped examples",
    )
    args = parser.parse_args()

    for path in (EXAMPLES, TARGET):
        if not path.exists():
            sys.exit(f"stamp-examples: {path} does not exist")
    examples = json.loads(EXAMPLES.read_text(encoding="utf-8"))["examples"]
    if not examples:
        sys.exit("stamp-examples: the shipped examples are empty")

    text = TARGET.read_text(encoding="utf-8")
    block, default = controls(examples)
    stamped, changed = stamp(text, block, default)

    if args.check:
        if stamped == text:
            print(f"stamp-examples-check: {len(examples)} examples, byte for byte")
            return 0
        print(
            f"stamp-examples-check: {TARGET} is stale ({'; '.join(changed)}) - "
            f"regenerate with `just website-examples`",
            file=sys.stderr,
        )
        return 1

    TARGET.write_text(stamped, encoding="utf-8")
    print(f"stamp-examples: wrote {TARGET} ({len(examples)} examples, opening on {default!r})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
