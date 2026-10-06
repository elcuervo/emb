#!/usr/bin/env python3
"""Check a running zone against the examples it ships.

`dns/examples.json` records, for each example, the glyph and slug the zone
returned when the examples were pinned. This asks the zone again and fails when
the answer moved, so a changed model, a rebuilt vocabulary, or a changed grammar
is a decision someone makes rather than a drift nobody notices.

It also reports the shape of the scores, because that is the honest measure of
whether the vocabulary is any good: how confident the model is across the set,
and how many examples land above a coin-flip's worth of similarity.

    python3 dns/tools/verify-emoji.py                     # the dev zone
    python3 dns/tools/verify-emoji.py --base https://dns.emb.is

Exit status is 0 when every example still returns what is recorded, 1 when one
does not, and 2 when the zone cannot be reached at all.
"""

from __future__ import annotations

import argparse
import json
import statistics
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
EXAMPLES = ROOT / "examples.json"
DEFAULT_BASE = "http://127.0.0.1:8099"
# The score at which a result stops being a coincidence. Chosen from the pinned
# set, not from theory: the puns sit below it and the honest answers above.
CONFIDENT = 0.5


def ask(base: str, name: str) -> dict:
    url = f"{base.rstrip('/')}/?q=" + urllib.parse.quote(name)
    try:
        with urllib.request.urlopen(url, timeout=30) as response:
            return json.load(response)
    except urllib.error.HTTPError as exc:
        raise SystemExit(f"verify-emoji: {name}: the zone refused it: {exc.code} {exc.read().decode().strip()}")
    except (urllib.error.URLError, TimeoutError) as exc:
        print(f"verify-emoji: cannot reach {base}: {exc}", file=sys.stderr)
        sys.exit(2)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", default=DEFAULT_BASE, help="the zone's HTTP surface")
    args = parser.parse_args()

    document = json.loads(EXAMPLES.read_text())
    examples = document["examples"]

    results = []
    for example in examples:
        document = ask(args.base, example["name"])
        records = document["records"]
        top = records[0] if records else {}
        # A sentence is answered with a sentence, and a query that is not a
        # sentence must not carry one: both directions are drift.
        sentence = document.get("sentence", {}).get("glyphs")
        expected_sentence = example.get("expect_sentence")
        ranked_ok = top.get("slug") == example["expect_slug"] and top.get("glyph") == example["expect_glyph"]
        sentence_ok = sentence == expected_sentence
        results.append(
            {
                "name": example["name"],
                "mode": example["mode"],
                "family": example["family"],
                "expected": {"glyph": example["expect_glyph"], "slug": example["expect_slug"]},
                "measured": {"glyph": top.get("glyph"), "slug": top.get("slug"), "score": round(top.get("score", 0.0), 3)},
                "pinned_score": example["measured_score"],
                "sentence": sentence,
                "expect_sentence": expected_sentence,
                "top3": [{"glyph": r["glyph"], "slug": r["slug"], "score": round(r["score"], 3)} for r in records],
                "ok": ranked_ok and sentence_ok,
            }
        )

    scores = [r["measured"]["score"] for r in results]
    failed = [r for r in results if not r["ok"]]
    summary = {
        "examples": len(results),
        "matching": len(results) - len(failed),
        "hit_rate": round((len(results) - len(failed)) / len(results), 4) if results else 0.0,
        "mean_score": round(statistics.mean(scores), 4) if scores else 0.0,
        "median_score": round(statistics.median(scores), 4) if scores else 0.0,
        "confident": sum(1 for s in scores if s >= CONFIDENT),
        "confident_at": CONFIDENT,
        "sentences": sum(1 for r in results if r["sentence"]),
    }

    for result in results:
        mark = "ok  " if result["ok"] else "DRIFT"
        top = result["top3"][0]
        runner = "  ".join(f"{r['glyph']} {r['slug']} {r['score']:.3f}" for r in result["top3"][1:])
        sentence = f"  {result['sentence']}" if result["sentence"] else ""
        line = f"{mark} {result['name']:<38} {top['glyph']} {top['slug']:<22} {top['score']:.3f}{sentence}"
        if not result["ok"]:
            line += f"   want {result['expected']['glyph']} {result['expected']['slug']}"
            if result["sentence"] != result["expect_sentence"]:
                line += f" and sentence {result['expect_sentence'] or '—'}"
        print(line)
        if runner:
            print(f"     {'':<38} then {runner}")
    print(
        f"\n{summary['matching']}/{summary['examples']} match "
        f"(hit rate {summary['hit_rate']:.2f}) · mean score {summary['mean_score']:.3f} · "
        f"median {summary['median_score']:.3f} · {summary['confident']}/{summary['examples']} at or above {CONFIDENT} · "
        f"{summary['sentences']} answered with an emoji sentence"
    )
    if failed:
        print(f"\n{len(failed)} example(s) moved. If the move is intended, re-pin with the measured values:", file=sys.stderr)
        for result in failed:
            print(f"  {result['name']}: {result['measured']['glyph']} {result['measured']['slug']} {result['measured']['score']:.3f}"
                  + (f"  sentence={result['sentence']}" if result["sentence"] else ""), file=sys.stderr)

    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
