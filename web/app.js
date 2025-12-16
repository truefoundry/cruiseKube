let currentClusterId = '';

function showStatus(message, isError = false) {
    const status = document.getElementById('status');
    status.textContent = message;
    status.className = 'status ' + (isError ? 'error' : 'success');
    status.style.display = 'block';
    setTimeout(() => {
        status.style.display = 'none';
    }, 5000);
}

async function loadClusters() {
    try {
        const response = await fetch('api/v1/clusters');
        if (!response.ok) {
            throw new Error('Failed to load clusters');
        }
        
        const data = await response.json();
        const clusters = (data.clusters || []).sort((a, b) => a.name.localeCompare(b.name));
        const select = document.getElementById('clusterSelect');
        
        select.innerHTML = '';
        if (!currentClusterId && clusters.length > 0) {
            currentClusterId = clusters[0].id;
        }
        
        clusters.forEach(cluster => {
            const option = document.createElement('option');
            option.value = cluster.id;
            option.textContent = cluster.name + (cluster.stats_available ? ' ✓' : ' ✗');
            option.selected = cluster.id === currentClusterId;
            select.appendChild(option);
        });
    } catch (error) {
        console.error('Error loading clusters:', error);
        showStatus('Failed to load clusters: ' + error.message, true);
    }
}

function switchCluster() {
  const select = document.getElementById('clusterSelect');
  const newClusterId = select.value;

  if (newClusterId !== currentClusterId) {
    currentClusterId = newClusterId;
    const isWorkloadsTab =
      (localStorage.getItem('ap_active_tab') || 'analysis') === 'workloads';
    if (isWorkloadsTab) {
      document.getElementById('loading').style.display = 'block';
      document.getElementById('loading').textContent = 'Loading workloads...';
      document.getElementById('workloadsTable').style.display = 'none';
    } else {
      document.getElementById('loading').style.display = 'none';
    }
    setStatsLoading();
    loadStats();
    loadAnalysis();
    if (isWorkloadsTab) {
      loadWorkloads();
    }
  }
}

function switchTab(which) {
  const workloadsBtn = document.getElementById('tab-workloads');
  const analysisBtn = document.getElementById('tab-analysis');
  const workloadsTable = document.getElementById('workloadsTable');
  const analysisWrap = document.getElementById('analysisWrap');

  if (which === 'analysis') {
    analysisBtn.classList.add('active');
    workloadsBtn.classList.remove('active');
    analysisWrap.style.display = 'block';
    workloadsTable.style.display = 'none';
    localStorage.setItem('ap_active_tab', 'analysis');
  } else {
    workloadsBtn.classList.add('active');
    analysisBtn.classList.remove('active');
    workloadsTable.style.display = 'table';
    analysisWrap.style.display = 'none';
    localStorage.setItem('ap_active_tab', 'workloads');
  }
}

async function loadWorkloads() {
  if (!currentClusterId) {
    document.getElementById('loading').textContent = 'Please select a cluster';
    return;
  }

  try {
    const response = await fetch(
      'api/v1/clusters/' + currentClusterId + '/workloads'
    );
    if (!response.ok) {
      throw new Error('Failed to load workloads');
    }

    const workloads = await response.json();
    displayWorkloads(workloads);

    document.getElementById('loading').style.display = 'none';
    const isWorkloadsTab = document
      .getElementById('tab-workloads')
      ?.classList.contains('active');
    if (isWorkloadsTab) {
      document.getElementById('workloadsTable').style.display = 'table';
    }
  } catch (error) {
    console.error('Error loading workloads:', error);
    showStatus('Failed to load workloads: ' + error.message, true);
    document.getElementById('loading').style.display = 'none';
  }
}

function setStatsLoading() {
  const ids = [
    'stat-cpu-used-allocated',
    'stat-cpu-requested-allocated',
    'stat-cpu-used-requested',
    'stat-mem-used-allocated',
    'stat-mem-requested-allocated',
    'stat-mem-used-requested',
    'stat-running-pods',
    'stat-nodes-count',
    'stat-total-current-cpu-requests',
    'stat-total-cpu-diff',
    'stat-total-current-memory-requests',
    'stat-total-memory-diff',
  ];
  ids.forEach((id) => {
    const el = document.getElementById(id);
    if (el) el.textContent = '…';
  });
}

function fmtPercent(v) {
  if (v === null || isNaN(v)) return '—';
  return `${Math.round(v)}%`;
}

