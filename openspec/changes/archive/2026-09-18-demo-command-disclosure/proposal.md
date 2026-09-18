## Why

The demos' `THE EXACT COMMANDS` section is static prose: a placeholder argv
(`<embed.lua sha1>`) and, on some plates, a command line rebuilt by hand in
the plate's script. None of it is the command the reader's own text produced,
so the spec's "the command shown to the reader is the command that was issued"
holds only for the model name and reply form — not for the arguments. Running
a demo is the one moment the real command exists, and today nothing shows it.

## What Changes

- Each of the eleven plates gains a disclosure in its live rig that unfolds the
  exact argv the plate issued during the reader's most recent run, with the
  reader's own arguments and the real preset digest.
- The fold is a native `<details>`; only the command text is scripted. It
  appears after a run and reflects that run, including a run that failed or was
  refused.
- The gallery module records every command at its one choke point (`exec`) and
  exposes it to the plate; a shared renderer prints the argv in the site's
  existing `code` / `t-cmd` atoms.
- Commands only: no server timing, no reply shape, and no browser-side
  sqlite-vec SQL. The disclosure is the argv, not a receipt.
- The hand-built command lines (`similarity.html`'s `source.textContent`, the
  unused `argvLine` import in `vector.html`) are removed; the disclosure is the
  single source for what a run sent.
- The clamp that keeps a long command from becoming a page also applies to the
  passages a plate quotes, so a corpus passage shows a few lines rather than its
  whole length.
- Each plate that has a next plate links to it from the top of the head as well
  as from the rail at the foot, in the rail's own vocabulary.
- The static `THE EXACT COMMANDS` section is unchanged and remains the no-JS
  reference, keeping its role distinct from the run's own disclosure.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `embedding-demos`: the requirement that the commands shown are the commands
  that ran gains a post-run disclosure, and its scenario covers the reader's own
  arguments rather than the model name and reply form alone.

## Impact

- `website/assets/js/demos.js`: command trace at `exec`, a `begin`/`trace` pair
  on the gallery, and one shared disclosure renderer beside `argvLine`.
- `website/demos/*.html` (11 plates): rig markup for the disclosure and a
  `begin`/render call in each plate's `run()`.
- `website/assets/css/styles.css`: disclosure atoms, reusing existing rules.
- No server, bridge, protocol, or Ruby client change. The static section, the
  no-JS reading, and the print view are unaffected.
