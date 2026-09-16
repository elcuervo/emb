#!/usr/bin/env python3
"""Acquire the gallery's corpus from Project Gutenberg and chunk it into passages.

The atlas needs a corpus whose meaning clusters into themes and whose every
word may be committed and shipped. Edgar Allan Poe's collected tales and poems
are public domain in every jurisdiction and, as one author's body of work, they
give the map real continents -- the sea, the grave, madness, detection, grief --
rather than a cloud of unrelated texts.

This tool is the only thing that produces `website/tools/demos/poe.jsonl`, and
it is deterministic: given the same Gutenberg bytes it writes the same corpus,
so a rebuild is a diff rather than a surprise.

    python3 website/tools/demos/fetch-poe.py            # write poe.jsonl
    python3 website/tools/demos/fetch-poe.py --check     # exit 1 if it drifted

What it does, in order:

  1. fetches each source volume from Project Gutenberg (cached under
     `.cache/`, which is not committed),
  2. strips everything outside the `*** START/END OF THE PROJECT GUTENBERG
     EBOOK ... ***` markers -- the licence, the transcriber's notes, the
     "START: FULL LICENSE" appendix -- and then *asserts* that no boilerplate
     survives anywhere in the output, so a Gutenberg layout change fails the
     build instead of shipping,
  3. slices each volume at the work headings listed below, in order, so a work
     this manifest does not name is a boundary rather than swallowed into its
     neighbour,
  4. reflows each work (the volumes hard-wrap at ~70 columns and mark emphasis
     with `_underscores_`), splits it at sentence boundaries, and packs
     sentences into passages of at most `CHUNK_WORDS` words, dropping fragments
     under `MIN_WORDS`.

Each passage carries its work, its first-publication year, its index within the
work, and a short snippet for list displays.

The years are the works' first publication; where a work was substantially
revised the later, canonical date is used. A work with no year in the manifest
is a boundary only -- its text is not shipped. Nothing here is a typographic
convenience: the manifest is the corpus's table of contents, and the attestation
in `poe.NOTICE.md` names the same sources.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[3]
HERE = Path(__file__).resolve().parent
CACHE_DIR = HERE / ".cache"
OUTPUT = HERE / "poe.jsonl"

SOURCE_URL = "https://www.gutenberg.org/cache/epub/{id}/pg{id}.txt"

# The five volumes of the Raven Edition: every tale and poem the corpus ships.
SOURCES = {
    2147: "The Works of Edgar Allan Poe — Volume 1",
    2148: "The Works of Edgar Allan Poe — Volume 2",
    2149: "The Works of Edgar Allan Poe — Volume 3",
    2150: "The Works of Edgar Allan Poe — Volume 4",
    2151: "The Works of Edgar Allan Poe — Volume 5",
}

# (volume, exact heading as it appears alone on a line, display title, year).
# year None => the heading is a boundary only; its text is not shipped, and its
# display title is unused. The headings, in this order, are the volume's table of
# contents: the slice for each entry runs to the next entry, whether or not that
# entry ships, so an unshipped work is a gap rather than text folded into its
# neighbour. Headings repeat where a volume does (two "TO HELEN", three
# "NOTES"), which is why the list is positional rather than a lookup.
WORKS = [
    (2147, 'THE UNPARALLELED ADVENTURES OF ONE HANS PFAAL (*1)', 'The Unparalleled Adventure of One Hans Pfaall', 1835),
    (2147, 'THE GOLD-BUG', 'The Gold-Bug', 1843),
    (2147, 'FOUR BEASTS IN ONE—THE HOMO-CAMELEOPARD', 'Four Beasts in One', 1836),
    (2147, 'THE MURDERS IN THE RUE MORGUE', 'The Murders in the Rue Morgue', 1841),
    (2147, 'THE MYSTERY OF MARIE ROGET.(*1)', 'The Mystery of Marie Rogêt', 1842),
    (2147, 'THE BALLOON-HOAX', 'The Balloon-Hoax', 1844),
    (2147, 'MS. FOUND IN A BOTTLE', 'MS. Found in a Bottle', 1833),
    (2147, 'THE OVAL PORTRAIT', 'The Oval Portrait', 1842),
    (2148, 'THE PURLOINED LETTER', 'The Purloined Letter', 1844),
    (2148, 'THE THOUSAND-AND-SECOND TALE OF SCHEHERAZADE', 'The Thousand-and-Second Tale of Scheherazade', 1845),
    (2148, 'A DESCENT INTO THE MAELSTROM.', 'A Descent into the Maelström', 1841),
    (2148, 'VON KEMPELEN AND HIS DISCOVERY', 'Von Kempelen and His Discovery', 1832),
    (2148, 'MESMERIC REVELATION', 'Mesmeric Revelation', 1844),
    (2148, 'THE FACTS IN THE CASE OF M. VALDEMAR', 'The Facts in the Case of M. Valdemar', 1845),
    (2148, 'THE BLACK CAT.', 'The Black Cat', 1843),
    (2148, 'THE FALL OF THE HOUSE OF USHER', 'The Fall of the House of Usher', 1839),
    (2148, 'SILENCE—A FABLE', 'Silence — A Fable', 1837),
    (2148, 'THE MASQUE OF THE RED DEATH.', 'The Masque of the Red Death', 1842),
    (2148, 'THE CASK OF AMONTILLADO.', 'The Cask of Amontillado', 1846),
    (2148, 'THE IMP OF THE PERVERSE', 'The Imp of the Perverse', 1845),
    (2148, 'THE ISLAND OF THE FAY', 'The Island of the Fay', 1841),
    (2148, 'THE ASSIGNATION', 'The Assignation', 1834),
    (2148, 'THE PIT AND THE PENDULUM', 'The Pit and the Pendulum', 1842),
    (2148, 'THE PREMATURE BURIAL', 'The Premature Burial', 1844),
    (2148, 'THE DOMAIN OF ARNHEIM', 'The Domain of Arnheim', 1847),
    (2148, 'LANDOR’S COTTAGE', 'Landor’s Cottage', 1849),
    (2148, 'WILLIAM WILSON', 'William Wilson', 1839),
    (2148, 'THE TELL-TALE HEART.', 'The Tell-Tale Heart', 1843),
    (2148, 'BERENICE', 'Berenice', 1835),
    (2148, 'ELEONORA', 'Eleonora', 1842),
    (2148, 'NOTES TO THE SECOND VOLUME', None, None),
    (2149, 'NARRATIVE OF A. GORDON PYM', 'The Narrative of Arthur Gordon Pym of Nantucket', 1838),
    (2149, 'NOTES TO THE THIRD VOLUME', None, None),
    (2149, 'LIGEIA', 'Ligeia', 1838),
    (2149, 'MORELLA', 'Morella', 1835),
    (2149, 'A TALE OF THE RAGGED MOUNTAINS', 'A Tale of the Ragged Mountains', 1839),
    (2149, 'THE SPECTACLES', 'The Spectacles', 1844),
    (2149, 'KING PEST', 'King Pest', 1835),
    (2149, 'THREE SUNDAYS IN A WEEK', 'Three Sundays in a Week', 1841),
    (2150, 'THE DEVIL IN THE BELFRY', 'The Devil in the Belfry', 1839),
    (2150, 'LIONIZING', 'Lionizing', 1835),
    (2150, 'X-ING A PARAGRAB', 'X-ing a Paragrab', 1844),
    (2150, 'METZENGERSTEIN', 'Metzengerstein', 1832),
    (2150, 'THE SYSTEM OF DOCTOR TARR AND PROFESSOR FETHER', 'The System of Doctor Tarr and Professor Fether', 1845),
    (2150, 'THE LITERARY LIFE OF THINGUM BOB, ESQ.', 'The Literary Life of Thingum Bob, Esq.', 1844),
    (2150, 'A PREDICAMENT', 'A Predicament', 1839),
    (2150, 'MYSTIFICATION', 'Mystification', 1832),
    (2150, 'DIDDLING', 'Diddling', 1843),
    (2150, 'THE ANGEL OF THE ODD', 'The Angel of the Odd', 1844),
    (2150, 'MELLONTA TAUTA', 'Mellonta Tauta', 1849),
    (2150, 'THE DUC DE L’OMELETTE.', 'The Duc de L’Omelette', 1844),
    (2150, 'THE OBLONG BOX.', 'The Oblong Box', 1844),
    (2150, 'LOSS OF BREATH', 'Loss of Breath', 1832),
    (2150, 'THE MAN THAT WAS USED UP', 'The Man That Was Used Up', 1839),
    (2150, 'THE BUSINESS MAN', 'The Business Man', 1840),
    (2150, 'THE LANDSCAPE GARDEN', 'The Landscape Garden', 1842),
    (2150, 'MAELZEL’S CHESS-PLAYER', None, None),
    (2150, 'THE POWER OF WORDS', 'The Power of Words', 1845),
    (2150, 'THE COLLOQUY OF MONOS AND UNA', 'The Colloquy of Monos and Una', 1841),
    (2150, 'THE CONVERSATION OF EIROS AND CHARMION', 'The Conversation of Eiros and Charmion', 1841),
    (2150, 'SHADOW—A PARABLE', 'Shadow — A Parable', 1835),
    (2151, 'A TALE OF JERUSALEM', 'A Tale of Jerusalem', 1832),
    (2151, 'THE SPHINX', 'The Sphinx', 1846),
    (2151, 'HOP-FROG', 'Hop-Frog', 1849),
    (2151, 'THE MAN OF THE CROWD.', 'The Man of the Crowd', 1840),
    (2151, 'NEVER BET THE DEVIL YOUR HEAD', 'Never Bet the Devil Your Head', 1841),
    (2151, 'THOU ART THE MAN', 'Thou Art the Man', 1844),
    (2151, 'WHY THE LITTLE FRENCHMAN WEARS HIS HAND IN A SLING', 'Why the Little Frenchman Wears His Hand in a Sling', 1844),
    (2151, 'BON-BON.', 'Bon-Bon', 1849),
    (2151, 'SOME WORDS WITH A MUMMY.', 'Some Words with a Mummy', 1845),
    (2151, 'THE POETIC PRINCIPLE', None, None),
    (2151, 'OLD ENGLISH POETRY (*)', None, None),
    (2151, 'POEMS', None, None),
    (2151, 'PREFACE', None, None),
    (2151, 'POEMS OF LATER LIFE', None, None),
    (2151, 'THE RAVEN.', 'The Raven', 1845),
    (2151, 'THE BELLS.', 'The Bells', 1849),
    (2151, 'ULALUME', 'Ulalume', 1847),
    (2151, 'TO HELEN', None, None),
    (2151, 'ANNABEL LEE.', 'Annabel Lee', 1849),
    (2151, 'A VALENTINE.', None, None),
    (2151, 'AN ENIGMA', None, None),
    (2151, '1847. TO MY MOTHER', None, None),
    (2151, 'FOR ANNIE', 'For Annie', 1849),
    (2151, 'TO F——.', None, None),
    (2151, 'TO FRANCES S. OSGOOD', None, None),
    (2151, 'ELDORADO.', 'Eldorado', 1849),
    (2151, 'TO MARIE LOUISE (SHEW)', None, None),
    (2151, 'TO MARIE LOUISE (SHEW)', None, None),
    (2151, 'THE CITY IN THE SEA.', 'The City in the Sea', 1845),
    (2151, 'THE SLEEPER.', 'The Sleeper', 1836),
    (2151, 'NOTES', None, None),
    (2151, 'POEMS OF MANHOOD', None, None),
    (2151, 'LENORE', 'Lenore', 1843),
    (2151, 'TO ONE IN PARADISE.', None, None),
    (2151, 'THE COLISEUM.', 'The Coliseum', 1833),
    (2151, 'THE HAUNTED PALACE.', 'The Haunted Palace', 1839),
    (2151, 'THE CONQUEROR WORM.', 'The Conqueror Worm', 1843),
    (2151, 'SILENCE', None, None),
    (2151, 'DREAM-LAND', 'Dream-Land', 1844),
    (2151, 'HYMN', None, None),
    (2151, 'TO ZANTE', None, None),
    (2151, 'AN UNPUBLISHED DRAMA.', None, None),
    (2151, 'POEMS OF YOUTH', None, None),
    (2151, 'INTRODUCTION TO POEMS—1831', None, None),
    (2151, 'SONNET—TO SCIENCE', 'Sonnet — To Science', 1829),
    (2151, 'AL AARAAF (*)', None, None),
    (2151, 'TAMERLANE', 'Tamerlane', 1827),
    (2151, 'TO HELEN', None, None),
    (2151, 'THE VALLEY OF UNREST', 'The Valley of Unrest', 1831),
    (2151, 'ISRAFEL*', None, None),
    (2151, 'TO THE RIVER——', None, None),
    (2151, 'SONG', None, None),
    (2151, 'SPIRITS OF THE DEAD', None, None),
    (2151, 'A DREAM', None, None),
    (2151, 'ROMANCE', 'Romance', 1829),
    (2151, 'FAIRY-LAND', None, None),
    (2151, 'THE LAKE —— TO——', 'The Lake — To ——', 1827),
    (2151, 'EVENING STAR', 'Evening Star', 1827),
    (2151, 'IMITATION', None, None),
    (2151, 'HYMN TO ARISTOGEITON AND HARMODIUS', None, None),
    (2151, 'DREAMS', None, None),
    (2151, 'NOTES', None, None),
    (2151, 'DOUBTFUL POEMS', None, None),
    (2151, 'ALONE', None, None),
    (2151, 'TO ISADORE', None, None),
    (2151, 'THE VILLAGE STREET', None, None),
    (2151, 'THE FOREST REVERIE', None, None),
    (2151, 'NOTES', None, None),
]

CHUNK_WORDS = 160
MIN_WORDS = 8
SNIPPET_CHARS = 140

# The bridge refuses a text above this, so a passage that trips it is a demo
# that cannot run rather than a style question.
MAX_TEXT_BYTES = 2 << 10
# Leave room for the packer to join two units with a space.
SAFE_BYTES = MAX_TEXT_BYTES - 64

START_MARKER = re.compile(r"^\*\*\* START OF (?:THE|THIS) PROJECT GUTENBERG EBOOK .*\*\*\*$", re.M)
END_MARKER = re.compile(r"^\*\*\* END OF (?:THE|THIS) PROJECT GUTENBERG EBOOK .*\*\*\*$", re.M)

# Case-insensitive, and deliberately broad: a Gutenberg layout change that
# leaks a footer into the corpus must fail the build, not ship.
BOILERPLATE = (
    "project gutenberg",
    "gutenberg.org",
    "gutenberg literary archive",
    "pglaaf",
    "*** start",
    "*** end",
    "start: full license",
)

# A full stop after one of these is not a sentence boundary.
ABBREVIATIONS = {
    "mr", "mrs", "ms", "m", "mme", "mlle", "dr", "st", "esq", "jr", "sr",
    "vol", "chap", "p", "pp", "no", "etc", "i.e", "e.g", "vs", "viz", "cf",
    "sen", "mad", "gen", "col", "capt", "lieut", "rev", "hon", "prof",
}


def fetch(pid: int, cache_dir: Path, refresh: bool) -> str:
    """Return the source volume's text, cached on disk between runs."""
    cache_dir.mkdir(parents=True, exist_ok=True)
    cached = cache_dir / f"pg{pid}.txt"
    if cached.exists() and not refresh:
        return cached.read_text(encoding="utf-8")
    url = SOURCE_URL.format(id=pid)
    request = urllib.request.Request(url, headers={"User-Agent": "emb-demos-corpus/1.0"})
    for attempt in range(3):
        try:
            with urllib.request.urlopen(request, timeout=90) as response:
                text = response.read().decode("utf-8")
            break
        except (urllib.error.URLError, TimeoutError) as err:  # retry a flaky network
            if attempt == 2:
                sys.exit(f"fetch-poe: {url}: {err}")
            time.sleep(2 * (attempt + 1))
    if len(text) < 10_000:
        sys.exit(f"fetch-poe: {url} answered {len(text)} bytes; refusing a truncated source")
    cached.write_text(text, encoding="utf-8")
    return text