async function fetchPromQuery(expr) {
  try {
    const url = `api/v1/clusters/${currentClusterId}/prometheus-proxy/api/v1/query?query=${encodeURIComponent(
      expr.trim()
    )}`;
    const res = await fetch(url);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const payload = await res.json();
    if (payload.status !== 'success') throw new Error('Query failed');
    const result =
      payload.data && payload.data.result ? payload.data.result : [];
    if (!result.length) return null;
    const value = parseFloat(result[0].value[1]);
    return isNaN(value) ? null : value;
  } catch (e) {
    console.error('Prom query error:', e);
    return null;
  }
}

async function loadStats() {
  if (!currentClusterId) return;

  // PromQL copied from Grafana small stat tiles
  const Q_CPU_USED_ALLOC = `sum( sum by (node) ( rate(container_cpu_usage_seconds_total{container!~"POD|",job="kubelet"}[1m]) ) unless sum by (node) (kube_node_status_allocatable{resource="nvidia_com_gpu", job="kube-state-metrics"} > 0) ) / ( sum( sum by (node) ( kube_node_status_allocatable{container!="",job="kube-state-metrics",resource="cpu"} ) unless sum by (node) (kube_node_status_allocatable{resource="nvidia_com_gpu", job="kube-state-metrics"} > 0) ) ) * 100`;

  const Q_CPU_REQ_ALLOC = ` ( sum( sum by (node) ( sum by (namespace, pod, node) ( kube_pod_container_resource_requests{container!="",job="kube-state-metrics",resource="cpu"} ) * on (namespace, pod) group_left () max by (namespace, pod) (kube_pod_status_phase{job="kube-state-metrics",phase="Running"} > 0) * on (namespace, pod) group_left () max by (namespace, pod) ( kube_pod_info{created_by_name!="cruisekube-pause-daemonset",job="kube-state-metrics"} ) ) unless sum by (node) (kube_node_status_allocatable{job="kube-state-metrics",resource="nvidia_com_gpu"} > 0) ) ) / ( sum( sum by (node) ( kube_node_status_allocatable{container!="",job="kube-state-metrics",resource="cpu"} ) unless sum by (node) (kube_node_status_allocatable{resource="nvidia_com_gpu", job="kube-state-metrics"} > 0) ) ) * 100`;

  const Q_CPU_USED_REQ = ` sum( sum by (node) ( rate(container_cpu_usage_seconds_total{container!~"POD|",job="kubelet"}[1m]) ) unless sum by (node) (kube_node_status_allocatable{resource="nvidia_com_gpu", job="kube-state-metrics"} > 0) ) / ( sum( sum by (node) ( sum by (namespace, pod, node) ( kube_pod_container_resource_requests{container!="",job="kube-state-metrics",resource="cpu"} ) * on (namespace, pod) group_left () max by (namespace, pod) (kube_pod_status_phase{job="kube-state-metrics",phase="Running"} > 0) * on (namespace, pod) group_left () max by (namespace, pod) ( kube_pod_info{created_by_name!="cruisekube-pause-daemonset",job="kube-state-metrics"} ) ) unless sum by (node) (kube_node_status_allocatable{job="kube-state-metrics",resource="nvidia_com_gpu"} > 0) ) ) * 100`;

  const Q_MEM_USED_ALLOC = ` ( sum( sum by (namespace, node, pod) ( container_memory_working_set_bytes{container!~"POD|",job="kubelet"} ) unless sum(kube_node_status_allocatable{resource="nvidia_com_gpu", job="kube-state-metrics"} > 0) ) / (1000*1000*1000) ) / ( ( sum( sum by (namespace, pod, node) ( kube_node_status_allocatable{container!="",job="kube-state-metrics",resource="memory"} ) unless sum(kube_node_status_allocatable{resource="nvidia_com_gpu", job="kube-state-metrics"} > 0) ) ) / (1000*1000*1000) ) * 100`;

  const Q_MEM_USED_REQ = ` ( sum( sum by (namespace, node, pod) (container_memory_working_set_bytes{container!~"POD|",job="kubelet"}) unless sum(kube_node_status_allocatable{job="kube-state-metrics",resource="nvidia_com_gpu"} > 0) ) / (1000 * 1000 * 1000) ) / ( sum( ( sum( sum by (namespace, pod, node) ( kube_pod_container_resource_requests{container!="",job="kube-state-metrics",resource="memory"} ) * on (namespace, pod) group_left () max by (namespace, pod) (kube_pod_status_phase{job="kube-state-metrics",phase="Running"} > 0) unless sum(kube_node_status_allocatable{job="kube-state-metrics",resource="nvidia_com_gpu"} > 0) ) ) ) / (1000 * 1000 * 1000) ) * 100`;

  const Q_MEM_REQ_ALLOC = ` ( ( sum( sum by (namespace, pod, node) ( kube_pod_container_resource_requests{container!="",job="kube-state-metrics",resource="memory"} ) * on (namespace, pod) group_left () max by (namespace, pod) (kube_pod_status_phase{job="kube-state-metrics",phase="Running"} > 0) unless sum(kube_node_status_allocatable{resource="nvidia_com_gpu", job="kube-state-metrics"} > 0) ) ) / (1000*1000*1000) ) / ( ( sum( sum by (namespace, pod, node) ( kube_node_status_allocatable{container!="",job="kube-state-metrics",resource="memory"} ) unless sum(kube_node_status_allocatable{resource="nvidia_com_gpu", job="kube-state-metrics"} > 0) ) ) / (1000*1000*1000) ) * 100`;

  const Q_RUNNING_PODS = `sum( (sum by (node) (kube_pod_status_phase{job="kube-state-metrics",phase=~"Running"})) unless ( sum by (node) (kube_node_status_allocatable{job="kube-state-metrics",resource="nvidia_com_gpu"} > 0) ) )`;
  const Q_NODES_COUNT = `sum( ( count by (node) (kube_node_labels{job="kube-state-metrics"}) ) unless ( sum by (node) (kube_node_status_allocatable{job="kube-state-metrics",resource="nvidia_com_gpu"} > 0) ) )`;

  setStatsLoading();
  const [
    cpuUsedAlloc,
    cpuReqAlloc,
    cpuUsedReq,
    memUsedAlloc,
    memReqAlloc,
    memUsedReq,
    runningPods,
    nodesCount,
  ] = await Promise.all([
    fetchPromQuery(Q_CPU_USED_ALLOC),
    fetchPromQuery(Q_CPU_REQ_ALLOC),
    fetchPromQuery(Q_CPU_USED_REQ),
    fetchPromQuery(Q_MEM_USED_ALLOC),
    fetchPromQuery(Q_MEM_REQ_ALLOC),
    fetchPromQuery(Q_MEM_USED_REQ),
    fetchPromQuery(Q_RUNNING_PODS),
    fetchPromQuery(Q_NODES_COUNT),
  ]);

  const set = (id, val) => {
    const el = document.getElementById(id);
    if (el) el.textContent = fmtPercent(val);
  };
  set('stat-cpu-used-allocated', cpuUsedAlloc);
  set('stat-cpu-requested-allocated', cpuReqAlloc);
  set('stat-cpu-used-requested', cpuUsedReq);
  set('stat-mem-used-allocated', memUsedAlloc);
  set('stat-mem-requested-allocated', memReqAlloc);
  set('stat-mem-used-requested', memUsedReq);
  const setRaw = (id, val) => {
    const el = document.getElementById(id);
    if (el)
      el.textContent = val == null || isNaN(val) ? '—' : `${Math.round(val)}`;
  };
  setRaw('stat-running-pods', runningPods);
  setRaw('stat-nodes-count', nodesCount);

  // Infinity summary tiles via recommendation-analysis
  try {
    const recRes = await fetch(
      `api/v1/clusters/${currentClusterId}/recommendation-analysis`
    );
    if (recRes.ok) {
      const rec = await recRes.json();
      const sum = rec && rec.summary ? rec.summary : {};
      const toFixedMaybe = (v) =>
        v === null || v === undefined || isNaN(v) ? null : Math.round(v);
      const totalCPUReq = toFixedMaybe(sum.total_current_cpu_requests);
      const totalCPUDiff = toFixedMaybe(sum.total_cpu_differences);
      const totalMemReqGB = toFixedMaybe(
        (sum.total_current_memory_requests || 0) / 1000
      );
      const totalMemDiffGB = toFixedMaybe(
        (sum.total_memory_differences || 0) / 1000
      );

      const setRaw = (id, val) => {
        const el = document.getElementById(id);
        if (!el) return;
        el.textContent = val === null ? '—' : `${val}`;
      };
      setRaw('stat-total-current-cpu-requests', totalCPUReq);
      setRaw('stat-total-cpu-diff', totalCPUDiff);
      setRaw('stat-total-current-memory-requests', totalMemReqGB);
      setRaw('stat-total-memory-diff', totalMemDiffGB);
    }
  } catch (e) {
    console.error('Failed to load recommendation summary:', e);
  }
}

