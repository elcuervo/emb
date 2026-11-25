# Spec Delta

## Purpose

A self-contained, read-only HTTP application that lets operators inspect a live EMB node or fleet from a browser without installing a remote observability service or exposing EMB commands to viewers.

## ADDED Requirements

### Requirement: The sidekick serves a configured EMB node or fleet
The sidekick SHALL accept one or more operator-configured EMB node specifications and serve a browser monitor for those nodes over HTTP. A single-node configuration SHALL open the node dashboard; a multi-node configuration SHALL open the fleet overview and allow drill-down to any node.

#### Scenario: One node
- **WHEN** an operator configures one reachable EMB address and opens the sidekick URL
- **THEN** the node dashboard displays that node's live measurements without an empty fleet shell

#### Scenario: Fleet and node drill-down
- **WHEN** an operator configures multiple EMB nodes or a name resolving to multiple nodes
- **THEN** the first view is the fleet overview, and choosing a node opens that node's dashboard with a way back to the fleet

#### Scenario: Missing targets
- **WHEN** the sidekick starts with no node specification
- **THEN** it exits non-zero with a diagnostic identifying the missing target instead of serving a misleading empty dashboard

### Requirement: The sidekick discovers and tracks fleet membership
The sidekick SHALL expand operator-configured DNS node names and maintain distinct, stable node identities as records change. An answering node whose DNS record disappears SHALL remain visible as orphaned until it becomes unreachable; a newly resolved node SHALL join without a sidekick restart.

#### Scenario: New peer appears
- **WHEN** a monitored DNS name begins resolving to another EMB node
- **THEN** the new node appears and is sampled without a browser or sidekick restart

#### Scenario: DNS loses a responsive peer
- **WHEN** a previously discovered node is no longer returned by DNS but still answers
- **THEN** it remains in the fleet as orphaned and continues contributing to aggregate measurements

#### Scenario: Unreachable node does not hide peers
- **WHEN** one fleet node cannot be polled
- **THEN** reachable peers keep updating and the failed node remains visible with its connection state and last successful sample time

### Requirement: One monitoring workload serves all viewers
The sidekick SHALL poll each configured EMB node independently on a bounded cadence, keep only a bounded recent history, and distribute the resulting samples to all viewers. Viewer count SHALL NOT multiply upstream polls, and a slow viewer SHALL NOT stall sampling or another viewer.

#### Scenario: Concurrent browsers
- **WHEN** several browsers watch the same fleet
- **THEN** each sees the same live measurements while each EMB node is polled once per sampling interval

#### Scenario: Stalled node
- **WHEN** one EMB node stops answering during a poll
- **THEN** polling and updates for other nodes continue on schedule

#### Scenario: Late viewer
- **WHEN** a viewer opens the page after monitoring has been running
- **THEN** the latest complete sample and available recent history are displayed without waiting for a new full history window

### Requirement: The fleet overview tells the operational story
The fleet overview SHALL show current fleet health, aggregate throughput and recent latency, node membership and load distribution, and one stable row per node with reachability, freshness, rates, and a link to its detail. An idle reachable node SHALL not be presented as disconnected or faulty solely because it received no traffic.

#### Scenario: One node is slow or disconnected
- **WHEN** one of several monitored nodes is degraded, stale, or unreachable
- **THEN** the fleet view identifies that node, states the reason, and keeps healthy peers distinguishable

#### Scenario: Idle node
- **WHEN** a node answers polls but handles no requests
- **THEN** its row says idle rather than treating lack of traffic as a connection failure

#### Scenario: Counter reset
- **WHEN** a node restarts and reports smaller cumulative counters
- **THEN** its computed rates are rebased rather than spiking or going negative

### Requirement: Node and model views expose measured diagnostics
The node view SHALL show recent request/token rates, request latency distribution, error rate, cache activity, memory and CPU usage, connection/load state, and the loaded models' own measured activity. Selecting a model SHALL expose its available latency, errors, run/dispatch/admission evidence and identity without assigning unmeasured phases to it.

#### Scenario: Inspect a busy model
- **WHEN** a loaded model serves requests
- **THEN** its view shows its measured recent rates and latency alongside its inference and wait evidence where reported

#### Scenario: Older node omits attribution
- **WHEN** an EMB node does not report per-request phases or a CPU plan
- **THEN** those fields are absent or marked unavailable, not displayed as measured zeroes

#### Scenario: No requests yet
- **WHEN** the node is reachable but no recent requests have completed
- **THEN** the latency and event views explicitly say there is no recent sample rather than inventing values

