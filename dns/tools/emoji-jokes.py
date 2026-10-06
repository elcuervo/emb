#!/usr/bin/env python3
"""Find the composition jokes this vocabulary can actually tell.

A conjunction (`a*b`) is answered by the entry with the greatest joint
similarity to both terms, with the terms themselves excluded. Which entries that
produces is a property of the model and the vocabulary, not of the operator, so
the examples the gallery ships are *found* here rather than written by hand.

Two things are reported:

1. **Reachable** — every pair of entries, scored against every candidate: for each
   entry, the pairs it is close to, keeping the greatest product each pair
   reaches. A pair whose terms are near-duplicates of each other is excluded:
   `↽*⇃` resolves to another harpoon barb, which is a sibling of the same symbol
   rather than a joke, and a winner sharing a word with either term is marked `~`
   so the lexical siblings can be told from the `·` concepts. The floors it
   searched under (a dissimilarity ceiling for the terms and a similarity floor
   for both legs of the winner) are printed with the results, so a re-run against
   the same vocabulary and model is comparable.

2. **Intended but unreachable** — for a pair whose joke is a specific answer
   (a country flag, say), the legs that answer would need beside the legs the
   winner actually has, so a ceiling is a measurement rather than an assertion.

The vectors come from the zone's own upstream emb, because they must be the
vectors the zone ranks with: same model, same descriptions, same normalization.

    just dns-dev          # in another terminal: emb on loopback
    just emoji-jokes

Exit status is 0 when the vocabulary was embedded, 1 when the upstream could not
be reached, and 2 when the vocabulary or its tooling is unusable.
"""

from __future__ import annotations

import argparse
import json
import shutil
import subprocess
import sys
from pathlib import Path

import numpy as np

ROOT = Path(__file__).resolve().parents[1]
ASSET = ROOT / "emoji-vocab.json"

DEFAULT_UPSTREAM = "127.0.0.1:16389"
DEFAULT_MODEL = "emojiml"

# The pair's own terms must be at most this similar, or the "third" entry is a
# sibling of the same symbol and the triple means nothing.
CEILING = 0.35
# Both legs of the winner must clear this to count as a joke rather than a
# coincidence.
FLOOR = 0.28
# A word shared by the winner and a term only means the two are the same thing if
# the word is rare enough to say something: two clock faces share `clock` (29
# entries) and are siblings, while a dragon and a panda share `animal` (122) and
# are not.
RARE = 30
# How many neighbours of each term to consider as the other term of a pair.
NEIGHBOURS = 40
# How many texts to embed per upstream call.
BATCH = 256
# How many candidate pairs to score at once. Each batch holds a matrix of
# (entries × pairs) products, so this is the tool's peak memory.
BATCH_PAIRS = 4096


def discover(
    similarity: np.ndarray, entries: list[dict[str, str]], ceiling: float, floor: float
) -> list[tuple[bool, float, int, int, int, tuple[float, float]]]:
    """Every pair whose best joint entry is close to both terms.

    A pair's winner is the entry with the greatest product of similarity to the
    two, over *every* entry — not merely over the entries close to both terms,
    because the winner can have one weak leg and still win (a temple and an
    elephant answer with `om` at 0.261 and 0.634, which beats every entry that
    is close to both). So the candidate pairs come from the entries' own
    neighbourhoods, and then each pair is scored against the whole vocabulary.
    """
    n = len(entries)
    counts = word_counts(entries)

    # Candidate pairs: every pair some entry is close to on both sides. A pair
    # whose winner clears the floor is necessarily in here, and the scoring pass
    # below decides which entry actually wins.
    left, right = [], []
    for c in range(n):
        near = np.flatnonzero(similarity[c] >= floor)
        near = near[near != c]
        if len(near) < 2:
            continue
        i, j = np.meshgrid(near, near, indexing="ij")
        keep = i < j
        left.append(i[keep])
        right.append(j[keep])
    if not left:
        return []
    keys = np.unique(np.concatenate(left) * n + np.concatenate(right))
    i, j = keys // n, keys % n

    # The true winner of each candidate pair, and the legs that won it.
    winners = np.empty(len(i), dtype=np.int32)
    for start in range(0, len(i), BATCH_PAIRS):
        si, sj = i[start : start + BATCH_PAIRS], j[start : start + BATCH_PAIRS]
        products = similarity[:, si] * similarity[:, sj]
        columns = np.arange(len(si))
        products[si, columns] = -2  # a term is not its own answer
        products[sj, columns] = -2
        winners[start : start + len(si)] = np.argmax(products, axis=0)

    triples = []
    for k in range(len(i)):
        c, a, b = int(winners[k]), int(i[k]), int(j[k])
        if similarity[a, b] > ceiling:
            continue
        legs = (float(similarity[c, a]), float(similarity[c, b]))
        if min(legs) < floor:
            continue
        # A winner sharing a *rare* word with a term is that word's sibling (two
        # clock faces answer with a third clock face); one sharing only a common
        # word is a concept both terms point at, which is the joke the gallery
        # ships.
        shared = words(entries[c]) & (words(entries[a]) | words(entries[b]))
        sibling = any(counts[word] <= RARE for word in shared)
        triples.append((not sibling, min(legs), a, b, c, legs))
    return triples

