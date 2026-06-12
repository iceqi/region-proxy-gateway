# Region Proxy Gateway

个人用的多端口地域代理网关。

它的核心模型很简单：

```text
3000 -> 日本 -> 自动优选 -> 10 分钟换一次
3001 -> 美国 -> 自动优选 -> 固定不换
3002 -> 韩国 -> 手动指定节点
```

3x-ui 里可以直接把不同端口配置成不同上游代理。

## 当前能力

- HTTP 代理。
- HTTP CONNECT。
- SOCKS5 用户名密码代理。
- 多个代理端口通道。
- 每个通道独立配置地区、端口、固定/轮换模式。
- VPNGate CSV 节点解析。
- 管理面板查看通道、节点、在线连接。
- Fake backend 本地测试。
- OpenVPN backend 进程生命周期。
- Linux 下 OpenVPN 出站 socket 绑定到通道设备，如 `rpg0`、`rpg1`。

## 节点来源

节点来自 VPNGate 官方 CSV：

```text
https://www.vpngate.net/api/iphone/
```

程序会解析 CSV 里的 `OpenVPN_ConfigData_Base64`，不需要你手工维护 `nodes.json`。

## 配置文件

首次启动会生成：

```text
data/config.json
```

示例：

```json
{
  "admin_host": "127.0.0.1",
  "admin_port": 28765,
  "admin_username": "admin",
  "admin_password": "change-me-admin",
  "proxy_username": "proxy",
  "proxy_password": "change-me-proxy",
  "node_refresh_interval": "20m",
  "data_dir": "./data",
  "openvpn_command": "openvpn",
  "tunnel_backend": "fake",
  "channels": [
    {
      "id": "jp-3000",
      "listen_host": "0.0.0.0",
      "listen_port": 3000,
      "region": "jp",
      "rotate_minutes": 10,
      "selection_mode": "auto",
      "enabled": true
    }
  ]
}
```

`selection_mode`：

- `auto`：自动从地区里选一个节点。
- `manual`：使用 `manual_node_id` 指定节点。

`rotate_minutes`：

- `0`：固定当前节点。
- 大于 `0`：后续用于自动轮换同地区节点。

## 代理地址

假设服务器 IP 是 `1.2.3.4`：

```text
http://proxy:change-me-proxy@1.2.3.4:3000
socks5://proxy:change-me-proxy@1.2.3.4:3000
```

地区由端口决定，不再需要用户名写 `jp-10`。

## 管理面板

首次生成配置时，管理端口会自动选择一个随机未占用的高位端口。实际地址以 `data/config.json` 里的 `admin_port` 为准。

```text
http://127.0.0.1:<admin_port>
```

当前面板支持查看：

- 通道列表。
- 当前节点。
- HTTP/SOCKS5 地址。
- 在线连接。

后续会继续补上页面里的新增、编辑、删除、手动切换按钮。

## 重要说明

`fake` backend 可以直接跑通本地测试。

`openvpn` backend 在 Linux 下会把出站 TCP socket 绑定到通道对应的 tun 设备，例如 `rpg0`、`rpg1`。运行时需要 root 或具备 `SO_BINDTODEVICE` 所需权限。非 Linux 系统会返回明确的不支持错误。

## Build

```bash
go test ./...
go build -o region-proxy-gateway ./cmd/region-proxy-gateway
```
