package dashboard

// dashboardHTML is the embedded HTML for the governance dashboard.
const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Azure Governance Dashboard</title>
<script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.7/dist/chart.umd.min.js"></script>
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{
  --bg:#0f172a;--surface:#1e293b;--border:#334155;--card:#1e293b;
  --text:#e2e8f0;--muted:#94a3b8;--accent:#38bdf8;--green:#4ade80;
  --red:#f87171;--amber:#fbbf24;--purple:#a78bfa;
  --radius:12px;--shadow:0 4px 24px rgba(0,0,0,.3);
}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:var(--bg);color:var(--text);line-height:1.6}
a{color:var(--accent);text-decoration:none}
.topbar{background:var(--surface);border-bottom:1px solid var(--border);padding:12px 32px;display:flex;align-items:center;justify-content:space-between;position:sticky;top:0;z-index:100}
.topbar h1{font-size:18px;font-weight:600;display:flex;align-items:center;gap:8px}
.topbar h1 span{color:var(--accent)}
.topbar .meta{font-size:13px;color:var(--muted)}
.container{max-width:1440px;margin:0 auto;padding:24px 32px}

/* Month Picker */
.filter-bar{display:flex;align-items:center;gap:16px;margin-bottom:20px;flex-wrap:wrap}
.filter-bar label{font-size:13px;color:var(--muted);font-weight:600;text-transform:uppercase;letter-spacing:.06em}
.month-select{background:var(--surface);color:var(--text);border:1px solid var(--border);border-radius:8px;padding:8px 16px;font-size:14px;cursor:pointer;outline:none;min-width:180px}
.month-select:focus{border-color:var(--accent)}
.month-select option{background:var(--bg);color:var(--text)}
.filter-info{font-size:12px;color:var(--muted);margin-left:auto}

/* KPI Cards */
.kpi-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:14px;margin-bottom:24px}
.kpi{background:var(--card);border:1px solid var(--border);border-radius:var(--radius);padding:18px 22px;box-shadow:var(--shadow);position:relative;overflow:hidden}
.kpi::before{content:'';position:absolute;top:0;left:0;width:4px;height:100%}
.kpi.blue::before{background:var(--accent)}
.kpi.green::before{background:var(--green)}
.kpi.red::before{background:var(--red)}
.kpi.amber::before{background:var(--amber)}
.kpi.purple::before{background:var(--purple)}
.kpi .label{font-size:11px;text-transform:uppercase;letter-spacing:.08em;color:var(--muted);margin-bottom:2px}
.kpi .value{font-size:26px;font-weight:700}
.kpi .sub{font-size:11px;color:var(--muted);margin-top:2px}

/* Tabs */
.tabs{display:flex;gap:4px;margin-bottom:20px;background:var(--surface);border-radius:8px;padding:4px;width:fit-content}
.tab{padding:8px 20px;border-radius:6px;cursor:pointer;font-size:13px;font-weight:500;color:var(--muted);transition:all .2s}
.tab.active{background:var(--accent);color:var(--bg)}
.tab:hover:not(.active){color:var(--text)}
.tab-content{display:none}
.tab-content.active{display:block}

/* Charts */
.chart-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(400px,1fr));gap:20px;margin-bottom:24px}
.chart-card{background:var(--card);border:1px solid var(--border);border-radius:var(--radius);padding:20px;box-shadow:var(--shadow)}
.chart-card h3{font-size:14px;font-weight:600;margin-bottom:12px;color:var(--muted)}
.chart-card canvas{width:100%!important;max-height:320px}

