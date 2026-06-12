# Personal Multi-Channel VPNGate Gateway Design

## Goal

Build a personal proxy gateway that lets one operator define any number of proxy channels. Each channel owns one local port, one VPNGate region, and one exit node selection policy.

## Scope

This replaces the earlier single-port username strategy model as the primary flow.

Keep:
- HTTP proxy, HTTP CONNECT, and SOCKS5 protocol handling.
- Connection tracking.
- OpenVPN process lifecycle.

Remove or demote:
- Username formats like `jp-10` as the main routing model.
- Manually maintained `data/nodes.json` as the main node source.
- Session-per-username behavior.

## Channel Model

Each channel has:

- `id`: stable channel ID.
- `listen_host` and `listen_port`: proxy listener address.
- `region`: VPNGate country code, such as `jp` or `us`.
- `rotate_minutes`: `0` means fixed until manually switched; greater than `0` means auto-rotate inside the same region.
- `selection_mode`: `auto` or `manual`.
- `manual_node_id`: selected node when mode is `manual`.
- `enabled`: whether the listener and tunnel should run.

The proxy username/password is only authentication. The port decides the channel.

## Node Source

The app fetches VPNGate nodes from:

```text
https://www.vpngate.net/api/iphone/
```

The app parses CSV rows, decodes `OpenVPN_ConfigData_Base64`, normalizes the country code to lowercase, and caches the result in memory. A cache file may be used later, but operator-maintained JSON nodes are no longer required.

Auto selection chooses an available node in the channel region, preferring higher speed and lower ping when those values exist.

## Runtime

Each enabled channel owns:

- One TCP proxy listener.
- One tunnel instance.
- One OpenVPN device name, such as `rpg0`, `rpg1`.

For now, the fake backend remains useful for local testing. The OpenVPN backend starts one process per channel. Real production routing still requires device-bound dialing or routing setup before OpenVPN traffic can be guaranteed per channel.

## Admin Panel

The admin panel is a simple local web UI at the admin address. It supports:

- View channels and current node.
- Add, edit, enable, disable, and delete channels.
- Select region and port.
- Choose fixed/manual node or auto selection.
- Trigger immediate node switch.
- View active connections.

Configuration is persisted to `data/config.json`.

## Non-Goals

- Public multi-user SaaS features.
- Billing, quotas, or user management.
- Complex network namespace orchestration in the first simplified pass.
