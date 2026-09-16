## MODIFIED Requirements

### Requirement: The showcase surface is fixed and read-only

The bridge SHALL accept only a fixed set of commands and MUST refuse everything
else with a legible error. The set is chosen so a visitor can embed text, run
multiple models in one call, read model and server metadata, preload-call a
preset script, and negotiate the protocol version. Commands that change server
configuration, mutate shared state, load or flush scripts, persist state, stream
other visitors' requests, carry credentials, or decode images directly MUST be
refused. The sandbox's **own preloaded image preset** is the one call that may
receive image bytes, and only as a bounded binary argument the bridge decodes
back to raw bytes; `EMB.IMG`/`EMB.IMGMULTI` themselves remain refused, so no
visitor can send an image to the server except through a script the sandbox
owns.

#### Scenario: A permitted command runs

- **WHEN** a visitor sends an embedding, a multi-model call, a metadata read, or a preset script call
- **THEN** the bridge forwards it and returns the server's reply

#### Scenario: A configuration change is refused

- **WHEN** a visitor attempts to set a configuration value, flush the cache or scripts, save state, authenticate, or monitor traffic
- **THEN** the bridge refuses it and names the permitted surface, and the server receives no such command

#### Scenario: A refusal is legible, not silent

- **WHEN** a command outside the permitted set is sent
- **THEN** the reply is an error in the same shape as the permitted replies, with a hint toward what is available

#### Scenario: A permitted command cannot be widened

- **WHEN** a permitted command is sent with a subcommand, model, or argument outside its permitted shape
- **THEN** the bridge refuses it rather than forwarding it

#### Scenario: The image preset is the only image path

- **WHEN** a visitor sends `EMB.IMG` or `EMB.IMGMULTI`, or names a binary argument for any command but the sandbox's own image preset
- **THEN** the bridge refuses it, and the server receives no image command

#### Scenario: An image reaches the server only as bounded bytes

- **WHEN** the image preset is called with a binary argument
- **THEN** the bridge enforces the image count and byte caps, decodes the argument to raw bytes, and forwards the command, and the bytes are used for that request only
