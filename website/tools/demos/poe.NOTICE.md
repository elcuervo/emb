# The gallery's corpus

`fetch-poe.py` builds `poe.jsonl` from five Project Gutenberg plain-text
editions of *The Works of Edgar Allan Poe* (the Raven Edition). This file is the
attestation that goes with it: what the corpus is, where it came from, and what
the page must say when it shows a passage.

## Status

**Public domain.** Edgar Allan Poe died in 1849; the works are in the public
domain in the United States and in every jurisdiction where the gallery is
served. The Gutenberg editions of them carry no additional copyright: Project
Gutenberg's own claim is on its trademark and on the boilerplate, not on the
public-domain text, and the boilerplate is stripped before anything is
embedded.

## Sources

| Volume | Project Gutenberg | Works shipped |
|---|---|---|
| *The Works of Edgar Allan Poe — Volume 1* | [#2147](https://www.gutenberg.org/ebooks/2147) | 8 |
| *The Works of Edgar Allan Poe — Volume 2* | [#2148](https://www.gutenberg.org/ebooks/2148) | 22 |
| *The Works of Edgar Allan Poe — Volume 3* | [#2149](https://www.gutenberg.org/ebooks/2149) | 7 |
| *The Works of Edgar Allan Poe — Volume 4* | [#2150](https://www.gutenberg.org/ebooks/2150) | 21 |
| *The Works of Edgar Allan Poe — Volume 5* | [#2151](https://www.gutenberg.org/ebooks/2151) | 28 |

Plain-text URLs are `https://www.gutenberg.org/cache/epub/<id>/pg<id>.txt`; the
tool fetches, caches them under `.cache/` (not committed), and slices each
volume at the work headings listed in `fetch-poe.py`. A work the manifest does
not name is a boundary, not text folded into its neighbour, which is why the
manifest is a table of contents rather than a filter.

## Attribution the page carries

> Texts: Edgar Allan Poe (1809–1849), public domain, via Project Gutenberg.

Every plate that shows a passage shows the passage's work and year beside it,
and the gallery index carries the line above. The passages are quoted, not
reproduced as a collection: the corpus is a demonstration's index, and its
source is named where a reader meets it.

## Acquisition

`python3 website/tools/demos/fetch-poe.py` — deterministic given the same
Gutenberg bytes, so a rebuild is a diff. It asserts:

* each source carries a recognizable `*** START/END OF THE PROJECT GUTENBERG
  EBOOK ... ***` marker pair, and nothing outside them survives;
* no boilerplate marker (`Project Gutenberg`, `gutenberg.org`, `PGLAF`,
  `START: FULL LICENSE`, `*** START`, `*** END`) appears anywhere in the output;
* every passage id is unique, and every passage is under the bridge's 2 KiB
  per-text cap.

`--check` re-derives the corpus and fails if the committed `poe.jsonl` differs.
