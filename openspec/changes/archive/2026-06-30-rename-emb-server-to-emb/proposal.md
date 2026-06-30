## Why

The project is named `emb`, but several files still reference `emb-server` — the Docker image name, spec requirements, and README examples. This inconsistency causes confusion: wrong image names in deployment, outdated docs, and mismatched expectations. The project is `emb`. Every reference should say `emb`.

## What Changes

- `README.md`: Update Docker run example from `elcuervo/emb-server` to `elcuervo/emb`
- `openspec/specs/docker-build/spec.md`: Update Docker Hub image references from `elcuervo/emb-server` to `elcuervo/emb`

## Modified Capabilities

- `docker-build`: Docker image name updated to `elcuervo/emb`

## Impact

| File | Change |
|------|--------|
| `README.md` | Docker run command: `elcuervo/emb-server` → `elcuervo/emb` |
| `openspec/specs/docker-build/spec.md` | Requirement text: `elcuervo/emb-server` → `elcuervo/emb` |
