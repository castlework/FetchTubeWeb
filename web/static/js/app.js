// === YouTube Downloader — Main Application Logic ===

// ---- localStorage key ----
const TASKS_STORAGE_KEY = 'ytdl_completed_tasks';

// ---- Global State ----
const state = {
  local: {
    info: null,
    selectedFormat: null,
    selectedAudioIds: [],
  },
  tasks: {},        // { taskID: { id, title, url, status, percent, speed_mbps, total_mb, save_dir, ... } }
  taskOrder: [],    // ordered task IDs for rendering
  selectedTasks: new Set(),  // 多选勾中的 taskID 集合
  pollTimer: null,
  folderTarget: null,
  fileTarget: null,
};

// ---- Initialization ----
document.addEventListener('DOMContentLoaded', () => {
  initTabs();
  loadConfig();
  restoreTasks();
  WS.onMessage = onProgress;
  WS.connect();
});

// ---- Tabs ----
function initTabs() {
  document.querySelectorAll('.tab-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
      document.querySelectorAll('.tab-content').forEach(t => t.classList.remove('active'));
      btn.classList.add('active');
      document.getElementById('tab-' + btn.dataset.tab).classList.add('active');
    });
  });
}

// ---- Config ----
async function loadConfig() {
  try {
    const cfg = await API.getConfig();
    applyConfig(cfg);
  } catch (e) {
    console.warn('加载配置失败:', e);
  }
}

async function saveConfig() {
  try {
    const cfg = collectConfig();
    await API.saveConfig(cfg);
  } catch (e) {
    console.warn('保存配置失败:', e);
  }
}

function applyConfig(cfg) {
  const l = cfg.local || {};
  setVal('l-url', l.last_url || '');
  setVal('l-proxy-mode', l.proxy_mode || '无');
  setVal('l-proxy-host', l.proxy_host || '127.0.0.1');
  setVal('l-proxy-port', l.proxy_port || '1080');
  setVal('l-fmt', l.output_format || 'mkv');
  setVal('l-frag', String(l.concurrent_fragments || 8));
  setVal('l-cookies', l.cookies || '无');
  setVal('l-cookies-path', l.cookies_path || '');
  setVal('l-save-dir', l.save_dir || '');
  setChecked('l-keep-temp', l.keep_temp_files || false);

  toggleProxy();
  toggleCookies();
}

function collectConfig() {
  return {
    local: {
      last_url: getVal('l-url'),
      proxy_mode: getVal('l-proxy-mode'),
      proxy_host: getVal('l-proxy-host'),
      proxy_port: getVal('l-proxy-port'),
      output_format: getVal('l-fmt'),
      concurrent_fragments: parseInt(getVal('l-frag')) || 8,
      cookies: getVal('l-cookies'),
      cookies_path: getVal('l-cookies-path'),
      save_dir: getVal('l-save-dir'),
      keep_temp_files: getChecked('l-keep-temp'),
    },
  };
}

// ---- Proxy Toggle ----
function toggleProxy() {
  const mode = getVal('l-proxy-mode');
  const disabled = mode === '无';
  setDisabled('l-proxy-host', disabled);
  setDisabled('l-proxy-port', disabled);
}

// ---- Cookies Toggle ----
function toggleCookies() {
  const choice = getVal('l-cookies');
  const isFile = choice === '文件';
  const isBrowser = ['Chrome', 'Edge', 'Brave', 'Opera'].includes(choice);
  const isFirefox = choice === 'Firefox';

  const fileGroup = document.getElementById('l-cookies-file-group');
  const hintGroup = document.getElementById('l-cookies-hint-group');
  const hint = document.getElementById('l-cookies-hint');

  if (fileGroup) fileGroup.style.display = isFile ? '' : 'none';
  if (hintGroup) hintGroup.style.display = (isBrowser || isFile || isFirefox) ? '' : 'none';

  if (isFirefox) {
    hint.textContent = '✅ Firefox 支持运行时直接读取 Cookie，无需关闭浏览器。推荐使用此模式。';
    hint.style.color = 'var(--success)';
  } else if (isBrowser) {
    hint.textContent = `⚠ ${choice} 会锁定 Cookie 数据库，使用前请先关闭浏览器。推荐改用 Firefox 模式（无需关闭）。`;
    hint.style.color = 'var(--warning)';
  } else if (isFile) {
    hint.textContent = '💡 请点击「📂 浏览」选择从浏览器扩展导出的 cookies.txt';
    hint.style.color = 'var(--text-muted)';
  } else {
    hint.textContent = '';
  }
}