function displayWorkloads(workloads) {
  const tbody = document.getElementById('workloadsBody');
  tbody.innerHTML = '';

  workloads.forEach((workload) => {
    const row = document.createElement('tr');
    row.innerHTML = `
            <td><code>${workload.workload_id}</code></td>
            <td class="workload-name">${workload.name}</td>
            <td class="namespace">${workload.namespace}</td>
            <td>${workload.kind}</td>
            <td>
                <input type="text" class="eviction-ranking-input" 
                       value=${workload.eviction_ranking || 3}
                       placeholder="Enter eviction ranking"
                       data-workload-id="${workload.workload_id}">
            </td>
            <td>
                <div style="display: flex; align-items: center;">
                    <label class="toggle-switch">
                        <input type="checkbox" 
                               ${workload.enabled ? 'checked' : ''} 
                               data-workload-id="${workload.workload_id}"
                               onchange="toggleEnabled('${
                                 workload.workload_id
                               }')">
                        <span class="slider"></span>
                    </label>
                    <span class="toggle-label" data-workload-id="${
                      workload.workload_id
                    }">
                        ${workload.enabled ? 'Enabled' : 'Disabled'}
                    </span>
                </div>
            </td>
            <td>
                <button class="update-btn" onclick="updateEvictionRanking('${
                  workload.workload_id
                }')">
                    Update
                </button>
            </td>
        `;
    tbody.appendChild(row);
  });
}

