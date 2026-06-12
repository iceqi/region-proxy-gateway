# Region Proxy Gateway Deep Test Queue Design

## Goal

Add OpenVPN deep testing as a queued background job, persist deep test results in SQLite, and improve channel rotation so the same channel avoids recently used nodes for 24 hours.

## Background

The current lightweight node check is good enough for TCP nodes because TCP connect latency can be measured directly. UDP OpenVPN nodes cannot be reliably verified with ping or UDP probes, because UDP is connectionless and many hosts block ICMP. The accurate test is to actually start a temporary OpenVPN connection, verify that it becomes usable, detect the exit IP, then stop and clean up the temporary process.

## Scope

This feature includes:

- A manual "deep test current list" action in the admin node list.
- A SQLite-backed job queue for deep testing nodes.
- A background worker with low concurrency, defaulting to one OpenVPN test at a time.
- Persistent deep test results for nodes.
- Channel connection and node-use history persisted in SQLite.
- Rotation logic that refreshes node data before selecting a replacement and avoids recently used nodes in the same channel.
- Lightweight admin APIs so page refresh does not return large OpenVPN configs or wait for deep tests.

This feature does not include:

- Automatically deep-testing every node during normal page load.
- Running high-concurrency OpenVPN probes.
- Replacing the existing lightweight TCP/ping check.
- A paid IP-risk or purity API integration.
- Returning raw OpenVPN config text from normal admin list APIs.

## Deep Test Queue

The admin panel will add a button named "深度测试当前列表". It submits the currently filtered visible node IDs to the backend.

The backend creates one queue job per node. The API returns quickly with a job summary instead of waiting for all OpenVPN tests to finish.

Queue behavior:

- Jobs are stored in SQLite.
- Duplicate pending jobs for the same node are ignored.
- Worker concurrency defaults to 1.
- A future config value may allow concurrency 2, but the first implementation should keep the default at 1.
- Each node test has a hard timeout, initially 20 seconds.
- Each test uses a temporary session name and temporary tun device.
- The OpenVPN process is stopped after success, failure, or timeout.
- Failed tests save the failure reason.

## Deep Test Result

Each node will have persistent deep test metadata:

- `node_id`
- `status`: `success`, `failed`, or `running`
- `exit_ip`
- `exit_country`
- `connect_ms`
- `tested_at`
- `fail_reason`

The node list should display:

- "深测成功" with exit IP and connect time.
- "深测失败" with a short reason.
- "未深测" if no result exists.

Deep test results are cached in SQLite and survive service restart.

## OpenVPN Test Method

For each deep test:

1. Write the node OpenVPN config to a temporary file under the data directory.
2. Start OpenVPN with a unique device name such as `rpgtest0`.
3. Wait for readiness using OpenVPN process state and route/device availability.
4. Verify the exit by making an HTTP request through the temporary tunnel/device when possible.
5. Record exit IP and connect time.
6. Stop OpenVPN and remove temporary files.

The first implementation must persist the exit IP fields. A deep test is considered successful only when OpenVPN starts and the worker can complete its configured success check before timeout. If the exit IP HTTP check fails, the result is saved as failed with reason `exit IP check failed`.

## Channel History

SQLite will store channel node usage history:

- `channel_id`
- `node_id`
- `exit_ip`
- `connected_at`
- `switched_at`

The channel list should display:

- current exit IP
- current connection time
- last IP switch time

`connected_at` means when the channel started using the current node. `switched_at` means when a channel changed from one node/IP to another.

## Rotation Rule

When a channel rotates:

1. Refresh VPNGate node data first.
2. Filter nodes by channel region.
3. Exclude the current node.
4. Exclude the fixed/manual node if the channel has one.
5. Exclude nodes used by this same channel in the last 24 hours.
6. Prefer nodes with successful deep test results.
7. Then prefer lower measured realtime latency.
8. Then prefer higher VPNGate speed.
9. If every node was used within 24 hours, choose the least recently used available node as a fallback.

The 24-hour avoid rule is per channel. A node used by channel `jp-3000` should not block channel `jp-3001`.

## Admin UI

Node list additions:

- "深度测试当前列表" button.
- Deep test status column.
- Job progress summary, such as pending/running/success/failed counts.

Channel list additions:

- current exit IP
- connection time
- last IP switch time

The UI should not block while deep testing. It should poll job status every few seconds while jobs are running.

## Admin API Performance

Normal page refresh must stay lightweight:

- `/api/nodes` returns a compact node view without the `openvpn` field.
- `/api/nodes/refresh` updates the in-memory and SQLite node cache but also returns compact node views.
- Deep-test enqueue APIs return immediately after creating jobs.
- Queue progress APIs return counts and recent cached results only.
- No page-load endpoint starts OpenVPN, runs deep tests, refreshes VPNGate data, or performs long network probes.

Go's HTTP server already handles requests concurrently. The main performance fix is avoiding large payloads and moving long work to background goroutines instead of trying to add extra processes around page APIs.

## Failure Handling

OpenVPN deep test failures should not crash the service.

Common failure reasons:

- missing OpenVPN config
- OpenVPN start failed
- timeout waiting for connection
- process exited early
- exit IP check failed

The worker must always attempt cleanup after each job.

## Testing

Tests should cover:

- SQLite migration creates queue, result, and history tables.
- Enqueue deep test jobs deduplicates pending jobs.
- Worker marks jobs success or failed.
- Node list includes deep test result fields.
- Node list and node refresh APIs do not include raw `openvpn` config text.
- Rotation avoids current node and nodes used within 24 hours.
- Rotation falls back to least recently used node when all nodes are inside the 24-hour window.
- Channel history records connected and switched timestamps.