def strip_boilerplate(text: str, pid: int) -> str:
    """Everything outside the Gutenberg markers, and the markers' own edges."""
    start = START_MARKER.search(text)
    end = END_MARKER.search(text)
    if not start or not end or end.start() < start.end():
        sys.exit(f"fetch-poe: pg{pid} carries no recognizable START/END marker pair")
    body = text[start.end():end.start()]
    if "START: FULL LICENSE" in body:
        body = body.split("START: FULL LICENSE", 1)[0]
    return body


# Two transcription errata in the source volumes, corrected rather than shipped:
# a dropped space welds two words into one token that reads as our typo. Every
# entry is audit-able against the source, and the list is short on purpose.
ERRATA = {
    "Wherethe": "Where the",  # The City in the Sea, vol. 5
}


def reflow(raw: str) -> str:
    """Undo Gutenberg's hard wrapping and its `_emphasis_` convention.

    The underscores are emphasis delimiters, not characters: Gutenberg writes
    `_italic_`, and where the italic runs into the surrounding text it drops a
    space instead (`the _appearance_of that truth`), so removing them outright
    welds two words together (`appearanceof`, `aMaison`). An underscore that
    joins two word characters becomes the space it stands in for; the rest are
    delimiters and vanish, taking any space they orphaned before punctuation.
    """
    text = re.sub(r"(?<=\w)_(?=\w)", " ", raw)
    text = text.replace("_", " ")
    for wrong, right in ERRATA.items():
        text = text.replace(wrong, right)
    text = re.sub(r"[ \t]+", " ", text)
    text = re.sub(r"\s+([.,;:!?])", r"\1", text)
    text = re.sub(r"\n\s*\n\s*", "\n\n", text)
    paragraphs = [" ".join(line.strip() for line in block.splitlines() if line.strip())
                  for block in text.split("\n\n")]
    return "\n\n".join(p for p in paragraphs if p)


