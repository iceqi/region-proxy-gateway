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
    header { padding: 20px 24px; background: #ffffff; border-bottom: 1px solid #dde3ea; }
    h1 { margin: 0; font-size: 22px; letter-spacing: 0; }
    main { max-width: 1120px; margin: 0 auto; padding: 20px; }
    section { margin-bottom: 24px; }
    h2 { font-size: 16px; margin: 0 0 12px; }
    table { width: 100%; border-collapse: collapse; background: #ffffff; border: 1px solid #dde3ea; }
    th, td { padding: 10px 12px; border-bottom: 1px solid #edf1f5; text-align: left; font-size: 14px; }
    th { background: #eef3f8; color: #344054; }
    code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 13px; }
    .muted { color: #667085; }
  </style>
</head>
<body>
  <header><h1>Region Proxy Gateway</h1></header>
  <main>
    <section>
      <h2>通道</h2>
      <table>
        <thead><tr><th>ID</th><th>端口</th><th>地区</th><th>模式</th><th>当前节点</th><th>HTTP</th><th>SOCKS5</th></tr></thead>
        <tbody id="channels"><tr><td colspan="7" class="muted">加载中</td></tr></tbody>
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
    async function load() {
      const [channelsRes, connectionsRes] = await Promise.all([
        fetch('/api/channels'),
        fetch('/api/connections')
      ]);
      const channels = (await channelsRes.json()).channels || [];
      const connections = (await connectionsRes.json()).connections || [];
      document.getElementById('channels').innerHTML = channels.length ? channels.map(ch =>
        '<tr>' +
          '<td><code>' + escapeHTML(ch.id) + '</code></td>' +
          '<td>' + ch.listen_port + '</td>' +
          '<td>' + escapeHTML(ch.region) + '</td>' +
          '<td>' + escapeHTML(ch.selection_mode) + ' / ' + ch.rotate_minutes + '分钟</td>' +
          '<td><code>' + escapeHTML(ch.current_node_id || '-') + '</code></td>' +
          '<td><code>' + escapeHTML(ch.proxy_url_http) + '</code></td>' +
          '<td><code>' + escapeHTML(ch.proxy_url_socks5) + '</code></td>' +
        '</tr>').join('') : '<tr><td colspan="7" class="muted">没有通道</td></tr>';
      document.getElementById('connections').innerHTML = connections.length ? connections.map(conn =>
        '<tr>' +
          '<td><code>' + escapeHTML(conn.id) + '</code></td>' +
          '<td><code>' + escapeHTML(conn.channel_id) + '</code></td>' +
          '<td>' + escapeHTML(conn.protocol) + '</td>' +
          '<td><code>' + escapeHTML(conn.target) + '</code></td>' +
          '<td>' + escapeHTML(conn.client_addr) + '</td>' +
        '</tr>').join('') : '<tr><td colspan="5" class="muted">没有在线连接</td></tr>';
    }
    function escapeHTML(value) {
      return String(value || '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
    }
    load();
    setInterval(load, 5000);
  </script>
</body>
</html>`
