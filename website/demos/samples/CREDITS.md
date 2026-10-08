# Image-plate samples

All six are public domain, fetched from Wikimedia Commons, downscaled to a 512px
long edge and re-encoded as JPEG for the gallery. They ship with the page; a
visitor uploads nothing.

| File | Work | Source |
|---|---|---|
| `raven.jpg` | Gustave Doré, illustration for *The Raven* (1884) | Wikimedia Commons |
| `manuscript.jpg` | Gustave Doré, *The Raven* (1884) | Wikimedia Commons |
| `storm.jpg` | Ivan Aivazovsky, *The Ninth Wave* (1850) | Wikimedia Commons |
| `portrait.jpg` | *Edgar Allan Poe*, daguerreotype attributed to W. S. Hartshorn (1849) | Wikimedia Commons |
| `ship.jpg` | Fitz Henry Lane, *The Ships "Winged Arrow" and "Southern Cross" in Boston Harbor* (1853) | Wikimedia Commons |
| `flowers.jpg` | Vincent van Gogh, *Sunflowers* (1888) | Wikimedia Commons |

# Media-plate assets

The media plate (`demos/medium.html`) ships every medium it reads and plays; a
visitor uploads nothing. The fingerprint database and the frame vectors are
built from these committed files by `website/tools/build-fingerprints.py` and
`website/tools/build-frame-index.py`, so the shipped audio/video and the shipped
hashes/vectors are the same bytes.

## Music (the audio the plate identifies)

Five short excerpts, ten to fifteen seconds each, re-encoded mono at 96 kbps for
the gallery. The six query excerpts under `music/queries/` are four-second cuts
of these (one degraded with pink noise), shipped so the plate can fingerprint a
real query without an upload.

| File | Title | Author | Licence |
|---|---|---|---|
| `music/brass.mp3` | Adjutant's Call | Ceremonial Brass, United States Air Force Band | Public domain (US government work) |
| `music/flute.mp3` | DiZi Chinese Flute Sample | Gorgoroth6669 | CC0 1.0 |
| `music/steel.mp3` | Steel guitar playing Hawaiian music | Eagledj | CC0 1.0 |
| `music/guitar.mp3` | PipingOfQueens | U Can Unlearn Guitar (Free Music Archive) | CC0 1.0 |
| `music/piano.mp3` | Prelude No. 14 in E-flat minor, Op. 28 | Frédéric Chopin, performed by Ivan Ilić | CC BY 3.0 |

All five are from Wikimedia Commons. The two CC BY 3.0 works (only `piano.mp3`
here) require attribution: **"Prelude No. 14 in E-flat minor, Op. 28" by Ivan
Ilić, licensed CC BY 3.0 (https://creativecommons.org/licenses/by/3.0/), via
Wikimedia Commons.** The other four carry no attribution obligation; credited
here anyway.

## Video (the clip the plate searches)

`clips/mallard.mp4` — "Mallard Duck Swims By Camera (Wide Screen)", from the
Internet Archive's `stock_footage` collection, dedicated to the public domain
under the Creative Commons Public Domain Dedication and Certification
(`http://creativecommons.org/licenses/publicdomain/`), no attribution required.
Its sixteen stills under `media/frames/` are cut from it with ffmpeg and embedded
by the sandbox's own model.
