## Purpose

Live per-instance resource reporting in `INFO`: measured process memory (RSS, Go heap, goroutines), cumulative CPU usage, and aggregate network bytes — so operators can confirm no memory leak and correlate TX/RX traffic with CPU directly from the server.

## Requirements

### Requirement: INFO Memory section reports live process memory

The `# Memory` section SHALL report measured process memory as `used_memory_rss_bytes` (resident set size including native/CGo allocations such as ONNX Runtime), `used_memory_heap_bytes` (Go heap in use), `goroutines` (live goroutine count), and `total_system_memory_bytes` (host RAM). On platforms without a resident-set-size source, `used_memory_rss_bytes` SHALL fall back to the Go heap in-use value. All values SHALL be non-negative integers; no field SHALL report an invented constant.

#### Scenario: Memory fields present and numeric

- **WHEN** a client sends `INFO memory`
- **THEN** the reply SHALL contain only the `# Memory` section with `used_memory_rss_bytes`, `used_memory_heap_bytes`, `goroutines`, and `total_system_memory_bytes`, each a non-negative integer

#### Scenario: RSS reflects native allocations

- **WHEN** the server has performed inference (which allocates outside the Go heap via CGo)
- **THEN** `used_memory_rss_bytes` SHALL be read from the platform's process-RSS source (Linux `/proc/self/statm`, macOS `proc_pidinfo`, …), never from Go heap statistics alone

#### Scenario: Fallback when RSS is unavailable

- **WHEN** the server runs on a platform with no resident-set-size source
- **THEN** `used_memory_rss_bytes` SHALL equal `used_memory_heap_bytes`

#### Scenario: Goroutine count tracks live goroutines

- **WHEN** the server is idle after a period of request activity
- **THEN** `goroutines` SHALL report the current live goroutine count, which SHALL NOT grow with cumulative request volume

### Requirement: INFO CPU section reports cumulative CPU usage

The `# CPU` section SHALL report `used_cpu_user_usec` and `used_cpu_sys_usec` as cumulative processor time in microseconds since process start (user and system, including work performed inside CGo), plus `gomaxprocs`. Cumulative values SHALL be non-decreasing across calls.

#### Scenario: CPU fields present and filterable

- **WHEN** a client sends `INFO cpu`
- **THEN** the reply SHALL contain only the `# CPU` section with `used_cpu_user_usec`, `used_cpu_sys_usec`, and `gomaxprocs`

#### Scenario: CPU time is monotonic

- **WHEN** a client sends `INFO cpu` twice with an `EMB` request in between
- **THEN** the second response SHALL report `used_cpu_user_usec` + `used_cpu_sys_usec` greater than or equal to the first

### Requirement: INFO Stats reports aggregate network bytes

The `# Stats` section SHALL include `total_net_input_bytes` and `total_net_output_bytes`: cumulative bytes received from and sent to all connections since process start, including RESP framing. The values SHALL be non-decreasing and SHALL increase when clients send commands and receive replies.

#### Scenario: Byte counters increase with traffic

- **WHEN** a client sends one `EMB` command and receives its reply
- **THEN** `total_net_input_bytes` SHALL have increased by at least the command's wire size and `total_net_output_bytes` SHALL have increased by at least the reply's wire size

#### Scenario: Byte counters never decrease

- **WHEN** a client polls `INFO stats` repeatedly
- **THEN** `total_net_input_bytes` and `total_net_output_bytes` SHALL never decrease between polls

### Requirement: Resource sections report only measured values

The `# Memory`, `# CPU`, and new `# Stats` fields SHALL report values obtained from the running process or configuration at snapshot time. A field SHALL report `0` only when its measurement is genuinely zero or unavailable on the platform; no field SHALL be assigned a placeholder or decorative value.

#### Scenario: No placeholder values

- **WHEN** a client inspects the `# Memory`, `# CPU`, and `# Stats` sections
- **THEN** every value SHALL correspond to a live measurement or real configuration (e.g. `gomaxprocs` is the actual runtime value), never a hardcoded placeholder

### Requirement: Memory and CPU sections obey INFO section filtering

`INFO memory`, `INFO cpu`, and `INFO stats` SHALL return only the requested sections, and combined arguments SHALL return the union, matching the existing `INFO <section...>` behavior.

#### Scenario: Combined section arguments

- **WHEN** a client sends `INFO memory cpu`
- **THEN** the reply SHALL contain both the `# Memory` and `# CPU` sections and nothing else

#### Scenario: Unknown section still empty

- **WHEN** a client sends `INFO memory bogus`
- **THEN** the reply SHALL contain the `# Memory` section only