// ---- Local Search ----
async function searchLocal() {
  const url = getVal('l-url').trim();
  if (!url) { alert('请输入 YouTube 链接'); return; }

  const btn = document.getElementById('l-search-btn');
  btn.textContent = '⏳ 搜索中...';
  btn.disabled = true;

  try {
    const proxy = buildProxy();
    const cookies = buildCookies();
    const info = await API.fetchInfo(url, proxy, cookies);
    state.local.info = info;
    renderLocalInfo(info);
    renderLocalFormats(info.formats || []);
    renderLocalAudio(info.audio_tracks || []);
    saveConfig();
  } catch (e) {
    alert('搜索失败: ' + e.message);
  } finally {
    btn.textContent = '🔍 搜索';
    btn.disabled = false;
  }
}

function renderLocalInfo(info) {
  showCard('l-info-card');
  showCard('l-format-card');
  document.getElementById('l-title').textContent = '📌 ' + info.title;
  document.getElementById('l-meta').textContent =
    `👤 ${info.uploader}  ⏱ ${info.duration_str}  👁 ${fmtCount(info.view_count)}  ❤ ${fmtCount(info.like_count)}`;
  document.getElementById('l-desc').textContent = info.description || '';
  if (info.thumbnail_url) {
    const thumb = document.getElementById('l-thumbnail');
    thumb.src = info.thumbnail_url;
    thumb.style.display = '';
  }
}

function renderLocalFormats(formats) {
  const tbody = document.getElementById('l-format-body');
  tbody.innerHTML = '';
  state.local.selectedFormat = null;

  if (!formats || formats.length === 0) {
    tbody.innerHTML = '<tr><td colspan="5" class="empty-row">⚠ 该视频暂无可用下载格式（可能需要登录 Cookie 或视频本身受限）</td></tr>';
    return;
  }

  formats.forEach((f, i) => {
    const tr = document.createElement('tr');
    tr.innerHTML = `
      <td>${f.note}</td>
      <td>${f.codec}</td>
      <td>${f.audio_codec}</td>
      <td>${f.file_size_mb > 0 ? f.file_size_mb.toFixed(1) + ' MB' : '未知'}</td>
      <td>${f.ext}</td>
    `;
    tr.addEventListener('click', () => {
      tbody.querySelectorAll('tr').forEach(r => r.classList.remove('selected'));
      tr.classList.add('selected');
      state.local.selectedFormat = f;
    });
    tbody.appendChild(tr);

    // 默认选择第一个（最高分辨率）
    if (i === 0) {
      tr.classList.add('selected');
      state.local.selectedFormat = f;
    }
  });

  showCard('l-format-card');
}

function renderLocalAudio(tracks) {
  const tbody = document.getElementById('l-audio-body');
  tbody.innerHTML = '';
  state.local.selectedAudioIds = [];

  if (!tracks || tracks.length === 0) {
    document.getElementById('l-audio-card').style.display = 'none';
    return;
  }
  showCard('l-audio-card');

  tracks.forEach(t => {
    const tr = document.createElement('tr');
    tr.innerHTML = `
      <td><input type="checkbox" data-format-id="${t.format_id}"></td>
      <td>${t.language}</td>
      <td>${t.codec}</td>
      <td>${t.abr} kbps</td>
      <td>${t.note || ''}</td>
    `;
    const cb = tr.querySelector('input[type="checkbox"]');
    cb.checked = true; // 默认全选
    state.local.selectedAudioIds.push(t.format_id);

    cb.addEventListener('change', () => {
      if (cb.checked) {
        state.local.selectedAudioIds.push(t.format_id);
      } else {
        state.local.selectedAudioIds = state.local.selectedAudioIds.filter(id => id !== t.format_id);
      }
    });

    tr.addEventListener('click', (e) => {
      if (e.target.tagName !== 'INPUT') {
        cb.checked = !cb.checked;
        cb.dispatchEvent(new Event('change'));
      }
    });

    tbody.appendChild(tr);
  });
}

