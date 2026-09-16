# sandbox-service Specification

## Purpose

A public, read-only showcase runtime that lets a visitor run real `emb`
commands from the browser and see the real replies, without exposing the server
to the internet or giving a stranger any way to change its configuration or
spend without bound.

## Requirements

### Requirement: One public surface, server on loopback

The sandbox SHALL expose exactly one public surface: a bridge that accepts the
clients' requests and forwards an allowlisted subset to a single `emb` process
running on loopback in the same machine. The `emb` listener MUST NOT be
reachable from outside the machine, and the browser MUST NOT be given any
credential for it.

#### Scenario: The server port is not public

- **WHEN** a request is made to the sandbox host on the server's own protocol port
- **THEN** it is not answered by `emb`

#### Scenario: No credential reaches the browser

- **WHEN** the client is inspected in a browser
- **THEN** it carries no password, token, or server address, because the bridge is the only party that speaks to `emb`

#### Scenario: The bridge is the only listener that talks to the server

- **WHEN** the server's configuration is inspected
- **THEN** it binds loopback only, so a misconfigured public route cannot reach it

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

### Requirement: Preset scripts are owned by the sandbox and called by digest

The sandbox SHALL ship a small set of preset scripts and MUST preload them into
the server so a visitor can evaluate one without sending Lua source. The digest
a client uses to call a preset MUST be derived from the same bytes the server
loaded, so the digest and the output cannot disagree.

#### Scenario: A preset is called without sending its source

- **WHEN** a visitor evaluates a preset
- **THEN** the request names the preset by its digest and carries only the runtime arguments, and no Lua source is accepted from the visitor

#### Scenario: The displayed digest matches the loaded script

- **WHEN** a preset's digest is shown on the site
- **THEN** it is the digest of the bytes the server preloaded, checked rather than transcribed

#### Scenario: Arbitrary script loading is refused

- **WHEN** a visitor attempts to load, flush, or evaluate a script that is not a preset
- **THEN** the bridge refuses it

### Requirement: Replies are represented without lossy text encoding

A reply SHALL be delivered to the client as structured data that preserves the
kind of every value, including nested aggregates, typed numbers, nulls, errors,
and binary values. A binary embedding MUST NOT be delivered through a text
encoding that can alter its bytes; it SHALL be delivered with a faithful
representation and enough metadata for the client to present it without
re-decoding a model.

#### Scenario: An embedding survives the bridge

- **WHEN** an embedding is returned as raw bytes
- **THEN** the client receives a representation that identifies it as a binary vector, reports its element count and type, and does not corrupt it

#### Scenario: Aggregate shapes survive the bridge

- **WHEN** a command returns a map, an array, a typed double, a null, or an error
- **THEN** the client receives the same kind and order, so a multi-model reply and a metadata reply stay distinguishable

#### Scenario: An error is distinguishable from a value

- **WHEN** the server returns an error
- **THEN** the client receives an error kind, not a string that looks like a successful value

### Requirement: The protocol version is a real transport choice

The sandbox SHALL be able to obtain both the RESP2 and the RESP3 form of a
reply, so a visitor can see the difference between the flat and the typed
encoding of the same command. The chosen version MUST be applied to the server
connection that carries the request rather than simulated before or after it.

#### Scenario: RESP3 shows typed values

- **WHEN** a visitor selects RESP3 and runs a command whose reply differs by protocol
- **THEN** the reply shows the RESP3 form, including typed numbers and maps, as returned by the server

#### Scenario: RESP2 shows the flat form

- **WHEN** a visitor selects RESP2 and runs the same command
- **THEN** the reply shows the RESP2 form, including flat arrays and `$-1` nulls, as returned by the server

#### Scenario: The version is not faked in the client

- **WHEN** the two forms are compared
- **THEN** the difference comes from the server connection's negotiated version, not from a client-side reformatting of one reply

### Requirement: The sandbox bounds what one visitor and a flood can spend

The sandbox SHALL bound inference work by request shape and by rate, and SHALL
bound total work independently of how many visitors arrive. The bounds MUST
apply to the server's expensive paths so that no combination of requests can
consume unbounded CPU.

#### Scenario: A single request is bounded

- **WHEN** a request carries more items, longer text, or more bytes than the sandbox allows
- **THEN** the request is refused or truncated to the bound, and the reply states which happened

#### Scenario: One visitor cannot monopolize the sandbox

- **WHEN** one client sends requests faster than the per-client allowance
- **THEN** further requests are refused for a cooling period while other clients continue to be served

#### Scenario: A flood cannot consume unbounded work

- **WHEN** requests arrive from many clients at once
- **THEN** a global bound admits a limited number at a time and a total-work bound stops serving expensive requests once a ceiling is reached

#### Scenario: The ceiling is reported

- **WHEN** the total-work bound is reached
- **THEN** expensive requests are refused with a message that says the sandbox is at capacity, rather than failing ambiguously

### Requirement: The sandbox refuses legibly when unavailable

The sandbox SHALL distinguish "not yet ready", "at capacity", and "unreachable"
from a successful reply, and the client MUST render each without an empty or
broken panel.

#### Scenario: The server is still starting

