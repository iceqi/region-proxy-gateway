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
- 管理面板可把当前筛选出的节点加入深度测试队列。
- 节点显示住宅/家宽、机房、移动网、代理风险等基础 IP 类型。
- 节点显示基础纯净度评分。
- SQLite 缓存深度测试结果、通道连接历史和最近使用节点。
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

## 配置和数据

首次启动会生成：

```text
data/config.json
data/region-proxy-gateway.db
```

`config.json` 保留站点级配置，方便手工修改；SQLite 数据库保存通道配置、VPNGate 节点缓存、深度测试结果和通道节点使用历史。

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
  "database_path": "./data/region-proxy-gateway.db",
  "openvpn_command": "openvpn",
  "tunnel_backend": "fake"
}
```

旧版本 `config.json` 里的 `channels` 会在首次启动时迁移进 SQLite，并从 JSON 里清空；迁移后通道增删改由管理面板写入数据库。

## 部署和使用

在服务器上执行一键安装脚本：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/iceqi/region-proxy-gateway/main/install.sh)
```

如果构建时下载 Go 依赖很慢，可以临时指定 Go 代理：

```bash
GOPROXY=https://goproxy.cn,direct bash <(curl -fsSL https://raw.githubusercontent.com/iceqi/region-proxy-gateway/main/install.sh)
```

脚本会自动完成：

- 安装 OpenVPN、Go、git、jq、SQLite 编译依赖等。
- 拉取或更新 `/opt/region-proxy-gateway`。
- 编译二进制。
- 生成 `data/config.json` 和 `data/region-proxy-gateway.db`。
- 管理面板监听 `0.0.0.0`，端口和 path 随机生成。
- 管理密码、代理密码随机生成。
- 默认启用 `openvpn` backend。
- 安装并启动 `region-proxy-gateway.service`。

重复执行脚本会更新代码并重启服务，已有配置会保留。

安装完成后，脚本会输出管理面板地址、账号和密码，例如：

```text
Admin: http://服务器IP:随机端口/随机path
Admin user: admin-xxxx
Admin password: xxxx
Proxy user: proxy-xxxx
Proxy password: xxxx
```

打开管理面板后，按这个流程使用：

1. 点击更新节点，获取最新 VPNGate 节点。
2. 新建通道，选择地区和端口，例如日本 `3000`、美国 `3001`。
3. 选择固定 IP 或设置几分钟自动轮换。
4. 需要手动指定出口时，在节点列表里筛选地区，再切换到对应通道。
5. 在通道列表复制 HTTP 或 SOCKS5 代理地址，填到 3x-ui 的上游代理里。

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
- 对当前筛选列表加入深度测试队列。
- 从节点列表右侧选择通道，并切换该通道到当前节点。

测速说明：

- 不使用 VPNGate CSV 里的 `Ping` 作为延迟，因为那不是从你的服务器测出来的。
- TCP OpenVPN 节点使用从当前服务器到节点 OpenVPN 端口的 TCP 连接耗时作为实时延迟。
- UDP OpenVPN 节点无法在不真正建立 VPN 握手的情况下完整验证端口，只能尝试 ICMP ping；如果对方禁 ping，会显示未知，不直接判定节点不可用。
- 自动优选会先对同地区候选节点做实时检测，优先选择实时可连接且延迟更低的节点。
- 深度测试会启动临时 OpenVPN 连接，并通过出口 IP 检查确认节点是否真的可用。
- 深度测试在后台队列里跑，默认一次只跑 1 个节点，避免服务器资源被打满。
- 页面刷新不会等待深度测试完成，也不会返回节点里的大段 OpenVPN 配置，所以打开页面会更快。

轮换说明：

- 自动轮换前会先更新一次 VPNGate 节点列表。
- 同一个通道会避开当前节点。
- 同一个通道会尽量避开 24 小时内已经用过的节点/IP。
- 如果节点都有 24 小时内使用记录，会选择最久没用过的节点作为兜底。
- 有深度测试成功记录的节点会优先于未测或失败节点。

自动更新节点间隔由 `node_refresh_interval` 控制，比如 `20m`、`1h`。管理面板可以保存这个值，重启服务后生效。

新增、编辑、删除通道会保存到 SQLite 数据库，并自动重启服务让端口监听生效。

## 重要说明

`fake` backend 可以直接跑通本地测试。

`openvpn` backend 在 Linux 下会把出站 TCP socket 绑定到通道对应的 tun 设备，例如 `rpg0`、`rpg1`。运行时需要 root 或具备 `SO_BINDTODEVICE` 所需权限。非 Linux 系统会返回明确的不支持错误。