### Requirement: Recent events are useful without exposing payloads
The sidekick SHALL display recent completed-request events, including available model, time, latency, error, cache-path and measured phase information. It SHALL NOT display request texts, embedding bytes, credentials, or raw authentication configuration.

#### Scenario: Error investigation
- **WHEN** a recent request fails
- **THEN** the event appears with its model, latency, and failure state but no input payload

#### Scenario: Bounded event history
- **WHEN** more events arrive than fit the recent monitoring window
- **THEN** old events are discarded and the page does not imply they remain available

### Requirement: The browser experience is operable and accessible
The sidekick SHALL present a responsive, purpose-built visual dashboard with readable charts and legible numeric alternatives, usable keyboard controls, visible focus, semantic labels and statuses, and an update rhythm that does not overwhelm assistive technology. Motion SHALL respect reduced-motion preference and never be the only way to understand state.

#### Scenario: Narrow screen
- **WHEN** an operator opens the monitor on a narrow viewport
- **THEN** node/model information remains legible and navigable without shrinking the entire dashboard into an unreadable terminal grid

#### Scenario: Keyboard and screen reader
- **WHEN** an operator navigates without a pointer or reads the page with assistive technology
- **THEN** views, node/model choices, chart summaries, and connection state remain available as text and controls with meaningful labels

#### Scenario: Reduced motion
- **WHEN** the browser requests reduced motion
- **THEN** optional transitions stop while all measurements and status changes remain visible

### Requirement: Missing and stale data never look live
The sidekick SHALL distinguish starting, connected, idle, stale, disconnected, and authentication-failure states, show the time of the last successful sample, and recover automatically when a node or browser stream reconnects. A lost stream SHALL NOT present the last measurement as current.

#### Scenario: EMB restarts
- **WHEN** a monitored node goes away and returns
- **THEN** its stale/unavailable state is shown, sampling resumes automatically, and new counters are rebased

#### Scenario: Browser stream interrupted
- **WHEN** the browser stops receiving updates
- **THEN** it shows a reconnecting or stale indication until fresh data arrives

### Requirement: Monitoring is read-only and targets are not browser-controlled
The HTTP surface SHALL expose only the monitor and its read-only data endpoints. Browser requests SHALL NOT be able to submit EMB commands, change monitoring targets, change node settings, or induce connections to arbitrary addresses. The listener SHALL default to loopback and require an explicit operator choice to bind a network-reachable address.

#### Scenario: Command submission refused
- **WHEN** a viewer posts a command or attempts to change a target through HTTP
- **THEN** the sidekick rejects it without forwarding a command or dialing that target

#### Scenario: Default listener
- **WHEN** the sidekick is started without a listener override
- **THEN** the HTTP listener is reachable only on loopback

#### Scenario: Private-network listener
- **WHEN** an operator explicitly binds the sidekick to a VPC interface
- **THEN** the monitor is reachable on that interface and the documentation states that ingress restriction is the operator's access-control boundary

### Requirement: Protected EMB nodes remain protected
The sidekick SHALL support authenticated and TLS-protected upstream EMB nodes with server-name verification. Credentials SHALL not be sent to browsers, logged, or embedded in browser assets; a rejected password or failed certificate check SHALL be reported as an affected-node connection state rather than silently accepting an insecure fallback.

#### Scenario: TLS with DNS discovery
- **WHEN** a discovered node has a certificate for its DNS name
- **THEN** the sidekick verifies that name even if it dials the node's resolved address

#### Scenario: Authentication fails
- **WHEN** the monitored node rejects the configured credential
- **THEN** that node is marked as failing authentication without exposing the credential

### Requirement: The sidekick runs without external services or fetched assets
The distributed sidekick SHALL serve its own browser assets and SHALL need no internet access, CDN, remote fonts, hosted service, external database, or package manager at runtime. Its only upstream application connections SHALL be to operator-configured EMB nodes; DNS resolution for configured names MAY use the VPC resolver. The build SHALL introduce no new third-party dependency.

#### Scenario: Air-gapped operation
- **WHEN** the sidekick and EMB nodes run in a closed VPC with no internet route
- **THEN** the operator can load the page and inspect live data without a request to an external origin

#### Scenario: No external asset request
- **WHEN** a browser loads every monitor view
- **THEN** scripts, styles, fonts if any, and data all come from the sidekick's own origin

### Requirement: Installation and offline limitations are explicit
The sidekick SHALL document its invocation for one node and a fleet, secure upstream configuration, private HTTP binding, and the retention boundary. It SHALL NOT advertise persistent history or alerts when only live in-memory measurements are available.

#### Scenario: Restart loses recent history
- **WHEN** the sidekick is restarted
- **THEN** it starts a new recent window, and the UI does not claim that pre-restart samples are available