- **WHEN** the bridge receives a request before the server answers
- **THEN** it replies that the sandbox is starting, and the client can retry without treating it as a failure of the command

#### Scenario: The server is gone

- **WHEN** the server process has exited
- **THEN** a request is answered as unavailable and the machine is eventually replaced, rather than the bridge serving errors indefinitely as if healthy

#### Scenario: The client is offline

- **WHEN** the client cannot reach the sandbox at all
- **THEN** the console renders an offline state that names the condition and offers a retry, and no transcript is shown as if it were a real reply

### Requirement: One client drives both surfaces

The landing console and the standalone terminal SHALL be driven by one client
with one request and reply contract, so a reply renders the same way on both and
a change to the contract cannot land on one surface only.

#### Scenario: The same command renders the same on both

- **WHEN** the same command and protocol are run on the landing page and on the standalone terminal
- **THEN** both render the same reply with the same kinds and formatting

#### Scenario: The standalone terminal is served by the sandbox

- **WHEN** the sandbox host is opened in a browser
- **THEN** it serves a terminal that speaks the same contract as the landing console

### Requirement: The sandbox starts and stops as one unit

The sandbox SHALL start the server and the bridge together under one entrypoint,
MUST NOT accept requests for the server before that server can answer, and MUST
treat the two processes as one lifecycle: a stop signal reaches both, and the
failure of either ends the unit so a replacement is started.

#### Scenario: Requests during startup are answered as starting

- **WHEN** the unit has started but the server has not yet accepted a connection
- **THEN** a request is answered as starting and no command is lost or run against a missing server

#### Scenario: A stop signal reaches the server

- **WHEN** the unit is asked to stop
- **THEN** the signal is forwarded to the server and the bridge waits for it, rather than the server being killed without its shutdown path

#### Scenario: Either process failing ends the unit

- **WHEN** the server exits unexpectedly
- **THEN** the bridge stops serving and the unit is replaced, so health is never reported by a bridge with no server behind it

### Requirement: The reply carries the server-measured call time

The bridge SHALL measure the elapsed time of the upstream command on its
loopback connection to `emb` and SHALL carry that value in the reply envelope,
so a client can report the server's answer time without timing its own network.
The measured bracket MUST span the command write through the reply read and
MUST NOT include connection negotiation. A reply the bridge produces without
reaching `emb` MUST carry no elapsed value.

#### Scenario: An answered command carries the server's time

- **WHEN** a permitted command is answered by the server
- **THEN** the reply envelope carries the elapsed time of the loopback round trip, measured by the bridge and not by the client

#### Scenario: A reply with no upstream call carries no time

- **WHEN** the bridge refuses a command or refuses it on a spend bound, so the server is never asked
- **THEN** the reply envelope carries no elapsed time, so the client renders no timing trailer for it

#### Scenario: The client's network is not measured

- **WHEN** the same command is run from two clients at different distances from the sandbox
- **THEN** the reported elapsed time reflects the server's answer time and does not grow with the clients' round trips

### Requirement: The sandbox's warm cache survives a restart

The sandbox SHALL persist its cache to the mounted volume and restore it on
startup, so a restart does not discard every cached embedding. The snapshot path
and its automatic-save interval SHALL be declared in the server configuration
the image ships, and the restore and the graceful-stop flush SHALL use the
server's existing defaults rather than a second mechanism. The persisted cache
SHALL stay within the configured byte budget, and the machine's forced-kill
window SHALL be longer than the server's own shutdown budget so a graceful stop
writes the snapshot. A snapshot that is missing, corrupt, or incompatible with
the loaded model set MUST NOT prevent the sandbox from becoming ready.

#### Scenario: The snapshot lives on the volume

- **WHEN** the sandbox's server configuration is inspected
- **THEN** it names a cache snapshot path on the same mounted volume as the models
- **AND** it names an automatic-save interval
- **AND** the server's default startup-restore and shutdown-save behavior is left enabled

#### Scenario: A graceful restart reloads the cache

- **WHEN** the machine is stopped and started again without changing the cache or the models
- **THEN** the server restores the snapshot before it reports ready
- **AND** an entry restored from it answers without re-running inference

#### Scenario: An ungraceful restart is bounded by the interval

- **WHEN** the machine is terminated without running the server's shutdown path
- **THEN** at most the entries inserted since the last automatic save are lost
- **AND** the next boot restores the last complete snapshot

#### Scenario: A bad snapshot starts cold rather than blocking ready

- **WHEN** the snapshot is missing, corrupt, or was written against a different model set
- **THEN** the server starts with an empty or partial cache and still reports ready
- **AND** becoming ready does not depend on the file existing

#### Scenario: The stop signal is not preempted

- **WHEN** the machine is asked to stop
- **THEN** the platform's forced-kill window is longer than the server's own shutdown budget
- **AND** the graceful flush completes rather than being killed mid-write

#### Scenario: A visitor still cannot trigger a write

- **WHEN** a visitor asks the bridge to save state
- **THEN** it is refused with a reason that describes the permitted surface
- **AND** the reason does not claim the sandbox keeps nothing, because its own lifecycle now writes the snapshot
