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
  <script src="https://cdn.jsdelivr.net/npm/dayjs@1.11.13/plugin/advancedFormat.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/dayjs@1.11.13/plugin/customParseFormat.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/dayjs@1.11.13/plugin/localeData.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/dayjs@1.11.13/plugin/quarterOfYear.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/dayjs@1.11.13/plugin/weekOfYear.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/dayjs@1.11.13/plugin/weekYear.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/dayjs@1.11.13/plugin/weekday.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/ant-design-vue@4.2.6/dist/antd.min.js"></script>
  <style>
    :root { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; color: #172033; background: #f2f6fb; }
    body { margin: 0; background: #f2f6fb; }
    [v-cloak] { display: none; }
    .shell { min-height: 100vh; background: #f2f6fb; }
    .topbar { position: sticky; top: 0; z-index: 20; min-height: 76px; padding: 0 28px; display: flex; align-items: center; justify-content: space-between; background: #162233; border-bottom: 1px solid rgba(255,255,255,.08); box-shadow: 0 10px 28px rgba(18, 32, 52, .18); }
    .brand { display: grid; gap: 4px; min-width: 0; color: #fff; }
    .brand h1 { margin: 0; font-size: 21px; font-weight: 750; letter-spacing: 0; }
    .brand span { color: #aebbd0; font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .page { max-width: 1560px; margin: 0 auto; padding: 28px 24px 40px; }
    .stats { display: grid; grid-template-columns: repeat(5, minmax(150px, 1fr)); gap: 14px; margin-bottom: 20px; }
    .stat { background: #fff; border: 1px solid #dfe7f0; border-radius: 8px; padding: 16px; box-shadow: 0 10px 24px rgba(23, 32, 51, .06); }
    .stat-label { color: #697386; font-size: 12px; margin-bottom: 8px; }
    .stat-value { font-size: 24px; font-weight: 760; color: #111827; }
    .content-panel { background: #fff; border: 1px solid #dfe7f0; border-radius: 8px; box-shadow: 0 14px 30px rgba(23, 32, 51, .07); overflow: hidden; }
    .content-panel .ant-tabs-nav { padding: 0 24px; margin: 0 !important; background: #fff; border-bottom: 1px solid #edf1f6; }
    .content-panel .ant-tabs-content-holder { padding: 20px 24px 24px; }
    .card { background: #fff; border: 1px solid #dfe7f0; border-radius: 8px; margin-bottom: 0; overflow: hidden; }
    .card-head { padding: 16px 18px; display: flex; align-items: center; justify-content: space-between; gap: 12px; border-bottom: 1px solid #edf1f6; background: #fbfcfe; }
    .card-title { font-size: 16px; font-weight: 760; }
    .card-body { padding: 18px; }
    .filter-grid { display: grid; grid-template-columns: repeat(7, minmax(120px, 1fr)); gap: 10px; margin-bottom: 12px; }
    .modal-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
    .mono { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 12px; }
    .muted { color: #697386; }
    .host-cell { display: grid; gap: 3px; overflow-wrap: anywhere; }
    .action-row { display: flex; flex-wrap: nowrap; gap: 8px; align-items: center; }
    .connection-box { display: grid; gap: 8px; }
    .connection-line { display: grid; grid-template-columns: 66px minmax(0, 1fr) auto; gap: 8px; align-items: center; padding: 8px; border: 1px solid #e6ecf3; background: #f8fafc; border-radius: 8px; min-width: 0; }
    .connection-line code { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: #0f172a; }
    .modal-filter { display: grid; grid-template-columns: repeat(4, minmax(120px, 1fr)); gap: 10px; margin-bottom: 12px; }
    .ant-tabs-nav { margin-bottom: 0 !important; }
    .ant-table-cell { vertical-align: top; }
    .ant-tabs-tab { font-weight: 650; }
    .ant-tabs-tab-btn { color: #334155; }
    .ant-table-wrapper .ant-table-thead > tr > th { background: #f6f8fb; font-weight: 700; color: #334155; }
    .topbar .ant-btn-default { border-color: rgba(255,255,255,.25); background: rgba(255,255,255,.08); color: #fff; }
    .topbar .ant-btn-default:hover { border-color: #8fb7ff; color: #fff; background: rgba(255,255,255,.14); }
    @media (max-width: 1100px) {
      .stats { grid-template-columns: 1fr 1fr; }
      .filter-grid, .modal-filter { grid-template-columns: 1fr 1fr; }
    }
    @media (max-width: 720px) {
      .topbar { height: auto; min-height: 60px; padding: 12px; align-items: flex-start; flex-direction: column; gap: 12px; }
      .page { padding: 12px; }
      .stats, .filter-grid, .modal-grid, .modal-filter { grid-template-columns: 1fr; }
      .connection-line { grid-template-columns: 1fr; }
      .content-panel .ant-tabs-nav { padding: 0 12px; }
      .content-panel .ant-tabs-content-holder { padding: 12px; }
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
        <a-button danger :loading="restarting" @click="restartService">重启服务</a-button>
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

      <a-tabs class="content-panel" v-model:active-key="activeTab">
        <a-tab-pane key="nodes" tab="节点">
          <section class="card">
            <div class="card-head">
              <div>
                <div class="card-title">节点列表</div>
                <div class="muted">点击切换会弹窗选择通道，然后把该通道切到当前节点</div>
              </div>
              <a-space>
                <a-button :loading="probingBatch" @click="probeVisibleNodes">测试当前列表延迟</a-button>
              </a-space>
            </div>
            <div class="card-body">
              <div class="filter-grid">
                <a-select v-model:value="filters.region" allow-clear placeholder="地区">
                  <a-select-option v-for="item in regions" :key="item" :value="item">{{ regionText(item) }}（{{ item }}）</a-select-option>
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

              <a-table :columns="nodeColumns" :data-source="visibleNodes" :row-key="record => record.id" size="small" bordered :pagination="false" :scroll="{ x: 1280, y: 620 }"></a-table>
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
              <a-table :columns="channelColumns" :data-source="channels" :row-key="record => record.id" size="small" bordered :pagination="false" :scroll="{ x: 1480 }"></a-table>
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
        <a-table :columns="switchNodeColumns" :data-source="switchDialogNodes" :row-key="record => record.id" size="small" bordered :pagination="{ pageSize: 8 }" :row-selection="{ type: 'radio', selectedRowKeys: channelSwitchDialog.nodeID ? [channelSwitchDialog.nodeID] : [], onChange: onSwitchNodeSelected }"></a-table>
      </a-modal>
    </main>
  </div>

  <script>
    const { createApp, h } = Vue;
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
          restarting: false,
          probingBatch: false,
          channels: [],
          nodes: [],
          connections: [],
          settings: { node_refresh_interval: '20m' },
          filters: { region: undefined, ipType: undefined, quality: undefined, available: undefined, maxLatency: null, keyword: '', limit: 120 },
          switchFilters: { ipType: undefined, quality: undefined, maxLatency: null, keyword: '' },
          channelForm: this.emptyChannelForm(),
          channelDialog: { open: false, editing: false, originalID: '' },
          nodeSwitchDialog: { open: false, node: null, channelID: undefined },
          channelSwitchDialog: { open: false, channel: null, nodeID: undefined },
          probing: {},
          nodeColumns: [
            { title: '地区', key: 'region', width: 110, customRender: ({ record }) => h('div', [h('div', this.regionText(record.region, record.country)), h('div', { class: 'muted' }, record.location || record.country || '')]) },
            { title: '主机 / IP', key: 'host', width: 220, customRender: ({ record }) => h('div', { class: 'host-cell' }, [h('span', { class: 'mono' }, record.hostname || '-'), h('span', { class: 'mono' }, record.ip || '-')]) },
            { title: '协议', key: 'proto', width: 92, customRender: ({ record }) => (record.proto || 'udp') + ':' + (record.port || 1194) },
            { title: '实时延迟', key: 'latency', width: 106, sorter: (a, b) => Number(a.latency_ms || 999999) - Number(b.latency_ms || 999999), customRender: ({ record }) => record.latency_ms ? record.latency_ms + ' ms' : '未测' },
            { title: '类型', key: 'type', width: 118, customRender: ({ record }) => h(antd.Tag, { color: this.ipTypeColor(record.ip_type) }, () => this.ipTypeText(record.ip_type) || '未知') },
            { title: '纯净度', key: 'purity', width: 122, sorter: (a, b) => Number(b.purity_score || 0) - Number(a.purity_score || 0), customRender: ({ record }) => h(antd.Tag, { color: this.purityColor(record.purity_score) }, () => (record.purity_score ? record.purity_score + '/100 ' : '未知 ') + this.qualityText(record.quality)) },
            { title: 'ASN / 运营商', key: 'owner', width: 190, customRender: ({ record }) => h('div', [h('div', record.owner || '-'), h('div', { class: 'muted' }, record.asn || record.as_name || '')]) },
            { title: '状态', key: 'status', width: 180, customRender: ({ record }) => h('div', [h(antd.Tag, { color: record.available ? 'green' : 'red' }, () => record.available ? '可用' : '不可用'), h('div', { class: 'muted' }, this.probeMessageText(record))]) },
            { title: '操作', key: 'actions', width: 150, fixed: 'right', customRender: ({ record }) => h('div', { class: 'action-row' }, [h(antd.Button, { size: 'small', loading: Boolean(this.probing[record.id]), onClick: () => this.probeNode(record.id) }, () => '测速'), h(antd.Button, { size: 'small', type: 'primary', onClick: () => this.openNodeSwitchDialog(record) }, () => '切换')]) }
          ],
          channelColumns: [
            { title: 'ID', dataIndex: 'id', width: 120 },
            { title: '端口', dataIndex: 'listen_port', width: 84 },
            { title: '地区', dataIndex: 'region', width: 120, customRender: ({ record }) => this.regionText(record.region) + '（' + record.region + '）' },
            { title: '模式', key: 'mode', width: 150, customRender: ({ record }) => h('div', [h(antd.Tag, { color: record.selection_mode === 'manual' ? 'purple' : 'blue' }, () => this.selectionModeText(record.selection_mode)), h('span', record.rotate_minutes ? record.rotate_minutes + ' 分钟轮换' : '固定')]) },
            { title: '当前节点', key: 'node', width: 280, customRender: ({ record }) => h('div', [h('div', { class: 'mono' }, record.current_node_id || '-'), h('div', { class: record.last_error ? '' : 'muted' }, record.last_error ? this.channelErrorText(record.last_error) : '网络失败会自动重试并切换')]) },
            { title: '连接方式', key: 'connect', width: 560, customRender: ({ record }) => h('div', { class: 'connection-box' }, [this.connectionLine('HTTP', this.proxyAddress(record, 'http')), this.connectionLine('SOCKS5', this.proxyAddress(record, 'socks5'))]) },
            { title: '操作', key: 'actions', width: 170, fixed: 'right', customRender: ({ record }) => h('div', { class: 'action-row' }, [h(antd.Button, { size: 'small', onClick: () => this.openChannelDialog(record) }, () => '编辑'), h(antd.Button, { size: 'small', type: 'primary', onClick: () => this.openChannelSwitchDialog(record) }, () => '切换'), h(antd.Button, { size: 'small', danger: true, onClick: () => this.deleteChannel(record.id) }, () => '删除')]) }
          ],
          connectionColumns: [
            { title: 'ID', dataIndex: 'id' },
            { title: '通道', dataIndex: 'channel_id', width: 130 },
            { title: '协议', dataIndex: 'protocol', width: 90 },
            { title: '目标', dataIndex: 'target' },
            { title: '客户端', dataIndex: 'client_addr' }
          ],
          switchNodeColumns: [
            { title: '节点', key: 'node', customRender: ({ record }) => h('div', [h('div', { class: 'mono' }, record.id), h('div', { class: 'muted' }, record.ip || record.hostname || '-')]) },
            { title: '实时延迟', key: 'latency', width: 106, customRender: ({ record }) => record.latency_ms ? record.latency_ms + ' ms' : '未测' },
            { title: '类型', key: 'type', width: 118, customRender: ({ record }) => h(antd.Tag, { color: this.ipTypeColor(record.ip_type) }, () => this.ipTypeText(record.ip_type) || '未知') },
            { title: '纯净度', key: 'purity', width: 110, customRender: ({ record }) => h(antd.Tag, { color: this.purityColor(record.purity_score) }, () => record.purity_score ? record.purity_score + '/100' : '未知') },
            { title: '运营商', key: 'owner', customRender: ({ record }) => record.owner || '-' }
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
        async restartService() {
          Modal.confirm({
            title: '确认重启服务？',
            content: '会断开当前代理连接，systemd 通常会在几秒内自动拉起服务。',
            okText: '重启',
            okType: 'danger',
            cancelText: '取消',
            onOk: async () => {
              this.restarting = true;
              try {
                await this.request('system/restart', { method: 'POST' });
                message.success('已发送重启命令，请等待几秒后刷新页面');
              } catch (err) {
                message.error(err.message);
              } finally {
                this.restarting = false;
              }
            }
          });
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
        async probeVisibleNodes() {
          const nodeIDs = this.visibleNodes.map(n => n.id).filter(Boolean).slice(0, 120);
          if (!nodeIDs.length) return message.warning('当前列表没有可测速的节点');
          this.probingBatch = true;
          try {
            const body = await this.request('nodes/probe-batch', { method: 'POST', body: JSON.stringify({ node_ids: nodeIDs }) });
            message.success('已测试 ' + (body.count || 0) + ' 个节点');
            await this.load(false);
          } catch (err) {
            message.error(err.message);
          } finally {
            this.probingBatch = false;
          }
        },
        openChannelDialog(row) {
          this.channelDialog.open = true;
          this.channelDialog.editing = Boolean(row);
          this.channelDialog.originalID = row ? row.id : '';
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
          const channel = Object.assign({}, this.channelForm, { region: String(this.channelForm.region || '').toLowerCase(), original_id: this.channelDialog.originalID });
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
        connectionLine(label, value) {
          return h('div', { class: 'connection-line' }, [
            h(antd.Tag, null, () => label),
            h('code', { class: 'mono', title: value }, value),
            h(antd.Button, { size: 'small', onClick: () => this.copyText(value) }, () => '复制')
          ]);
        },
        nodeLabel(n) {
          return [n.id, this.regionText(n.region, n.country), n.latency_ms ? n.latency_ms + 'ms' : '', this.ipTypeText(n.ip_type), n.purity_score ? '纯净' + n.purity_score : ''].filter(Boolean).join(' / ');
        },
        copyText(text) {
          if (!text) return;
          if (navigator.clipboard && window.isSecureContext) {
            navigator.clipboard.writeText(text).then(() => message.success('已复制')).catch(() => this.copyTextFallback(text));
            return;
          }
          this.copyTextFallback(text);
        },
        copyTextFallback(text) {
          const input = document.createElement('textarea');
          input.value = text;
          input.setAttribute('readonly', '');
          input.style.position = 'fixed';
          input.style.left = '-9999px';
          document.body.appendChild(input);
          input.select();
          try {
            document.execCommand('copy') ? message.success('已复制') : message.warning('复制失败，请手动选中复制');
          } catch (err) {
            message.warning('复制失败，请手动选中复制');
          } finally {
            document.body.removeChild(input);
          }
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
        regionText(region, country) {
          const value = String(region || '').toLowerCase();
          const byRegion = {
            jp: '日本', us: '美国', kr: '韩国', hk: '中国香港', tw: '中国台湾', sg: '新加坡',
            gb: '英国', uk: '英国', de: '德国', fr: '法国', ca: '加拿大', au: '澳大利亚',
            nl: '荷兰', ru: '俄罗斯', in: '印度', th: '泰国', vn: '越南', id: '印度尼西亚',
            my: '马来西亚', ph: '菲律宾', br: '巴西', mx: '墨西哥', tr: '土耳其'
          };
          const byCountry = {
            Japan: '日本', 'United States': '美国', Korea: '韩国', 'Korea Republic of': '韩国',
            Singapore: '新加坡', Germany: '德国', France: '法国', Canada: '加拿大',
            Australia: '澳大利亚', Netherlands: '荷兰', Russia: '俄罗斯', India: '印度',
            Thailand: '泰国', Vietnam: '越南', Indonesia: '印度尼西亚', Malaysia: '马来西亚',
            Philippines: '菲律宾', Brazil: '巴西', Mexico: '墨西哥', Turkey: '土耳其',
            'United Kingdom': '英国'
          };
          return byRegion[value] || byCountry[country] || country || region || '';
        },
        selectionModeText(mode) {
          return ({ auto: '自动优选', manual: '手动节点' })[mode] || mode || '';
        },
        channelErrorText(text) {
          return String(text || '')
            .replace('dial failed after 3 retries', '访问失败并重试 3 次后仍失败')
            .replace('and node rotation', '，切换节点后')
            .replace('rotate failed', '切换节点失败');
        },
        qualityText(type) {
          return ({ normal: '普通', datacenter: '数据中心', mobile: '移动端', proxy: '代理风险' })[type] || type || '';
        },
        probeMessageText(record) {
          const text = record.probe_message || record.fail_reason || '';
          if (!text) return '';
          return text
            .replace('ping ok; udp port cannot be fully verified without vpn handshake', 'Ping 正常；UDP 端口需建立 VPN 后才能完全确认')
            .replace('ping ok; tcp port ok', 'Ping 正常；TCP 端口可连接')
            .replace('tcp port ok', 'TCP 端口可连接')
            .replace('ping failed; udp cannot be verified without vpn handshake:', 'Ping 失败；UDP 节点需建立 VPN 才能完全确认：')
            .replace('ping failed:', 'Ping 失败：')
            .replace('tcp connect failed:', 'TCP 连接失败：')
            .replace('missing host', '缺少主机或 IP');
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
