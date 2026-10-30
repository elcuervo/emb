# Spec Delta

## MODIFIED Requirements

### Requirement: The showcase surface is fixed and read-only

The bridge SHALL accept only a fixed set of commands and MUST refuse everything
else with a legible error. The set is chosen so a visitor can embed text, run
multiple models in one call, read model and server metadata, preload-call a
preset script, and negotiate the protocol version. Commands that change server
configuration, mutate shared state, load or flush scripts, persist state, stream
other visitors' requests, carry credentials, fetch a URL, or decode images
directly MUST be refused. Image bytes reach the server through two calls the
sandbox owns and no others: its **preloaded image preset**, and one shaped
`EMB.IMG` form naming the sandbox's **one image model** — the depth model
(output `depth`) — its `BLOB` representation, an optional subset of that
model's configured outputs, and exactly one image. `EMB.IMGMULTI`, `VALUES`, the
dense-features model and every other name, and any widening of that shape remain
refused, and the refusal names `depth`.

#### Scenario: A permitted command runs

- **WHEN** a visitor sends an embedding, a multi-model call, a metadata read, a preset script call, or the shaped image command for the depth model
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

- **WHEN** a visitor names a binary argument for a script call, a preset, or any command other than the image calls this capability allows
- **THEN** the bridge refuses it, and the image preset remains the only scripted call that accepts bytes

#### Scenario: The shaped image command cannot be widened

- **WHEN** a visitor sends `EMB.IMGMULTI`, a `VALUES` image reply, an image command naming the dense-features model or any other, an output name the depth model does not configure, or more than one image in one call
- **THEN** the bridge refuses it and names the one image form and the one permitted model, and the server receives no widened command

#### Scenario: An image reaches the server only as bounded bytes

- **WHEN** the image preset or the shaped image command is called with a binary argument
- **THEN** the bridge enforces the image count and byte caps, decodes the argument to raw bytes, and forwards the command, and the bytes are used for that request only

### Requirement: Replies are represented without lossy text encoding

A reply SHALL be delivered to the client as structured data that preserves the
kind of every value, including nested aggregates, typed numbers, nulls, errors,
and binary values. Whether a command's bulk values are binary SHALL follow from
the representation the command asked for, never from a heuristic over the bytes
it returned, so a payload that happens to be valid text is still carried as
bytes. A binary value MUST NOT be delivered through a text encoding that can
alter it; it SHALL be delivered with a faithful representation and enough
metadata for the client to present it without re-decoding a model.

#### Scenario: An embedding survives the bridge

- **WHEN** an embedding is returned as raw bytes
- **THEN** the client receives a representation that identifies it as a binary vector, reports its element count and type, and does not corrupt it

#### Scenario: A binary grid survives the bridge

- **WHEN** the image command asks for a patch grid and the returned bytes are also valid text
- **THEN** the client still receives them as binary values with their element count and type, because the command asked for `BLOB`

#### Scenario: A binary map survives the bridge

- **WHEN** the image command asks for a two-dimensional map and the returned bytes are also valid text
- **THEN** the client still receives them as binary values with their element count and type, because the command asked for `BLOB`, and reads the map's rows and dimension from `EMB.INFO`

#### Scenario: Aggregate shapes survive the bridge

- **WHEN** a command returns a map, an array, a typed double, a null, or an error
- **THEN** the client receives the same kind and order, so a multi-model reply, a metadata reply and an image reply stay distinguishable

#### Scenario: An error is distinguishable from a value

- **WHEN** the server returns an error
- **THEN** the client receives an error kind, not a string that looks like a successful value

### Requirement: The configured model set fits the machine

The shipped server configuration and the machine size SHALL be chosen together
so the resident memory of the loaded model set stays inside the machine's limit
through a boot and its warm-up, because the sandbox loads every configured model
at startup and the platform replaces a machine whose server is killed for
memory. Every model added for a demo SHALL be accounted for at its resident
cost rather than its file size, and its first call on a shared machine MUST
answer inside the bridge's per-command deadline.

#### Scenario: The size follows the resident cost, not the file size

- **WHEN** the sandbox's machine size and model configuration are inspected
- **THEN** the size accounts for the resident cost of the weights, which the CPU execution provider multiplies several times over their files
- **AND** the checkpoint that dominates the footprint mounts a pre-quantized export

#### Scenario: A boot completes without a kill

- **WHEN** the sandbox boots with the shipped configuration and its models already on the volume
- **THEN** it reaches ready without the platform killing the server for memory, and it stays up while the preloaded presets warm their replies

#### Scenario: A cold image call answers on the shared machine

- **WHEN** the depth model is called through the bridge for the first time since boot
- **THEN** the reply arrives inside the bridge's deadline, and the reply reports the server-measured call time rather than a client estimate

#### Scenario: The image model set is one, and its cost is accounted for

- **WHEN** the sandbox boots with the shipped configuration
- **THEN** `EMB.MODELS` lists one image model, its resident cost is accounted for in the CPU plan, its quantized export is the one mounted, and the dense-features model is absent from the configuration and from the bridge's surface
