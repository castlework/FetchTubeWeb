// WebSocket 进度推送

const WS = {
  conn: null,
  reconnectTimer: null,
  onMessage: null,  // 回调函数

  connect() {
    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${protocol}//${location.host}/ws/progress`;

    try {
      this.conn = new WebSocket(url);

      this.conn.onopen = () => {
        console.log('[ws] 已连接');
        if (this.reconnectTimer) {
          clearTimeout(this.reconnectTimer);
          this.reconnectTimer = null;
        }
      };

      this.conn.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          if (this.onMessage) {
            this.onMessage(data);
          }
        } catch (e) {
          console.error('[ws] 解析失败:', e);
        }
      };

      this.conn.onclose = () => {
        console.log('[ws] 已断开');
        this.scheduleReconnect();
      };

      this.conn.onerror = (err) => {
        console.error('[ws] 错误:', err);
      };
    } catch (e) {
      console.error('[ws] 连接失败:', e);
      this.scheduleReconnect();
    }
  },

  scheduleReconnect() {
    if (this.reconnectTimer) return;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, 3000);
  },

  close() {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.conn) {
      this.conn.close();
      this.conn = null;
    }
  },
};
