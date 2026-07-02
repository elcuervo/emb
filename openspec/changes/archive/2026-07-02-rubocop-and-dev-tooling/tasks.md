## 1. Add RuboCop to emb gem

- [x] 1.1 Add `rubocop`, `rubocop-rspec`, `rubocop-rake` to gems/emb/Gemfile
- [x] 1.2 Create `.rubocop.yml` in gems/emb/ with config targeting Ruby 3.3, enabling rspec and rake cops
- [x] 1.3 Run `bundle exec rubocop -A` and fix all offenses
- [x] 1.4 Create `console` Rake task in gems/emb/Rakefile
- [x] 1.5 Verify `bundle exec rubocop` passes with zero offenses

## 2. Add RuboCop to emb-server gem

- [x] 2.1 Add `rubocop`, `rubocop-rake` to gems/emb-server/Gemfile
- [x] 2.2 Create `.rubocop.yml` in gems/emb-server/ with config targeting Ruby 3.3
- [x] 2.3 Run `bundle exec rubocop -A` and fix all offenses
- [x] 2.4 Create Rakefile with `console` and `rubocop` tasks
- [x] 2.5 Verify `bundle exec rubocop` passes with zero offenses

## 3. Fix just dev workflow

- [x] 3.1 Verify `config.yaml` is valid and has required models
- [x] 3.2 Fix `ort_lib` resolution in justfile to handle missing DYLD_LIBRARY_PATH gracefully
- [x] 3.3 Verify `just dev` builds and starts the server

## 4. Add just console recipe

- [x] 4.1 Add `just console` recipe that runs `bundle exec rake console` in gems/emb/