# Pairs whose joke is a specific answer, with the answer they want. The point of
# listing them is to measure the ceiling: if the wanted entry cannot beat the
# winner, that is a fact about the vocabulary and it is reported as one.
INTENDED: list[tuple[str, str, str, str]] = [
    ("🍕", "🦅", "flag_united_states", "pizza + eagle = USA"),
    ("🍔", "🦅", "flag_united_states", "burger + eagle = USA"),
    ("🗽", "🏈", "flag_united_states", "liberty + football = USA"),
    ("🍣", "🌸", "flag_japan", "sushi + blossom = Japan"),
    ("🍝", "🍕", "flag_italy", "pasta + pizza = Italy"),
    ("🌮", "🌵", "flag_mexico", "taco + cactus = Mexico"),
    ("🦘", "🐨", "flag_australia", "kangaroo + koala = Australia"),
    ("🐉", "🥟", "flag_china", "dragon + dumpling = China"),
    ("🐘", "🛕", "flag_india", "elephant + temple = India"),
    ("🩰", "🥐", "flag_france", "ballet + croissant = France"),
    ("🐻", "🍁", "flag_canada", "bear + maple leaf = Canada"),
]


def embed(upstream: str, model: str, texts: list[str]) -> np.ndarray:
    """Embed texts through the zone's upstream emb, in batches."""
    if not shutil.which("redis-cli"):
        raise SystemExit("emoji-jokes: redis-cli is not on PATH (run inside `nix develop`)")
    out = []
    for i in range(0, len(texts), BATCH):
        batch = texts[i : i + BATCH]
        reply = subprocess.run(
            ["redis-cli", "-p", upstream.rsplit(":", 1)[1], "--json", "EMB", model, "VALUES", *batch],
            capture_output=True,
            text=True,
        )
        if reply.returncode != 0 or not reply.stdout.strip():
            detail = (reply.stderr or reply.stdout).strip() or "no reply"
            print(f"emoji-jokes: {upstream} could not embed: {detail}", file=sys.stderr)
            sys.exit(1)
        envelope = json.loads(reply.stdout)
        values = np.array([float(v) for v in envelope["values"]], dtype=np.float32)
        out.append(values.reshape(envelope["shape"][0], envelope["shape"][1]))
    return np.vstack(out)


def words(entry: dict[str, str]) -> set[str]:
    """The words of an entry's description, which is what the model embeds."""
    return set(entry["description"].lower().replace(":", " ").split())


def word_counts(entries: list[dict[str, str]]) -> dict[str, int]:
    """How many entries each word appears in, counted once per entry."""
    counts: dict[str, int] = {}
    for entry in entries:
        for word in words(entry):
            counts[word] = counts.get(word, 0) + 1
    return counts


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--upstream", default=DEFAULT_UPSTREAM, help="the zone's emb, host:port")
    parser.add_argument("--model", default=DEFAULT_MODEL, help="the model the zone embeds with")
    parser.add_argument("--ceiling", type=float, default=CEILING, help="the greatest similarity a pair's terms may have")
    parser.add_argument("--floor", type=float, default=FLOOR, help="the least similarity both legs of a winner must have")
    parser.add_argument("--top", type=int, default=20, help="how many reachable examples to print")
    args = parser.parse_args()

    entries = json.loads(ASSET.read_text())["entries"]
    model = args.model
    rows = embed(args.upstream, model, [e["description"] for e in entries])
    rows /= np.linalg.norm(rows, axis=1, keepdims=True)
    similarity = rows @ rows.T
    slugs = [e["slug"] for e in entries]
    at = {e["glyph"]: i for i, e in enumerate(entries)}

    print(f"# {len(entries)} entries · {model} · {rows.shape[1]} dimensions")
    print(
        f"# floors: terms at most {args.ceiling:.2f} similar · both legs at least {args.floor:.2f} · "
        f"a shared word counts as a sibling only below {RARE} entries"
    )

    triples = discover(similarity, entries, args.ceiling, args.floor)
    triples.sort(key=lambda t: (not t[0], -t[1], slugs[t[2]], slugs[t[3]]))
    concepts = sum(1 for t in triples if t[0])

    print(f"\n## reachable: {len(triples)} pairs whose best third entry is close to both terms")
    print(f"## of those, {concepts} answer with a concept and {len(triples) - concepts} with a sibling of a term\n")
    for concept, _, i, j, c, legs in triples[: args.top]:
        print(
            f"{'·' if concept else '~'} {entries[i]['glyph']}*{entries[j]['glyph']} -> {entries[c]['glyph']}"
            f" {slugs[c]:<32} legs {legs[0]:.3f} {legs[1]:.3f}  ({entries[c]['description'][:44]})"
        )

    print("\n## intended but unreachable\n")
    for a, b, wanted, joke in INTENDED:
        i, j = at[a], at[b]
        if wanted not in slugs:
            print(f"{a}*{b} -> {wanted}: not in the vocabulary   ({joke})")
            continue
        t = slugs.index(wanted)
        products = similarity[:, i] * similarity[:, j]
        products[i] = products[j] = -2
        winner = int(np.argmax(products))
        won = slugs[winner] == wanted
        print(
            f"{a}*{b} -> wants {wanted} legs {similarity[t, i]:.3f} {similarity[t, j]:.3f}"
            f" | {'WINS' if won else 'loses to'} {entries[winner]['glyph']} {slugs[winner]}"
            f" legs {similarity[winner, i]:.3f} {similarity[winner, j]:.3f}"
            f"   ({joke})"
        )
    return 0


if __name__ == "__main__":
    sys.exit(main())