# Zero-width on the quote: a terminator followed by a closing quotation mark is
# still one boundary, and the mark belongs to the sentence rather than to the
# separator. Consuming it here would drop a `”` from every quoted sentence.
SENTENCE_END = re.compile(r"(?<=[.!?][\"'’”)\]])[ \t]+|(?<=[.!?])[ \t]+")


def split_sentences(paragraph: str) -> list[str]:
    """Split at sentence boundaries, guarding the abbreviations Poe leans on."""
    parts = SENTENCE_END.split(paragraph)
    sentences: list[str] = []
    for part in parts:
        if sentences:
            previous = sentences[-1]
            tail = previous.rstrip("\"'’”)]").rstrip(".")
            last = tail.split()[-1].lower() if tail.split() else ""
            if last in ABBREVIATIONS or (len(last) == 1 and last.isalpha()):
                sentences[-1] = f"{previous} {part}"
                continue
        sentences.append(part)
    return [s.strip() for s in sentences if s.strip()]


def split_long(sentence: str) -> list[str]:
    """Break a sentence that a single passage cannot hold, at its own clauses.

    Poe writes 200-word sentences. Sentence boundaries alone cannot bound a
    passage, and the bridge's 2 KiB text cap is a hard refusal, so an overlong
    sentence is broken at its punctuation first and only then at a word.
    """
    def fits(part: str) -> bool:
        return len(part.encode("utf-8")) <= SAFE_BYTES and len(part.split()) <= CHUNK_WORDS

    if fits(sentence):
        return [sentence]
    pieces: list[str] = []
    remainder = sentence
    while not fits(remainder):
        if len(remainder.split()) > CHUNK_WORDS:
            # Char offset of the word cap, so both bounds bite.
            window = " ".join(remainder.split()[:CHUNK_WORDS])
        else:
            window = remainder[: SAFE_BYTES - 1]
        cut = max(window.rfind(sep) + len(sep) for sep in (", ", "; ", ": ", " — "))
        if cut <= 0:
            cut = window.rfind(" ")
        if cut <= 0:
            cut = len(window)
        pieces.append(remainder[:cut].strip())
        remainder = remainder[cut:].strip()
    if remainder:
        pieces.append(remainder)
    return [p for p in pieces if p]


