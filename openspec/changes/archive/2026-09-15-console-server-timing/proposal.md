## Why

The site's live console prints a `(x ms)` trailer on every reply, but that
number is a browser wall-clock delta measured from submit to reply receipt
(`website/repl/terminal.js` `startedAt`/`nowMs`). It therefore folds in two
network legs between the visitor and the Fly edge on top of the server's work.
A reader in a distant region sees emb "cost" hundreds of milliseconds when the
inference was single-digit milliseconds, so the panel advertises the network
instead of the product.

## What Changes

- The bridge measures the upstream call itself and reports it in the reply
  envelope as a server-measured elapsed time. The bracket is the command write
  through the reply read on the loopback connection to `emb`, so the value is
  the server's answer time and excludes connection negotiation.
- The console's timing trailer renders that server-measured value instead of a
  client timer. The client no longer starts a clock at submit; the visitor's
  round trip is not shown anywhere.
- Replies the bridge produces without reaching `emb` (a refusal or a capacity
  bound) carry no elapsed value, so they render no timing trailer rather than a
  fabricated `0 ms`.
- The docs wording for the trailer changes from "the wait it cost from submit"
  to the server's measured answer time.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `sandbox-service`: a new requirement that the bridge measures the upstream
  command's elapsed time on the loopback connection and carries it in the reply
  envelope, and that a reply with no upstream call carries no elapsed value.
- `product-site`: the "reply states what it cost" scenario now states that the
  trailer reports the server-measured call time rather than a client-measured
  wall-clock delta from submit.

## Impact

- `website/repl/bridge.go` — measure the write→read bracket in `send` (or
  `roundTrip`) and set the elapsed value on the envelope.
- `website/repl/envelope.go` — add the elapsed field to `Envelope`.
- `website/repl/terminal.js` — render the envelope's elapsed value; remove the
  client `startedAt`/`nowMs` timing path.
- `website/README.md` — the trailer's description.
- No change to the `emb` wire protocol, the RESP client, or the Ruby gems.
