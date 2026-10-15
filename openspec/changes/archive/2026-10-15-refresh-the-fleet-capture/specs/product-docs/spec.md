# Spec Delta

## MODIFIED Requirements

### Requirement: The docs surface is the hosted form of the reference

The site SHALL carry a documentation surface at `website/docs/index.html`.
`README.md` remains the source of truth; the docs surface MUST NOT introduce a
capability, command, reply shape, configuration key, or number the repository
does not support, and it MUST NOT contradict `README.md`, `BENCHMARK.md`,
`examples/scripts/`, or the shipped code. The reference it carries SHALL name the
same fields and the same scope the shipped `docs/commands.md` and
`docs/operations.md` name for each command and counter, so a surface entry cannot
describe an older wire shape than the version the surface stamps.

#### Scenario: A documented command exists in the reference

- **WHEN** the docs surface presents a Redis command, its arity, or its reply shape
- **THEN** that command, arity, and reply shape appear in `README.md`

#### Scenario: A documented number carries provenance

- **WHEN** the docs surface presents a measured number
- **THEN** that number appears in `BENCHMARK.md` together with the machine and method or the reproduction command that produced it

#### Scenario: A single fact has one home

- **WHEN** a fact appears on both the landing page and the docs surface
- **THEN** the docs surface carries the complete form of the fact, the landing carries at most one line of it, and the two cannot disagree about a value

#### Scenario: A reply's fields reach the surface

- **WHEN** a command's reply gains a field and the shipped `docs/commands.md` names it
- **THEN** the docs surface's entry for that command names the same fields, so a reader on the site cannot learn an older wire shape than the version the surface stamps

#### Scenario: A counter's scope reaches the surface

- **WHEN** a counter's scope changes in the shipped code and `docs/operations.md` states the new scope
- **THEN** the docs surface states the new scope rather than the one the previous release had

#### Scenario: The surface names no field the release does not emit

- **WHEN** the surface's command and reply entries are checked against the shipped code
- **THEN** every field they name is one the server emits
