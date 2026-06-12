package admin

const indexHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Region Proxy Gateway</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/element-plus@2.8.8/dist/index.css">
  <script src="https://cdn.jsdelivr.net/npm/vue@3.5.13/dist/vue.global.prod.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/element-plus@2.8.8/dist/index.full.min.js"></script>
  <style>
    :root { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; color: #1f2937; background: #f5f7fb; }
    body { margin: 0; background: #f5f7fb; }
    [v-cloak] { display: none; }
    .topbar { height: 58px; padding: 0 22px; display: flex; align-items: center; justify-content: space-between; background: #fff; border-bottom: 1px solid #e5e7eb; position: sticky; top: 0; z-index: 10; }
    .brand { display: flex; align-items: baseline; gap: 10px; min-width: 0; }
    .brand h1 { margin: 0; font-size: 19px; font-weight: 650; letter-spacing: 0; }
    .brand span { color: #6b7280; font-size: 13px; }
    .page { max-width: 1440px; margin: 0 auto; padding: 18px; }
    .toolbar { display: flex; flex-wrap: wrap; gap: 10px; align-items: center; justify-content: space-between; margin-bottom: 14px; }
    .toolbar-left, .toolbar-right { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
    .stats { display: grid; grid-template-columns: repeat(4, minmax(130px, 1fr)); gap: 12px; margin-bottom: 14px; }
    .stat { background: #fff; border: 1px solid #e5e7eb; border-radius: 8px; padding: 13px 14px; }
    .stat .label { color: #6b7280; font-size: 12px; margin-bottom: 6px; }
    .stat .value { font-size: 22px; font-weight: 650; }
    .panel { background: #fff; border: 1px solid #e5e7eb; border-radius: 8px; margin-bottom: 14px; overflow: hidden; }
    .panel-head { padding: 13px 14px; display: flex; justify-content: space-between; align-items: center; gap: 10px; border-bottom: 1px solid #eef0f3; }
    .panel-title { font-size: 15px; font-weight: 650; }
    .panel-body { padding: 14px; }
    .form-grid { display: grid; grid-template-columns: repeat(4, minmax(150px, 1fr)); gap: 12px; align-items: end; }
    .filter-grid { display: grid; grid-template-columns: repeat(7, minmax(120px, 1fr)); gap: 10px; margin-bottom: 12px; align-items: end; }
    .mono { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 12px; }
    .muted { color: #6b7280; }
    .host-cell { display: grid; gap: 3px; overflow-wrap: anywhere; }
    .actions { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
    .node-action { display: grid; grid-template-columns: minmax(130px, 1fr) auto auto; gap: 8px; align-items: center; min-width: 320px; }
    .copy-lines { display: grid; gap: 4px; }
    .el-tabs__header { margin-bottom: 14px; }
    .el-table .cell { line-height: 1.45; }
    @media (max-width: 1100px) {
      .stats { grid-template-columns: 1fr 1fr; }
      .form-grid, .filter-grid { grid-template-columns: 1fr 1fr; }
    }
    @media (max-width: 700px) {
      .topbar { height: auto; padding: 12px; align-items: flex-start; flex-direction: column; }
      .page { padding: 12px; }
      .stats, .form-grid, .filter-grid { grid-template-columns: 1fr; }
      .node-action { grid-template-columns: 1fr; min-width: 220px; }
    }
  </style>
</head>
<body>
  <div id="app" v-cloak>
    <div class="topbar">
      <div class="brand">
        <h1>Region Proxy Gateway</h1>
        <span>{{ apiBase }}</span>
      </div>
      <div class="actions">
        <el-button :loading="loading" @click="load">刷新</el-button>
        <el-button type="primary" :loading="updatingNodes" @click="updateNodes">更新节点</el-button>
      </div>
    </div>

    <main class="page">
      <div class="stats">
        <div class="stat"><div class="label">通道</div><div class="value">{{ channels.length }}</div></div>
        <div class="stat"><div class="label">节点</div><div class="value">{{ filteredNodes.length }} / {{ nodes.length }}</div></div>
        <div class="stat"><div class="label">在线连接</div><div class="value">{{ connections.length }}</div></div>
        <div class="stat"><div class="label">节点更新间隔</div><div class="value">{{ settings.node_refresh_interval || '-' }}</div></div>
      </div>

      <el-tabs v-model="activeTab">
        <el-tab-pane label="节点" name="nodes">
          <section class="panel">
            <div class="panel-head">
              <div class="panel-title">节点列表</div>
              <div class="muted">可筛选、测速，并把指定通道切换到当前节点</div>
            </div>
            <div class="panel-body">
              <div class="filter-grid">
                <el-select v-model="filters.region" clearable placeholder="地区">
                  <el-option v-for="item in regions" :key="item" :label="item" :value="item"></el-option>
                </el-select>
                <el-select v-model="filters.ipType" clearable placeholder="IP 类型">
                  <el-option label="住宅/家宽" value="residential"></el-option>
                  <el-option label="机房" value="hosting"></el-option>
                  <el-option label="移动网" value="mobile"></el-option>
                  <el-option label="代理" value="proxy"></el-option>
                </el-select>
                <el-select v-model="filters.quality" clearable placeholder="质量">
                  <el-option label="普通" value="normal"></el-option>
                  <el-option label="数据中心" value="datacenter"></el-option>
                  <el-option label="移动端" value="mobile"></el-option>
                  <el-option label="代理风险" value="proxy"></el-option>
                </el-select>
                <el-select v-model="filters.available" clearable placeholder="状态">
                  <el-option label="可用" value="true"></el-option>
                  <el-option label="不可用" value="false"></el-option>
                </el-select>
                <el-input-number v-model="filters.maxLatency" :min="0" :controls="false" placeholder="最大延迟"></el-input-number>
                <el-input v-model="filters.keyword" clearable placeholder="IP/ASN/运营商"></el-input>
                <el-input-number v-model="filters.limit" :min="10" :max="500" :step="10" placeholder="显示数量"></el-input-number>
              </div>

              <el-table :data="visibleNodes" stripe border height="640">
                <el-table-column prop="region" label="地区" width="86">
                  <template #default="{ row }">
                    <div>{{ row.region }}</div>
                    <div class="muted">{{ row.country || '' }}</div>
                  </template>
                </el-table-column>
                <el-table-column label="主机 / IP" min-width="210">
                  <template #default="{ row }">
                    <div class="host-cell">
                      <span class="mono">{{ row.hostname || '-' }}</span>
                      <span class="mono">{{ row.ip || '-' }}</span>
                    </div>
                  </template>
                </el-table-column>
                <el-table-column label="协议" width="96">
                  <template #default="{ row }">{{ row.proto || 'udp' }}:{{ row.port || 1194 }}</template>
                </el-table-column>
                <el-table-column label="延迟" width="92">
                  <template #default="{ row }">{{ row.latency_ms ? row.latency_ms + ' ms' : '-' }}</template>
                </el-table-column>
                <el-table-column label="类型" width="116">
                  <template #default="{ row }">
                    <el-tag :type="ipTypeTag(row.ip_type)" effect="light">{{ ipTypeText(row.ip_type) || '未知' }}</el-tag>
                  </template>
                </el-table-column>
                <el-table-column label="纯净度" width="130">
                  <template #default="{ row }">
                    <el-tag :type="purityTag(row.purity_score)" effect="light">{{ row.purity_score || '未知' }}{{ row.purity_score ? '/100' : '' }} {{ qualityText(row.quality) }}</el-tag>
                  </template>
                </el-table-column>
                <el-table-column label="ASN / 运营商" min-width="190">
                  <template #default="{ row }">
                    <div>{{ row.owner || '-' }}</div>
                    <div class="muted">{{ row.asn || row.as_name || '' }}</div>
                  </template>
                </el-table-column>
                <el-table-column label="状态" min-width="160">
                  <template #default="{ row }">
                    <el-tag :type="row.available ? 'success' : 'danger'" effect="light">{{ row.available ? '可用' : '不可用' }}</el-tag>
                    <div class="muted">{{ row.probe_message || row.fail_reason || '' }}</div>
                  </template>
                </el-table-column>
                <el-table-column label="操作" width="370" fixed="right">
                  <template #default="{ row }">
                    <div class="node-action">
                      <el-select v-model="nodeChannel[row.id]" placeholder="选择通道">
                        <el-option v-for="ch in channelsByRegion(row.region)" :key="ch.id" :label="ch.id + ' :' + ch.listen_port" :value="ch.id"></el-option>
                      </el-select>
                      <el-button :loading="probing[row.id]" @click="probeNode(row.id)">测速</el-button>
                      <el-button type="primary" @click="switchChannelNode(nodeChannel[row.id], row.id)">切换</el-button>
                    </div>
                  </template>
                </el-table-column>
              </el-table>
            </div>
          </section>
        </el-tab-pane>

        <el-tab-pane label="通道" name="channels">
          <section class="panel">
            <div class="panel-head">
              <div class="panel-title">新建或更新通道</div>
              <el-button @click="resetChannelForm">清空</el-button>
            </div>
            <div class="panel-body">
              <div class="form-grid">
                <el-input v-model="channelForm.id" placeholder="ID，如 jp-3000"></el-input>
                <el-input v-model="channelForm.listen_host" placeholder="监听地址"></el-input>
                <el-input-number v-model="channelForm.listen_port" :min="1" :max="65535" :controls="false" placeholder="端口"></el-input-number>
                <el-input v-model="channelForm.region" placeholder="地区，如 jp/us"></el-input>
                <el-input-number v-model="channelForm.rotate_minutes" :min="0" :controls="false" placeholder="轮换分钟"></el-input-number>
                <el-select v-model="channelForm.selection_mode" placeholder="模式">
                  <el-option label="自动优选" value="auto"></el-option>
                  <el-option label="手动节点" value="manual"></el-option>
                </el-select>
                <el-input v-model="channelForm.manual_node_id" placeholder="手动节点 ID"></el-input>
                <el-switch v-model="channelForm.enabled" active-text="启用"></el-switch>
                <el-button type="primary" @click="saveChannel">保存通道</el-button>
              </div>
            </div>
          </section>

          <section class="panel">
            <div class="panel-head"><div class="panel-title">通道列表</div></div>
            <div class="panel-body">
              <el-table :data="channels" stripe border>
                <el-table-column prop="id" label="ID" width="130"></el-table-column>
                <el-table-column prop="listen_port" label="端口" width="88"></el-table-column>
                <el-table-column prop="region" label="地区" width="86"></el-table-column>
                <el-table-column label="模式" width="130">
                  <template #default="{ row }">{{ row.selection_mode }} / {{ row.rotate_minutes }} 分钟</template>
                </el-table-column>
                <el-table-column prop="current_node_id" label="当前节点" min-width="180"></el-table-column>
                <el-table-column label="代理地址" min-width="260">
                  <template #default="{ row }">
                    <div class="copy-lines">
                      <span class="mono">{{ row.proxy_url_http }}</span>
                      <span class="mono">{{ row.proxy_url_socks5 }}</span>
                    </div>
                  </template>
                </el-table-column>
                <el-table-column label="操作" width="320">
                  <template #default="{ row }">
                    <div class="actions">
                      <el-button @click="editChannel(row)">编辑</el-button>
                      <el-select v-model="channelNode[row.id]" placeholder="选择节点" style="width: 145px">
                        <el-option v-for="n in nodesByRegion(row.region)" :key="n.id" :label="nodeLabel(n)" :value="n.id"></el-option>
                      </el-select>
                      <el-button type="primary" @click="switchChannelNode(row.id, channelNode[row.id])">切换</el-button>
                      <el-button type="danger" @click="deleteChannel(row.id)">删除</el-button>
                    </div>
                  </template>
                </el-table-column>
              </el-table>
            </div>
          </section>
        </el-tab-pane>

        <el-tab-pane label="在线连接" name="connections">
          <section class="panel">
            <div class="panel-head"><div class="panel-title">在线连接</div></div>
            <div class="panel-body">
              <el-table :data="connections" stripe border>
                <el-table-column prop="id" label="ID" min-width="180"></el-table-column>
                <el-table-column prop="channel_id" label="通道" width="130"></el-table-column>
                <el-table-column prop="protocol" label="协议" width="90"></el-table-column>
                <el-table-column prop="target" label="目标" min-width="220"></el-table-column>
                <el-table-column prop="client_addr" label="客户端" min-width="170"></el-table-column>
              </el-table>
            </div>
          </section>
        </el-tab-pane>

        <el-tab-pane label="设置" name="settings">
          <section class="panel">
            <div class="panel-head"><div class="panel-title">基础设置</div></div>
            <div class="panel-body">
              <div class="form-grid">
                <el-input v-model="settings.node_refresh_interval" placeholder="节点更新间隔，如 20m"></el-input>
                <el-button type="primary" @click="saveSettings">保存设置</el-button>
                <span class="muted">保存后重启服务，定时更新间隔才会重新加载。</span>
              </div>
            </div>
          </section>
        </el-tab-pane>
      </el-tabs>
    </main>
  </div>

  <script>
    const { createApp } = Vue;
    const apiBase = (() => {
      const path = window.location.pathname.endsWith('/') ? window.location.pathname : window.location.pathname + '/';
      return path.replace(/\/$/, '') + '/api/';
    })();

    createApp({
      data() {
        return {
          apiBase,
          activeTab: 'nodes',
          loading: false,
          updatingNodes: false,
          channels: [],
          nodes: [],
          connections: [],
          settings: { node_refresh_interval: '20m' },
          filters: { region: '', ipType: '', quality: '', available: '', maxLatency: null, keyword: '', limit: 120 },
          channelForm: this.emptyChannelForm(),
          channelNode: {},
          nodeChannel: {},
          probing: {}
        };
      },
      computed: {
        regions() {
          return Array.from(new Set(this.nodes.map(n => n.region).filter(Boolean))).sort();
        },
        filteredNodes() {
          const keyword = String(this.filters.keyword || '').toLowerCase();
          return this.nodes.filter(n => {
            if (this.filters.region && n.region !== this.filters.region) return false;
            if (this.filters.ipType && n.ip_type !== this.filters.ipType) return false;
            if (this.filters.quality && n.quality !== this.filters.quality) return false;
            if (this.filters.available && String(Boolean(n.available)) !== this.filters.available) return false;
            if (this.filters.maxLatency && (!Number(n.latency_ms) || Number(n.latency_ms) > Number(this.filters.maxLatency))) return false;
            if (keyword) {
              const hay = [n.id, n.ip, n.hostname, n.owner, n.asn, n.as_name, n.location].join(' ').toLowerCase();
              if (!hay.includes(keyword)) return false;
            }
            return true;
          });
        },
        visibleNodes() {
          return this.filteredNodes.slice(0, Number(this.filters.limit || 120));
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
            ElementPlus.ElMessage.error(err.message);
          } finally {
            this.loading = false;
          }
        },
        async request(path, options = {}) {
          const res = await fetch(apiBase + path.replace(/^\//, ''), Object.assign({
            headers: { 'Content-Type': 'application/json' }
          }, options));
          const body = await res.json().catch(() => ({}));
          if (!res.ok) throw new Error(body.error || '请求失败');
          return body;
        },
        async updateNodes() {
          this.updatingNodes = true;
          try {
            await this.request('nodes/refresh', { method: 'POST' });
            ElementPlus.ElMessage.success('节点已更新');
            await this.load(false);
          } catch (err) {
            ElementPlus.ElMessage.error(err.message);
          } finally {
            this.updatingNodes = false;
          }
        },
        async probeNode(nodeID) {
          this.probing[nodeID] = true;
          try {
            await this.request('nodes/' + encodeURIComponent(nodeID) + '/probe', { method: 'POST' });
            ElementPlus.ElMessage.success('测速完成');
            await this.load(false);
          } catch (err) {
            ElementPlus.ElMessage.error(err.message);
          } finally {
            this.probing[nodeID] = false;
          }
        },
        async switchChannelNode(channelID, nodeID) {
          if (!channelID) return ElementPlus.ElMessage.warning('请先选择通道');
          if (!nodeID) return ElementPlus.ElMessage.warning('请先选择节点');
          try {
            await this.request('channels/' + encodeURIComponent(channelID) + '/switch', {
              method: 'POST',
              body: JSON.stringify({ node_id: nodeID })
            });
            ElementPlus.ElMessage.success('已切换节点');
            await this.load(false);
          } catch (err) {
            ElementPlus.ElMessage.error(err.message);
          }
        },
        async saveChannel() {
          const channel = Object.assign({}, this.channelForm, { region: String(this.channelForm.region || '').toLowerCase() });
          if (channel.selection_mode !== 'manual') delete channel.manual_node_id;
          try {
            await this.request('channels', { method: 'POST', body: JSON.stringify(channel) });
            ElementPlus.ElMessage.success('通道已保存，新增端口需要重启服务生效');
            await this.load(false);
          } catch (err) {
            ElementPlus.ElMessage.error(err.message);
          }
        },
        editChannel(row) {
          this.channelForm = {
            id: row.id,
            listen_host: row.listen_host,
            listen_port: row.listen_port,
            region: row.region,
            rotate_minutes: row.rotate_minutes,
            selection_mode: row.selection_mode,
            manual_node_id: row.manual_node_id || row.current_node_id || '',
            enabled: row.enabled
          };
          this.activeTab = 'channels';
          window.scrollTo({ top: 0, behavior: 'smooth' });
        },
        resetChannelForm() {
          this.channelForm = this.emptyChannelForm();
        },
        async deleteChannel(channelID) {
          try {
            await ElementPlus.ElMessageBox.confirm('删除通道 ' + channelID + '？保存后需要重启服务生效。', '确认删除', { type: 'warning' });
            await this.request('channels/' + encodeURIComponent(channelID), { method: 'DELETE' });
            ElementPlus.ElMessage.success('通道已删除，需要重启服务生效');
            await this.load(false);
          } catch (err) {
            if (err !== 'cancel') ElementPlus.ElMessage.error(err.message || String(err));
          }
        },
        async saveSettings() {
          try {
            await this.request('settings', { method: 'POST', body: JSON.stringify(this.settings) });
            ElementPlus.ElMessage.success('设置已保存，重启后生效');
          } catch (err) {
            ElementPlus.ElMessage.error(err.message);
          }
        },
        nodesByRegion(region) {
          return this.nodes.filter(n => n.region === region).slice(0, 150);
        },
        channelsByRegion(region) {
          return this.channels.filter(ch => ch.region === region && ch.enabled);
        },
        nodeLabel(n) {
          return [n.id, n.latency_ms ? n.latency_ms + 'ms' : '', this.ipTypeText(n.ip_type), n.purity_score ? '纯净' + n.purity_score : ''].filter(Boolean).join(' / ');
        },
        ipTypeText(type) {
          return ({ residential: '住宅/家宽', hosting: '机房', mobile: '移动网', proxy: '代理' })[type] || type || '';
        },
        qualityText(type) {
          return ({ normal: '普通', datacenter: '数据中心', mobile: '移动端', proxy: '代理风险' })[type] || type || '';
        },
        ipTypeTag(type) {
          if (type === 'residential') return 'success';
          if (type === 'proxy') return 'danger';
          if (type === 'hosting') return 'warning';
          return 'info';
        },
        purityTag(score) {
          score = Number(score || 0);
          if (score >= 75) return 'success';
          if (score >= 40) return 'warning';
          if (score > 0) return 'danger';
          return 'info';
        }
      }
    }).use(ElementPlus).mount('#app');
  </script>
</body>
</html>`
