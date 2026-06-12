# Region Proxy Gateway

Region Proxy Gateway is a Go single-port region proxy gateway. It accepts
HTTP proxy, HTTP CONNECT, and SOCKS5 traffic on one proxy port and selects a
region strategy from the proxy username.

## Username Strategy

The proxy username format is:

```text
<region>-<minutes>
```

Examples:

```text
jp-10
us-0
```

`minutes=0` means fixed current node. `minutes>0` means rotate within the same region.

## Current Status

Implemented:

- HTTP proxy and HTTP CONNECT.
- SOCKS5 username/password proxy.
- Strategy parsing from usernames such as `jp-10`.
- Shared sessions for the same strategy.
- Connection tracking.
- Admin JSON API.
- Fake tunnel backend.
- OpenVPN process lifecycle backend.

Important limitation:

The OpenVPN backend can start and switch OpenVPN processes, but per-session
route isolation is not implemented yet. Until a later Linux network namespace
or policy routing phase is complete, do not treat proxy traffic as if it is
already guaranteed to use a regional OpenVPN exit. OpenVPN `DialContext`
returns a routing-isolation error in this phase.

## Proxy Examples

```text
http://jp-10:PASSWORD@SERVER_IP:3000
socks5://jp-10:PASSWORD@SERVER_IP:3000
http://us-0:PASSWORD@SERVER_IP:3000
socks5://us-0:PASSWORD@SERVER_IP:3000
```

## Tunnel Backends

The default tunnel backend is:

```text
fake
```

The fake backend keeps the gateway runnable without root privileges, OpenVPN,
or a tun device.

To opt into the OpenVPN process lifecycle backend, configure:

```json
{
  "tunnel_backend": "openvpn",
  "data_dir": "./data",
  "openvpn_command": "openvpn"
}
```

OpenVPN nodes are loaded from:

```text
data/nodes.json
```

Example `data/nodes.json`:

```json
[
  {
    "id": "jp-example-1",
    "region": "jp",
    "country": "Japan",
    "ip": "203.0.113.10",
    "hostname": "vpn-jp.example.net",
    "openvpn": "client\nremote vpn-jp.example.net 1194 udp\nroute-nopull\n"
  }
]
```

## Admin API

The default admin address is:

```text
http://127.0.0.1:8787
```

Endpoints:

```text
GET /api/status
GET /api/sessions
GET /api/nodes
GET /api/connections
```

## Build

```bash
go test ./...
go build -o region-proxy-gateway ./cmd/region-proxy-gateway
```