def chunk_work(text: str) -> list[str]:
    """Pack sentences into passages of at most CHUNK_WORDS words."""
    units: list[str] = []
    for paragraph in text.split("\n\n"):
        for sentence in split_sentences(paragraph):
            units.extend(split_long(sentence))

    passages: list[str] = []
    current: list[str] = []
    words = 0
    for unit in units:
        count = len(unit.split())
        over = (current and words + count > CHUNK_WORDS) or \
               (current and len(" ".join(current + [unit]).encode("utf-8")) > SAFE_BYTES)
        if over:
            passages.append(" ".join(current))
            current, words = [], 0
        current.append(unit)
        words += count
    if current:
        passages.append(" ".join(current))
    return [p for p in passages if len(p.split()) >= MIN_WORDS]


def slug(text: str) -> str:
    return re.sub(r"[^a-z0-9]+", "-", text.lower()).strip("-")


def slice_works(volume: str, pid: int) -> list[str]:
    """Split a volume at its work headings; anything unnamed is a boundary.

    Returns one body per manifest heading for this volume, in order, so a
    repeated heading (two "TO HELEN", three "NOTES") stays positional.
    """
    lines = volume.splitlines()
    headings = [heading for (v, heading, _, _) in WORKS if v == pid]
    # The contents page repeats most headings, so the body starts at the last
    # appearance of the volume's first work rather than at its contents entry.
    first = [i for i, line in enumerate(lines) if line.strip() == headings[0]]
    if not first:
        sys.exit(f"fetch-poe: pg{pid}: heading {headings[0]!r} does not appear as a line")
    positions: list[int] = [first[-1]]
    cursor = first[-1] + 1
    for heading in headings[1:]:
        found = [i for i in range(cursor, len(lines)) if lines[i].strip() == heading]
        if not found:
            sys.exit(f"fetch-poe: pg{pid}: heading {heading!r} does not follow its predecessor; the manifest has drifted from the source")
        positions.append(found[0])
        cursor = found[0] + 1

    bodies: list[str] = []
    for i, start in enumerate(positions):
        end = positions[i + 1] if i + 1 < len(positions) else len(lines)
        bodies.append(reflow("\n".join(lines[start + 1:end])))
    return bodies


