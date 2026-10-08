## ADDED Requirements

### Requirement: Idle-first script session dispatch

Scripted `emb.run` / `emb.run_batch` calls SHALL run on any named-tensor session that is free at the moment of the call. A call SHALL NOT wait for a busy session while another session of the same model is idle. When all sessions are busy, waiting calls SHALL be served in arrival order. Dispatch SHALL NOT change replies.

#### Scenario: Idle session is used while another is busy

- **GIVEN** a model with `script_workers: 2` and one session held by a long-running evaluation
- **WHEN** a second evaluation calls `emb.run`
- **THEN** it SHALL run on the idle session without waiting for the busy one

#### Scenario: Replies are unchanged

- **WHEN** the GLiNER2 parity corpus is evaluated with idle-first dispatch and with the previous release
- **THEN** every reply SHALL be byte-identical

#### Scenario: Tail latency under concurrency

- **WHEN** the GLiNER2 corpus runs at 4 sessions × 2 threads with concurrency 8 and the reply cache disabled
- **THEN** p99 latency SHALL be at most 110 ms on the reference host, serial p50 SHALL stay within 5% of the baseline, and throughput SHALL NOT decrease

### Requirement: Thread-budget default for script sessions

When `intra_op_threads` is unset and a model opens more than one named-tensor session, each script session SHALL default to `max(1, (cores − 2) / script_workers)` intra-op threads. An explicitly configured `intra_op_threads` SHALL be honoured verbatim.

#### Scenario: Unset threads divide the budget

- **WHEN** `script_workers: 4` is configured without `intra_op_threads` on a 10-core host
- **THEN** each script session SHALL use 2 intra-op threads

#### Scenario: Explicit threads are honoured

- **WHEN** `script_workers: 4` and `intra_op_threads: 8` are configured
- **THEN** each script session SHALL use 8 intra-op threads

### Requirement: Shared script sessions with optional spinning

A model SHALL accept `script_callers_per_session` (default 1). Up to that many evaluations SHALL run concurrently on each named-tensor session. Concurrent runs on one session SHALL return the same outputs as serial runs. A model SHALL accept `allow_spinning` (default true), which maps to ORT `session.intra_op.allow_spinning`.

#### Scenario: Concurrent runs on one session are correct

- **GIVEN** a model with `script_workers: 1` and `script_callers_per_session: 4`
- **WHEN** four evaluations with different texts run concurrently
- **THEN** all four SHALL run without waiting for each other, and each reply SHALL equal its serial reply byte for byte

#### Scenario: Spinning disabled

- **WHEN** a model sets `allow_spinning: false`
- **THEN** its sessions SHALL be created with `session.intra_op.allow_spinning` set to `0`