// ---- Local Download ----
async function startDownload() {
  if (!state.local.info || !state.local.selectedFormat) {
    alert('请先搜索视频并选择分辨率');
    return;
  }

  const saveDir = getVal('l-save-dir').trim();
  if (!saveDir) {
    alert('请选择保存目录');
    return;
  }

  let formatId = state.local.selectedFormat.video_id;
  if (state.local.selectedAudioIds.length > 0) {
    formatId = formatId + '+' + state.local.selectedAudioIds.join('+');
  }

  const opts = {
    url: state.local.info.url,
    format_id: formatId,
    output_ext: getVal('l-fmt'),
    save_dir: saveDir,
    concurrent_fragments: parseInt(getVal('l-frag')) || 8,
    resume: true,
    proxy: buildProxy(),
    cookies: buildCookies(),
    keep_temp_files: getChecked('l-keep-temp'),
    title: state.local.info.title,
  };

  try {
    const result = await API.startDownload(opts);
    // 仅在 WebSocket 尚未创建此任务时才创建（避免竞态覆盖）
    if (!state.tasks[result.task_id]) {
      state.tasks[result.task_id] = {
        id: result.task_id,
        title: opts.title || opts.url,
        url: opts.url,
        status: 'queued',
        percent: 0, speed_mbps: 0, total_mb: 0, downloaded_mb: 0,
        eta_seconds: 0, elapsed_seconds: 0,
        fragment_index: 0, fragment_count: 0,
        error_message: '', merge_elapsed: 0, merge_remaining: 0,
        avg_speed_mbps: 0, final_size_mb: 0, download_elapsed: 0, merge_elapsed_final: 0,
        save_dir: opts.save_dir,
      };
    }
    if (!state.taskOrder.includes(result.task_id)) {
      state.taskOrder.push(result.task_id);
    }
    persistTasks();
    renderQueue();
  } catch (e) {
    alert('启动下载失败: ' + e.message);
  }
}

function cancelTask(taskID) {
  const task = state.tasks[taskID];
  if (task) {
    task.status = 'cancelled';
    persistTasks();
    renderQueue();
  }
  API.cancelTask(taskID).catch(() => {});
}

// 删除单个已完成任务
async function deleteTask(taskID) {
  state.selectedTasks.delete(taskID);
  delete state.tasks[taskID];
  state.taskOrder = state.taskOrder.filter(id => id !== taskID);
  persistTasks();
  renderQueue();
  API.deleteTask(taskID).catch(() => {});
}

// legacy cancel (cancels first active task)
function cancelDownload() {
  for (const id of state.taskOrder) {
    const t = state.tasks[id];
    if (t && (t.status === 'queued' || t.status === 'downloading' || t.status === 'merging' || t.status === 'starting')) {
      cancelTask(id);
      return;
    }
  }
}

// ---- Queue Rendering ----
function renderQueue() {
  const empty = document.getElementById('l-queue-empty');
  const list = document.getElementById('l-queue-list');
  const toolbar = document.getElementById('l-batch-toolbar');

  const taskList = state.taskOrder.map(id => state.tasks[id]).filter(Boolean);
  if (taskList.length === 0) {
    if (empty) empty.style.display = '';
    if (list) list.innerHTML = '';
    if (toolbar) toolbar.style.display = 'none';
    return;
  }

  if (empty) empty.style.display = 'none';
  if (list) list.innerHTML = taskList.map(t => renderTaskItem(t)).join('');

  // 批量操作工具栏
  renderBatchToolbar(taskList);
}

