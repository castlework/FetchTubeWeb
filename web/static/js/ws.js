// WebSocket progress push

const WS = {
  conn: null,
  reconnectTimer: null,
  onMessage: null,  // callback function

  connect() {
    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${protocol}//${location.host}/ws/progress`;

    try {
      this.conn = new WebSocket(url);

      this.conn.onopen = () => {
        console.log('[ws] connected');
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
          console.error('[ws] parse failed:', e);
        }
      };

      this.conn.onclose = () => {
        console.log('[ws] disconnected');
        this.scheduleReconnect();
      };

      this.conn.onerror = (err) => {
        console.error('[ws] error:', err);
      };
    } catch (e) {
      console.error('[ws] connection failed:', e);
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
