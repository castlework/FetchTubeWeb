// API 调用封装

const API = {
  base: '',

  async get(path, params = {}) {
    const qs = new URLSearchParams(params).toString();
    const url = `${this.base}${path}${qs ? '?' + qs : ''}`;
    const resp = await fetch(url);
    if (!resp.ok) {
      const data = await resp.json().catch(() => ({}));
      throw new Error(data.error || `HTTP ${resp.status}`);
    }
    return resp.json();
  },

  async post(path, body = {}) {
    const resp = await fetch(`${this.base}${path}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!resp.ok) {
      const data = await resp.json().catch(() => ({}));
      throw new Error(data.error || `HTTP ${resp.status}`);
    }
    return resp.json();
  },

  async put(path, body = {}) {
    const resp = await fetch(`${this.base}${path}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!resp.ok) {
      const data = await resp.json().catch(() => ({}));
      throw new Error(data.error || `HTTP ${resp.status}`);
    }
    return resp.json();
  },

  async del(path, body = {}) {
    const resp = await fetch(`${this.base}${path}`, {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!resp.ok) {
      const data = await resp.json().catch(() => ({}));
      throw new Error(data.error || `HTTP ${resp.status}`);
    }
    return resp.json();
  },

  // Video info
  fetchInfo(url, proxy, cookies) {
    return this.get('/api/info', { url, proxy, cookies });
  },

  // Download
  startDownload(opts) {
    return this.post('/api/download', opts);
  },

  cancelTask(taskID) {
    return this.post(`/api/tasks/${encodeURIComponent(taskID)}/cancel`);
  },

  deleteTask(taskID) {
    return this.del(`/api/tasks/${encodeURIComponent(taskID)}`);
  },

  batchDeleteTasks(taskIDs) {
    return this.post('/api/tasks/batch-delete', { task_ids: taskIDs });
  },

  openDir(dirPath) {
    return this.post('/api/open-dir', { path: dirPath });
  },

  pickFolder() {
    return this.post('/api/pick-folder');
  },

  listTasks() {
    return this.get('/api/tasks');
  },

  // legacy
  cancelDownload() {
    return this.post('/api/cancel');
  },

  // Config
  getConfig() {
    return this.get('/api/config');
  },

  saveConfig(cfg) {
    return this.put('/api/config', cfg);
  },

  // Health
  health() {
    return this.get('/api/health');
  },

  // Browse
  browseDir(path) {
    return this.get('/api/browse', { path });
  },

  getDrives() {
    return this.get('/api/drives');
  },

  // Relay
  relayTest(host, port) {
    return this.post('/api/relay/test', { host, port });
  },

  relaySubmit(host, port, url, formatId, outputExt) {
    return this.post('/api/relay/submit', { host, port, url, format_id: formatId, output_ext: outputExt });
  },

  relayTasks(host, port) {
    return this.get('/api/relay/tasks', { host, port });
  },

  relayFiles(host, port) {
    return this.get('/api/relay/files', { host, port });
  },

  relayDownload(host, port, filename, saveDir) {
    return this.post('/api/relay/download', { host, port, filename, save_dir: saveDir });
  },

  relayDeleteFile(host, port, filename) {
    return this.del('/api/relay/file', { host, port, filename });
  },
};
