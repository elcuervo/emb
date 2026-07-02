## MODIFIED Requirements

### Requirement: onnxruntime gem dependency

The gemspec SHALL declare `onnxruntime ~> 0.11` as a runtime dependency, ensuring ORT 1.27.0+ is bundled.

#### Scenario: ORT runtime version compatibility

- **GIVEN** the `emb-server` gem is installed in a project
- **WHEN** `bundle exec emb -model-repo Xenova/all-MiniLM-L6-v2` is run
- **THEN** the Gemfile.lock SHALL resolve to `onnxruntime ~> 0.11`
- **THEN** the bundled ORT SHALL be version 1.27.0 or newer
- **THEN** the server SHALL start and accept EMB commands