def build(refresh: bool) -> list[dict]:
    passages: list[dict] = []
    for pid in SOURCES:
        volume = strip_boilerplate(fetch(pid, CACHE_DIR, refresh), pid)
        bodies = slice_works(volume, pid)
        listed = [(heading, title, year) for (v, heading, title, year) in WORKS if v == pid]
        for (heading, title, year), body in zip(listed, bodies):
            if year is None:
                continue
            for index, text in enumerate(chunk_work(body), start=1):
                if len(text.encode("utf-8")) > MAX_TEXT_BYTES:
                    sys.exit(f"fetch-poe: {title} passage {index} is {len(text.encode('utf-8'))} bytes, above the bridge's {MAX_TEXT_BYTES}")
                snippet = text[:SNIPPET_CHARS].rsplit(" ", 1)[0] + "…"
                passages.append({
                    "id": f"{slug(title)}-{index:03d}",
                    "work": title,
                    "year": year,
                    "index": index,
                    "text": text,
                    "snippet": snippet,
                })
    return passages


def assert_clean(passages: list[dict]) -> None:
    for passage in passages:
        lowered = passage["text"].lower()
        for marker in BOILERPLATE:
            if marker in lowered:
                sys.exit(f"fetch-poe: {passage['id']} carries boilerplate {marker!r}")
    ids = [p["id"] for p in passages]
    if len(set(ids)) != len(ids):
        sys.exit("fetch-poe: duplicate passage ids")
    short = [p["id"] for p in passages if len(p["text"].split()) < MIN_WORDS]
    if short:
        sys.exit(f"fetch-poe: {len(short)} passage(s) under {MIN_WORDS} words, e.g. {short[0]}")