/* Tables */
.section{margin-bottom:28px}
.section-head{display:flex;align-items:center;justify-content:space-between;margin-bottom:12px}
.section-head h2{font-size:16px;font-weight:600}
.badge{display:inline-block;background:var(--accent);color:var(--bg);font-size:11px;font-weight:700;padding:2px 10px;border-radius:99px}
.tbl-wrap{background:var(--card);border:1px solid var(--border);border-radius:var(--radius);overflow:hidden;box-shadow:var(--shadow)}
table{width:100%;border-collapse:collapse;font-size:13px}
th{background:#0f172a;text-align:left;padding:10px 14px;font-weight:600;color:var(--muted);text-transform:uppercase;letter-spacing:.06em;font-size:11px;position:sticky;top:0}
td{padding:8px 14px;border-top:1px solid var(--border)}
tr:hover td{background:rgba(56,189,248,.04)}
.scroll-tbl{max-height:450px;overflow-y:auto}
.cost-val{font-weight:600;text-align:right}
.pill{display:inline-block;padding:2px 8px;border-radius:6px;font-size:11px;font-weight:600}
.pill.high{background:rgba(248,113,113,.15);color:var(--red)}
.pill.medium{background:rgba(251,191,36,.15);color:var(--amber)}
.pill.low{background:rgba(74,222,128,.15);color:var(--green)}

/* Loading */
.loader{display:flex;align-items:center;justify-content:center;padding:60px;color:var(--muted);font-size:14px}
.spinner{width:24px;height:24px;border:3px solid var(--border);border-top-color:var(--accent);border-radius:50%;animation:spin .8s linear infinite;margin-right:12px}
@keyframes spin{to{transform:rotate(360deg)}}
@media(max-width:768px){.container{padding:16px}.chart-grid{grid-template-columns:1fr}.kpi-grid{grid-template-columns:repeat(2,1fr)}}
</style>
</head>
<body>

<div class="topbar">
  <h1><span>&#9670;</span> Azure Governance Dashboard</h1>
  <div class="meta" id="lastUpdated">Loading...</div>
</div>

<div class="container">
  <!-- Month Filter -->
  <div class="filter-bar">
    <label>Period:</label>
    <select class="month-select" id="monthPicker">
      <option value="ALL">YTD (All Months)</option>
    </select>
    <div class="filter-info" id="filterInfo"></div>
  </div>

  <!-- KPI Row -->
  <div class="kpi-grid">
    <div class="kpi blue"><div class="label">Subscriptions</div><div class="value" id="kpi-subs">—</div><div class="sub" id="kpi-subs-sub">with cost data</div></div>
    <div class="kpi green"><div class="label">Total Spend</div><div class="value" id="kpi-spend">—</div><div class="sub" id="kpi-spend-sub"></div></div>
    <div class="kpi purple"><div class="label">Resource Groups</div><div class="value" id="kpi-rgs">—</div><div class="sub" id="kpi-rgs-sub">with cost data</div></div>
    <div class="kpi amber"><div class="label">Right-Sizing Alerts</div><div class="value" id="kpi-rs">—</div><div class="sub">Underutilized VMs</div></div>
    <div class="kpi red"><div class="label">Orphaned Resources</div><div class="value" id="kpi-idle">—</div><div class="sub" id="kpi-idle-sub"></div></div>
  </div>

  <!-- Tabs -->
  <div class="tabs">
    <div class="tab active" data-tab="sub">Subscription Costs</div>
    <div class="tab" data-tab="rg">Resource Group Costs</div>
    <div class="tab" data-tab="charts">Charts</div>
    <div class="tab" data-tab="rightsizing">Right-Sizing</div>
    <div class="tab" data-tab="idle">Orphaned Resources</div>
  </div>

  <!-- Subscription Costs Tab -->
  <div class="tab-content active" id="tab-sub">
    <div class="section">
      <div class="section-head"><h2>Subscription-wise Cost</h2><span class="badge" id="subBadge">0</span></div>
      <div class="tbl-wrap"><div class="scroll-tbl">
        <table><thead><tr><th>#</th><th>Subscription</th><th>ID</th><th style="text-align:right">Cost</th><th>Currency</th></tr></thead>
        <tbody id="tblSub"></tbody></table>
      </div></div>
    </div>
  </div>

  <!-- Resource Group Costs Tab -->
  <div class="tab-content" id="tab-rg">
    <div class="section">
      <div class="section-head"><h2>Resource Group-wise Cost</h2><span class="badge" id="rgBadge">0</span></div>
      <div class="tbl-wrap"><div class="scroll-tbl">
        <table><thead><tr><th>#</th><th>Resource Group</th><th>Subscription</th><th style="text-align:right">Cost</th><th>Currency</th></tr></thead>
        <tbody id="tblRG"></tbody></table>
      </div></div>
    </div>
  </div>

  <!-- Charts Tab -->
  <div class="tab-content" id="tab-charts">
    <div class="chart-grid">
      <div class="chart-card"><h3 id="chartSubTitle">Cost by Subscription</h3><canvas id="chartSub"></canvas></div>
      <div class="chart-card"><h3>Monthly Cost Trend</h3><canvas id="chartMonthly"></canvas></div>
    </div>
    <div class="chart-grid">
      <div class="chart-card"><h3 id="chartRGTitle">Top 15 Resource Groups</h3><canvas id="chartRG"></canvas></div>
      <div class="chart-card"><h3>Subscription Cost Distribution</h3><canvas id="chartSubPie"></canvas></div>
    </div>
  </div>

  <!-- Right-Sizing Tab -->
  <div class="tab-content" id="tab-rightsizing">
    <div class="chart-grid">
      <div class="chart-card"><h3>By Subscription</h3><canvas id="chartRSSub"></canvas></div>
      <div class="chart-card"><h3>By Impact Level</h3><canvas id="chartRSImpact"></canvas></div>
    </div>
    <div class="section">
      <div class="section-head"><h2>VM Right-Sizing Recommendations</h2><span class="badge" id="rsBadge">0</span></div>
      <div class="tbl-wrap"><div class="scroll-tbl">
        <table><thead><tr><th>#</th><th>Resource</th><th>Type</th><th>Resource Group</th><th>Subscription</th><th>Impact</th><th>Problem</th></tr></thead>
        <tbody id="tblRS"></tbody></table>
      </div></div>
    </div>
  </div>

  <!-- Idle Tab -->
  <div class="tab-content" id="tab-idle">
    <div class="chart-grid">
      <div class="chart-card"><h3>By Type</h3><canvas id="chartIdleType"></canvas></div>
      <div class="chart-card"><h3>Waste by Type ($)</h3><canvas id="chartIdleWaste"></canvas></div>
    </div>
    <div class="section">
      <div class="section-head"><h2>All Orphaned / Idle Resources</h2><span class="badge" id="idleBadge">0</span></div>
      <div class="tbl-wrap"><div class="scroll-tbl">
        <table><thead><tr><th>#</th><th>Resource</th><th>Type</th><th>Resource Group</th><th>Subscription</th><th>Reason</th><th style="text-align:right">Est. Monthly</th></tr></thead>
        <tbody id="tblIdle"></tbody></table>
      </div></div>
    </div>
  </div>
</div>

<script>
const $ = s => document.querySelector(s);
const $$ = s => document.querySelectorAll(s);
const fmt = n => n.toLocaleString('en-US',{minimumFractionDigits:2,maximumFractionDigits:2});
const COLORS = ['#38bdf8','#4ade80','#fbbf24','#f87171','#a78bfa','#fb923c','#2dd4bf','#e879f9','#60a5fa','#facc15','#34d399','#f472b6','#818cf8','#93c5fd','#86efac'];
const MONTH_NAMES = {'01':'Jan','02':'Feb','03':'Mar','04':'Apr','05':'May','06':'Jun','07':'Jul','08':'Aug','09':'Sep','10':'Oct','11':'Nov','12':'Dec'};

// Chart defaults
Chart.defaults.color = '#94a3b8';
Chart.defaults.borderColor = '#334155';
Chart.defaults.font.family = '-apple-system,BlinkMacSystemFont,Segoe UI,Roboto,sans-serif';

// Tabs
$$('.tab').forEach(t => t.addEventListener('click', () => {
  $$('.tab').forEach(x => x.classList.remove('active'));
  $$('.tab-content').forEach(x => x.classList.remove('active'));
  t.classList.add('active');
  $('#tab-' + t.dataset.tab).classList.add('active');
}));

// Globals to hold fetched data
let costData = null;
let rsData = null;
let idleData = null;
let charts = {};

function destroyChart(key) { if (charts[key]) { charts[key].destroy(); delete charts[key]; } }

function monthLabel(m) {
  const parts = m.split('-');
  return (MONTH_NAMES[parts[1]] || parts[1]) + ' ' + parts[0];
}

function getSubCost(sub, month) {
  if (month === 'ALL') return sub.ytdTotal;
  return (sub.monthlyTotals && sub.monthlyTotals[month]) || 0;
}

function getRGCost(rg, month) {
  if (month === 'ALL') return rg.ytdTotal;
  return (rg.monthlyTotals && rg.monthlyTotals[month]) || 0;
}

// Render everything for selected month
function renderCostView(month) {
  if (!costData) return;
  const subs = costData.subscriptions || [];

  // Filter subs that have cost for this month
  const activeSubs = subs.filter(s => getSubCost(s, month) > 0);
  const totalSpend = subs.reduce((t, s) => t + getSubCost(s, month), 0);

  // Collect all RGs across all subs
  let allRGs = [];
  let rgSet = new Set();
  subs.forEach(s => {
    (s.resourceGroups || []).forEach(rg => {
      const cost = getRGCost(rg, month);
      if (cost > 0) {
        allRGs.push({ name: rg.name, sub: s.name, currency: s.currency, cost });
        rgSet.add(rg.name);
      }
    });
  });
  allRGs.sort((a, b) => b.cost - a.cost);

  const periodLabel = month === 'ALL' ? 'YTD ' + costData.year : monthLabel(month);

  // KPIs
  $('#kpi-subs').textContent = activeSubs.length;
  $('#kpi-subs-sub').textContent = month === 'ALL' ? 'with cost YTD' : 'with cost in ' + monthLabel(month);
  $('#kpi-spend').textContent = fmt(totalSpend);
  $('#kpi-spend-sub').textContent = periodLabel;
  $('#kpi-rgs').textContent = rgSet.size;
  $('#kpi-rgs-sub').textContent = month === 'ALL' ? 'with cost YTD' : 'with cost in ' + monthLabel(month);
  $('#filterInfo').textContent = 'Showing: ' + periodLabel + ' | ' + activeSubs.length + ' subscriptions, ' + rgSet.size + ' resource groups';

  // Sub table
  const sortedSubs = [...subs].map(s => ({...s, cost: getSubCost(s, month)})).filter(s => s.cost > 0).sort((a,b) => b.cost - a.cost);
  $('#subBadge').textContent = sortedSubs.length;
  $('#tblSub').innerHTML = sortedSubs.map((s, i) =>
    '<tr><td>' + (i+1) + '</td><td>' + esc(s.name) + '</td><td style="font-size:11px;color:#94a3b8">' + s.id + '</td><td class="cost-val">' + fmt(s.cost) + '</td><td>' + s.currency + '</td></tr>'
  ).join('');

  // RG table
  $('#rgBadge').textContent = allRGs.length;
  $('#tblRG').innerHTML = allRGs.map((r, i) =>
    '<tr><td>' + (i+1) + '</td><td>' + esc(r.name) + '</td><td>' + esc(r.sub) + '</td><td class="cost-val">' + fmt(r.cost) + '</td><td>' + r.currency + '</td></tr>'
  ).join('');

  // Charts
  renderCostCharts(month, subs, allRGs, periodLabel);
}

function renderCostCharts(month, subs, allRGs, periodLabel) {
  // Sub bar chart
  const subsSorted = [...subs].map(s => ({name: s.name, cost: getSubCost(s, month)})).filter(s => s.cost > 0).sort((a,b) => b.cost - a.cost).slice(0, 15);
  destroyChart('sub');
  $('#chartSubTitle').textContent = 'Cost by Subscription — ' + periodLabel;
  charts['sub'] = new Chart($('#chartSub'), {
    type: 'bar',
    data: { labels: subsSorted.map(s => s.name), datasets: [{ data: subsSorted.map(s => s.cost), backgroundColor: COLORS.slice(0, subsSorted.length), borderRadius: 4 }] },
    options: { indexAxis: 'y', plugins: { legend: { display: false } }, scales: { x: { ticks: { callback: v => (v/1000).toFixed(0) + 'k' }}}}
  });

  // Monthly trend (always show all months)
  const months = costData.months || [];
  const trendSubs = subs.filter(s => s.ytdTotal > 0).sort((a,b) => b.ytdTotal - a.ytdTotal).slice(0, 8);
  destroyChart('monthly');
  charts['monthly'] = new Chart($('#chartMonthly'), {
    type: 'bar',
    data: {
      labels: months.map(m => monthLabel(m)),
      datasets: trendSubs.map((s, i) => ({
        label: s.name,
        data: months.map(m => (s.monthlyTotals && s.monthlyTotals[m]) || 0),
        backgroundColor: COLORS[i % COLORS.length],
        borderRadius: 2,
      }))
    },
    options: { plugins: { legend: { position: 'bottom', labels: { boxWidth: 12, padding: 6, font: { size: 11 }}}}, scales: { x: { stacked: true }, y: { stacked: true, ticks: { callback: v => (v/1000).toFixed(0) + 'k' }}}}
  });

  // Top 15 RG bar
  const topRGs = allRGs.slice(0, 15);
  destroyChart('rg');
  $('#chartRGTitle').textContent = 'Top 15 Resource Groups — ' + periodLabel;
  charts['rg'] = new Chart($('#chartRG'), {
    type: 'bar',
    data: { labels: topRGs.map(r => r.name.length > 25 ? r.name.substring(0,22) + '...' : r.name), datasets: [{ data: topRGs.map(r => r.cost), backgroundColor: COLORS.slice(0, topRGs.length), borderRadius: 4 }] },
    options: { indexAxis: 'y', plugins: { legend: { display: false } }, scales: { x: { ticks: { callback: v => (v/1000).toFixed(0) + 'k' }}}}
  });

  // Pie chart
  const pieSubs = [...subs].map(s => ({name: s.name, cost: getSubCost(s, month)})).filter(s => s.cost > 0).sort((a,b) => b.cost - a.cost);
  destroyChart('subpie');
  charts['subpie'] = new Chart($('#chartSubPie'), {
    type: 'doughnut',
    data: { labels: pieSubs.map(s => s.name), datasets: [{ data: pieSubs.map(s => s.cost), backgroundColor: COLORS }] },
    options: { plugins: { legend: { position: 'right', labels: { boxWidth: 12, padding: 6, font: { size: 11 }}}}}
  });
}

function renderRightsizing(data) {
  rsData = data;
  const items = data.recommendations || [];
  const bySub = data.bySubscription || {};
  const byImpact = data.byImpact || {};

  $('#kpi-rs').textContent = data.total || 0;
  $('#rsBadge').textContent = items.length;

  // Sub doughnut
  destroyChart('rssub');
  charts['rssub'] = new Chart($('#chartRSSub'), {
    type: 'doughnut',
    data: { labels: Object.keys(bySub), datasets: [{ data: Object.values(bySub), backgroundColor: COLORS }] },
    options: { plugins: { legend: { position: 'right', labels: { boxWidth: 12, padding: 6, font: { size: 11 }}}}}
  });

  // Impact doughnut
  const impactColors = { High: '#f87171', Medium: '#fbbf24', Low: '#4ade80' };
  destroyChart('rsimpact');
  charts['rsimpact'] = new Chart($('#chartRSImpact'), {
    type: 'doughnut',
    data: { labels: Object.keys(byImpact), datasets: [{ data: Object.values(byImpact), backgroundColor: Object.keys(byImpact).map(k => impactColors[k] || '#94a3b8') }] },
    options: { plugins: { legend: { position: 'right', labels: { boxWidth: 12, padding: 6, font: { size: 11 }}}}}
  });

  // Table
  $('#tblRS').innerHTML = items.map((r, i) =>
    '<tr><td>' + (i+1) + '</td><td style="font-weight:500">' + esc(r.resource) + '</td><td>' + esc(r.type) + '</td><td>' + esc(r.resourceGroup) + '</td><td>' + esc(r.subscription) + '</td><td><span class="pill ' + r.impact.toLowerCase() + '">' + r.impact + '</span></td><td style="font-size:12px">' + esc(r.problem) + '</td></tr>'
  ).join('');
}

function renderIdle(data) {
  idleData = data;
  const items = data.resources || [];
  const byType = data.byType || {};
  const wasteByType = data.wasteByType || {};

  $('#kpi-idle').textContent = data.total || 0;
  $('#kpi-idle-sub').textContent = 'Est. waste: ' + fmt(data.totalWaste || 0) + '/mo';
  $('#idleBadge').textContent = items.length;

  // Type doughnut
  destroyChart('idletype');
  charts['idletype'] = new Chart($('#chartIdleType'), {
    type: 'doughnut',
    data: { labels: Object.keys(byType), datasets: [{ data: Object.values(byType), backgroundColor: COLORS }] },
    options: { plugins: { legend: { position: 'right', labels: { boxWidth: 12, padding: 6, font: { size: 11 }}}}}
  });

  // Waste bar
  const wLabels = Object.keys(wasteByType).filter(k => wasteByType[k] > 0);
  destroyChart('idlewaste');
  charts['idlewaste'] = new Chart($('#chartIdleWaste'), {
    type: 'bar',
    data: { labels: wLabels, datasets: [{ label: 'Monthly Waste', data: wLabels.map(k => wasteByType[k]), backgroundColor: '#f87171', borderRadius: 4 }] },
    options: { indexAxis: 'y', plugins: { legend: { display: false }}}
  });

  // Table
  $('#tblIdle').innerHTML = items.map((r, i) =>
    '<tr><td>' + (i+1) + '</td><td style="font-weight:500">' + esc(r.name) + '</td><td>' + esc(r.type) + '</td><td>' + esc(r.resourceGroup) + '</td><td>' + esc(r.subscription) + '</td><td style="font-size:12px">' + esc(r.reason) + '</td><td class="cost-val">' + fmt(r.estMonthlyCost) + '</td></tr>'
  ).join('');
}

function esc(s) { const d = document.createElement('div'); d.textContent = s || ''; return d.innerHTML; }

// Month picker change
$('#monthPicker').addEventListener('change', function() {
  renderCostView(this.value);
});

// Load data sequentially with status updates
async function loadDashboard() {
  try {
    $('#lastUpdated').textContent = 'Loading cost data from Azure...';
    const costResp = await fetch('/api/dashboard/costs');
    if (!costResp.ok) throw new Error('Cost API: HTTP ' + costResp.status);
    costData = await costResp.json();

    // Populate month picker
    const picker = $('#monthPicker');
    (costData.months || []).forEach(m => {
      const opt = document.createElement('option');
      opt.value = m;
      opt.textContent = monthLabel(m);
      picker.appendChild(opt);
    });

    renderCostView('ALL');

    $('#lastUpdated').textContent = 'Loading rightsizing data...';
    const rsResp = await fetch('/api/dashboard/rightsizing');
    if (!rsResp.ok) throw new Error('Rightsizing API: HTTP ' + rsResp.status);
    renderRightsizing(await rsResp.json());

    $('#lastUpdated').textContent = 'Loading idle resources...';
    const idleResp = await fetch('/api/dashboard/idle');
    if (!idleResp.ok) throw new Error('Idle API: HTTP ' + idleResp.status);
    renderIdle(await idleResp.json());

    $('#lastUpdated').textContent = 'Last updated: ' + new Date().toLocaleString();
  } catch (err) {
    $('#lastUpdated').textContent = 'Error: ' + err.message;
    console.error(err);
  }
}

loadDashboard();
</script>
</body>
</html>` + "\n"
