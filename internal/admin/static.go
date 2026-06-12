package admin

const indexHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Region Proxy Gateway</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/ant-design-vue@4.2.6/dist/reset.css">
  <script src="https://cdn.jsdelivr.net/npm/vue@3.5.13/dist/vue.global.prod.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/dayjs@1.11.13/dayjs.min.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/ant-design-vue@4.2.6/dist/antd.min.js"></script>
  <style>
    :root { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; color: #172033; background: #f5f7fb; }
    body { margin: 0; background: #f5f7fb; }
    [v-cloak] { display: none; }
    .shell { min-height: 100vh; }
    .topbar { position: sticky; top: 0; z-index: 20; height: 60px; padding: 0 22px; display: flex; align-items: center; justify-content: space-between; background: #fff; border-bottom: 1px solid #e6eaf0; }
    .brand { display: flex; align-items: baseline; gap: 10px; min-width: 0; }
    .brand h1 { margin: 0; font-size: 19px; font-weight: 700; letter-spacing: 0; }
    .brand span { color: #697386; font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .page { max-width: 1480px; margin: 0 auto; padding: 18px; }
    .stats { display: grid; grid-template-columns: repeat(5, minmax(140px, 1fr)); gap: 12px; margin-bottom: 14px; }
    .stat { background: #fff; border: 1px solid #e6eaf0; border-radius: 8px; padding: 13px 14px; }
    .stat-label { color: #697386; font-size: 12px; margin-bottom: 6px; }
    .stat-value { font-size: 22px; font-weight: 700; }
    .card { background: #fff; border: 1px solid #e6eaf0; border-radius: 8px; margin-bottom: 14px; }
    .card-head { padding: 13px 14px; display: flex; align-items: center; justify-content: space-between; gap: 10px; border-bottom: 1px solid #eef1f5; }
    .card-title { font-size: 15px; font-weight: 700; }
    .card-body { padding: 14px; }
    .filter-grid { display: grid; grid-template-columns: repeat(7, minmax(120px, 1fr)); gap: 10px; margin-bottom: 12px; }
    .modal-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
    .mono { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 12px; }
    .muted { color: #697386; }
    .host-cell { display: grid; gap: 3px; overflow-wrap: anywhere; }
    .action-row { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
    .connection-box { display: grid; gap: 6px; }
    .connection-line { display: grid; grid-template-columns: 58px 1fr auto; gap: 8px; align-items: center; }
    .connection-line code { overflow-wrap: anywhere; white-space: normal; }
    .modal-filter { display: grid; grid-template-columns: repeat(4, minmax(120px, 1fr)); gap: 10px; margin-bottom: 12px; }
    .ant-tabs-nav { margin-bottom: 14px !important; }
    .ant-table-cell { vertical-align: top; }
    @media (max-width: 1100px) {
      .stats { grid-template-columns: 1fr 1fr; }
      .filter-grid, .modal-filter { grid-template-columns: 1fr 1fr; }
    }
    @media (max-width: 720px) {
      .topbar { height: auto; min-height: 60px; padding: 12px; align-items: flex-start; flex-direction: column; }
      .page { padding: 12px; }
      .stats, .filter-grid, .modal-grid, .modal-filter { grid-template-columns: 1fr; }
      .connection-line { grid-template-columns: 1fr; }
    }
  </style>
</head>
<body>
  <div id="app" class="shell" v-cloak>
    <header class="topbar">
      <div class="brand">
        <h1>Region Proxy Gateway</h1>
        <span>{{ apiBase }}</span>
      </div>
      <a-space>
        <a-button :loading="loading" @click="load">刷新</a-button>
        <a-button type="primary" :loading="updatingNodes" @click="updateNodes">更新节点</a-button>
      </a-space>
    </header>

    <main class="page">
      <section class="stats">
        <div class="stat"><div class="stat-label">通道</div><div class="stat-value">{{ channels.length }}</div></div>
        <div class="stat"><div class="stat-label">节点</div><div class="stat-value">{{ filteredNodes.length }} / {{ nodes.length }}</div></div>
        <div class="stat"><div class="stat-label">在线连接</div><div class="stat-value">{{ connections.length }}</div></div>
        <div class="stat"><div class="stat-label">节点更新间隔</div><div class="stat-value">{{ settings.node_refresh_interval || '-' }}</div></div>
        <div class="stat"><div class="stat-label">代理主机</div><div class="stat-value" style="font-size:16px">{{ proxyHost }}</div></div>
      </section>

      <a-tabs v-model:active-key="activeTab">
        <a-tab-pane key="nodes" tab="节点">
          <section class="card">
            <div class="card-head">
              <div>
                <div class="card-title">节点列表</div>
                <div class="muted">点击切换会弹窗选择通道，然后把该通道切到当前节点</div>
              </div>
            </div>
            <div class="card-body">
              <div class="filter-grid">
                <a-select v-model:value="filters.region" allow-clear placeholder="地区">
                  <a-select-option v-for="item in regions" :key="item" :value="item">{{ item }}</a-select-option>
                </a-select>
                <a-select v-model:value="filters.ipType" allow-clear placeholder="IP 类型">
                  <a-select-option value="residential">住宅/家宽</a-select-option>
                  <a-select-option value="hosting">机房</a-select-option>
                  <a-select-option value="mobile">移动网</a-select-option>
                  <a-select-option value="proxy">代理</a-select-option>
                </a-select>
                <a-select v-model:value="filters.quality" allow-clear placeholder="质量">
                  <a-select-option value="normal">普通</a-select-option>
                  <a-select-option value="datacenter">数据中心</a-select-option>
                  <a-select-option value="mobile">移动端</a-select-option>
                  <a-select-option value="proxy">代理风险</a-select-option>
                </a-select>
                <a-select v-model:value="filters.available" allow-clear placeholder="状态">
                  <a-select-option value="true">可用</a-select-option>
                  <a-select-option value="false">不可用</a-select-option>
                </a-select>
                <a-input-number v-model:value="filters.maxLatency" :min="0" placeholder="最大延迟 ms" style="width:100%"></a-input-number>
                <a-input v-model:value="filters.keyword" allow-clear placeholder="IP/ASN/运营商"></a-input>
                <a-input-number v-model:value="filters.limit" :min="10" :max="500" :step="10" style="width:100%"></a-input-number>
              </div>

              <a-table :columns="nodeColumns" :data-source="visibleNodes" :row-key="record => record.id" size="small" bordered :pagination="false" :scroll="{ x: 1280, y: 620 }">
                <template #bodyCell="{ column, record }">
                  <template v-if="column.key === 'region'">
                    <div>{{ record.region }}</div>
                    <div class="muted">{{ record.country || '' }}</div>
                  </template>
                  <template v-else-if="column.key === 'host'">
                    <div class="host-cell">
                      <span class="mono">{{ record.hostname || '-' }}</span>
                      <span class="mono">{{ record.ip || '-' }}</span>
                    </div>
                  </template>
                  <template v-else-if="column.key === 'proto'">{{ record.proto || 'udp' }}:{{ record.port || 1194 }}</template>
                  <template v-else-if="column.key === 'latency'">{{ record.latency_ms ? record.latency_ms + ' ms' : '-' }}</template>
                  <template v-else-if="column.key === 'type'"><a-tag :color="ipTypeColor(record.ip_type)">{{ ipTypeText(record.ip_type) || '未知' }}</a-tag></template>
                  <template v-else-if="column.key === 'purity'"><a-tag :color="purityColor(record.purity_score)">{{ record.purity_score || '未知' }}{{ record.purity_score ? '/100' : '' }} {{ qualityText(record.quality) }}</a-tag></template>
                  <template v-else-if="column.key === 'owner'">
                    <div>{{ record.owner || '-' }}</div>
                    <div class="muted">{{ record.asn || record.as_name || '' }}</div>
                  </template>
                  <template v-else-if="column.key === 'status'">
                    <a-tag :color="record.available ? 'green' : 'red'">{{ record.available ? '可用' : '不可用' }}</a-tag>
                    <div class="muted">{{ record.probe_message || record.fail_reason || '' }}</div>
                  </template>
                  <template v-else-if="column.key === 'actions'">
                    <div class="action-row">
                      <a-button size="small" :loading="probing[record.id]" @click="probeNode(record.id)">测速</a-button>
                      <a-button size="small" type="primary" @click="openNodeSwitchDialog(record)">切换</a-button>
                    </div>
                  </template>
                </template>
              </a-table>
            </div>
          </section>
        </a-tab-pane>

        <a-tab-pane key="channels" tab="通道">
          <section class="card">
            <div class="card-head">
              <div>
                <div class="card-title">通道列表</div>
                <div class="muted">连接方式会展示带账号密码的 HTTP 和 SOCKS5 地址</div>
              </div>
              <a-button type="primary" @click="openChannelDialog()">新增通道</a-button>
            </div>
            <div class="card-body">
              <a-table :columns="channelColumns" :data-source="channels" :row-key="record => record.id" size="small" bordered :pagination="false" :scroll="{ x: 1300 }">
                <template #bodyCell="{ column, record }">
                  <template v-if="column.key === 'mode'">
                    <a-tag color="blue">{{ record.selection_mode }}</a-tag>
                    <span>{{ record.rotate_minutes }} 分钟</span>
                  </template>
                  <template v-else-if="column.key === 'node'">
                    <div class="mono">{{ record.current_node_id || '-' }}</div>
                    <div class="muted">{{ record.last_error || '' }}</div>
                  </template>
                  <template v-else-if="column.key === 'connect'">
                    <div class="connection-box">
                      <div class="connection-line"><a-tag>HTTP</a-tag><code class="mono">{{ proxyAddress(record, 'http') }}</code><a-button size="small" @click="copyText(proxyAddress(record, 'http'))">复制</a-button></div>
                      <div class="connection-line"><a-tag>SOCKS5</a-tag><code class="mono">{{ proxyAddress(record, 'socks5') }}</code><a-button size="small" @click="copyText(proxyAddress(record, 'socks5'))">复制</a-button></div>
                    </div>
                  </template>
                  <template v-else-if="column.key === 'actions'">
                    <div class="action-row">
                      <a-button size="small" @click="openChannelDialog(record)">编辑</a-button>
                      <a-button size="small" type="primary" @click="openChannelSwitchDialog(record)">切换</a-button>
                      <a-button size="small" danger @click="deleteChannel(record.id)">删除</a-button>
                    </div>
                  </template>
                </template>
              </a-table>
            </div>
          </section>
        </a-tab-pane>

        <a-tab-pane key="connections" tab="在线连接">
          <section class="card">
            <div class="card-head"><div class="card-title">在线连接</div></div>
            <div class="card-body">
              <a-table :columns="connectionColumns" :data-source="connections" :row-key="record => record.id" size="small" bordered :pagination="false"></a-table>
            </div>
          </section>
        </a-tab-pane>

        <a-tab-pane key="settings" tab="设置">
          <section class="card">
            <div class="card-head"><div class="card-title">基础设置</div></div>
            <div class="card-body">
              <a-space wrap>
                <a-input v-model:value="settings.node_refresh_interval" placeholder="节点更新间隔，如 20m" style="width: 260px"></a-input>
                <a-button type="primary" @click="saveSettings">保存设置</a-button>
                <span class="muted">保存后重启服务，定时更新间隔才会重新加载。</span>
              </a-space>
            </div>
          </section>
        </a-tab-pane>
      </a-tabs>

      <a-modal v-model:open="channelDialog.open" :title="channelDialog.editing ? '编辑通道' : '新增通道'" width="720px" @ok="saveChannel" ok-text="保存" cancel-text="取消">
        <div class="modal-grid">
          <a-form-item label="ID"><a-input v-model:value="channelForm.id" placeholder="jp-3000"></a-input></a-form-item>
          <a-form-item label="监听地址"><a-input v-model:value="channelForm.listen_host" placeholder="0.0.0.0"></a-input></a-form-item>
          <a-form-item label="端口"><a-input-number v-model:value="channelForm.listen_port" :min="1" :max="65535" style="width:100%"></a-input-number></a-form-item>
          <a-form-item label="地区"><a-input v-model:value="channelForm.region" placeholder="jp/us/kr"></a-input></a-form-item>
          <a-form-item label="轮换分钟"><a-input-number v-model:value="channelForm.rotate_minutes" :min="0" style="width:100%"></a-input-number></a-form-item>
          <a-form-item label="模式">
            <a-select v-model:value="channelForm.selection_mode">
              <a-select-option value="auto">自动优选</a-select-option>
              <a-select-option value="manual">手动节点</a-select-option>
            </a-select>
          </a-form-item>
          <a-form-item label="手动节点 ID"><a-input v-model:value="channelForm.manual_node_id" placeholder="manual 模式填写"></a-input></a-form-item>
          <a-form-item label="启用"><a-switch v-model:checked="channelForm.enabled"></a-switch></a-form-item>
        </div>
      </a-modal>

      <a-modal v-model:open="nodeSwitchDialog.open" title="选择通道" width="560px" @ok="confirmNodeSwitch" ok-text="切换" cancel-text="取消">
        <a-alert v-if="nodeSwitchDialog.node" type="info" show-icon :message="'节点：' + nodeLabel(nodeSwitchDialog.node)" style="margin-bottom: 12px"></a-alert>
        <a-select v-model:value="nodeSwitchDialog.channelID" placeholder="请选择要切换的通道" style="width:100%">
          <a-select-option v-for="ch in channelsByRegion(nodeSwitchDialog.node ? nodeSwitchDialog.node.region : '')" :key="ch.id" :value="ch.id">{{ ch.id }} :{{ ch.listen_port }} / {{ ch.region }}</a-select-option>
        </a-select>
      </a-modal>

      <a-modal v-model:open="channelSwitchDialog.open" title="选择节点" width="920px" @ok="confirmChannelSwitch" ok-text="切换" cancel-text="取消">
        <a-alert v-if="channelSwitchDialog.channel" type="info" show-icon :message="'通道：' + channelSwitchDialog.channel.id + ' / 地区：' + channelSwitchDialog.channel.region" style="margin-bottom: 12px"></a-alert>
        <div class="modal-filter">
          <a-select v-model:value="switchFilters.ipType" allow-clear placeholder="IP 类型">
            <a-select-option value="residential">住宅/家宽</a-select-option>
            <a-select-option value="hosting">机房</a-select-option>
            <a-select-option value="mobile">移动网</a-select-option>
            <a-select-option value="proxy">代理</a-select-option>
          </a-select>
          <a-select v-model:value="switchFilters.quality" allow-clear placeholder="质量">
            <a-select-option value="normal">普通</a-select-option>
            <a-select-option value="datacenter">数据中心</a-select-option>
            <a-select-option value="mobile">移动端</a-select-option>
            <a-select-option value="proxy">代理风险</a-select-option>
          </a-select>
          <a-input-number v-model:value="switchFilters.maxLatency" :min="0" placeholder="最大延迟" style="width:100%"></a-input-number>
          <a-input v-model:value="switchFilters.keyword" allow-clear placeholder="IP/ASN/运营商"></a-input>
        </div>
        <a-table :columns="switchNodeColumns" :data-source="switchDialogNodes" :row-key="record => record.id" size="small" bordered :pagination="{ pageSize: 8 }" :row-selection="{ type: 'radio', selectedRowKeys: channelSwitchDialog.nodeID ? [channelSwitchDialog.nodeID] : [], onChange: onSwitchNodeSelected }">
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'node'">
              <div class="mono">{{ record.id }}</div>
              <div class="muted">{{ record.ip || record.hostname || '-' }}</div>
            </template>
            <template v-else-if="column.key === 'latency'">{{ record.latency_ms ? record.latency_ms + ' ms' : '-' }}</template>
            <template v-else-if="column.key === 'type'"><a-tag :color="ipTypeColor(record.ip_type)">{{ ipTypeText(record.ip_type) || '未知' }}</a-tag></template>
            <template v-else-if="column.key === 'purity'"><a-tag :color="purityColor(record.purity_score)">{{ record.purity_score || '未知' }}{{ record.purity_score ? '/100' : '' }}</a-tag></template>
            <template v-else-if="column.key === 'owner'">{{ record.owner || '-' }}</template>
          </template>
        </a-table>
      </a-modal>
    </main>
  </div>

  <script>
    const { createApp } = Vue;
    const { message, Modal } = antd;
    const apiBase = (() => {
      const path = window.location.pathname.endsWith('/') ? window.location.pathname : window.location.pathname + '/';
      return path.replace(/\/$/, '') + '/api/';
    })();

    createApp({
      data() {
        return {
          apiBase,
          proxyHost: window.location.hostname || 'SERVER_IP',
          activeTab: 'nodes',
          loading: false,
          updatingNodes: false,
          channels: [],
          nodes: [],
          connections: [],
          settings: { node_refresh_interval: '20m' },
          filters: { region: undefined, ipType: undefined, quality: undefined, available: undefined, maxLatency: null, keyword: '', limit: 120 },
          switchFilters: { ipType: undefined, quality: undefined, maxLatency: null, keyword: '' },
          channelForm: this.emptyChannelForm(),
          channelDialog: { open: false, editing: false },
          nodeSwitchDialog: { open: false, node: null, channelID: undefined },
          channelSwitchDialog: { open: false, channel: null, nodeID: undefined },
          probing: {},
          nodeColumns: [
            { title: '地区', key: 'region', width: 88 },
            { title: '主机 / IP', key: 'host', width: 220 },
            { title: '协议', key: 'proto', width: 92 },
            { title: '延迟', key: 'latency', width: 92, sorter: (a, b) => Number(a.latency_ms || 999999) - Number(b.latency_ms || 999999) },
            { title: '类型', key: 'type', width: 118 },
            { title: '纯净度', key: 'purity', width: 122, sorter: (a, b) => Number(b.purity_score || 0) - Number(a.purity_score || 0) },
            { title: 'ASN / 运营商', key: 'owner', width: 190 },
            { title: '状态', key: 'status', width: 170 },
            { title: '操作', key: 'actions', width: 150, fixed: 'right' }
          ],
          channelColumns: [
            { title: 'ID', dataIndex: 'id', width: 130 },
            { title: '端口', dataIndex: 'listen_port', width: 86 },
            { title: '地区', dataIndex: 'region', width: 86 },
            { title: '模式', key: 'mode', width: 140 },
            { title: '当前节点', key: 'node', width: 190 },
            { title: '连接方式', key: 'connect', width: 430 },
            { title: '操作', key: 'actions', width: 230, fixed: 'right' }
          ],
          connectionColumns: [
            { title: 'ID', dataIndex: 'id' },
            { title: '通道', dataIndex: 'channel_id', width: 130 },
            { title: '协议', dataIndex: 'protocol', width: 90 },
            { title: '目标', dataIndex: 'target' },
            { title: '客户端', dataIndex: 'client_addr' }
          ],
          switchNodeColumns: [
            { title: '节点', key: 'node' },
            { title: '延迟', key: 'latency', width: 92 },
            { title: '类型', key: 'type', width: 118 },
            { title: '纯净度', key: 'purity', width: 110 },
            { title: '运营商', key: 'owner' }
          ]
        };
      },
      computed: {
        regions() {
          return Array.from(new Set(this.nodes.map(n => n.region).filter(Boolean))).sort();
        },
        filteredNodes() {
          return this.filterNodeList(this.nodes, this.filters);
        },
        visibleNodes() {
          return this.filteredNodes.slice(0, Number(this.filters.limit || 120));
        },
        switchDialogNodes() {
          if (!this.channelSwitchDialog.channel) return [];
          const nodes = this.nodes.filter(n => n.region === this.channelSwitchDialog.channel.region);
          return this.filterNodeList(nodes, this.switchFilters).slice(0, 200);
        }
      },
      mounted() {
        this.load();
        setInterval(() => this.load(false), 7000);
      },
      methods: {
        emptyChannelForm() {
          return { id: '', listen_host: '0.0.0.0', listen_port: 3000, region: '', rotate_minutes: 0, selection_mode: 'auto', manual_node_id: '', enabled: true };
        },
        filterNodeList(nodes, filters) {
          const keyword = String(filters.keyword || '').toLowerCase();
          return nodes.filter(n => {
            if (filters.region && n.region !== filters.region) return false;
            if (filters.ipType && n.ip_type !== filters.ipType) return false;
            if (filters.quality && n.quality !== filters.quality) return false;
            if (filters.available && String(Boolean(n.available)) !== filters.available) return false;
            if (filters.maxLatency && (!Number(n.latency_ms) || Number(n.latency_ms) > Number(filters.maxLatency))) return false;
            if (keyword) {
              const hay = [n.id, n.ip, n.hostname, n.owner, n.asn, n.as_name, n.location].join(' ').toLowerCase();
              if (!hay.includes(keyword)) return false;
            }
            return true;
          });
        },
        async load(showLoading = true) {
          if (showLoading) this.loading = true;
          try {
            const [status, channels, connections, nodes] = await Promise.all([
              this.request('status'),
              this.request('channels'),
              this.request('connections'),
              this.request('nodes')
            ]);
            this.channels = channels.channels || [];
            this.connections = connections.connections || [];
            this.nodes = nodes.nodes || [];
            if (status.settings) this.settings = Object.assign({}, this.settings, status.settings);
          } catch (err) {
            message.error(err.message);
          } finally {
            this.loading = false;
          }
        },
        async request(path, options = {}) {
          const res = await fetch(apiBase + path.replace(/^\//, ''), Object.assign({ headers: { 'Content-Type': 'application/json' } }, options));
          const body = await res.json().catch(() => ({}));
          if (!res.ok) throw new Error(body.error || '请求失败');
          return body;
        },
        async updateNodes() {
          this.updatingNodes = true;
          try {
            await this.request('nodes/refresh', { method: 'POST' });
            message.success('节点已更新');
            await this.load(false);
          } catch (err) {
            message.error(err.message);
          } finally {
            this.updatingNodes = false;
          }
        },
        async probeNode(nodeID) {
          this.probing[nodeID] = true;
          try {
            await this.request('nodes/' + encodeURIComponent(nodeID) + '/probe', { method: 'POST' });
            message.success('测速完成');
            await this.load(false);
          } catch (err) {
            message.error(err.message);
          } finally {
            this.probing[nodeID] = false;
          }
        },
        openChannelDialog(row) {
          this.channelDialog.open = true;
          this.channelDialog.editing = Boolean(row);
          this.channelForm = row ? {
            id: row.id,
            listen_host: row.listen_host,
            listen_port: row.listen_port,
            region: row.region,
            rotate_minutes: row.rotate_minutes,
            selection_mode: row.selection_mode,
            manual_node_id: row.manual_node_id || row.current_node_id || '',
            enabled: row.enabled
          } : this.emptyChannelForm();
        },
        async saveChannel() {
          const channel = Object.assign({}, this.channelForm, { region: String(this.channelForm.region || '').toLowerCase() });
          if (channel.selection_mode !== 'manual') delete channel.manual_node_id;
          try {
            await this.request('channels', { method: 'POST', body: JSON.stringify(channel) });
            message.success('通道已保存，新增端口需要重启服务生效');
            this.channelDialog.open = false;
            await this.load(false);
          } catch (err) {
            message.error(err.message);
          }
        },
        openNodeSwitchDialog(node) {
          this.nodeSwitchDialog = { open: true, node, channelID: undefined };
        },
        async confirmNodeSwitch() {
          if (!this.nodeSwitchDialog.node || !this.nodeSwitchDialog.channelID) return message.warning('请先选择通道');
          await this.switchChannelNode(this.nodeSwitchDialog.channelID, this.nodeSwitchDialog.node.id);
          this.nodeSwitchDialog.open = false;
        },
        openChannelSwitchDialog(channel) {
          this.switchFilters = { ipType: undefined, quality: undefined, maxLatency: null, keyword: '' };
          this.channelSwitchDialog = { open: true, channel, nodeID: undefined };
        },
        onSwitchNodeSelected(keys) {
          this.channelSwitchDialog.nodeID = keys[0];
        },
        async confirmChannelSwitch() {
          if (!this.channelSwitchDialog.channel || !this.channelSwitchDialog.nodeID) return message.warning('请先选择节点');
          await this.switchChannelNode(this.channelSwitchDialog.channel.id, this.channelSwitchDialog.nodeID);
          this.channelSwitchDialog.open = false;
        },
        async switchChannelNode(channelID, nodeID) {
          if (!channelID) return message.warning('请先选择通道');
          if (!nodeID) return message.warning('请先选择节点');
          try {
            await this.request('channels/' + encodeURIComponent(channelID) + '/switch', { method: 'POST', body: JSON.stringify({ node_id: nodeID }) });
            message.success('已切换节点');
            await this.load(false);
          } catch (err) {
            message.error(err.message);
          }
        },
        async deleteChannel(channelID) {
          Modal.confirm({
            title: '确认删除通道？',
            content: '删除通道 ' + channelID + ' 后需要重启服务生效。',
            okText: '删除',
            okType: 'danger',
            cancelText: '取消',
            onOk: async () => {
              await this.request('channels/' + encodeURIComponent(channelID), { method: 'DELETE' });
              message.success('通道已删除，需要重启服务生效');
              await this.load(false);
            }
          });
        },
        async saveSettings() {
          try {
            await this.request('settings', { method: 'POST', body: JSON.stringify(this.settings) });
            message.success('设置已保存，重启后生效');
          } catch (err) {
            message.error(err.message);
          }
        },
        channelsByRegion(region) {
          return this.channels.filter(ch => ch.region === region && ch.enabled);
        },
        nodeLabel(n) {
          return [n.id, n.latency_ms ? n.latency_ms + 'ms' : '', this.ipTypeText(n.ip_type), n.purity_score ? '纯净' + n.purity_score : ''].filter(Boolean).join(' / ');
        },
        copyText(text) {
          if (!text) return;
          navigator.clipboard.writeText(text).then(() => message.success('已复制')).catch(() => message.warning('浏览器不允许自动复制'));
        },
        proxyAddress(channel, scheme) {
          const source = scheme === 'socks5'
            ? (channel.proxy_auth_socks5 || channel.proxy_url_socks5 || '')
            : (channel.proxy_auth_http || channel.proxy_url_http || '');
          if (!source) return '';
          try {
            const url = new URL(source);
            if (url.hostname === '0.0.0.0' || url.hostname === '127.0.0.1' || url.hostname === 'localhost' || url.hostname === '::') {
              url.hostname = window.location.hostname || url.hostname;
            }
            return url.toString();
          } catch (err) {
            const port = channel.listen_port || '';
            return scheme + '://' + this.proxyHost + (port ? ':' + port : '');
          }
        },
        ipTypeText(type) {
          return ({ residential: '住宅/家宽', hosting: '机房', mobile: '移动网', proxy: '代理' })[type] || type || '';
        },
        qualityText(type) {
          return ({ normal: '普通', datacenter: '数据中心', mobile: '移动端', proxy: '代理风险' })[type] || type || '';
        },
        ipTypeColor(type) {
          return ({ residential: 'green', hosting: 'orange', mobile: 'blue', proxy: 'red' })[type] || 'default';
        },
        purityColor(score) {
          score = Number(score || 0);
          if (score >= 75) return 'green';
          if (score >= 40) return 'orange';
          if (score > 0) return 'red';
          return 'default';
        }
      }
    }).use(antd).mount('#app');
  </script>
</body>
</html>`