// reserved helper (unused for now)
function setCell(row, colName, value) {
  const td = row.querySelector(`td[data-col="${colName}"]`);
  if (td) td.textContent = value == null ? '—' : `${value}`;
}

function formatNum(v) {
  if (v === null || v === undefined || isNaN(v)) return '—';
  const num = Number(v);
  if (Math.abs(num) >= 100) return Math.round(num);
  return Math.round(num * 100) / 100;
}

async function loadAnalysis() {
  try {
    const res = await fetch(
      `api/v1/clusters/${currentClusterId}/recommendation-analysis`
    );
    if (!res.ok) return;
    const data = await res.json();
    const tbody = document.getElementById('analysisBody');
    if (!tbody) return;
    tbody.innerHTML = '';
    const items = data && data.analysis ? data.analysis : [];
    const rows = items.map((item) => ({
      ...item,
      workload_id: `${item.workload_type || ''}:${
        item.workload_namespace || ''
      }:${item.workload_name || ''}`,
    }));
    rows.forEach((item) => {
      const tr = document.createElement('tr');
      tr.innerHTML = `
        <td><code>${item.workload_id}</code></td>
        <td class="workload-name">${item.workload_name || ''}</td>
        <td class="namespace">${item.workload_namespace || ''}</td>
        <td>${item.workload_type || ''}</td>
        <td>${item.pod_name || ''}</td>
        <td>${item.container_name || ''}</td>
        <td>${item.cpu_usage_7_days || 'N/A'}</td>
        <td>${item.autoscaling_on_cpu || 'No'}</td>
        <td>${item.blocking_karpenter || 'No'}</td>
        <td>${formatNum(item.current_requested_cpu)}</td>
        <td>${formatNum(item.recommended_cpu)}</td>
        <td>${formatNum(item.cpu_difference)}</td>
        <td>${formatNum(item.current_requested_memory)}</td>
        <td>${formatNum(item.recommended_memory)}</td>
        <td>${formatNum(item.memory_difference)}</td>
      `;
      tbody.appendChild(tr);
    });

    // enable sorting on CPU Diff and Memory Difference columns
    enableRecommendationsSorting(rows);
  } catch (e) {
    console.error('Failed to load analysis table:', e);
  }
}

