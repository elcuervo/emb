## 1. flake.nix — add act

- [x] 1.1 Add `act` to `flake.nix` devShell `buildInputs`

## 2. Switch to trusted publishing

- [x] 2.1 Add `permissions: contents: read, id-token: write` and `environment: release` to `release-emb` job
- [x] 2.2 Add same permissions and environment to `release-emb-server` job
- [x] 2.3 Add `rubygems/configure-rubygems-credentials@main` step before `gem push` in both jobs
- [x] 2.4 Remove `RUBYGEMS_API_KEY` env from both jobs

## 3. Post-build gem validation in release workflow

- [x] 3.1 In `release-emb`, after `gem build` add `gem install --local` + `ruby -e "require 'emb'; puts Emb::VERSION"`
- [x] 3.2 In `release-emb-server`, after `gem build` add `gem install --local` + `which emb` + `emb -version`

## 4. Local validation recipe

- [x] 4.1 Add `validate-gems` recipe to `justfile` that builds, installs, and smokes both gems

## 5. Align gem CI triggers

- [x] 5.1 Add `tags: ["v*"]` and `workflow_dispatch` to `gems/emb/.github/workflows/test.yml`
- [x] 5.2 Create `gems/emb-server/.github/workflows/test.yml` with matching trigger pattern and gem build validation
