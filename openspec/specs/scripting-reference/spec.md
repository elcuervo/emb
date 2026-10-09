# scripting-reference Specification

## Purpose
TBD - created by archiving change add-scripting-reference-surface. Update Purpose after archive.

## Requirements

### Requirement: A scripting reference surface exists

The site SHALL carry a reference surface at `website/scripting/index.html`
documenting the complete scripting engine. It SHALL be a **Read** surface: it
explains, and it inherits the shared world in `DESIGN.md` and `assets/css/styles.css`
plus the `assets/css/docs.css` primitives, adding no stylesheet, colour, font, or
component language of its own.

#### Scenario: The surface exists and is reachable

- **WHEN** a reader opens `/scripting/`
- **THEN** the page renders the scripting engine's properties as its own
  reference surface, linked from the primary nav on every page

#### Scenario: The world is inherited, not forked

- **WHEN** the surface is rendered
- **THEN** it uses the shared tokens, footers, masthead, block grounds and type
  floors, and introduces no second palette, font, card grid, gradient, rounded
  panel or drop shadow

#### Scenario: The page reads without scripting

- **WHEN** JavaScript is disabled
- **THEN** the full reference is complete and legible, and no control is left inert

### Requirement: The reference documents the script lifecycle and sandbox

The surface SHALL document how a script is loaded and run and the environment it
runs in: the commands `EMB.EVAL`, `EMB.EVSHA`, and `EMB.SCRIPT LOAD|EXISTS|FLUSH`;
boot preloading through the model config's `scripts:` entries and
`emb.script.config`; and the sandbox — the stripped libraries and the
determinism guarantee.

#### Scenario: A host command is documented

- **WHEN** the surface lists a scripting command
- **THEN** that command exists in the shipped server with the stated arity and
  reply shape

#### Scenario: The sandbox is stated

- **WHEN** a reader looks for what a script cannot do
- **THEN** the surface names the removed libraries and the pure-compute guarantee

### Requirement: The reference carries the complete host surface

The surface SHALL list every host function a script can call, sourced from
`internal/script`, and SHALL omit none: `emb.API_VERSION`, `emb.run`,
`emb.run_batch`, `emb.embed`, `emb.similarity`, `emb.distance`, the
`emb.tokenize.*` block, the `emb.image.*` block, every `emb.math.*` operation,
and `json.encode` / `json.decode` / `json.decode_ordered` / `json.null`.

#### Scenario: No host function is omitted

- **WHEN** the surface's host-function list is compared with the registered
  surface
- **THEN** every registered function appears and no unregistered function is named

### Requirement: The reference carries tensors, replies, budgets and identity

The surface SHALL document the tensor input spec (`data` / `fill` / `bytes`,
dtypes `f32`/`i64`/`b1`, packed little-endian form), the Lua-to-RESP reply
grammar, every evaluation budget and output cap with its value, and the per-model
SHA1 script cache with its reply-cache key composition.

#### Scenario: A budget carries its value

- **WHEN** the surface presents an evaluation limit or output cap
- **THEN** it states the value the shipped code enforces

#### Scenario: The version is derived, never typed

- **WHEN** the surface displays the scripting version
- **THEN** the value is stamped from `VERSION` rather than transcribed
