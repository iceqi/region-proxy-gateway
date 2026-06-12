package admin

const indexHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Region Proxy Gateway</title>
  <style>
    :root { color-scheme: light; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    body { margin: 0; background: #f5f7fa; color: #1f2933; }
    header { padding: 16px 22px; background: #fff; border-bottom: 1px solid #dde3ea; display: flex; align-items: center; justify-content: space-between; gap: 14px; }
    h1 { margin: 0; font-size: 21px; letter-spacing: 0; }
    main { max-width: 1320px; margin: 0 auto; padding: 18px; }
    section { margin-bottom: 20px; }
    h2 { font-size: 16px; margin: 0 0 10px; display: flex; gap: 10px; align-items: center; flex-wrap: wrap; }
    table { width: 100%; border-collapse: collapse; background: #fff; border: 1px solid #dde3ea; }
    th, td { padding: 9px 10px; border-bottom: 1px solid #edf1f5; text-align: left; font-size: 13px; vertical-align: top; }
    th { background: #eef3f8; color: #344054; white-space: nowrap; }
    code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; }
    label { display: grid; gap: 5px; font-size: 13px; color: #344054; }
    input, select { min-height: 33px; border: 1px solid #c9d3df; border-radius: 6px; padding: 6px 8px; font: inherit; background: #fff; min-width: 0; }
    button { min-height: 33px; border: 1px solid #b8c4d1; border-radius: 6px; padding: 6px 10px; background: #fff; cursor: pointer; font: inherit; }
    button.primary { background: #1769aa; color: #fff; border-color: #1769aa; }
    button.danger { color: #b42318; border-color: #f1b8b2; }
    button:disabled { opacity: .55; cursor: wait; }
    .grid { display: grid; grid-template-columns: repeat(6, minmax(110px, 1fr)); gap: 11px; align-items: end; background: #fff; border: 1px solid #dde3ea; padding: 14px; }
    .filters { display: grid; grid-template-columns: repeat(7, minmax(110px, 1fr)); gap: 10px; align-items: end; background: #fff; border: 1px solid #dde3ea; padding: 12px; margin-bottom: 10px; }
    .muted { color: #667085; }
    .notice { padding: 10px 12px; background: #fff8db; border: 1px solid #ead88b; margin-bottom: 14px; font-size: 14px; display: none; }
    .actions { display: flex; flex-wrap: wrap; gap: 7px; align-items: center; }
    .node-select { max-width: 220px; }
    .badge { display: inline-flex; align-items: center; min-height: 22px; padding: 1px 8px; border-radius: 999px; font-size: 12px; border: 1px solid #d8e0e8; background: #f8fafc; white-space: nowrap; }
    .good { background: #ecfdf3; color: #067647; border-color: #abefc6; }
    .warn { background: #fffaeb; color: #b54708; border-color: #fedf89; }
    .bad { background: #fef3f2; color: #b42318; border-color: #fecdca; }
    .nowrap { white-space: nowrap; }
    .host { max-width: 210px; overflow-wrap: anywhere; }
    @media (max-width: 980px) {
      header { align-items: flex-start; flex-direction: column; }
      main { padding: 12px; }
      .grid, .filters { grid-template-columns: 1fr 1fr; }
      table { display: block; overflow-x: auto; }
    }
  </style>
</head>
<body>
  <header>
    <h1>Region Proxy Gateway</h1>
    <div class="actions">
      <button onclick="load()">刷新页面</button>
      <button class="primary" onclick="updateNodes()">更新节点</button>
    </div>
  </header>
  <main>
    <div id="notice" class="notice"></div>
    <section>
      <h2>设置</h2>
      <form id="settings-form" class="grid">
        <label>节点更新间隔<input id="node-refresh-interval" placeholder="20m"></label>
        <button class="primary" type="submit">保存设置</button>
        <span class="muted">保存后重启服务，定时更新间隔才会重新加载。</span>
      </form>
    </section>
    <section>
      <h2>新建或更新通道</h2>
      <form id="channel-form" class="grid">
        <label>ID<input id="channel-id" placeholder="jp-3000" required></label>
        <label>监听地址<input id="listen-host" value="0.0.0.0" required></label>
        <label>端口<input id="listen-port" type="number" min="1" max="65535" value="3000" required></label>
        <label>地区<input id="region" placeholder="jp/us/kr" required></label>
        <label>轮换分钟<input id="rotate-minutes" type="number" min="0" value="0"></label>
        <label>模式
          <select id="selection-mode">
            <option value="auto">自动优选</option>
            <option value="manual">手动节点</option>
          </select>
        </label>
        <label>手动节点ID<input id="manual-node-id" placeholder="manual 模式填写"></label>
        <label>启用
          <select id="enabled">
            <option value="true">启用</option>
            <option value="false">停用</option>
          </select>
        </label>
        <button class="primary" type="submit">保存通道</button>
      </form>
    </section>
    <section>
      <h2>通道</h2>
      <table>
        <thead><tr><th>ID</th><th>端口</th><th>地区</th><th>模式</th><th>当前节点</th><th>代理地址</th><th>操作</th></tr></thead>
        <tbody id="channels"><tr><td colspan="7" class="muted">加载中</td></tr></tbody>
      </table>
    </section>
    <section>
      <h2>节点 <span id="node-count" class="muted"></span></h2>
      <div class="filters">
        <label>地区<select id="filter-region" onchange="renderNodes()"></select></label>
        <label>IP 类型
          <select id="filter-ip-type" onchange="renderNodes()">
            <option value="">全部</option>
            <option value="residential">住宅/家宽</option>
            <option value="hosting">机房</option>
            <option value="mobile">移动网</option>
            <option value="proxy">代理</option>
          </select>
        </label>
        <label>质量
          <select id="filter-quality" onchange="renderNodes()">
            <option value="">全部</option>
            <option value="normal">普通</option>
            <option value="datacenter">数据中心</option>
            <option value="mobile">移动端</option>
            <option value="proxy">代理风险</option>
          </select>
        </label>
        <label>状态
          <select id="filter-available" onchange="renderNodes()">
            <option value="">全部</option>
            <option value="true">可用</option>
            <option value="false">不可用</option>
          </select>
        </label>
        <label>最大延迟<input id="filter-max-latency" type="number" min="0" placeholder="ms" oninput="renderNodes()"></label>
        <label>关键字<input id="filter-keyword" placeholder="IP/ASN/运营商" oninput="renderNodes()"></label>
        <label>显示数量<input id="filter-limit" type="number" min="10" max="500" value="120" oninput="renderNodes()"></label>
      </div>
      <table>
        <thead><tr><th>地区</th><th>主机 / IP</th><th>协议</th><th>延迟</th><th>住宅/机房</th><th>纯净度</th><th>ASN / 运营商</th><th>状态</th><th>操作</th></tr></thead>
        <tbody id="nodes"><tr><td colspan="9" class="muted">加载中</td></tr></tbody>
      </table>
    </section>
    <section>
      <h2>在线连接</h2>
      <table>
        <thead><tr><th>ID</th><th>通道</th><th>协议</th><th>目标</th><th>客户端</th></tr></thead>
        <tbody id="connections"><tr><td colspan="5" class="muted">加载中</td></tr></tbody>
      </table>
    </section>
  </main>
  <script>
    let allNodes = [];
    let currentChannels = [];

    document.getElementById('settings-form').addEventListener('submit', async event => {
      event.preventDefault();
      await request('api/settings', {
        method: 'POST',
        body: JSON.stringify({ node_refresh_interval: value('node-refresh-interval') })
      });
      showNotice('设置已保存，重启服务后定时更新间隔生效。');
      await load();
    });

    document.getElementById('channel-form').addEventListener('submit', async event => {
      event.preventDefault();
      const channel = {
        id: value('channel-id'),
        listen_host: value('listen-host'),
        listen_port: numberValue('listen-port'),
        region: value('region').toLowerCase(),
        rotate_minutes: numberValue('rotate-minutes'),
        selection_mode: value('selection-mode'),
        manual_node_id: value('manual-node-id'),
        enabled: value('enabled') === 'true'
      };
      if (channel.selection_mode !== 'manual') delete channel.manual_node_id;
      await request('api/channels', { method: 'POST', body: JSON.stringify(channel) });
      showNotice('通道配置已保存，需要重启 region-proxy-gateway 后端口监听才会生效。');
      await load();
    });

    async function load() {
      const [statusRes, channelsRes, connectionsRes, nodesRes] = await Promise.all([
        fetch('api/status'),
        fetch('api/channels'),
        fetch('api/connections'),
        fetch('api/nodes')
      ]);
      const status = await statusRes.json();
      currentChannels = (await channelsRes.json()).channels || [];
      const connections = (await connectionsRes.json()).connections || [];
      allNodes = (await nodesRes.json()).nodes || [];
      if (status.settings) setValue('node-refresh-interval', status.settings.node_refresh_interval || '20m');
      renderChannels(currentChannels);
      renderRegionFilter();
      renderNodes();
      renderConnections(connections);
    }

    function renderChannels(channels) {
      document.getElementById('channels').innerHTML = channels.length ? channels.map(ch =>
        '<tr>' +
          '<td><code>' + escapeHTML(ch.id) + '</code></td>' +
          '<td>' + ch.listen_port + '</td>' +
          '<td>' + escapeHTML(ch.region) + '</td>' +
          '<td>' + escapeHTML(ch.selection_mode) + ' / ' + ch.rotate_minutes + '分钟</td>' +
          '<td><code>' + escapeHTML(ch.current_node_id || '-') + '</code></td>' +
          '<td><code>' + escapeHTML(ch.proxy_url_http) + '</code><br><code>' + escapeHTML(ch.proxy_url_socks5) + '</code></td>' +
          '<td><div class="actions">' +
            '<button onclick="fillForm(' + quote(ch.id) + ')">编辑</button>' +
            '<select class="node-select" id="channel-node-' + escapeAttr(ch.id) + '">' + nodeOptions(ch.region) + '</select>' +
            '<button onclick="switchChannelNode(' + quote(ch.id) + ', getSelectValue(' + quote('channel-node-' + ch.id) + '))">切换</button>' +
            '<button class="danger" onclick="deleteChannel(' + quote(ch.id) + ')">删除</button>' +
          '</div></td>' +
        '</tr>').join('') : '<tr><td colspan="7" class="muted">没有通道</td></tr>';
    }

    function renderRegionFilter() {
      const select = document.getElementById('filter-region');
      const current = select.value;
      const regions = Array.from(new Set(allNodes.map(n => n.region).filter(Boolean))).sort();
      select.innerHTML = '<option value="">全部</option>' + regions.map(r => '<option value="' + escapeAttr(r) + '">' + escapeHTML(r) + '</option>').join('');
      select.value = regions.includes(current) ? current : '';
    }

    function renderNodes() {
      const region = value('filter-region');
      const ipType = value('filter-ip-type');
      const quality = value('filter-quality');
      const available = value('filter-available');
      const maxLatency = numberValue('filter-max-latency');
      const keyword = value('filter-keyword').toLowerCase();
      const limit = numberValue('filter-limit') || 120;
      const filtered = allNodes.filter(n => {
        if (region && n.region !== region) return false;
        if (ipType && n.ip_type !== ipType) return false;
        if (quality && n.quality !== quality) return false;
        if (available && String(Boolean(n.available)) !== available) return false;
        if (maxLatency && (Number(n.latency_ms || 0) === 0 || Number(n.latency_ms) > maxLatency)) return false;
        if (keyword) {
          const hay = [n.id, n.ip, n.hostname, n.owner, n.asn, n.as_name, n.location].join(' ').toLowerCase();
          if (!hay.includes(keyword)) return false;
        }
        return true;
      });
      document.getElementById('node-count').textContent = filtered.length + ' / ' + allNodes.length;
      document.getElementById('nodes').innerHTML = filtered.length ? filtered.slice(0, limit).map(n =>
        '<tr>' +
          '<td class="nowrap">' + escapeHTML(n.region) + '<br><span class="muted">' + escapeHTML(n.country || '') + '</span></td>' +
          '<td class="host"><code>' + escapeHTML(n.hostname || '-') + '</code><br><code>' + escapeHTML(n.ip || '-') + '</code></td>' +
          '<td class="nowrap">' + escapeHTML((n.proto || 'udp') + ':' + (n.port || 1194)) + '</td>' +
          '<td class="nowrap">' + (n.latency_ms ? n.latency_ms + ' ms' : '-') + '</td>' +
          '<td>' + ipTypeBadge(n.ip_type) + '</td>' +
          '<td>' + purityBadge(n) + '</td>' +
          '<td class="host">' + escapeHTML(n.owner || '-') + '<br><span class="muted">' + escapeHTML(n.asn || n.as_name || '') + '</span></td>' +
          '<td>' + statusBadge(n) + '<br><span class="muted">' + escapeHTML(n.probe_message || n.fail_reason || '') + '</span></td>' +
          '<td><div class="actions">' +
            '<button onclick="probeNode(' + quote(n.id) + ', this)">测速</button>' +
            '<select id="node-channel-' + escapeAttr(n.id) + '">' + channelOptions(n.region) + '</select>' +
            '<button class="primary" onclick="switchChannelNode(getSelectValue(' + quote('node-channel-' + n.id) + '), ' + quote(n.id) + ')">切换到此节点</button>' +
          '</div></td>' +
        '</tr>').join('') : '<tr><td colspan="9" class="muted">没有符合筛选条件的节点</td></tr>';
    }

    function renderConnections(connections) {
      document.getElementById('connections').innerHTML = connections.length ? connections.map(conn =>
        '<tr>' +
          '<td><code>' + escapeHTML(conn.id) + '</code></td>' +
          '<td><code>' + escapeHTML(conn.channel_id) + '</code></td>' +
          '<td>' + escapeHTML(conn.protocol) + '</td>' +
          '<td><code>' + escapeHTML(conn.target) + '</code></td>' +
          '<td>' + escapeHTML(conn.client_addr) + '</td>' +
        '</tr>').join('') : '<tr><td colspan="5" class="muted">没有在线连接</td></tr>';
    }

    function fillForm(channelID) {
      const ch = currentChannels.find(item => item.id === channelID);
      if (!ch) return;
      setValue('channel-id', ch.id);
      setValue('listen-host', ch.listen_host);
      setValue('listen-port', ch.listen_port);
      setValue('region', ch.region);
      setValue('rotate-minutes', ch.rotate_minutes);
      setValue('selection-mode', ch.selection_mode);
      setValue('manual-node-id', ch.manual_node_id || ch.current_node_id || '');
      setValue('enabled', String(ch.enabled));
      window.scrollTo({ top: 0, behavior: 'smooth' });
    }

    async function switchChannelNode(channelID, nodeID) {
      if (!channelID) return showNotice('请先选择通道');
      if (!nodeID) return showNotice('请先选择节点');
      await request('api/channels/' + encodeURIComponent(channelID) + '/switch', {
        method: 'POST',
        body: JSON.stringify({ node_id: nodeID })
      });
      showNotice('已切换节点。');
      await load();
    }

    async function probeNode(nodeID, button) {
      const oldText = button ? button.textContent : '';
      if (button) { button.disabled = true; button.textContent = '测速中'; }
      try {
        await request('api/nodes/' + encodeURIComponent(nodeID) + '/probe', { method: 'POST' });
        showNotice('测速完成。');
        await load();
      } finally {
        if (button) { button.disabled = false; button.textContent = oldText; }
      }
    }

    async function deleteChannel(channelID) {
      if (!confirm('删除通道 ' + channelID + '？保存后需要重启服务生效。')) return;
      await request('api/channels/' + encodeURIComponent(channelID), { method: 'DELETE' });
      showNotice('通道已删除，需要重启 region-proxy-gateway 后生效。');
      await load();
    }

    async function updateNodes() {
      showNotice('正在更新节点，会重新从 VPNGate 拉取并补充 IP 类型。');
      await request('api/nodes/refresh', { method: 'POST' });
      showNotice('节点已更新。');
      await load();
    }

    async function request(url, options) {
      const res = await fetch(url, Object.assign({ headers: { 'Content-Type': 'application/json' } }, options));
      const body = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(body.error || '请求失败');
      return body;
    }

    function nodeOptions(region) {
      const nodes = allNodes.filter(n => n.region === region);
      if (!nodes.length) return '<option value="">没有节点</option>';
      return '<option value="">选择节点</option>' + nodes.slice(0, 150).map(n =>
        '<option value="' + escapeAttr(n.id) + '">' + escapeHTML(nodeLabel(n)) + '</option>').join('');
    }

    function channelOptions(region) {
      const channels = currentChannels.filter(ch => ch.region === region && ch.enabled);
      if (!channels.length) return '<option value="">没有同地区通道</option>';
      return '<option value="">选择通道</option>' + channels.map(ch =>
        '<option value="' + escapeAttr(ch.id) + '">' + escapeHTML(ch.id + ' :' + ch.listen_port) + '</option>').join('');
    }

    function nodeLabel(n) {
      return [n.id, n.latency_ms ? n.latency_ms + 'ms' : '', ipTypeText(n.ip_type), n.purity_score ? '纯净' + n.purity_score : ''].filter(Boolean).join(' / ');
    }
    function ipTypeBadge(type) {
      const text = ipTypeText(type);
      const cls = type === 'residential' ? 'good' : (type === 'hosting' || type === 'proxy' ? 'warn' : '');
      return '<span class="badge ' + cls + '">' + escapeHTML(text || '未知') + '</span>';
    }
    function purityBadge(n) {
      const score = Number(n.purity_score || 0);
      const cls = score >= 75 ? 'good' : (score >= 40 ? 'warn' : 'bad');
      return '<span class="badge ' + cls + '">' + (score ? score + '/100' : '未知') + ' ' + escapeHTML(qualityText(n.quality)) + '</span>';
    }
    function statusBadge(n) {
      const ok = Boolean(n.available);
      const status = n.probe_status || (ok ? 'available' : 'unavailable');
      return '<span class="badge ' + (ok ? 'good' : 'bad') + '">' + escapeHTML(status === 'available' ? '可用' : '不可用') + '</span>';
    }
    function ipTypeText(type) {
      return ({ residential: '住宅/家宽', hosting: '机房', mobile: '移动网', proxy: '代理' })[type] || type || '';
    }
    function qualityText(type) {
      return ({ normal: '普通', datacenter: '数据中心', mobile: '移动端', proxy: '代理风险' })[type] || type || '';
    }
    function getSelectValue(id) {
      const el = document.getElementById(id);
      return el ? el.value : '';
    }
    function value(id) { const el = document.getElementById(id); return el ? el.value.trim() : ''; }
    function numberValue(id) { return Number(value(id) || 0); }
    function setValue(id, val) { const el = document.getElementById(id); if (el) el.value = val == null ? '' : val; }
    function showNotice(text) {
      const el = document.getElementById('notice');
      el.textContent = text;
      el.style.display = 'block';
    }
    function quote(value) { return JSON.stringify(String(value || '')); }
    function escapeAttr(value) { return escapeHTML(value).replace(/"/g, '&quot;'); }
    function escapeHTML(value) {
      return String(value || '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
    }
    load().catch(err => showNotice(err.message));
    setInterval(() => load().catch(() => {}), 7000);
  </script>
</body>
</html>`
