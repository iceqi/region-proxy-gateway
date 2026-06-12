# Personal Multi-Channel VPNGate Gateway Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert the app from a single-port username-strategy proxy into a personal multi-port VPNGate channel gateway.

**Architecture:** Configuration defines channels. Each enabled channel has its own proxy listener and tunnel. VPNGate CSV is the primary node source. The admin server exposes JSON APIs and a minimal static panel for channel management.

**Tech Stack:** Go standard library, existing proxy handlers, existing tunnel interface, VPNGate CSV API, JSON persistence.

---

### Task 1: Simplify Configuration

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] Replace single proxy port/session fields with a `Channel` model and `Channels []Channel`.
- [ ] Add JSON file loading and saving for `data/config.json`.
- [ ] Validate unique proxy ports, valid admin port, non-empty auth, and valid channel fields.
- [ ] Update tests for default config, duplicate ports, invalid channel region, invalid rotate minutes, and round-trip persistence.

### Task 2: Add VPNGate Fetcher

**Files:**
- Create: `internal/vpngate/client.go`
- Create: `internal/vpngate/client_test.go`

- [ ] Parse VPNGate CSV with a `#` header line.
- [ ] Decode `OpenVPN_ConfigData_Base64`.
- [ ] Convert rows into `node.Node`.
- [ ] Sort preferred nodes by speed descending and ping ascending.
- [ ] Test parser with a small embedded CSV fixture.

### Task 3: Add Channel Manager

**Files:**
- Create: `internal/channel/manager.go`
- Create: `internal/channel/manager_test.go`
- Modify: `internal/connection/tracker.go`

- [ ] Own channel state, tunnel instances, current node, and listeners.
- [ ] Start one listener per enabled channel.
- [ ] Switch channels automatically or manually.
- [ ] Expose snapshots for admin status.
- [ ] Track connections by channel ID instead of username strategy.

### Task 4: Refactor Proxy Server To Port-Based Channels

**Files:**
- Modify: `internal/proxy/server.go`
- Modify: `internal/proxy/http.go`
- Modify: `internal/proxy/socks5.go`
- Modify: `internal/proxy/*_test.go`

- [ ] Replace strategy authentication with username/password-only auth.
- [ ] Replace session provider with a channel dialer.
- [ ] Use the channel ID for connection tracking.
- [ ] Keep HTTP, CONNECT, and SOCKS5 behavior unchanged.

### Task 5: Build Admin API And Panel

**Files:**
- Modify: `internal/admin/server.go`
- Modify: `internal/admin/server_test.go`
- Create: `internal/admin/static.go`

- [ ] Add channel CRUD endpoints.
- [ ] Add node listing filtered by region.
- [ ] Add manual switch endpoint.
- [ ] Serve a minimal HTML management panel.

### Task 6: Wire Main And Docs

**Files:**
- Modify: `cmd/region-proxy-gateway/main.go`
- Modify: `cmd/region-proxy-gateway/main_test.go`
- Modify: `README.md`
- Modify: `install.sh`

- [ ] Load or create `data/config.json`.
- [ ] Fetch VPNGate nodes on startup, falling back to demo nodes only for fake mode tests.
- [ ] Start channel manager and admin server.
- [ ] Update README and install notes for the personal multi-channel model.

### Verification

- [ ] Run `go test ./...`.
- [ ] Run `go build -o /tmp/region-proxy-gateway ./cmd/region-proxy-gateway`.
