## Why

The Ruby gems (`emb` client, `emb-server`) lack linting, consistent code style enforcement, and a convenient dev workflow. This leads to formatting inconsistencies, missed issues, and friction when validating changes during development.

## What Changes

- Add RuboCop configuration to `gems/emb/` and `gems/emb-server/`
- Add RuboCop and related dev gems to both Gemfiles
- Fix all RuboCop warnings across both gems
- Add a `console` Rake task to both gems for loading an `irb` session with the gem
- Add a `just console` recipe for convenience
- Fix `just dev` workflow so it builds and starts the server with the correct config

## Capabilities

### New Capabilities

- `rubocop-dev-tooling`: Shared RuboCop configuration, linting Rake tasks, and interactive console for validating gem code during development

### Modified Capabilities

- `emb-ruby-client`: Rakefile updated with `console` and `rubocop` tasks; Gemfile updated with dev dependencies

## Impact

- `gems/emb/Gemfile` — add `rubocop`, `rubocop-rspec`, `rubocop-rake` dev gems
- `gems/emb-server/Gemfile` — add `rubocop`, `rubocop-rake` dev gems  
- `gems/emb/` — add `.rubocop.yml`, update `Rakefile`
- `gems/emb-server/` — add `.rubocop.yml`, update `Rakefile` (create if needed)
- Multiple `.rb` files — style fixes from RuboCop auto-correct
- `justfile` — fix `dev` recipe (ensure `ort_lib` resolution works), add `console` recipe
- Root `config.yaml` — verify it exists and is valid for `just dev`