function renderBatchToolbar(taskList) {
  const toolbar = document.getElementById('l-batch-toolbar');
  if (!toolbar) return;

  const doneTasks = taskList.filter(t => ['finished', 'error', 'cancelled'].includes(t.status));
  const selectedCount = state.selectedTasks.size;

  if (doneTasks.length > 0) {
    toolbar.style.display = 'flex';
    document.getElementById('l-batch-count').textContent =
      selectedCount > 0 ? `已选 ${selectedCount} 项` : `共 ${doneTasks.length} 个已完成`;
  } else {
    toolbar.style.display = 'none';
  }
}

function toggleTaskSelect(taskID, checked) {
  if (checked) {
    state.selectedTasks.add(taskID);
  } else {
    state.selectedTasks.delete(taskID);
  }
  renderBatchToolbar(state.taskOrder.map(id => state.tasks[id]).filter(Boolean));
}

function toggleSelectAll(checked) {
  state.selectedTasks.clear();
  if (checked) {
    state.taskOrder.forEach(id => {
      const t = state.tasks[id];
      if (t && ['finished', 'error', 'cancelled'].includes(t.status)) {
        state.selectedTasks.add(id);
      }
    });
  }
  // 更新所有 checkbox
  document.querySelectorAll('.task-checkbox').forEach(cb => {
    cb.checked = checked;
  });
  renderBatchToolbar(state.taskOrder.map(id => state.tasks[id]).filter(Boolean));
}

async function batchDeleteTasks() {
  if (state.selectedTasks.size === 0) {
    alert('请先勾选要删除的任务');
    return;
  }
  if (!confirm(`确定要删除已选的 ${state.selectedTasks.size} 个任务记录吗？`)) return;

  const ids = Array.from(state.selectedTasks);
  for (const id of ids) {
    delete state.tasks[id];
    state.taskOrder = state.taskOrder.filter(tid => tid !== id);
  }
  state.selectedTasks.clear();
  persistTasks();
  renderQueue();
  API.batchDeleteTasks(ids).catch(() => {});
}

async function openTaskFolder(taskID) {
  const task = state.tasks[taskID];
  if (!task || !task.save_dir) {
    alert('无法获取保存目录');
    return;
  }
  try {
    await API.openDir(task.save_dir);
  } catch (e) {
    alert('打开目录失败: ' + e.message);
  }
}

