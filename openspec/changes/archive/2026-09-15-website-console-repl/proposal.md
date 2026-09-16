## Why

The console is the only place on the site where a developer can *use* `emb`
instead of reading about it, and it opens as a demonstrator: two mode tabs, a
protocol dropdown, a RUN button, and an idle line that teaches nothing. Every one
of those is a control someone has to operate before they get a vector back. The
person this panel exists for has never installed `emb` and should be holding a
reply within one click, then keep typing.

Its design also stopped short of the surface it is: the plate is already a
terminal, but it is dressed as a two-mode panel with a form bolted underneath it.

The same recording the landing page plays is absent from the documentation
surface, where a reader arrives precisely to find out what `emb-top` is.

## What Changes

- **The console becomes a REPL.** One prompt, one transcript, one status strip.
- **The `REDIS` / `SCRIPTS` mode tabs are removed.** The script demonstration
  becomes an example command rather than a mode; both stamped preset digests stay
  in use, so the extensibility proof stays on the page without a mode to pick.
- **Examples are the idle state.** The empty box that currently holds one hint
  sentence becomes a ruled, numbered list of real permitted commands, each one
  click to run. A clicked example is a submitted command, so it lands in history.
- **Command history.** `ArrowUp` / `ArrowDown` walk previously submitted commands,
  stopping at both ends and preserving the draft being typed. `Enter` submits,
  as it does today.
- **The bar becomes a status strip**, carrying the console's own state and the
  one remaining choice.
- **`RESP 2|3` stays.** The selective reply shape is the product's headline
  behaviour and a spec'd, observable difference; it is restyled, not removed.
- **The `RUN` button becomes a quiet `↵` submit affordance** rather than being
  deleted, because a soft keyboard and a touch target need it.
- **The standalone terminal receives the same treatment**, so the two surfaces
  driven by the one client module do not drift into different consoles.
- **The documentation surface gains the `emb-top` plate**: the run's still frame
  with an explicit play control, never autoplay. This retires the "the
  documentation surface ships no JavaScript" invariant, which is a documented
  property of `website/README.md` and not an assertion anywhere in the build.
- **The plate's driver is extracted** from the landing-only `main.js` into a
  module both pages load, so the plate has one implementation and two callers
  rather than one implementation with a surface conditional.
- **BREAKING** (spec-level, not to any consumer): the `product-site` requirement
  *Console modes demonstrate the two special functions* is replaced.

## Capabilities

### New Capabilities

None. This change alters the behaviour of a capability the project already
declares, and introduces no new one.

### Modified Capabilities

- `product-site`: the console's modes are replaced by an examples-led REPL with
  command history; the console's idle state and its state set are restated; the
  documentation surface is required to carry the `emb-top` plate, and the
  no-scripting guarantee is restated for a page that now ships a script.

`site-deployment` needs no delta: its served-tree requirement already says the
origin serves "the assets those three reference", which is exactly what the
extracted plate module is. `emb-top-capture` needs none either: the plate on the
documentation surface plays the same recording the landing page plays, which its
requirements already permit and already forbid-as-an-image.

## Impact

- `website/index.html` — console markup (bar, idle state, form).
- `website/docs/index.html` — a new plate in *06 · Operations*, plus the player
  and plate module references.
- `website/repl/index.html` — the same console simplification.
- `website/assets/js/main.js` — loses the console's tab machinery and the plate
  driver.
- `website/assets/js/topviz.js` — new; the plate driver, loaded by both pages.
- `website/assets/css/styles.css` — the console bar, mode and example rules; the
  plate rules move from landing-only to shared.
- `website/assets/css/docs.css` — the plate's placement inside the reference
  measure.
- `website/repl/terminal.js` — gains recall over submitted commands. It is served
  from the sandbox origin, so the sandbox must be redeployed for the landing page
  to see it.
- `website/tools/topviz/publish.py` — a third stamp target, so the documentation
  plate's frame, cast, caption and figures are written rather than typed.
- `website/tools/published-tree.py` — the served set gains the new module.
- `website/README.md` — the console's description and the retired JavaScript
  invariant.

Nothing in the server, the client gems, the bridge, or the recorded cast changes.
