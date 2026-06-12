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
- 管理面板随机端口和随机 path。
- 管理面板更新节点、筛选节点、测速节点。
- 节点显示住宅/家宽、机房、移动网、代理风险等基础 IP 类型。
- 节点显示基础纯净度评分。
- Fake backend 本地测试。
- OpenVPN backend 进程生命周期。
- Linux 下 OpenVPN 出站 socket 绑定到通道设备，如 `rpg0`、`rpg1`。

## 节点来源

节点来自 VPNGate 官方 CSV：

```text
https://www.vpngate.net/api/iphone/
```

程序会解析 CSV 里的 `OpenVPN_ConfigData_Base64`，不需要你手工维护 `nodes.json`。

节点 IP 类型和基础纯净度来自：

```text
http://ip-api.com/batch
```

它会返回 `proxy`、`hosting`、`mobile`、ASN、运营商等字段。程序按这些字段做基础判断：

- `residential`：住宅/家宽，基础纯净度较高。
- `hosting`：机房/数据中心。
- `mobile`：移动网络。
- `proxy`：代理风险。

这个纯净度是基础风险判断，不等于付费风控库的最终结论。`ip.net.coffee` 可以当人工复核工具使用，但目前没有看到公开稳定的 API 文档，所以没有直接依赖它。

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
  "admin_path": "/admin-gJ8mK2xP9qR4sT6v",
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

## 一键安装

服务器上执行：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/iceqi/region-proxy-gateway/main/install.sh)
```

脚本会自动完成：

- 安装 OpenVPN、Go、git、jq 等依赖。
- 拉取或更新 `/opt/region-proxy-gateway`。
- 编译二进制。
- 生成 `data/config.json`。
- 管理面板监听 `0.0.0.0`，端口和 path 随机生成。
- 管理密码、代理密码随机生成。
- 默认启用 `openvpn` backend。
- 安装并启动 `region-proxy-gateway.service`。

重复执行脚本会更新代码并重启服务，已有配置会保留。

安装后常用命令：

```bash
systemctl status region-proxy-gateway
journalctl -u region-proxy-gateway -f
systemctl restart region-proxy-gateway
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

首次生成配置时，管理端口和管理 path 都会随机生成。实际地址以 `data/config.json` 里的 `admin_port` 和 `admin_path` 为准。

```text
http://127.0.0.1:<admin_port><admin_path>
```

管理面板使用 HTTP Basic Auth，账号密码来自 `data/config.json`：

```text
admin_username
admin_password
```

当前面板支持查看：

- 通道列表。
- 当前节点。
- HTTP/SOCKS5 地址。
- 在线连接。
- 新建、编辑、删除通道配置。
- 选择节点并立即切换当前通道出口。
- 手动更新 VPNGate 节点列表。
- 筛选地区、IP 类型、质量、可用状态、最大延迟、ASN/运营商关键字。
- 对单个节点测速。
- 从节点列表右侧选择通道，并切换该通道到当前节点。

测速说明：

- 优先使用系统 `ping` 获取延迟。
- TCP OpenVPN 节点会额外做 TCP 端口连通检查。
- UDP OpenVPN 节点无法在不真正建立 VPN 握手的情况下完整验证端口，只能判断主机 ping 可达和延迟。

自动更新节点间隔由 `node_refresh_interval` 控制，比如 `20m`、`1h`。管理面板可以保存这个值，重启服务后生效。

新增、编辑、删除通道会保存到 `data/config.json`。新增端口监听需要重启服务后生效：

```bash
systemctl restart region-proxy-gateway
```

## 重要说明

`fake` backend 可以直接跑通本地测试。

`openvpn` backend 在 Linux 下会把出站 TCP socket 绑定到通道对应的 tun 设备，例如 `rpg0`、`rpg1`。运行时需要 root 或具备 `SO_BINDTODEVICE` 所需权限。非 Linux 系统会返回明确的不支持错误。

## Build

```bash
go test ./...
go build -o region-proxy-gateway ./cmd/region-proxy-gateway
```