function renderTaskItem(t) {
  const statusLabels = {
    queued: '排队中', starting: '启动中', downloading: '下载中',
    merging: '合并中', finished: '完成', error: '失败', cancelled: '已取消',
  };
  const statusLabel = statusLabels[t.status] || t.status;
  const pct = t.percent.toFixed(1);
  const isActive = ['downloading', 'merging', 'queued', 'starting'].includes(t.status);
  const isDone = ['finished', 'error', 'cancelled'].includes(t.status);
  const isFinished = t.status === 'finished';
  const barClass = (t.status === 'merging' && t.percent >= 100) ? 'indeterminate' : '';

  // 完成时显示统计信息（平均速度/实际大小/下载耗时/合并耗时），进行中显示实时数据
  let speed, size, elapsed, eta, etaLabel;
  if (isFinished) {
    speed = t.avg_speed_mbps > 0 ? `均速 ${t.avg_speed_mbps.toFixed(2)} MB/s` : '--';
    size = t.final_size_mb > 0 ? `${t.final_size_mb.toFixed(1)} MB`
      : (t.total_mb > 0 ? `${t.total_mb.toFixed(1)} MB` : '--');
    elapsed = t.download_elapsed > 0 ? fmtTime(t.download_elapsed) : '--';
    etaLabel = '合并';
    eta = t.merge_elapsed_final > 0 ? fmtTime(Math.round(t.merge_elapsed_final)) : '--';
  } else {
    speed = t.speed_mbps > 0 ? t.speed_mbps.toFixed(1) + ' MB/s' : '--';
    size = t.total_mb > 0
      ? `${t.downloaded_mb.toFixed(1)} / ${t.total_mb.toFixed(1)} MB`
      : (t.downloaded_mb > 0 ? t.downloaded_mb.toFixed(1) + ' MB' : '--');
    eta = t.eta_seconds > 0 ? fmtTime(t.eta_seconds) : '--';
    elapsed = t.elapsed_seconds > 0 ? fmtTime(t.elapsed_seconds) : '--';
    etaLabel = '剩余';
  }

  let fragInfo = '';
  if (!isFinished && t.fragment_count > 0) {
    fragInfo = `<span class="task-meta-item">分片 <span class="task-meta-val">${t.fragment_index || 0}/${t.fragment_count}</span></span>`;
  } else if (t.status === 'merging') {
    const rem = t.merge_remaining || 0;
    fragInfo = `<span class="task-meta-item">合并剩余 <span class="task-meta-val">${fmtTime(rem)}</span></span>`;
  }

  let errorBlock = '';
  if (t.error_message) {
    errorBlock = `<div class="task-error-msg">${escapeHtml(t.error_message)}</div>`;
  }

  // 多选复选框（仅已完成任务显示）
  const checkbox = isDone
    ? `<input type="checkbox" class="task-checkbox" ${state.selectedTasks.has(t.id) ? 'checked' : ''}
        onclick="event.stopPropagation(); toggleTaskSelect('${t.id}', this.checked);" title="选择此任务">`
    : '';

  // 操作按钮
  let actionBtns = '';
  if (isActive) {
    actionBtns = `<button class="btn btn-danger btn-sm" onclick="cancelTask('${t.id}')">取消</button>`;
  } else if (isDone) {
    // 完成/失败/取消的任务：打开目录 + 删除按钮
    const folderBtn = t.save_dir
      ? `<button class="btn btn-ghost btn-sm" onclick="openTaskFolder('${t.id}')" title="打开下载目录">📂</button>`
      : '';
    const deleteBtn = `<button class="btn btn-ghost btn-sm btn-delete-task" onclick="deleteTask('${t.id}')" title="删除记录">🗑</button>`;
    actionBtns = folderBtn + deleteBtn;
  }

  return `
    <div class="task-card" id="task-${t.id}">
      <div class="task-top">
        ${checkbox}
        <span class="task-name" title="${escapeHtml(t.title)}">${escapeHtml(t.title)}</span>
        <div class="task-actions">
          <span class="task-badge ${t.status}">${statusLabel}</span>
          ${actionBtns}
        </div>
      </div>
      <div class="task-bar-wrap">
        <div class="task-bar-fill ${barClass}" style="width:${t.percent}%"></div>
      </div>
      <div class="task-meta">
        <span class="task-meta-item">速度 <span class="task-meta-val">${speed}</span></span>
        <span class="task-meta-item">大小 <span class="task-meta-val">${size}</span></span>
        <span class="task-meta-item">耗时 <span class="task-meta-val">${elapsed}</span></span>
        <span class="task-meta-item">${etaLabel} <span class="task-meta-val">${eta}</span></span>
        ${fragInfo}
      </div>
      ${errorBlock}
    </div>`;
}

function escapeHtml(str) {
  const div = document.createElement('div');
  div.textContent = str || '';
  return div.innerHTML;
}

// ---- localStorage 持久化 ----
function persistTasks() {
  try {
    // 只持久化已完成/失败/取消的任务（活跃任务在页面刷新后由 WebSocket 恢复）
    const toPersist = {};
    for (const id of state.taskOrder) {
      const t = state.tasks[id];
      if (t && ['finished', 'error', 'cancelled'].includes(t.status)) {
        toPersist[id] = {
          id: t.id,
          title: t.title,
          url: t.url,
          status: t.status,
          percent: t.percent,
          speed_mbps: t.speed_mbps,
          total_mb: t.total_mb,
          downloaded_mb: t.downloaded_mb,
          eta_seconds: t.eta_seconds,
          elapsed_seconds: t.elapsed_seconds,
          fragment_index: t.fragment_index,
          fragment_count: t.fragment_count,
          error_message: t.error_message,
          merge_elapsed: t.merge_elapsed,
          merge_remaining: t.merge_remaining,
          avg_speed_mbps: t.avg_speed_mbps,
          final_size_mb: t.final_size_mb,
          download_elapsed: t.download_elapsed,
          merge_elapsed_final: t.merge_elapsed_final,
          save_dir: t.save_dir,
        };
      }
    }
    const data = {
      tasks: toPersist,
      order: state.taskOrder.filter(id => {
        const t = state.tasks[id];
        return t && ['finished', 'error', 'cancelled'].includes(t.status);
      }),
    };
    localStorage.setItem(TASKS_STORAGE_KEY, JSON.stringify(data));
  } catch (e) {
    console.warn('持久化任务列表失败:', e);
  }
}

