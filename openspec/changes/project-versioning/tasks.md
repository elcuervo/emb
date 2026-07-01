## 1. Root VERSION file

- [x] 1.1 Create `VERSION` at repo root with content `0.1.0`

## 2. Gem version paths

- [x] 2.1 Update `gems/emb/emb.gemspec` to read `../../VERSION` relative to the gemspec directory
- [x] 2.2 Remove `gems/emb/VERSION` (replaced by gemspec reading root directly)
- [x] 2.3 Update `gems/emb-server/emb-server.gemspec` to read `../../VERSION`
- [x] 2.4 Remove `gems/emb-server/VERSION` (replaced by gemspec reading root directly)
- [x] 2.5 Remove `gems/emb-server/lib/emb-server/version.rb` — version is in gemspec only, no runtime require needed

## 3. Justfile version

- [x] 3.1 Change `image_tag` in `justfile` to read root `VERSION` file instead of `git describe`

## 4. CI version injection

- [x] 4.1 Add a step at the start of `release.yml` that writes the tag version to root `VERSION` when triggered by a tag push
