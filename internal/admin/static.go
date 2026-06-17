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
    :root { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; color: #172033; background: #eef4fb; }
    body { margin: 0; background: radial-gradient(circle at top left, #dceaff 0, transparent 34%), linear-gradient(135deg, #f7fbff 0%, #edf3fb 48%, #f5f7fb 100%); }
    [v-cloak] { display: none; }
    .shell { min-height: 100vh; background: transparent; }
    .topbar { position: sticky; top: 0; z-index: 20; min-height: 76px; padding: 0 28px; display: flex; align-items: center; justify-content: space-between; background: linear-gradient(135deg, #101827 0%, #182a44 52%, #0f766e 130%); border-bottom: 1px solid rgba(255,255,255,.10); box-shadow: 0 18px 44px rgba(15, 23, 42, .22); }
    .brand { display: flex; align-items: center; min-width: 0; color: #fff; }
    .brand h1 { margin: 0; font-size: 21px; font-weight: 800; letter-spacing: .01em; }
    .page { max-width: 1560px; margin: 0 auto; padding: 28px 24px 40px; }
    .hero-strip { margin-bottom: 18px; padding: 18px 20px; border: 1px solid rgba(148, 163, 184, .28); border-radius: 18px; background: linear-gradient(135deg, rgba(255,255,255,.92), rgba(240,247,255,.82)); box-shadow: 0 18px 48px rgba(30, 41, 59, .08); display: flex; align-items: center; justify-content: space-between; gap: 16px; }
    .hero-title { font-size: 18px; font-weight: 800; color: #0f172a; }
    .hero-subtitle { color: #64748b; margin-top: 4px; }
    .signal-dot { width: 10px; height: 10px; border-radius: 999px; background: #22c55e; box-shadow: 0 0 0 6px rgba(34,197,94,.14); display: inline-block; margin-right: 8px; }
    .login-wrap { min-height: calc(100vh - 76px); display: grid; place-items: center; padding: 44px 18px; position: relative; overflow: hidden; }
    .login-wrap::before { content: ''; position: absolute; width: 560px; height: 560px; right: -180px; top: -180px; border-radius: 50%; background: radial-gradient(circle, rgba(14,165,233,.28), rgba(14,165,233,0) 66%); }
    .login-wrap::after { content: ''; position: absolute; width: 520px; height: 520px; left: -180px; bottom: -220px; border-radius: 50%; background: radial-gradient(circle, rgba(34,197,94,.22), rgba(34,197,94,0) 68%); }
    .login-stage { width: min(1040px, 100%); display: grid; grid-template-columns: 1.08fr .92fr; gap: 22px; position: relative; z-index: 1; }
    .login-visual, .login-card { border: 1px solid rgba(203,213,225,.76); border-radius: 28px; background: rgba(255,255,255,.78); box-shadow: 0 30px 90px rgba(15, 23, 42, .16); backdrop-filter: blur(18px); }
    .login-visual { min-height: 500px; padding: 34px; color: #e0f2fe; background: radial-gradient(circle at 76% 22%, rgba(45,212,191,.38), transparent 30%), linear-gradient(145deg, #0f172a 0%, #164e63 58%, #047857 120%); position: relative; overflow: hidden; }
    .login-visual::before { content: ''; position: absolute; inset: 22px; border-radius: 24px; border: 1px solid rgba(255,255,255,.12); pointer-events: none; }
    .login-orbit { position: absolute; width: 260px; height: 260px; right: 36px; top: 54px; border-radius: 50%; border: 1px solid rgba(186,230,253,.32); }
    .login-orbit::before, .login-orbit::after { content: ''; position: absolute; border-radius: 50%; background: #67e8f9; box-shadow: 0 0 34px rgba(103,232,249,.7); }
    .login-orbit::before { width: 13px; height: 13px; left: 32px; top: 34px; }
    .login-orbit::after { width: 9px; height: 9px; right: 46px; bottom: 24px; background: #86efac; }
    .login-grid { position: absolute; inset: auto 0 0 0; height: 180px; opacity: .22; background-image: linear-gradient(rgba(255,255,255,.32) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,.32) 1px, transparent 1px); background-size: 36px 36px; mask-image: linear-gradient(transparent, #000); }
    .login-badge { display: inline-flex; align-items: center; gap: 8px; padding: 8px 12px; border: 1px solid rgba(186,230,253,.24); border-radius: 999px; background: rgba(15,23,42,.35); color: #bae6fd; font-size: 12px; font-weight: 650; position: relative; z-index: 1; }
    .login-badge-dot { width: 8px; height: 8px; border-radius: 50%; background: #22c55e; box-shadow: 0 0 0 5px rgba(34,197,94,.15); }
    .login-visual h2 { margin: 76px 0 14px; font-size: clamp(34px, 5vw, 56px); line-height: 1.02; color: #fff; letter-spacing: -.045em; max-width: 460px; position: relative; z-index: 1; }
    .login-visual p { margin: 0; color: rgba(224,242,254,.78); font-size: 15px; line-height: 1.8; max-width: 430px; position: relative; z-index: 1; }
    .login-metrics { position: absolute; left: 34px; right: 34px; bottom: 34px; display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; z-index: 1; }
    .login-metric { padding: 14px; border: 1px solid rgba(255,255,255,.13); border-radius: 18px; background: rgba(15,23,42,.34); }
    .login-metric strong { display: block; color: #fff; font-size: 18px; margin-bottom: 4px; }
    .login-metric span { color: rgba(224,242,254,.68); font-size: 12px; }
    .login-card { padding: 34px; align-self: stretch; display: flex; flex-direction: column; justify-content: center; background: rgba(255,255,255,.94); }
    .login-card h2 { margin: 0; font-size: 28px; color: #0f172a; letter-spacing: -.03em; }
    .login-card p { margin: 10px 0 26px; color: #64748b; line-height: 1.7; }
    .login-form { display: grid; gap: 15px; }
    .login-form .ant-input-affix-wrapper, .login-form .ant-input { border-radius: 13px; }
    .login-form .ant-btn-primary { height: 46px; border-radius: 13px; font-weight: 750; background: linear-gradient(135deg, #0ea5e9, #10b981); box-shadow: 0 16px 30px rgba(14,165,233,.22); }
    .login-tip { margin-top: 18px; padding: 12px 14px; border-radius: 16px; background: #f0f9ff; border: 1px solid #dbeafe; color: #475569; font-size: 12px; line-height: 1.65; }
    .stats { display: grid; grid-template-columns: repeat(6, minmax(150px, 1fr)); gap: 14px; margin-bottom: 20px; }
    .stat { background: linear-gradient(145deg, rgba(255,255,255,.96), rgba(241,247,255,.9)); border: 1px solid rgba(203, 213, 225, .78); border-radius: 18px; padding: 16px; box-shadow: 0 16px 38px rgba(23, 32, 51, .08); position: relative; overflow: hidden; }
    .stat::after { content: ''; position: absolute; width: 72px; height: 72px; right: -26px; top: -26px; border-radius: 50%; background: rgba(14,165,233,.11); }
    .stat-label { color: #697386; font-size: 12px; margin-bottom: 8px; position: relative; z-index: 1; }
    .stat-value { font-size: 24px; font-weight: 780; color: #111827; position: relative; z-index: 1; }
    .content-panel { background: rgba(255,255,255,.72); border: 1px solid rgba(203,213,225,.84); border-radius: 24px; box-shadow: 0 24px 70px rgba(23, 32, 51, .11); overflow: hidden; backdrop-filter: blur(14px); }
    .content-panel .ant-tabs-nav { padding: 10px 18px 0; margin: 0 !important; background: linear-gradient(180deg, rgba(255,255,255,.96), rgba(248,251,255,.92)); border-bottom: 1px solid #e6edf7; }
    .content-panel .ant-tabs-tab { padding: 13px 16px !important; border-radius: 14px 14px 0 0; font-weight: 700; }
    .content-panel .ant-tabs-tab-active { background: #eef7ff; }
    .content-panel .ant-tabs-ink-bar { height: 3px; border-radius: 999px; background: linear-gradient(90deg, #0ea5e9, #10b981); }
    .content-panel .ant-tabs-content-holder { padding: 22px 24px 24px; }
    .card { background: #fff; border: 1px solid rgba(203,213,225,.82); border-radius: 22px; margin-bottom: 0; overflow: hidden; box-shadow: 0 18px 46px rgba(15,23,42,.07); }
    .card-head { padding: 20px 22px; display: flex; align-items: center; justify-content: space-between; gap: 14px; border-bottom: 1px solid #eaf0f7; background: radial-gradient(circle at 96% 0%, rgba(34,197,94,.13), transparent 32%), linear-gradient(135deg, #fbfdff 0%, #f4f9ff 100%); }
    .card-heading { display: flex; align-items: center; gap: 13px; min-width: 0; }
    .section-icon { width: 42px; height: 42px; border-radius: 15px; display: grid; place-items: center; color: #fff; font-size: 20px; box-shadow: 0 14px 28px rgba(14,165,233,.18); flex: none; }
    .section-icon.nodes { background: linear-gradient(135deg, #0ea5e9, #2563eb); }
    .section-icon.channels { background: linear-gradient(135deg, #10b981, #0f766e); }
    .section-icon.connections { background: linear-gradient(135deg, #8b5cf6, #2563eb); }
    .section-icon.settings { background: linear-gradient(135deg, #f97316, #ef4444); }
    .card-title { font-size: 17px; font-weight: 800; color: #0f172a; }
    .card-subtitle { color: #64748b; margin-top: 4px; line-height: 1.55; }
    .card-actions { display: flex; align-items: center; justify-content: flex-end; gap: 10px; flex-wrap: wrap; }
    .module-chip { display: inline-flex; align-items: center; gap: 6px; padding: 7px 10px; border-radius: 999px; background: #f0f9ff; border: 1px solid #dbeafe; color: #075985; font-size: 12px; font-weight: 700; }
    .module-chip.green { background: #ecfdf5; border-color: #bbf7d0; color: #047857; }
    .module-chip.purple { background: #f5f3ff; border-color: #ddd6fe; color: #6d28d9; }
    .card-body { padding: 20px 22px 22px; background: linear-gradient(180deg, #fff, #fbfdff); }
    .filter-grid { display: grid; grid-template-columns: repeat(7, minmax(120px, 1fr)); gap: 10px; margin-bottom: 14px; padding: 12px; border: 1px solid #e6edf7; border-radius: 16px; background: #f8fbff; }
    .table-shell { border: 1px solid #e6edf7; border-radius: 16px; overflow: hidden; background: #fff; }
    .table-shell .ant-table-thead > tr > th { background: #f7fbff !important; color: #334155; font-weight: 760; }
    .table-shell .ant-table-tbody > tr:hover > td { background: #f8fcff !important; }
    .settings-grid { display: grid; grid-template-columns: repeat(3, minmax(180px, 1fr)); gap: 14px; margin-bottom: 16px; }
    .settings-item { padding: 14px 14px 6px; border: 1px solid #e6edf7; border-radius: 18px; background: linear-gradient(180deg, #fff, #f8fbff); }
    .settings-item .ant-form-item { margin-bottom: 10px; }
    .settings-save { display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 14px; border-radius: 18px; background: #f8fbff; border: 1px solid #e6edf7; }
    .gateway-panel { margin: 16px 0; padding: 16px; border: 1px solid #dbeafe; border-radius: 18px; background: linear-gradient(135deg, #f8fbff, #eff6ff); }
    .gateway-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
    .gateway-title { font-weight: 800; color: #0f172a; }
    .gateway-list { display: grid; gap: 12px; }
    .gateway-card { padding: 12px; border: 1px solid #e2e8f0; border-radius: 16px; background: rgba(255,255,255,.86); }
    .gateway-meta { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin-bottom: 10px; }
    .modal-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
    .mono { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 12px; }
    .muted { color: #697386; }
    .host-cell { display: grid; gap: 3px; overflow-wrap: anywhere; }
    .action-row { display: flex; flex-wrap: nowrap; gap: 8px; align-items: center; }
    .connection-box { display: grid; gap: 8px; }
    .connection-line { display: grid; grid-template-columns: 88px minmax(0, 1fr) auto; gap: 8px; align-items: center; padding: 9px 10px; border: 1px solid #dbe5f0; background: linear-gradient(135deg, #f8fbff, #f1f6fb); border-radius: 12px; min-width: 0; box-shadow: inset 0 1px 0 rgba(255,255,255,.78); }
    .connection-line:hover { border-color: #93c5fd; background: #f8fbff; }
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
      .filter-grid, .modal-filter, .settings-grid { grid-template-columns: 1fr 1fr; }
      .login-stage { grid-template-columns: 1fr; max-width: 560px; }
      .login-visual { min-height: 360px; }
    }
    @media (max-width: 720px) {
      .topbar { height: auto; min-height: 60px; padding: 12px; align-items: flex-start; flex-direction: column; gap: 12px; }
      .page { padding: 12px; }
      .login-wrap { padding: 22px 12px; }
      .login-visual { display: none; }
      .login-card { padding: 24px; border-radius: 22px; }
      .stats, .filter-grid, .modal-grid, .modal-filter, .settings-grid { grid-template-columns: 1fr; }
      .card-head, .settings-save { align-items: stretch; flex-direction: column; }
      .card-actions { justify-content: flex-start; }
      .connection-line { grid-template-columns: 1fr; }
      .content-panel .ant-tabs-nav { padding: 8px 10px 0; }
      .content-panel .ant-tabs-content-holder { padding: 12px; }
      .card-body { padding: 14px; }
    }
  </style>
</head>
<body>
  <div id="app" class="shell" v-cloak>
    <header class="topbar">
      <div class="brand">
        <h1>Region Proxy Gateway</h1>
      </div>
      <a-space>
        <a-button v-if="isLoggedIn" :loading="loading" @click="load">刷新</a-button>
        <a-button v-if="isLoggedIn" type="primary" :loading="updatingNodes" @click="updateNodes">更新节点</a-button>
        <a-button v-if="isLoggedIn" danger :loading="restarting" @click="restartService">重启服务</a-button>
        <a-button v-if="isLoggedIn" @click="logout">退出登录</a-button>
      </a-space>
    </header>

    <section v-if="!isLoggedIn" class="login-wrap">
      <div class="login-stage">
        <aside class="login-visual" aria-hidden="true">
          <div class="login-orbit"></div>
          <div class="login-grid"></div>
          <div class="login-badge"><span class="login-badge-dot"></span>Encrypted proxy control plane</div>
          <h2>Route traffic with cleaner regional exits.</h2>
          <p>Monitor tunnel health, rotate exits, and keep every proxy channel isolated from one focused gateway console.</p>
          <div class="login-metrics">
            <div class="login-metric"><strong>HTTP</strong><span>Forward proxy</span></div>
            <div class="login-metric"><strong>SOCKS5</strong><span>Credential access</span></div>
            <div class="login-metric"><strong>IP</strong><span>Auto rotation</span></div>
          </div>
        </aside>
        <div class="login-card">
          <h2>登录管理后台</h2>
          <p>使用后台管理账号进入控制台。勾选后会在当前浏览器本地保存账号密码。</p>
          <div class="login-form">
            <a-input v-model:value="loginForm.username" size="large" placeholder="后台账号" @press-enter="login"></a-input>
            <a-input-password v-model:value="loginForm.password" size="large" placeholder="后台密码" @press-enter="login"></a-input-password>
            <a-checkbox v-model:checked="loginForm.rememberCredentials">本地记住账号密码</a-checkbox>
            <a-button type="primary" size="large" :loading="loginLoading" @click="login">进入控制台</a-button>
          </div>
          <div class="login-tip">登录页只展示静态外壳；节点、通道、连接串等敏感数据仍需认证后通过 API 加载。</div>
        </div>
      </div>
    </section>

    <main v-else class="page">
      <section class="hero-strip">
        <div>
          <div class="hero-title"><span class="signal-dot"></span>代理通道控制台</div>
          <div class="hero-subtitle">统一管理节点、轮换、出口 IP 与连接串，保存后尽量热更新不中断服务。</div>
        </div>
        <a-space wrap>
          <a-tag color="blue">{{ channels.length }} 个通道</a-tag>
          <a-tag color="green">{{ connections.length }} 个在线连接</a-tag>
          <a-tag color="purple">{{ filteredNodes.length }} 个可见节点</a-tag>
        </a-space>
      </section>

      <section class="stats">
        <div class="stat"><div class="stat-label">通道</div><div class="stat-value">{{ channels.length }}</div></div>
        <div class="stat"><div class="stat-label">节点</div><div class="stat-value">{{ filteredNodes.length }} / {{ nodes.length }}</div></div>
        <div class="stat"><div class="stat-label">节点扫描</div><div class="stat-value" style="font-size:16px">{{ nodeScanText }}</div></div>
        <div class="stat"><div class="stat-label">在线连接</div><div class="stat-value">{{ connections.length }}</div></div>
        <div class="stat"><div class="stat-label">深测队列</div><div class="stat-value" style="font-size:16px">{{ deepTestSummary }}</div></div>
        <div class="stat"><div class="stat-label">节点更新间隔</div><div class="stat-value">{{ settings.node_refresh_interval || '-' }}</div></div>
        <div class="stat"><div class="stat-label">提取缓存</div><div class="stat-value">{{ settings.proxy_extract_cache_ttl || '30s' }}</div></div>
        <div class="stat"><div class="stat-label">提取 Token</div><div class="stat-value">{{ settings.proxy_extract_api_token ? '已配置' : '-' }}</div></div>
        <div class="stat"><div class="stat-label">代理主机</div><div class="stat-value" style="font-size:16px">{{ proxyHost }}</div></div>
      </section>

      <a-tabs class="content-panel" v-model:active-key="activeTab">
        <a-tab-pane key="nodes" tab="节点">
          <section class="card module-card nodes-card">
            <div class="card-head">
              <div class="card-heading">
                <div class="section-icon nodes">◎</div>
                <div>
                  <div class="card-title">节点雷达</div>
                  <div class="card-subtitle">按地区、IP 类型、质量和延迟筛选节点；切换时可指定目标通道。</div>
                </div>
              </div>
              <div class="card-actions">
                <span class="module-chip">可见 {{ visibleNodes.length }}</span>
                <span class="module-chip green">地区 {{ regions.length }}</span>
                <a-button :loading="probingBatch" @click="probeVisibleNodes">测试当前列表延迟</a-button>
                <a-button type="primary" :loading="deepTesting" @click="enqueueDeepTestVisibleNodes">深度测试当前列表</a-button>
              </div>
            </div>
            <div class="card-body">
              <div class="filter-grid module-filter">
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

              <div class="table-shell nodes-table-shell"><a-table :columns="nodeColumns" :data-source="visibleNodes" :row-key="record => record.id" size="small" bordered :pagination="false" :scroll="{ x: 1280, y: 620 }"></a-table></div>
            </div>
          </section>
        </a-tab-pane>

        <a-tab-pane key="channels" tab="通道">
          <section class="card module-card channels-card">
            <div class="card-head">
              <div class="card-heading">
                <div class="section-icon channels">⇄</div>
                <div>
                  <div class="card-title">通道控制</div>
                  <div class="card-subtitle">集中查看端口、出口、轮换状态和三种连接串，复制更顺手。</div>
                </div>
              </div>
              <div class="card-actions">
                <span class="module-chip green">{{ channels.length }} 个通道</span>
                <a-button type="primary" @click="openChannelDialog()">新增通道</a-button>
              </div>
            </div>
            <div class="card-body">
              <div class="table-shell channels-table-shell"><a-table :columns="channelColumns" :data-source="channels" :row-key="record => record.id" size="small" bordered :pagination="false" :scroll="{ x: 1480 }"></a-table></div>
            </div>
          </section>
        </a-tab-pane>

        <a-tab-pane key="connections" tab="在线连接">
          <section class="card module-card connections-card">
            <div class="card-head">
              <div class="card-heading">
                <div class="section-icon connections">●</div>
                <div>
                  <div class="card-title">实时连接</div>
                  <div class="card-subtitle">观察当前客户端、目标地址和所使用的代理通道。</div>
                </div>
              </div>
              <div class="card-actions"><span class="module-chip purple">在线 {{ connections.length }}</span></div>
            </div>
            <div class="card-body">
              <div class="table-shell connections-table-shell"><a-table :columns="connectionColumns" :data-source="connections" :row-key="record => record.id" size="small" bordered :pagination="false"></a-table></div>
            </div>
          </section>
        </a-tab-pane>

        <a-tab-pane key="settings" tab="设置">
          <section class="card module-card settings-card">
            <div class="card-head">
              <div class="card-heading">
                <div class="section-icon settings">⚙</div>
                <div>
                  <div class="card-title">访问与认证</div>
                  <div class="card-subtitle">修改管理入口、后台账号、代理账号、节点刷新间隔、代理提取缓存和提取 Token。</div>
                </div>
              </div>
              <div class="card-actions"><span class="module-chip">热更新配置</span></div>
            </div>
            <div class="card-body">
              <div class="settings-grid">
                <div class="settings-item"><a-form-item label="登录入口"><a-input v-model:value="settings.admin_path" placeholder="/admin-xxxx"></a-input></a-form-item></div>
                <div class="settings-item"><a-form-item label="后台账号"><a-input v-model:value="settings.admin_username" placeholder="admin"></a-input></a-form-item></div>
                <div class="settings-item"><a-form-item label="后台新密码"><a-input-password v-model:value="settings.admin_password" placeholder="留空则不修改"></a-input-password></a-form-item></div>
                <div class="settings-item"><a-form-item label="代理账号"><a-input v-model:value="settings.proxy_username" placeholder="proxy"></a-input></a-form-item></div>
                <div class="settings-item"><a-form-item label="代理新密码"><a-input-password v-model:value="settings.proxy_password" placeholder="留空则不修改"></a-input-password></a-form-item></div>
                <div class="settings-item"><a-form-item label="提取 Token"><a-input-group compact><a-input v-model:value="settings.proxy_extract_api_token" placeholder="留空不修改" style="width:calc(100% - 160px)"></a-input><a-button @click="generateExtractToken">生成</a-button><a-button @click="copyText(settings.proxy_extract_api_token)">复制</a-button></a-input-group></a-form-item></div>
                <div class="settings-item"><a-form-item label="节点更新间隔"><a-input v-model:value="settings.node_refresh_interval" placeholder="10m"></a-input></a-form-item></div>
                <div class="settings-item"><a-form-item label="提取缓存 TTL"><a-input v-model:value="settings.proxy_extract_cache_ttl" placeholder="30s"></a-input></a-form-item></div>
                <div class="settings-item"><a-form-item label="空闲释放 TTL"><a-input v-model:value="settings.proxy_extract_idle_ttl" placeholder="10m"></a-input></a-form-item></div>
              </div>
              <a-alert type="info" show-icon style="margin-bottom: 16px" :message="'API 提取代理：' + extractApiExample"></a-alert>
              <div class="gateway-panel">
                <div class="gateway-head">
                  <div>
                    <div class="gateway-title">旋转网关代理</div>
                    <div class="muted">后端自动创建固定入口端口，每次连接从有效节点池选择出口。</div>
                  </div>
                  <a-tag color="blue">旋转 {{ settings.rotating_gateway_port || '-' }} / API代理 {{ settings.proxy_extract_api_port || '-' }}</a-tag>
                </div>
                <div class="gateway-list">
                  <div class="gateway-card">
                    <div class="gateway-meta">
                      <a-tag color="geekblue">旋转网关</a-tag>
                      <span class="muted">直接作为代理使用</span>
                    </div>
                    <div class="connection-box">
                      <div class="connection-line"><a-tag>HTTP</a-tag><code class="mono" :title="settings.rotating_gateway_http">{{ settings.rotating_gateway_http || '-' }}</code><a-button size="small" @click="copyText(settings.rotating_gateway_http)">复制</a-button></div>
                      <div class="connection-line"><a-tag>SOCKS5</a-tag><code class="mono" :title="settings.rotating_gateway_socks5">{{ settings.rotating_gateway_socks5 || '-' }}</code><a-button size="small" @click="copyText(settings.rotating_gateway_socks5)">复制</a-button></div>
                      <div class="connection-line"><a-tag>NO-SCHEME</a-tag><code class="mono" :title="settings.rotating_gateway_no_scheme">{{ settings.rotating_gateway_no_scheme || '-' }}</code><a-button size="small" @click="copyText(settings.rotating_gateway_no_scheme)">复制</a-button></div>
                    </div>
                  </div>
                  <div class="gateway-card">
                    <div class="gateway-meta">
                      <a-tag color="purple">API返回代理</a-tag>
                      <span class="muted">提取 API 返回的固定调用出口</span>
                    </div>
                    <div class="connection-box">
                      <div class="connection-line"><a-tag>HTTP</a-tag><code class="mono" :title="settings.extract_proxy_http">{{ settings.extract_proxy_http || '-' }}</code><a-button size="small" @click="copyText(settings.extract_proxy_http)">复制</a-button></div>
                      <div class="connection-line"><a-tag>SOCKS5</a-tag><code class="mono" :title="settings.extract_proxy_socks5">{{ settings.extract_proxy_socks5 || '-' }}</code><a-button size="small" @click="copyText(settings.extract_proxy_socks5)">复制</a-button></div>
                      <div class="connection-line"><a-tag>NO-SCHEME</a-tag><code class="mono" :title="settings.extract_proxy_no_scheme">{{ settings.extract_proxy_no_scheme || '-' }}</code><a-button size="small" @click="copyText(settings.extract_proxy_no_scheme)">复制</a-button></div>
                    </div>
                  </div>
                </div>
              </div>
              <div class="settings-save">
                <span class="muted">提取 API 需要后台账号密码；支持 format=json|text、scheme=http|socks5|no-scheme、region 和 count 参数。</span>
                <a-button @click="copyText(extractApiExample)">复制提取 URL</a-button>
                <a-button type="primary" @click="saveSettings">保存设置</a-button>
              </div>
            </div>
          </section>
        </a-tab-pane>
      </a-tabs>

      <a-modal v-model:open="channelDialog.open" :title="channelDialog.editing ? '编辑通道' : '新增通道'" width="720px" @ok="saveChannel" ok-text="保存" cancel-text="取消">
        <div class="modal-grid">
          <a-form-item label="ID"><a-input v-model:value="channelForm.id" placeholder="jp-3000"></a-input></a-form-item>
          <a-form-item label="监听地址"><a-input v-model:value="channelForm.listen_host" placeholder="0.0.0.0"></a-input></a-form-item>
          <a-form-item label="端口"><a-input-number v-model:value="channelForm.listen_port" :min="0" :max="65535" placeholder="留空/0 自动分配" style="width:100%"></a-input-number></a-form-item>
          <a-form-item label="地区">
            <a-select v-model:value="channelForm.region" allow-clear placeholder="不限地区 / 随机可用节点">
              <a-select-option value="">不限地区（*）</a-select-option>
              <a-select-option v-for="item in regions" :key="item" :value="item">{{ regionText(item) }}（{{ item }}）</a-select-option>
            </a-select>
          </a-form-item>
          <a-form-item label="轮换分钟"><a-input-number v-model:value="channelForm.rotate_minutes" :min="0" style="width:100%"></a-input-number></a-form-item>
          <a-form-item label="旋转网关"><a-switch v-model:checked="channelForm.rotate_on_dial"></a-switch></a-form-item>
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
        <div class="muted" style="margin-bottom: 8px">候选通道：{{ channelsByRegion(nodeSwitchDialog.node ? nodeSwitchDialog.node.region : '').length }}</div>
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
          isLoggedIn: false,
          loginLoading: false,
          adminAuth: { username: '', password: '' },
          loginForm: { username: '', password: '', rememberCredentials: false },
          proxyHost: window.location.hostname || 'SERVER_IP',
          activeTab: 'nodes',
          loading: false,
          tickNow: Date.now(),
          updatingNodes: false,
          restarting: false,
          probingBatch: false,
          deepTesting: false,
          channels: [],
          nodes: [],
          connections: [],
          deepStats: { pending: 0, running: 0, success: 0, failed: 0 },
          nodeScan: { running: false, total: 0, success: 0, failed: 0, last_error: '' },
          settings: { node_refresh_interval: '10m', proxy_extract_cache_ttl: '30s', proxy_extract_idle_ttl: '10m', proxy_extract_api_token: '', admin_path: '/admin', admin_username: 'admin', admin_password: '', proxy_username: 'proxy', proxy_password: '' },
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
            { title: '深度测试', key: 'deep', width: 170, customRender: ({ record }) => this.deepTestCell(record) },
            { title: '类型', key: 'type', width: 118, customRender: ({ record }) => h(antd.Tag, { color: this.ipTypeColor(record.ip_type) }, () => this.ipTypeText(record.ip_type) || '未知') },
            { title: '纯净度', key: 'purity', width: 122, sorter: (a, b) => Number(b.purity_score || 0) - Number(a.purity_score || 0), customRender: ({ record }) => h(antd.Tag, { color: this.purityColor(record.purity_score) }, () => (record.purity_score ? record.purity_score + '/100 ' : '未知 ') + this.qualityText(record.quality)) },
            { title: 'ASN / 运营商', key: 'owner', width: 190, customRender: ({ record }) => h('div', [h('div', record.owner || '-'), h('div', { class: 'muted' }, record.asn || record.as_name || '')]) },
            { title: '状态', key: 'status', width: 180, customRender: ({ record }) => h('div', [h(antd.Tag, { color: record.available ? 'green' : 'red' }, () => record.available ? '可用' : '不可用'), h('div', { class: 'muted' }, this.probeMessageText(record))]) },
            { title: '操作', key: 'actions', width: 210, fixed: 'right', customRender: ({ record }) => h('div', { class: 'action-row' }, [h(antd.Button, { size: 'small', loading: Boolean(this.probing[record.id]), onClick: () => this.probeNode(record.id) }, () => '测速'), h(antd.Button, { size: 'small', type: 'primary', onClick: () => this.openNodeSwitchDialog(record) }, () => '切换'), h(antd.Button, { size: 'small', onClick: () => this.openChannelDialogForNode(record) }, () => '建通道')]) }
          ],
          channelColumns: [
            { title: 'ID', dataIndex: 'id', width: 120 },
            { title: '端口', dataIndex: 'listen_port', width: 84 },
            { title: '地区', dataIndex: 'region', width: 120, customRender: ({ record }) => this.channelRegionText(record.region) },
            { title: '模式', key: 'mode', width: 170, customRender: ({ record }) => h('div', [h(antd.Tag, { color: record.selection_mode === 'manual' ? 'purple' : 'blue' }, () => this.selectionModeText(record.selection_mode)), h('span', record.rotate_on_dial ? '每次新连接轮换' : (record.rotate_minutes ? record.rotate_minutes + ' 分钟轮换' : '固定'))]) },
            { title: '当前节点', key: 'node', width: 280, customRender: ({ record }) => h('div', [h('div', { class: 'mono' }, record.current_node_id || '-'), h('div', { class: record.last_error ? '' : 'muted' }, record.last_error ? this.channelErrorText(record.last_error) : '网络失败会自动重试并切换')]) },
            { title: '出口 IP', key: 'exit', width: 160, customRender: ({ record }) => h('div', [h('div', { class: 'mono' }, this.channelExitAddress(record)), h('div', { class: 'muted' }, record.current_node && record.current_node.owner ? record.current_node.owner : '')]) },
            { title: '轮换状态', key: 'rotation', width: 360, customRender: ({ record }) => this.rotationStateCell(record) },
            { title: '连接方式', key: 'connect', width: 660, customRender: ({ record }) => h('div', { class: 'connection-box' }, [this.connectionLine('HTTP', this.proxyAddress(record, 'http')), this.connectionLine('SOCKS5', this.proxyAddress(record, 'socks5')), this.connectionLine('NO-SCHEME', this.proxyAddressNoScheme(record))]) },
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
        deepTestSummary() {
          const s = this.deepStats || {};
          return '待 ' + Number(s.pending || 0) + ' / 跑 ' + Number(s.running || 0) + ' / 成 ' + Number(s.success || 0) + ' / 败 ' + Number(s.failed || 0);
        },
        nodeScanText() {
          const s = this.nodeScan || {};
          return (s.running ? '扫描中 ' : '空闲 ') + Number(s.success || 0) + '/' + Number(s.total || 0);
        },
        extractApiExample() {
          const token = this.settings.proxy_extract_api_token || 'YOUR_TOKEN';
          return window.location.origin + apiBase + 'proxies/extract?token=' + encodeURIComponent(token) + '&format=text&scheme=http&count=1&rotate=1';
        },
        rotatingGatewayChannels() {
          return this.channels.filter(channel => channel.enabled && channel.rotate_on_dial);
        },
        switchDialogNodes() {
          if (!this.channelSwitchDialog.channel) return [];
          const nodes = this.nodes.filter(n => this.matchChannelRegion(this.channelSwitchDialog.channel.region, n.region));
          return this.filterNodeList(nodes, this.switchFilters).slice(0, 200);
        }
      },
      mounted() {
        this.restoreLogin();
        if (this.isLoggedIn) this.load();
        setInterval(() => this.load(false), 7000);
        setInterval(() => { this.tickNow = Date.now(); }, 1000);
      },
      methods: {
        restoreLogin() {
          const raw = localStorage.getItem('regionProxyGatewayAdminAuth');
          if (!raw) return;
          try {
            const saved = JSON.parse(raw);
            if (!saved.username || !saved.password) return;
            this.loginForm = { username: saved.username, password: saved.password, rememberCredentials: true };
            this.adminAuth = { username: saved.username, password: saved.password };
            this.isLoggedIn = true;
          } catch (err) {
            localStorage.removeItem('regionProxyGatewayAdminAuth');
          }
        },
        async login() {
          const username = String(this.loginForm.username || '').trim();
          const password = String(this.loginForm.password || '');
          if (!username || !password) return message.warning('请输入后台账号和密码');
          this.loginLoading = true;
          this.adminAuth = { username, password };
          try {
            await this.request('status');
            this.isLoggedIn = true;
            if (this.loginForm.rememberCredentials) {
              localStorage.setItem('regionProxyGatewayAdminAuth', JSON.stringify({ username, password }));
            } else {
              localStorage.removeItem('regionProxyGatewayAdminAuth');
            }
            message.success('登录成功');
            await this.load(false);
          } catch (err) {
            this.isLoggedIn = false;
            message.error(err.message === 'unauthorized' ? '账号或密码错误' : err.message);
          } finally {
            this.loginLoading = false;
          }
        },
        logout() {
          this.isLoggedIn = false;
          this.adminAuth = { username: '', password: '' };
          this.loginForm.password = '';
          this.loginForm.rememberCredentials = false;
          localStorage.removeItem('regionProxyGatewayAdminAuth');
          message.success('已退出登录');
        },
        emptyChannelForm() {
          return { id: '', listen_host: '0.0.0.0', listen_port: null, region: '', rotate_minutes: 0, rotate_on_dial: false, selection_mode: 'auto', manual_node_id: '', enabled: true };
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
          if (!this.isLoggedIn) return;
          if (showLoading) this.loading = true;
          try {
            const [status, channels, connections, nodes, deepStatus] = await Promise.all([
              this.request('status'),
              this.request('channels'),
              this.request('connections'),
              this.request('nodes'),
              this.request('deep-tests/status').catch(() => ({ stats: this.deepStats }))
            ]);
            this.channels = channels.channels || [];
            this.connections = connections.connections || [];
            this.nodes = nodes.nodes || [];
            this.deepStats = deepStatus.stats || this.deepStats;
            this.nodeScan = status.node_scan || this.nodeScan;
            if (status.settings) this.settings = Object.assign({}, this.settings, status.settings, { admin_password: '', proxy_password: '' });
          } catch (err) {
            message.error(err.message);
          } finally {
            this.loading = false;
          }
        },
        async request(path, options = {}) {
          const headers = Object.assign({ 'Content-Type': 'application/json' }, options.headers || {});
          if (this.adminAuth.username || this.adminAuth.password) {
            headers.Authorization = 'Basic ' + btoa(unescape(encodeURIComponent(this.adminAuth.username + ':' + this.adminAuth.password)));
          }
          const res = await fetch(apiBase + path.replace(/^\//, ''), Object.assign({}, options, { headers }));
          const body = await res.json().catch(() => ({}));
          if (res.status === 401) {
            this.isLoggedIn = false;
            throw new Error(body.error || 'unauthorized');
          }
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
        async enqueueDeepTestVisibleNodes() {
          const nodeIDs = this.visibleNodes.map(n => n.id).filter(Boolean).slice(0, 500);
          if (!nodeIDs.length) return message.warning('当前列表没有可深度测试的节点');
          this.deepTesting = true;
          try {
            const body = await this.request('deep-tests', { method: 'POST', body: JSON.stringify({ node_ids: nodeIDs }) });
            this.deepStats = body.stats || this.deepStats;
            const summary = body.summary || {};
            message.success('已加入深测队列：新增 ' + Number(summary.created || 0) + '，跳过 ' + Number(summary.skipped || 0));
            await this.load(false);
          } catch (err) {
            message.error(err.message);
          } finally {
            this.deepTesting = false;
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
            rotate_on_dial: row.rotate_on_dial,
            selection_mode: row.selection_mode,
            manual_node_id: row.manual_node_id || row.current_node_id || '',
            enabled: row.enabled
          } : this.emptyChannelForm();
        },
        openChannelDialogForNode(node) {
          const form = this.emptyChannelForm();
          form.id = this.suggestChannelID(node.region);
          form.region = node.region || '';
          form.selection_mode = 'manual';
          form.manual_node_id = node.id;
          form.rotate_minutes = 10;
          this.channelDialog.open = true;
          this.channelDialog.editing = false;
          this.channelDialog.originalID = '';
          this.channelForm = form;
        },
        async saveChannel() {
          const channel = Object.assign({}, this.channelForm, { region: String(this.channelForm.region || '').toLowerCase(), original_id: this.channelDialog.originalID });
          if (!channel.listen_port) channel.listen_port = 0;
          if (channel.selection_mode !== 'manual') delete channel.manual_node_id;
          try {
            const body = await this.request('channels', { method: 'POST', body: JSON.stringify(channel) });
            this.noticeRuntimeResult(body, '通道已保存并热更新');
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
            const body = await this.request('channels/' + encodeURIComponent(channelID) + '/switch', { method: 'POST', body: JSON.stringify({ node_id: nodeID }) });
            this.noticeRuntimeResult(body, '已切换节点');
            await this.load(false);
          } catch (err) {
            message.error(err.message);
          }
        },
        async deleteChannel(channelID) {
          Modal.confirm({
            title: '确认删除通道？',
            content: '删除通道 ' + channelID + ' 后会立即关闭对应代理端口。',
            okText: '删除',
            okType: 'danger',
            cancelText: '取消',
            onOk: async () => {
              const body = await this.request('channels/' + encodeURIComponent(channelID), { method: 'DELETE' });
              this.noticeRuntimeResult(body, '通道已删除并热更新');
              await this.load(false);
            }
          });
        },
        async saveSettings() {
          try {
            const body = await this.request('settings', { method: 'POST', body: JSON.stringify(this.settings) });
            if (body.settings) this.settings = Object.assign({}, this.settings, body.settings, { admin_password: '', proxy_password: '' });
            this.noticeRuntimeResult(body, '设置已保存并热更新');
            await this.load(false);
          } catch (err) {
            message.error(err.message);
          }
        },
        async generateExtractToken() {
          try {
            const body = await this.request('settings/proxy-extract-token', { method: 'POST' });
            if (body.settings) this.settings = Object.assign({}, this.settings, body.settings, { admin_password: '', proxy_password: '' });
            message.success('提取 Token 已生成');
          } catch (err) {
            message.error(err.message);
          }
        },
        channelsByRegion(region) {
          const target = this.normalizeRegion(region);
          return this.channels.filter(ch => this.matchChannelRegion(ch.region, target) && ch.enabled);
        },
        normalizeRegion(region) {
          return String(region || '').trim().toLowerCase();
        },
        suggestChannelID(region) {
          const prefix = this.normalizeRegion(region) || 'any';
          return prefix + '-' + Date.now().toString().slice(-6);
        },
        isAnyRegion(region) {
          const normalized = this.normalizeRegion(region);
          return normalized === '' || normalized === '*';
        },
        matchChannelRegion(channelRegion, nodeRegion) {
          if (this.isAnyRegion(channelRegion)) return true;
          return this.normalizeRegion(channelRegion) === this.normalizeRegion(nodeRegion);
        },
        channelExitAddress(channel) {
          const current = channel.current_node || {};
          return channel.current_exit_ip || current.ip || current.hostname || '-';
        },
        rotationExitText(value) {
          return value || '-';
        },
        rotationStateCell(record) {
          const status = this.rotationStatus(record);
          return h('div', { class: 'rotation-box' }, [
            h('div', { class: 'rotation-head' }, [h(antd.Tag, { color: status.color }, () => status.text), h('span', { class: 'muted' }, status.detail)]),
            this.rotationStateLine('上次出口', this.rotationExitText(record.last_exit_ip)),
            this.rotationStateLine('当前出口', this.rotationExitText(record.current_exit_ip || this.channelExitAddress(record))),
            this.rotationStateLine('上次轮换', this.formatChannelTime(record.last_rotation_at)),
            this.rotationStateLine('下次轮换', this.formatChannelTime(record.next_rotation_at))
          ]);
        },
        rotationStatus(record) {
          if (!record.enabled) return { color: 'default', text: '已停用', detail: '通道未启用' };
          if (record.selection_mode === 'manual') return { color: 'purple', text: '手动节点', detail: '不会自动轮换' };
          if (!record.rotate_minutes) return { color: 'default', text: '固定出口', detail: '未设置轮换间隔' };
          if (record.last_error) return { color: 'red', text: '轮换异常', detail: '请求失败会继续自恢复' };
          const remaining = this.rotationRemainingText(record.next_rotation_at);
          return { color: 'green', text: '自动轮换', detail: remaining ? '约 ' + remaining + ' 后轮换' : '等待下一次计划' };
        },
        rotationRemainingText(value) {
          if (!value) return '';
          const date = new Date(value);
          if (Number.isNaN(date.getTime())) return '';
          const ms = date.getTime() - this.tickNow;
          if (ms <= 0) return '即将';
          const totalSeconds = Math.ceil(ms / 1000);
          if (totalSeconds < 60) return totalSeconds + ' 秒';
          const minutes = Math.floor(totalSeconds / 60);
          const seconds = totalSeconds % 60;
          if (minutes < 60) return minutes + ' 分钟 ' + seconds + ' 秒';
          const hours = Math.floor(minutes / 60);
          const rest = minutes % 60;
          return hours + ' 小时 ' + rest + ' 分钟 ' + seconds + ' 秒';
        },
        rotationStateLine(label, value) {
          return h('div', [h('span', { class: 'muted' }, label + '：'), h('span', { class: 'mono' }, value || '-')]);
        },
        formatChannelTime(value) {
          if (!value) return '-';
          const date = new Date(value);
          if (Number.isNaN(date.getTime())) return '-';
          if (date.getFullYear() <= 1) return '-';
          return date.toLocaleString();
        },
        noticeRuntimeResult(body, fallback) {
          if (body && body.runtime_error) return message.warning(fallback + '，但热更新失败：' + body.runtime_error);
          return message.success(fallback);
        },
        connectionLine(label, value) {
          return h('div', { class: 'connection-line' }, [
            h(antd.Tag, null, () => label),
            h('code', { class: 'mono', title: value }, value),
            h(antd.Button, { size: 'small', onClick: () => this.copyText(value) }, () => '复制')
          ]);
        },
        deepTestCell(record) {
          const result = record.deep_test;
          if (!result) return h('span', { class: 'muted' }, '未深测');
          if (result.status === 'success') {
            return h('div', [
              h(antd.Tag, { color: 'green' }, () => '成功'),
              h('div', { class: 'mono' }, result.exit_ip || '-'),
              h('div', { class: 'muted' }, (result.connect_ms ? result.connect_ms + ' ms' : '') + (result.exit_country ? ' / ' + result.exit_country : ''))
            ]);
          }
          return h('div', [
            h(antd.Tag, { color: 'red' }, () => '失败'),
            h('div', { class: 'muted', title: result.fail_reason || '' }, this.shortText(result.fail_reason || '失败', 42))
          ]);
        },
        shortText(value, max) {
          value = String(value || '');
          return value.length > max ? value.slice(0, max) + '...' : value;
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
        proxyAddressNoScheme(channel) {
          const source = channel.proxy_auth_http || channel.proxy_auth_socks5 || channel.proxy_url_http || channel.proxy_url_socks5 || '';
          if (!source) return '';
          try {
            const url = new URL(source);
            const host = (url.hostname === '0.0.0.0' || url.hostname === '127.0.0.1' || url.hostname === 'localhost' || url.hostname === '::')
              ? (window.location.hostname || url.hostname)
              : url.hostname;
            const auth = url.username || url.password ? decodeURIComponent(url.username) + ':' + decodeURIComponent(url.password) + '@' : '';
            return auth + host + (url.port ? ':' + url.port : '');
          } catch (err) {
            const http = this.proxyAddress(channel, 'http');
            return http.replace(/^https?:\/\//, '').replace(/^socks5:\/\//, '');
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
        channelRegionText(region) {
          if (this.isAnyRegion(region)) return '不限地区（*）';
          return this.regionText(region) + '（' + region + '）';
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
            .replace('udp host unreachable; deprioritized until deep test or successful ping', '主机不可达；已降低优先级，深测成功或 Ping 恢复后再优先使用')
            .replace('ping ok; udp port cannot be fully verified without vpn handshake', 'Ping 正常；UDP 端口未握手确认')
            .replace('ping ok; tcp port ok', 'Ping 正常；TCP 端口可连接')
            .replace('tcp port ok', 'TCP 端口可连接')
            .replace('ping failed; udp cannot be verified without vpn handshake:', 'Ping 失败；已降低优先级：')
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