function restoreTasks() {
  try {
    const raw = localStorage.getItem(TASKS_STORAGE_KEY);
    if (!raw) return;
    const data = JSON.parse(raw);
    if (data.tasks) {
      for (const [id, t] of Object.entries(data.tasks)) {
        if (!state.tasks[id]) {
          state.tasks[id] = t;
        }
      }
    }
    if (data.order) {
      for (const id of data.order) {
        if (!state.taskOrder.includes(id) && state.tasks[id]) {
          state.taskOrder.push(id);
        }
      }
    }
    if (state.taskOrder.length > 0) {
      renderQueue();
    }
  } catch (e) {
    console.warn('恢复任务列表失败:', e);
  }
}

// ---- WebSocket Progress Handler ----
function onProgress(data) {
  const taskID = data.task_id;
  if (!taskID) return;

  let task = state.tasks[taskID];
  if (!task) {
    // 新任务（来自其他客户端或 WS 先于 HTTP 到达）
    task = {
      id: taskID,
      title: data.title || data.url || '',
      url: data.url || '',
      status: data.status,
      percent: 0, speed_mbps: 0, total_mb: 0, downloaded_mb: 0,
      eta_seconds: 0, elapsed_seconds: 0,
      fragment_index: 0, fragment_count: 0,
      error_message: '', merge_elapsed: 0, merge_remaining: 0,
      avg_speed_mbps: 0, final_size_mb: 0, download_elapsed: 0, merge_elapsed_final: 0,
      save_dir: data.save_dir || '',
    };
    state.tasks[taskID] = task;
    if (!state.taskOrder.includes(taskID)) {
      state.taskOrder.push(taskID);
    }
  }

  // 只更新有值的字段（避免 undefined 覆盖已有数据）
  if (data.status) task.status = data.status;
  if (data.percent !== undefined) task.percent = data.percent;
  if (data.speed_mbps !== undefined) task.speed_mbps = data.speed_mbps;
  if (data.total_mb !== undefined) task.total_mb = data.total_mb;
  if (data.downloaded_mb !== undefined) task.downloaded_mb = data.downloaded_mb;
  if (data.eta_seconds !== undefined) task.eta_seconds = data.eta_seconds;
  if (data.elapsed_seconds !== undefined) task.elapsed_seconds = data.elapsed_seconds;
  if (data.fragment_index !== undefined) task.fragment_index = data.fragment_index;
  if (data.fragment_count !== undefined) task.fragment_count = data.fragment_count;
  if (data.error_message) task.error_message = data.error_message;
  if (data.merge_elapsed !== undefined) task.merge_elapsed = data.merge_elapsed;
  if (data.merge_remaining !== undefined) task.merge_remaining = data.merge_remaining;
  if (data.title) task.title = data.title;
  if (data.save_dir) task.save_dir = data.save_dir;
  // 完成时的统计信息
  if (data.avg_speed_mbps !== undefined) task.avg_speed_mbps = data.avg_speed_mbps;
  if (data.final_size_mb !== undefined) task.final_size_mb = data.final_size_mb;
  if (data.download_elapsed !== undefined) task.download_elapsed = data.download_elapsed;
  if (data.merge_elapsed_final !== undefined) task.merge_elapsed_final = data.merge_elapsed_final;

  // 任务变为终态时持久化
  if (task.status === 'finished' || task.status === 'error' || task.status === 'cancelled') {
    persistTasks();
  }

  renderQueue();
}

// ---- Folder Browser ----
async function browseFolder(targetId) {
  state.folderTarget = targetId;

  // 优先使用原生系统文件夹选择对话框
  try {
    const result = await API.pickFolder();
    if (result && result.path) {
      document.getElementById(targetId).value = result.path;
      saveConfig();
      return;
    }
    if (result && result.cancelled) {
      // 用户主动取消，静默返回，不弹出回退对话框
      return;
    }
  } catch (e) {
    // API 不可用时回退到网页式浏览器
  }

  // 回退：网页式目录浏览器（仅在原生对话框不可用时触发）
  const modal = document.getElementById('folder-modal');
  modal.classList.add('show');

  const drives = await API.getDrives();
  renderFolderList(drives, '选择驱动器');
}

