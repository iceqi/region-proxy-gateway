# Region Proxy Gateway OpenVPN Backend Design

Date: 2026-06-12

## Goal

Add the first real tunnel backend for Region Proxy Gateway so a strategy session can start, stop, and switch an OpenVPN process using configured regional nodes.

This phase turns the current fake tunnel into a production-shaped OpenVPN backend foundation. It does not yet claim full per-session route isolation for proxy traffic. The routing problem is called out explicitly because OpenVPN process management and outbound dial isolation are separate pieces of work.

## Current State

The project already has:

- Single-port HTTP and SOCKS5 proxy entry.
- Username strategy parsing such as `jp-10` and `us-0`.
- Shared session manager keyed by strategy.
- Node store and best-node selection.
- Tunnel interface.
- Fake tunnel backend.
- OpenVPN command builder with `route-nopull`.
- Admin API for status, sessions, nodes, and active connections.
- Linux install script with OpenVPN package dependency.

The current fake tunnel dials directly from the host network. It does not create a regional exit path.

## Scope

This phase adds:

- Local node file loading from `data/nodes.json`.
- OpenVPN config material handling from node data.
- OpenVPN process start and stop.
- OpenVPN status tracking.
- Session switch support by restarting OpenVPN on the selected node.
- Main wiring to select `openvpn` or `fake` tunnel backend from config.
- README and install script updates explaining current capabilities and limits.

This phase does not add:

- VPNGate API crawling.
- Benchmarking real nodes.
- Full admin web UI.
- Admin authentication UI/session cookies.
- Full per-session network namespace routing.
- Billing, users, or subscription plans.

## Node Configuration

The first real node source is a local JSON file:

```text
data/nodes.json
```

Example:

```json
[
  {
    "id": "jp-tokyo-1",
    "region": "jp",
    "country": "Japan",
    "ip": "203.0.113.10",
    "hostname": "vpn-jp.example.net",
    "openvpn": "client\nremote vpn-jp.example.net 1194 udp\n..."
  }
]
```

The loader validates:

- `id` is not empty.
- `region` is not empty.
- `openvpn` is not empty for OpenVPN backend.
- duplicate IDs are rejected.

If `data/nodes.json` is missing, the service can still start with demo fake nodes only when backend is `fake`. If backend is `openvpn`, missing or empty node config is a startup error.

## Configuration

Add fields:

```go
TunnelBackend string `json:"tunnel_backend"`
```

Allowed values:

- `fake`
- `openvpn`

Default value:

```text
fake
```

Reason: the current binary must remain runnable without root privileges, OpenVPN, or tun device access. Operators opt into real OpenVPN behavior by setting `tunnel_backend` to `openvpn`.

Existing fields reused:

- `DataDir`
- `OpenVPNCommand`

Derived paths:

```text
<data_dir>/nodes.json
<data_dir>/sessions/<strategy-key>/client.ovpn
```

## OpenVPN Backend

Create a real tunnel implementation in `internal/tunnel/openvpn.go`.

Behavior:

- `Start(ctx, node, opts)` writes the node OpenVPN config to a session directory.
- Starts the OpenVPN process using the existing command builder.
- Records PID, node ID, start time, and readiness state.
- Fails if the node has no OpenVPN config.
- Fails if already started.
- `Stop(ctx)` terminates the OpenVPN process and waits for exit.
- If graceful termination times out, kills the process.
- `Switch(ctx, node)` stops the current process, then starts a new process on the new node.
- `Status()` returns current state without exposing internal mutable state.

Readiness:

- Initial readiness is process-start based.
- If `exec.Command.Start` succeeds, status becomes ready.
- Later phases can add management socket probing, tun device readiness checks, and public IP verification.

Dial behavior:

- `DialContext` remains explicit about its limitation in this phase.
- For `fake`, it keeps direct host dialing.
- For `openvpn`, until network namespace or policy routing exists, it should either:
  - return a clear error saying route isolation is not implemented; or
  - support direct host dialing only behind an explicit unsafe config flag.

Recommended behavior for this phase: return a clear error for OpenVPN `DialContext`.

Reason: silently direct-dialing from host network while OpenVPN is running is worse than failing, because it would make users think traffic is regional when it is not.

## Routing Isolation Boundary

OpenVPN process management alone does not force a Go TCP dial to exit through a specific tunnel.

Future routing phase should choose one of:

1. Network namespace per session.
2. Policy routing plus fwmark per session.
3. Local helper process per namespace that accepts local connections and dials inside the namespace.

Recommended next routing design:

- Create one Linux network namespace per session.
- Move the tun device into that namespace.
- Run a small dial helper inside the namespace.
- Proxy relay connects to the helper, and helper dials the target from the namespace.

This keeps Go proxy code simple and avoids process-global namespace switching risks.

## Session Switching

For this phase, switching is cold:

- Stop current OpenVPN process.
- Write new config.
- Start new OpenVPN process.
- Update status and session node.

Existing client connections may fail during switch because dial routing is not yet implemented. Warm handoff remains a future routing/session feature.

## Admin API Impact

Existing endpoints remain:

- `GET /api/status`
- `GET /api/sessions`
- `GET /api/nodes`
- `GET /api/connections`

Session status should expose the tunnel status through the existing session list if possible. If that is too invasive, this phase can keep status internal and only verify it through tests.

Manual switch endpoint is not required in this phase because the earlier admin API skeleton does not yet expose write endpoints for session switching. That should be a separate admin-control phase.

## Installer And README

Update installer:

- Keep OpenVPN package installation.
- Create `<install_dir>/data`.
- Create an example `data/nodes.example.json`.
- Do not overwrite existing `data/nodes.json`.

Update README:

- Explain default fake backend.
- Explain how to enable OpenVPN backend.
- Explain `data/nodes.json` format.
- State clearly that per-session route isolation is not implemented yet, so OpenVPN backend starts processes but proxy traffic will not be routed until the routing phase is complete.

## Error Handling

Startup errors:

- `openvpn` backend with missing nodes file: fail startup.
- invalid nodes JSON: fail startup.
- node missing OpenVPN config: reject node.
- invalid backend name: fail config validation.

Runtime errors:

- OpenVPN command start failure propagates to session creation.
- OpenVPN process exit updates status error.
- Stop timeout kills process.
- Switch failure keeps error in tunnel status.

## Testing

Use TDD for every code change.

Tests required:

- Config validates `fake` and `openvpn`, rejects unknown backend.
- Node loader reads valid `nodes.json`.
- Node loader rejects duplicate IDs.
- Node loader rejects missing OpenVPN config when OpenVPN is required.
- OpenVPN backend writes config file.
- OpenVPN backend starts command through an injectable process starter.
- OpenVPN backend stop calls terminate/wait through a fake process.
- OpenVPN backend switch stops old process and starts new config.
- Main wiring uses local node file for OpenVPN backend.
- Main wiring keeps demo nodes for fake backend.
- `go test ./...` passes.
- `go build -o /tmp/region-proxy-gateway ./cmd/region-proxy-gateway` passes.

## Success Criteria

At the end of this phase:

- The project has a real OpenVPN tunnel implementation for process lifecycle.
- Operators can provide `data/nodes.json`.
- The service can create sessions backed by OpenVPN process state.
- The service refuses to silently pretend host-network dials are regional OpenVPN dials.
- Documentation is clear about what works now and what still requires the routing phase.

## Follow-Up Phases

Recommended next phases:

1. Linux network namespace routing and dial helper.
2. Admin write endpoints for switch/restart/select-node.
3. Admin web panel.
4. VPNGate node fetcher and benchmarker.
5. Automatic rotation scheduler.