function enableRecommendationsSorting(items) {
  const header = document.querySelector('#analysisTable thead');
  if (!header) return;
  header.onclick = (ev) => {
    const th = ev.target.closest('th');
    if (!th) return;
    if (!th.classList.contains('sortable')) return; // only sort marked columns
    const key = th.getAttribute('data-sort-key') || '';
    if (!key) return; // safety

    const type = th.getAttribute('data-sort-type') || 'string';
    const currentDir = th.getAttribute('data-sort-dir') || 'desc';
    const nextDir = currentDir === 'desc' ? 'asc' : 'desc';
    th.setAttribute('data-sort-dir', nextDir);

    const sorted = [...items].sort((a, b) => {
      let av = a[key];
      let bv = b[key];
      if (type === 'number') {
        av = Number(av ?? 0);
        bv = Number(bv ?? 0);
        return nextDir === 'asc' ? av - bv : bv - av;
      }
      // string compare
      av = (av ?? '').toString();
      bv = (bv ?? '').toString();
      return nextDir === 'asc' ? av.localeCompare(bv) : bv.localeCompare(av);
    });

    const tbody = document.getElementById('analysisBody');
    if (!tbody) return;
    tbody.innerHTML = '';
    sorted.forEach((item) => {
      const tr = document.createElement('tr');
      tr.innerHTML = `
        <td><code>${item.workload_id}</code></td>
        <td class="workload-name">${item.workload_name || ''}</td>
        <td class="namespace">${item.workload_namespace || ''}</td>
        <td>${item.workload_type || ''}</td>
        <td>${item.pod_name || ''}</td>
        <td>${item.container_name || ''}</td>
        <td>${item.cpu_usage_7_days || 'N/A'}</td>
        <td>${item.autoscaling_on_cpu || 'No'}</td>
        <td>${item.blocking_karpenter || 'No'}</td>
        <td>${formatNum(item.current_requested_cpu)}</td>
        <td>${formatNum(item.recommended_cpu)}</td>
        <td>${formatNum(item.cpu_difference)}</td>
        <td>${formatNum(item.current_requested_memory)}</td>
        <td>${formatNum(item.recommended_memory)}</td>
        <td>${formatNum(item.memory_difference)}</td>
      `;
      tbody.appendChild(tr);
    });
  };
}

async function updateEvictionRanking(workloadId) {
  const input = document.querySelector(
    `input.eviction-ranking-input[data-workload-id="${workloadId}"]`
  );
  const checkbox = document.querySelector(
    `input[type="checkbox"][data-workload-id="${workloadId}"]`
  );
  const button =
    input.parentElement.nextElementSibling.nextElementSibling.querySelector(
      '.update-btn'
    );
  const evictionRanking = parseInt(input.value);
  const enabled = checkbox.checked;

  if (isNaN(evictionRanking)) {
    showStatus('Eviction ranking cannot be a non-number', true);
    return;
  }
  if (evictionRanking < 1 || evictionRanking > 4) {
    showStatus('Eviction ranking must be between 1 and 4', true);
    return;
  }

  button.disabled = true;
  button.textContent = 'Updating...';

  try {
    const response = await fetch(
      `api/v1/clusters/${currentClusterId}/workloads/${workloadId}/overrides`,
      {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          eviction_ranking: evictionRanking,
          enabled: enabled,
        }),
      }
    );

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || 'Failed to update workload settings');
    }

    showStatus(`Workload settings updated successfully for ${workloadId}`);
  } catch (error) {
    console.error('Error updating workload settings:', error);
    showStatus('Failed to update workload settings: ' + error.message, true);
  } finally {
    button.disabled = false;
    button.textContent = 'Update';
  }
}

function toggleEnabled(workloadId) {
  const checkbox = document.querySelector(
    `input[type="checkbox"][data-workload-id="${workloadId}"]`
  );
  const label = document.querySelector(
    `span.toggle-label[data-workload-id="${workloadId}"]`
  );

  const isEnabled = checkbox.checked;
  label.textContent = isEnabled ? 'Enabled' : 'Disabled';
}

async function refreshWorkloads() {
  const refreshBtn = document.getElementById('refreshBtn');
  refreshBtn.disabled = true;
  refreshBtn.textContent = 'Refreshing...';

  const isWorkloadsTab = document
    .getElementById('tab-workloads')
    ?.classList.contains('active');
  if (isWorkloadsTab) {
    document.getElementById('loading').style.display = 'block';
    document.getElementById('loading').textContent = 'Refreshing workloads...';
    document.getElementById('workloadsTable').style.display = 'none';
  }

  await loadWorkloads();

  refreshBtn.disabled = false;
  refreshBtn.textContent = 'Refresh';
}

async function initialize() {
  let last = localStorage.getItem('ap_active_tab');
  if (!last) {
    // default to recommendations if nothing stored
    last = 'analysis';
    localStorage.setItem('ap_active_tab', last);
  }
  if (last === 'workloads') {
    switchTab('workloads');
  } else {
    switchTab('analysis');
  }

  await loadClusters();
  await loadStats();
  await loadAnalysis();
  if (last === 'workloads') {
    await loadWorkloads();
  } else {
    // keep current tab; optionally fetch workloads silently if needed
    // loadWorkloads();
  }
}

// Initialize when the page loads
initialize();