let currentBrowsePath = '';

async function navigateTo(path) {
  currentBrowsePath = path;
  const data = await API.browseDir(path);
  renderFolderList(data.entries, data.current);
  document.getElementById('current-path').textContent = data.current;
}

function renderFolderList(entries, currentPath) {
  currentBrowsePath = currentPath;
  document.getElementById('current-path').textContent = currentPath || '';

  const list = document.getElementById('folder-list');
  list.innerHTML = '';

  entries.forEach(e => {
    if (!e.is_dir) return;
    const item = document.createElement('div');
    item.className = 'folder-item';
    item.innerHTML = `<span class="folder-icon">📁</span><span>${e.name}</span>`;
    item.addEventListener('click', () => navigateTo(e.path));
    item.addEventListener('dblclick', () => navigateTo(e.path));
    list.appendChild(item);
  });
}

function confirmFolder() {
  if (state.folderTarget && currentBrowsePath) {
    document.getElementById(state.folderTarget).value = currentBrowsePath;
    saveConfig();
  }
  closeModal();
}

function closeModal() {
  document.getElementById('folder-modal').classList.remove('show');
  state.folderTarget = null;
}

// ---- File Browser ----
function browseFile() {
  state.fileTarget = 'l-cookies-path';
  const modal = document.getElementById('file-modal');
  modal.classList.add('show');

  const list = document.getElementById('file-list');
  list.innerHTML = '';

  // 简单文件路径输入（浏览器安全限制，无法直接浏览文件系统）
  const input = document.createElement('input');
  input.type = 'file';
  input.accept = '.txt';
  input.style.cssText = 'width:100%;padding:40px;text-align:center;cursor:pointer;';
  input.addEventListener('change', (e) => {
    if (e.target.files.length > 0) {
      document.getElementById('l-cookies-path').value = e.target.files[0].name;
      saveConfig();
      closeFileModal();
    }
  });
  list.appendChild(input);
}

function closeFileModal() {
  document.getElementById('file-modal').classList.remove('show');
}

// Close modal on overlay click
document.addEventListener('click', (e) => {
  if (e.target.classList.contains('modal-overlay')) {
    closeModal();
    closeFileModal();
  }
});

// ---- Helpers ----
function buildProxy() {
  const mode = getVal('l-proxy-mode');
  if (mode === '无') return '';
  const host = getVal('l-proxy-host');
  const port = getVal('l-proxy-port');
  if (!host || !port) return '';
  const scheme = mode === 'SOCKS5' ? 'socks5' : 'http';
  return scheme + '://' + host + ':' + port;
}

function buildCookies() {
  const choice = getVal('l-cookies');
  if (choice === '无') return '';
  if (choice === '文件') return getVal('l-cookies-path');
  return choice.toLowerCase();
}

// DOM helpers
function getVal(id) { return document.getElementById(id)?.value || ''; }
function setVal(id, val) { const el = document.getElementById(id); if (el) el.value = val; }
function getChecked(id) { return document.getElementById(id)?.checked || false; }
function setChecked(id, val) { const el = document.getElementById(id); if (el) el.checked = val; }
function setDisabled(id, val) { const el = document.getElementById(id); if (el) el.disabled = val; }
function showCard(id) { const el = document.getElementById(id); if (el) el.style.display = ''; }

function fmtCount(n) {
  if (!n) return '--';
  if (n >= 10000) return (n / 10000).toFixed(1) + '万';
  return String(n);
}

function fmtTime(seconds) {
  if (!seconds || seconds <= 0) return '--:--';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  if (h > 0) return `${pad(h)}:${pad(m)}:${pad(s)}`;
  return `${pad(m)}:${pad(s)}`;
}

function pad(n) { return String(n).padStart(2, '0'); }
