## Context

Two files in the live project tree still reference the old Docker image name `elcuervo/emb-server`. Archived change artifacts also contain the old name but are historical records — they should not be modified.

## Goals / Non-Goals

**Goals:**
- Replace all `emb-server` references in live project files with `emb`
- Keep archived changes untouched (historical records)

**Non-Goals:**
- Renaming Go packages or binary names (already `emb`)
- Modifying archived/closed changes

## Decisions

### Scope: live files only

Only two files need changes:

| File | Occurrence | Replacement |
|------|------------|-------------|
| `README.md:203` | `elcuervo/emb-server \` | `elcuervo/emb \` |
| `openspec/specs/docker-build/spec.md:17,24` | `elcuervo/emb-server` | `elcuervo/emb` |

Archived changes in `openspec/changes/archive/` are snapshots of past work. They document what was built at the time. Modifying them would rewrite history.

### already-renamed files

The following files already use `elcuervo/emb` and need no changes: `.goreleaser.yaml`, `release.yml`, `justfile`. These were updated in the `goreleaser-release` change.

## Risks / Trade-offs

- [Archived changes show old name] → Intentional — they're historical records. Readers can see the rename happened between changes.
