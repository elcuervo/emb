## 1. Rename emb-server to emb in live files

- [x] 1.1 Update `README.md` — replace `elcuervo/emb-server` with `elcuervo/emb` in Docker run example
- [x] 1.2 Update `openspec/specs/docker-build/spec.md` — replace `elcuervo/emb-server` with `elcuervo/emb`
- [x] 2.1 Confirmed: zero `emb-server` references remain in live project files
- [x] 2.2 `go build ./...` — passes
- [x] 2.3 `go test ./...` — passes
