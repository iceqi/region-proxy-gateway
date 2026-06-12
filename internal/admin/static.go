package admin

const indexHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Region Proxy Gateway</title>
  <style>
    :root { color-scheme: light; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    body { margin: 0; background: #f6f7f9; color: #1f2933; }
    header { padding: 18px 24px; background: #fff; border-bottom: 1px solid #dde3ea; display: flex; align-items: center; justify-content: space-between; gap: 16px; }
    h1 { margin: 0; font-size: 22px; letter-spacing: 0; }
    main { max-width: 1180px; margin: 0 auto; padding: 20px; }
    section { margin-bottom: 24px; }
    h2 { font-size: 16px; margin: 0 0 12px; }
    table { width: 100%; border-collapse: collapse; background: #fff; border: 1px solid #dde3ea; }
    th, td { padding: 10px 12px; border-bottom: 1px solid #edf1f5; text-align: left; font-size: 14px; vertical-align: top; }
    th { background: #eef3f8; color: #344054; }
    code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 13px; }
    label { display: grid; gap: 5px; font-size: 13px; color: #344054; }
    input, select { min-height: 34px; border: 1px solid #c9d3df; border-radius: 6px; padding: 6px 8px; font: inherit; background: #fff; }
    button { min-height: 34px; border: 1px solid #b8c4d1; border-radius: 6px; padding: 6px 10px; background: #fff; cursor: pointer; font: inherit; }
    button.primary { background: #1769aa; color: #fff; border-color: #1769aa; }
    button.danger { color: #b42318; border-color: #f1b8b2; }
    .grid { display: grid; grid-template-columns: repeat(6, minmax(120px, 1fr)); gap: 12px; align-items: end; background: #fff; border: 1px solid #dde3ea; padding: 14px; }
    .muted { color: #667085; }
    .notice { padding: 10px 12px; background: #fff8db; border: 1px solid #ead88b; margin-bottom: 14px; font-size: 14px; display: none; }
    .actions { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
    .node-select { max-width: 220px; }
    @media (max-width: 860px) {
      header { align-items: flex-start; flex-direction: column; }
      main { padding: 12px; }
      .grid { grid-template-columns: 1fr 1fr; }
      table { display: block; overflow-x: auto; }
    }
  </style>
</head>
<body>
  <header>
    <h1>Region Proxy Gateway</h1>
    <button onclick="load()">刷新</button>
  </header>
  <main>
    <div id="notice" class="notice"></div>
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
      <h2>节点</h2>
      <table>
        <thead><tr><th>ID</th><th>地区</th><th>主机</th><th>IP</th><th>Ping</th><th>速度</th></tr></thead>
        <tbody id="nodes"><tr><td colspan="6" class="muted">加载中</td></tr></tbody>
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
      await request('/api/channels', { method: 'POST', body: JSON.stringify(channel) });
      showNotice('通道配置已保存，需要重启 region-proxy-gateway 后端口监听才会生效。');
      await load();
    });

    async function load() {
      const [channelsRes, connectionsRes, nodesRes] = await Promise.all([
        fetch('/api/channels'),
        fetch('/api/connections'),
        fetch('/api/nodes')
      ]);
      const channels = (await channelsRes.json()).channels || [];
      const connections = (await connectionsRes.json()).connections || [];
      allNodes = (await nodesRes.json()).nodes || [];
      renderChannels(channels);
      renderNodes(allNodes);
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
            '<select class="node-select" id="node-' + escapeAttr(ch.id) + '">' + nodeOptions(ch.region) + '</select>' +
            '<button onclick="switchNode(' + quote(ch.id) + ')">切换</button>' +
            '<button class="danger" onclick="deleteChannel(' + quote(ch.id) + ')">删除</button>' +
          '</div></td>' +
        '</tr>').join('') : '<tr><td colspan="7" class="muted">没有通道</td></tr>';
      window.currentChannels = channels;
    }

    function renderNodes(nodes) {
      document.getElementById('nodes').innerHTML = nodes.length ? nodes.slice(0, 80).map(n =>
        '<tr>' +
          '<td><code>' + escapeHTML(n.id) + '</code></td>' +
          '<td>' + escapeHTML(n.region) + '</td>' +
          '<td>' + escapeHTML(n.hostname) + '</td>' +
          '<td><code>' + escapeHTML(n.ip) + '</code></td>' +
          '<td>' + (n.latency_ms || '-') + '</td>' +
          '<td>' + (n.speed || '-') + '</td>' +
        '</tr>').join('') : '<tr><td colspan="6" class="muted">没有节点</td></tr>';
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
      const ch = (window.currentChannels || []).find(item => item.id === channelID);
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

    async function switchNode(channelID) {
      const select = document.getElementById('node-' + channelID);
      if (!select || !select.value) return showNotice('请先选择节点');
      await request('/api/channels/' + encodeURIComponent(channelID) + '/switch', {
        method: 'POST',
        body: JSON.stringify({ node_id: select.value })
      });
      showNotice('已切换节点。');
      await load();
    }

    async function deleteChannel(channelID) {
      if (!confirm('删除通道 ' + channelID + '？保存后需要重启服务生效。')) return;
      await request('/api/channels/' + encodeURIComponent(channelID), { method: 'DELETE' });
      showNotice('通道已删除，需要重启 region-proxy-gateway 后生效。');
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
      return '<option value="">选择节点</option>' + nodes.slice(0, 100).map(n =>
        '<option value="' + escapeAttr(n.id) + '">' + escapeHTML(n.id) + '</option>').join('');
    }
    function value(id) { return document.getElementById(id).value.trim(); }
    function numberValue(id) { return Number(document.getElementById(id).value || 0); }
    function setValue(id, val) { document.getElementById(id).value = val == null ? '' : val; }
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
    setInterval(() => load().catch(() => {}), 5000);
  </script>
</body>
</html>`
