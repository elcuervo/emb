# Spec Delta

## ADDED Requirements

### Requirement: A displayed score states its transform and shows its margin

The distribution a scripted image call displays SHALL be the model's own
similarity, or a stated transform of it. The plate SHALL name the transform,
SHALL show the model's similarity or the runner-up margin beside the winner, and
MUST NOT present the distribution as a probability that the input *is* the
winning label.

#### Scenario: The transform is stated

- **WHEN** a plate displays a distribution over labels
- **THEN** it names the transform it applied, so a reader can tell a cosine from a softmax

#### Scenario: A weak win reads as weak

- **WHEN** a label wins by a small margin
- **THEN** the runner-up's score is shown beside the winner's, so the margin is visible

#### Scenario: The model's own similarity is available

- **WHEN** a reader inspects a scored label
- **THEN** the model's similarity for that label is shown or disclosed, rather than only a rescaled percentage