def encode(passages: list[dict]) -> str:
    return "".join(json.dumps(p, ensure_ascii=False, sort_keys=True) + "\n" for p in passages)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true",
                        help="do not write; exit 1 if poe.jsonl differs from a fresh build")
    parser.add_argument("--verify", action="store_true",
                        help="re-assert the committed corpus's invariants without the network")
    parser.add_argument("--refresh", action="store_true",
                        help="re-fetch the source volumes instead of using .cache/")
    args = parser.parse_args()

    if args.verify:
        if not OUTPUT.exists():
            sys.exit(f"fetch-poe: {OUTPUT} is missing")
        passages = [json.loads(line) for line in OUTPUT.read_text(encoding="utf-8").splitlines() if line.strip()]
        assert_clean(passages)
        print(f"fetch-poe: ok ({len(passages)} passages · {len({p['work'] for p in passages})} works "
              f"· {sum(len(p['text'].split()) for p in passages)} words)")
        return 0

    passages = build(refresh=args.refresh)
    assert_clean(passages)
    output = encode(passages)

    digest = hashlib.sha256(output.encode("utf-8")).hexdigest()[:12]
    works = len({p["work"] for p in passages})
    words = sum(len(p["text"].split()) for p in passages)
    summary = (f"{len(passages)} passages · {works} works · {words} words · "
               f"sha256:{digest} · {len(output.encode('utf-8'))} bytes")

    if args.check:
        current = OUTPUT.read_text(encoding="utf-8") if OUTPUT.exists() else ""
        if current != output:
            print("fetch-poe: poe.jsonl is stale", file=sys.stderr)
            print(f"  committed: {len(current.splitlines())} line(s)", file=sys.stderr)
            print(f"  fresh:     {summary}", file=sys.stderr)
            print("  run: python3 website/tools/demos/fetch-poe.py", file=sys.stderr)
            return 1
        print(f"fetch-poe: ok ({summary})")
        return 0

    OUTPUT.write_text(output, encoding="utf-8")
    print(f"fetch-poe: wrote {OUTPUT.relative_to(REPO_ROOT)} ({summary})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
