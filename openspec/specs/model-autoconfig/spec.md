## ADDED Requirements

### Requirement: Image preprocessing autodetection

When a model declares an `image:` block but omits fields, the server SHALL auto-detect them where possible: the image input tensor name and target size from the ONNX graph's image-input dimensions, and the rescale, mean, std, crop, and resample settings from the model's `preprocessor_config.json` when that file is present. Explicit configuration SHALL always override auto-detected values. When a required value cannot be determined, the server SHALL fail model loading with a descriptive error naming the missing field rather than guessing.

#### Scenario: Size detected from the ONNX graph

- **WHEN** an `image:` block omits `size` and the ONNX image input has static spatial dimensions `[batch, 3, H, W]`
- **THEN** the server uses `H`/`W` as the configured size

#### Scenario: Preprocessing constants detected from preprocessor_config.json

- **WHEN** the model directory contains `preprocessor_config.json` with image mean/std/rescale/size
- **THEN** the server uses those values for preprocessing

#### Scenario: Explicit config wins

- **WHEN** an `image:` block sets `mean` explicitly and `preprocessor_config.json` provides a different mean
- **THEN** the explicit configured mean is used

#### Scenario: Undetectable required field errors

- **WHEN** the size cannot be determined from the graph or config files and is not set explicitly
- **THEN** the server fails model loading with a descriptive error
