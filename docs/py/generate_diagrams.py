"""
FetchTubeWeb — Documentation Diagram Generator
================================================
Generates all diagrams for the tutorial document as SVG files.
Pure Python, no external dependencies needed.
Output: docs/pic/*.svg
"""

import os

# ============================================================
# SVG helper library
# ============================================================

class SVG:
    """Minimal SVG drawing helper."""
    def __init__(self, width, height, filename):
        self.w = width
        self.h = height
        self.filename = filename
        self.elements = []
        self.defs = ""

    def add_def(self, d):
        self.defs += d + "\n"

    def rect(self, x, y, w, h, rx=4, fill="#fff", stroke="#333", sw=1.5, dash=None, cls=""):
        d = f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{rx}" fill="{fill}" stroke="{stroke}" stroke-width="{sw}"'
        if dash: d += f' stroke-dasharray="{dash}"'
        d += f' class="{cls}"/>'
        self.elements.append(d)

    def text(self, x, y, txt, size=14, fill="#333", anchor="start", bold=False, family="sans-serif"):
        fw = "bold" if bold else "normal"
        lines = txt.split("\n")
        result = ""
        for i, line in enumerate(lines):
            result += f'<text x="{x}" y="{y + i * (size + 4)}" font-family="{family}" font-size="{size}" fill="{fill}" text-anchor="{anchor}" font-weight="{fw}">{self._esc(line)}</text>\n'
        self.elements.append(result.strip())

    def multiline_text(self, x, y, txt, size=13, fill="#333", anchor="start"):
        """Draw multi-line text with explicit \n handling."""
        lines = txt.split("\n")
        for i, line in enumerate(lines):
            self.elements.append(
                f'<text x="{x}" y="{y + i * (size + 5)}" font-family="sans-serif" font-size="{size}" fill="{fill}" text-anchor="{anchor}">{self._esc(line)}</text>'
            )

    def line(self, x1, y1, x2, y2, stroke="#666", sw=1.5, dash=None, marker_end=None):
        d = f'<line x1="{x1}" y1="{y1}" x2="{x2}" y2="{y2}" stroke="{stroke}" stroke-width="{sw}"'
        if dash: d += f' stroke-dasharray="{dash}"'
        if marker_end: d += f' marker-end="{marker_end}"'
        d += '/>'
        self.elements.append(d)

    def arrow(self, x1, y1, x2, y2, stroke="#555", sw=1.5, dash=None):
        """Draw an arrow line (head built-in via polygon)."""
        self.line(x1, y1, x2, y2, stroke, sw, dash)
        # arrowhead
        angle = 0.5
        length = 10
        import math
        dx = x2 - x1
        dy = y2 - y1
        L = math.sqrt(dx*dx + dy*dy)
        if L < 1: return
        ux, uy = dx/L, dy/L
        # back points
        bx1 = x2 - length*ux*math.cos(angle) + length*uy*math.sin(angle)
        by1 = y2 - length*uy*math.cos(angle) - length*ux*math.sin(angle)
        bx2 = x2 - length*ux*math.cos(angle) - length*uy*math.sin(angle)
        by2 = y2 - length*uy*math.cos(angle) + length*ux*math.sin(angle)
        self.elements.append(f'<polygon points="{x2},{y2} {bx1:.1f},{by1:.1f} {bx2:.1f},{by2:.1f}" fill="{stroke}"/>')

    def polyline(self, points, stroke="#555", sw=1.5, fill="none", dash=None):
        pts = " ".join(f"{x},{y}" for x, y in points)
        d = f'<polyline points="{pts}" fill="{fill}" stroke="{stroke}" stroke-width="{sw}"'
        if dash: d += f' stroke-dasharray="{dash}"'
        d += '/>'
        self.elements.append(d)

    def circle(self, cx, cy, r, fill="#fff", stroke="#333", sw=1.5):
        self.elements.append(f'<circle cx="{cx}" cy="{cy}" r="{r}" fill="{fill}" stroke="{stroke}" stroke-width="{sw}"/>')

    def group(self, elements, transform=""):
        g = f'<g transform="{transform}">\n'
        g += "\n".join(elements) + "\n</g>"
        self.elements.append(g)

    def _esc(self, s):
        return s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")

    def save(self):
        svg = f'''<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="{self.w}" height="{self.h}" viewBox="0 0 {self.w} {self.h}">
<style>
  .box {{ filter: drop-shadow(0 2px 3px rgba(0,0,0,0.1)); }}
  .title {{ font-weight: bold; }}
</style>
<defs>
{self.defs}
</defs>
<rect width="{self.w}" height="{self.h}" fill="#fafafa"/>
{chr(10).join(self.elements)}
</svg>'''
        os.makedirs(os.path.dirname(self.filename), exist_ok=True)
        with open(self.filename, 'w', encoding='utf-8') as f:
            f.write(svg)
        print(f"  [OK] Generated: {self.filename} ({self.w}x{self.h})")


# ============================================================
# Diagram 1: System Architecture
# ============================================================

def diagram_architecture():
    s = SVG(900, 620, os.path.join(OUT_DIR, "architecture.svg"))

    COLORS = {
        "browser": ("#e3f2fd", "#1565c0"),    # blue
        "golang": ("#e8f5e9", "#2e7d32"),     # green
        "external": ("#fff3e0", "#e65100"),    # orange
        "storage": ("#f3e5f5", "#7b1fa2"),     # purple
        "arrow": "#455a64",
    }

    def box(x, y, w, h, label, color_key):
        fill, stroke = COLORS[color_key]
        s.rect(x, y, w, h, fill=fill, stroke=stroke, sw=2, rx=6)
        s.text(x + w/2, y + 18, label, size=14, fill=stroke, anchor="middle", bold=True)
        return x, y, w, h

    def sub_box(x, y, w, h, label, size=11):
        s.rect(x, y, w, h, fill="#fff", stroke="#ccc", sw=1, rx=3)
        s.text(x + w/2, y + h/2 + 4, label, size=size, fill="#555", anchor="middle")

    # Browser
    box(30, 30, 200, 120, "🌐 Browser (Web UI)", "browser")
    sub_box(50, 65, 160, 24, "HTML + CSS (static/index.html)")
    sub_box(50, 95, 80, 24, "app.js (logic)")
    sub_box(140, 95, 70, 24, "ws.js (WS)")

    # Go Server
    box(290, 30, 320, 540, "⚙️ Go HTTP Server", "golang")

    # Handler
    sub_box(310, 65, 280, 50, "")
    s.text(330, 82, "HTTP Handlers", size=12, fill="#2e7d32", bold=True)
    s.text(330, 100, "info / download / tasks / browse / config / thumbnail", size=9, fill="#666")

    # WS Hub
    sub_box(310, 125, 135, 50, "")
    s.text(330, 142, "WebSocket Hub", size=12, fill="#2e7d32", bold=True)
    s.text(330, 160, "broadcast progress", size=9, fill="#666")

    # Task Manager
    sub_box(455, 125, 135, 50, "")
    s.text(475, 142, "Task Manager", size=12, fill="#2e7d32", bold=True)
    s.text(475, 160, "queue / cancel", size=9, fill="#666")

    # Config
    sub_box(310, 185, 280, 35, "Config Manager  (load / save / merge JSON)", 10)

    # yt-dlp wrapper
    sub_box(310, 230, 280, 50, "")
    s.text(330, 247, "yt-dlp Wrapper", size=12, fill="#2e7d32", bold=True)
    s.text(330, 265, "FetchInfo / BuildDownloadArgs / KillProcessTree", size=9, fill="#666")

    # Download Manager
    sub_box(310, 290, 280, 50, "")
    s.text(330, 307, "Download Manager", size=12, fill="#2e7d32", bold=True)
    s.text(330, 325, "run download / parse progress / merge timeout detect", size=9, fill="#666")

    # Progress parser
    sub_box(310, 350, 280, 50, "")
    s.text(330, 367, "Progress Parser", size=12, fill="#2e7d32", bold=True)
    s.text(330, 385, "parseProgressLine / scanProgressLines / printConsole", size=9, fill="#666")

    # Models
    sub_box(310, 410, 135, 55, "")
    s.text(325, 428, "Data Models", size=12, fill="#2e7d32", bold=True)
    s.text(325, 445, "VideoInfo / FormatOption", size=8, fill="#666")
    s.text(325, 457, "ProgressData / AudioTrack", size=8, fill="#666")

    # Embedded web
    sub_box(455, 410, 135, 55, "")
    s.text(475, 428, "Embedded Web", size=12, fill="#2e7d32", bold=True)
    s.text(475, 445, "//go:embed static/*", size=8, fill="#666")
    s.text(475, 457, "embed.FS", size=8, fill="#666")

    # External tools
    box(670, 30, 200, 140, "🔧 External Tools", "external")
    sub_box(690, 65, 160, 24, "yt-dlp.exe (video download)")
    sub_box(690, 95, 160, 24, "ffmpeg.exe (audio/video merge)")
    sub_box(690, 125, 160, 24, "node.exe (JS runtime)")

    # YouTube API
    box(670, 200, 200, 50, "☁️ YouTube Servers", "external")

    # Storage
    box(670, 280, 200, 120, "💾 Local Storage", "storage")
    sub_box(690, 315, 160, 24, "Config JSON (~/.FetchTubeWeb)")
    sub_box(690, 345, 160, 24, "Download directory")
    sub_box(690, 375, 160, 24, "Browser localStorage")

    # ---- Arrows ----
    # Browser -> Server
    s.arrow(230, 80, 290, 80)
    s.text(260, 72, "HTTP", size=10, fill="#666", anchor="middle")

    s.arrow(230, 140, 290, 140)
    s.text(260, 132, "WS", size=10, fill="#666", anchor="middle")

    # Server -> External
    s.arrow(610, 90, 670, 90)
    s.text(640, 82, "exec", size=10, fill="#666", anchor="middle")

    # Server -> YouTube
    s.arrow(610, 225, 670, 225)
    s.text(640, 217, "HTTP", size=10, fill="#666", anchor="middle")

    # Server -> Storage
    s.arrow(610, 340, 670, 340)
    s.text(640, 332, "R/W", size=10, fill="#666", anchor="middle")

    # External -> External
    s.arrow(770, 170, 770, 200)
    s.text(780, 188, "API", size=9, fill="#666", anchor="start")

    # Legend
    s.text(30, 180, "Data Flow:", size=11, fill="#333", bold=True)
    s.arrow(30, 200, 80, 200)
    s.text(85, 204, "Request / Response / Event", size=10, fill="#555")

    s.save()


# ============================================================
# Diagram 2: Request Flow (Sequence Diagram)
# ============================================================

def diagram_request_flow():
    s = SVG(960, 680, os.path.join(OUT_DIR, "request-flow.svg"))

    # Actors
    actors = [
        ("Browser", 60),
        ("main.go\n(router)", 230),
        ("Handler", 400),
        ("yt-dlp\nwrapper", 570),
        ("yt-dlp\n(external)", 740),
        ("Disk", 880),
    ]

    for name, x in actors:
        s.rect(x - 30, 20, 100, 40, rx=20, fill="#e3f2fd", stroke="#1565c0", sw=1.5)
        s.text(x + 20, 34, name, size=10, fill="#1565c0", anchor="middle", bold=True)
        # lifeline
        s.line(x + 20, 60, x + 20, 650, stroke="#ccc", sw=1, dash="4,4")

    # Arrow helper
    y = 80
    def step(ypos, frm, to, label, color="#455a64"):
        fx = actors[frm][1] + 20
        tx = actors[to][1] + 20
        mid = (fx + tx) / 2
        s.arrow(fx, ypos, tx, ypos, stroke=color, sw=1.5)
        s.text(mid, ypos - 8, label, size=10, fill=color, anchor="middle")
        return ypos + 30

    # Separator lines
    def section(ypos, title):
        s.line(40, ypos, 920, ypos, stroke="#e0e0e0", sw=1)
        s.text(50, ypos - 5, title, size=11, fill="#888", bold=True)
        return ypos + 20

    y = section(y, "1. Video Search")
    y = step(y, 0, 1, "GET /api/info?url=...")
    y = step(y, 1, 2, "handleInfo()")
    y = step(y, 2, 3, "FetchInfo(url, proxy, cookies)")
    y = step(y, 3, 4, "--dump-json <url>")
    y = step(y, 4, 3, "JSON (title, formats, ...)")
    y = step(y, 3, 2, "ParseVideoInfo(raw)")
    y = step(y, 2, 1, "VideoInfo JSON")
    y = step(y, 1, 0, "renderLocalInfo() + renderLocalFormats()")

    y = section(y + 10, "2. Start Download")
    y = step(y, 0, 1, "POST /api/download {url,format_id,save_dir}")
    y = step(y, 1, 2, "handleDownload()")
    y = step(y, 2, 2, "taskManager.Enqueue(opts, ...)")
    y = step(y, 2, 0, "WS→ {status:'queued', task_id:'ts_xxx'}", "#666")

    y = section(y + 10, "3. Download Execution (goroutine)")
    y = step(y, 2, 3, "DownloadManager.Download(opts)", "#2e7d32")
    y = step(y, 3, 4, "yt-dlp --newline --format ... <url>", "#2e7d32")
    y = step(y, 4, 3, "stdout: [download] 45.3% of ~100MiB ...", "#2e7d32")
    y = step(y, 3, 3, "parseProgressLine(line)", "#666")
    y = step(y, 3, 0, "WS→ {status:'downloading', percent:45.3, ...}", "#e65100")
    y = step(y, 4, 5, "Write .part / .ytdl files", "#7b1fa2")
    y = step(y, 4, 3, "[Merger] Merging formats...", "#2e7d32")
    y = step(y, 3, 0, "WS→ {status:'merging', ...}", "#e65100")
    y = step(y, 4, 5, "ffmpeg merge → final .mkv", "#7b1fa2")
    y = step(y, 3, 0, "WS→ {status:'finished', avg_speed_mbps:..., final_size_mb:...}", "#e65100")

    y = section(y + 10, "4. Progress Display")
    y = step(y, 0, 0, "onProgress(data) → update state.tasks → renderQueue()", "#666")

    s.save()


# ============================================================
# Diagram 3: Download State Machine
# ============================================================

def diagram_state_machine():
    s = SVG(800, 520, os.path.join(OUT_DIR, "state-machine.svg"))

    states = [
        ("queued", "Queued\n排队中", 400, 420),
        ("starting", "Starting\n启动中", 250, 310),
        ("downloading", "Downloading\n下载中", 250, 160),
        ("merging", "Merging\n合并中", 550, 160),
        ("finished", "Finished ✓\n完成", 550, 310),
        ("error", "Error ✗\n失败", 400, 420),   # moved down
        ("cancelled", "Cancelled\n已取消", 150, 420),
    ]

    # Adjust positions
    positions = {
        "queued": (400, 430),
        "starting": (200, 300),
        "downloading": (200, 140),
        "merging": (600, 140),
        "finished": (600, 300),
        "error": (450, 400),
        "cancelled": (100, 400),
        "retry": (400, 50),
    }

    colors = {
        "queued": ("#fff9c4", "#f9a825"),
        "starting": ("#e3f2fd", "#1565c0"),
        "downloading": ("#e3f2fd", "#1565c0"),
        "merging": ("#e8f5e9", "#2e7d32"),
        "finished": ("#c8e6c9", "#1b5e20"),
        "error": ("#ffcdd2", "#c62828"),
        "cancelled": ("#f3e5f5", "#7b1fa2"),
        "retry": ("#ffe0b2", "#e65100"),
    }

    # Draw states
    radius = 52
    for key, (x, y) in positions.items():
        fill, stroke = colors[key]
        s.circle(x, y, radius, fill=fill, stroke=stroke, sw=2.5)

        label = {
            "queued": "Queued\n排队中",
            "starting": "Starting\n启动中",
            "downloading": "Downloading\n下载中",
            "merging": "Merging\n合并中",
            "finished": "Finished ✓\n完成",
            "error": "Error ✗\n失败",
            "cancelled": "Cancelled\n已取消",
            "retry": "Retry\n重试",
        }[key]
        s.text(x, y - 6, label, size=11, fill=stroke, anchor="middle", bold=True)

    # Transitions
    transitions = [
        ("queued", "starting", "acquire\nsemaphore", "#555"),
        ("starting", "downloading", "yt-dlp\nstarted", "#555"),
        ("downloading", "merging", "[Merger]\ndetected", "#555"),
        ("merging", "finished", "file size\nstable → done", "#555"),
        ("downloading", "cancelled", "user cancel\n(SIGTERM)", "#7b1fa2"),
        ("merging", "cancelled", "user cancel\n(SIGTERM)", "#7b1fa2"),
        ("starting", "cancelled", "user cancel\n(SIGTERM)", "#7b1fa2"),
        ("downloading", "error", "yt-dlp\ncrashed", "#c62828"),
        ("merging", "error", "ffmpeg\nfailed", "#c62828"),
        ("merging", "retry", "merge\ntimeout", "#e65100"),
        ("retry", "downloading", "re-download\n(≤2 retries)", "#e65100"),
        ("retry", "error", "retries\nexhausted", "#c62828"),
    ]

    for frm, to, label, color in transitions:
        fx, fy = positions[frm]
        tx, ty = positions[to]

        # Calculate edge point on circles
        dx = tx - fx
        dy = ty - fy
        L = (dx*dx + dy*dy) ** 0.5
        if L == 0: continue
        ux, uy = dx/L, dy/L

        # Start from edge of source circle
        sx = fx + ux * (radius + 2)
        sy = fy + uy * (radius + 2)
        # End at edge of target circle
        ex = tx - ux * (radius + 2)
        ey = ty - uy * (radius + 2)

        s.arrow(sx, sy, ex, ey, stroke=color, sw=1.3)

        # Label at midpoint, offset perpendicular
        mx = (sx + ex) / 2
        my = (sy + ey) / 2
        # offset perpendicular
        px = -uy * 18
        py = ux * 18
        s.text(mx + px, my + py, label, size=8, fill=color, anchor="middle")

    # Initial arrow
    s.arrow(400, 510, 400, 482, stroke="#333", sw=1.5)
    s.text(410, 500, "POST\n/api/download", size=8, fill="#555")

    # Title
    s.text(400, 25, "Download Task State Machine", size=16, fill="#333", anchor="middle", bold=True)
    s.text(400, 45, "One task flows through these states in a goroutine", size=11, fill="#888", anchor="middle")

    s.save()


# ============================================================
# Diagram 4: WebSocket Communication Flow
# ============================================================

def diagram_ws_flow():
    s = SVG(900, 550, os.path.join(OUT_DIR, "ws-communication.svg"))

    # Title
    s.text(450, 25, "WebSocket Real-time Progress Communication", size=16, fill="#333", anchor="middle", bold=True)

    # Components
    def comp(x, y, w, h, title, desc, color):
        fill, stroke = color
        s.rect(x, y, w, h, rx=8, fill=fill, stroke=stroke, sw=2)
        s.text(x + w/2, y + 22, title, size=13, fill=stroke, anchor="middle", bold=True)
        s.text(x + w/2, y + 42, desc, size=10, fill="#666", anchor="middle")

    comp(30, 60, 200, 60, "Browser Client", "app.js → WS.onMessage = onProgress", ("#e3f2fd", "#1565c0"))
    comp(300, 60, 300, 60, "WebSocket Hub", "goroutine loop: register / unregister / broadcast", ("#e8f5e9", "#2e7d32"))
    comp(670, 60, 200, 60, "Download Goroutine", "callback(data) → hub.Broadcast(data)", ("#fff3e0", "#e65100"))

    # Sequence
    y = 150
    def seq(actor, msg, direction="→"):
        labels = {0: "Browser", 1: "WS Hub", 2: "Goroutine"}
        x_positions = [130, 450, 770]
        x = x_positions[actor]
        col = "#333"
        s.text(x, y, msg, size=11, fill=col, anchor="middle")
        return y + 22

    def seq_arrow(frm, to, ypos):
        x_positions = [130, 450, 770]
        s.arrow(x_positions[frm], ypos, x_positions[to], ypos, stroke="#455a64", sw=1.3)

    # Connection setup
    y = seq(0, "new WebSocket('/ws/progress')")
    y = seq(1, "upgrader.Upgrade(w, r)")
    y = seq(1, "register ← conn  (add to clients map)")
    y = seq(0, "console.log('[ws] connected')", "←")

    y += 10
    # Progress broadcasting
    y = seq(2, "callback(ProgressData{percent:45.3, ...})")
    y = seq(1, "hub.Broadcast(msg) → broadcast channel")
    y = seq(1, "for conn in clients: conn.WriteMessage(json)")
    y = seq(0, "onmessage → JSON.parse → onProgress(data)")
    y = seq(0, "update state.tasks[taskID] + renderQueue()")

    y += 10
    # Disconnection
    y = seq(0, "browser tab closed")
    y = seq(1, "read loop detects EOF → unregister ← conn")
    y = seq(1, "delete(clients, conn) + conn.Close()")

    # Box annotations
    y += 15
    s.rect(30, 340, 840, 100, rx=5, fill="#f5f5f5", stroke="#ddd", sw=1, dash="5,5")
    s.text(50, 365, "Key Design Points:", size=12, fill="#333", bold=True)
    items = [
        "① Hub.run() is a single goroutine using select{} — all map mutations are serialized, no mutex needed for channels",
        "② broadcast channel has buffer=64 to avoid blocking download goroutines on brief send stalls",
        "③ JSON serialization happens once per broadcast (fan-out to N clients)",
        "④ Each client has a read-loop goroutine that detects disconnection and triggers cleanup",
    ]
    for i, item in enumerate(items):
        s.text(60, 388 + i * 18, item, size=10, fill="#555")

    s.save()


# ============================================================
# Diagram 5: Project Directory Structure
# ============================================================

def diagram_directory():
    s = SVG(800, 600, os.path.join(OUT_DIR, "directory-structure.svg"))

    tree = [
        ("FetchTubeWeb/", 0, True),
        ("├── main.go", 1, False),
        ("├── go.mod / go.sum", 1, False),
        ("├── Makefile", 1, False),
        ("├── build_release.ps1", 1, False),
        ("├── yt-dlp.exe  (external)", 1, False),
        ("├── ffmpeg.exe  (external)", 1, False),
        ("├── node.exe    (external)", 1, False),
        ("├── internal/", 1, True),
        ("│   ├── config/", 2, True),
        ("│   │   └── config.go", 3, False),
        ("│   ├── handler/", 2, True),
        ("│   │   ├── handler.go", 3, False),
        ("│   │   ├── browse.go", 3, False),
        ("│   │   ├── config.go", 3, False),
        ("│   │   ├── download.go", 3, False),
        ("│   │   ├── taskmanager.go", 3, False),
        ("│   │   └── ws.go", 3, False),
        ("│   ├── models/", 2, True),
        ("│   │   └── models.go", 3, False),
        ("│   └── ytdlp/", 2, True),
        ("│       ├── ytdlp.go", 3, False),
        ("│       ├── info.go", 3, False),
        ("│       └── manager.go", 3, False),
        ("├── web/", 1, True),
        ("│   ├── embed.go", 2, False),
        ("│   └── static/", 2, True),
        ("│       ├── index.html", 3, False),
        ("│       └── js/", 3, True),
        ("│           ├── api.js", 4, False),
        ("│           ├── app.js", 4, False),
        ("│           └── ws.js", 4, False),
        ("└── .github/workflows/", 1, True),
        ("    └── release.yml", 2, False),
    ]

    x_base = 40
    y_base = 40
    indent = 24
    line_h = 22

    # Color coding
    def color_for(name):
        if name.endswith("/"):
            return "#1565c0"  # directory
        if ".go" in name:
            return "#2e7d32"  # Go file
        if ".js" in name or ".html" in name:
            return "#e65100"  # frontend
        if ".ps1" in name or ".yml" in name or "Makefile" in name:
            return "#7b1fa2"  # script
        return "#333"

    for i, (name, level, is_dir) in enumerate(tree):
        x = x_base + level * indent
        y = y_base + i * line_h
        col = color_for(name)
        weight = "bold" if is_dir else "normal"
        s.text(x, y + 14, name, size=12, fill=col, bold=(weight == "bold"))

    # Annotation boxes on the right
    annotations = [
        (460, 60, "Entry point\nHTTP server, middleware, routes", "#2e7d32"),
        (460, 150, "Business logic\nConfig, download orchestration,\nWebSocket progress, directory browsing", "#2e7d32"),
        (460, 340, "Data types\nFormatOption, VideoInfo, ProgressData", "#2e7d32"),
        (460, 400, "External tool wrapper\nyt-dlp invocation, progress parsing,\nerror translation", "#2e7d32"),
        (460, 500, "Embedded frontend\n//go:embed compiles HTML/JS\ninto the binary", "#e65100"),
    ]

    for x, y, text, color in annotations:
        s.rect(x, y, 310, 55, rx=4, fill="#fff", stroke=color, sw=1, dash="4,4")
        s.text(x + 10, y + 18, text, size=10, fill="#333")

    s.text(40, 580, "Color:  Blue = directory  |  Green = .go  |  Orange = frontend  |  Purple = script", size=10, fill="#888")

    s.save()


# ============================================================
# Diagram 6: Config Load/Save Cycle
# ============================================================

def diagram_config_cycle():
    s = SVG(750, 420, os.path.join(OUT_DIR, "config-cycle.svg"))

    # Title
    s.text(375, 30, "Configuration Lifecycle", size=16, fill="#333", anchor="middle", bold=True)

    # Flow
    boxes = [
        (150, 70, "App Start\nmain.go", "#1565c0"),
        (400, 70, "config.Load()\nRead JSON file", "#2e7d32"),
        (650, 70, "Merge with\nDefaults", "#e65100"),
        (650, 200, "Server holds\ncfg in memory", "#7b1fa2"),
        (400, 320, "PUT /api/config\nSave to disk", "#c62828"),
        (150, 320, "Browser\nloads config", "#f9a825"),
    ]

    for x, y, label, color in boxes:
        w, h = 150, 55
        s.rect(x - w//2, y, w, h, rx=6, fill="#fff", stroke=color, sw=2)
        s.text(x, y + 22, label, size=11, fill=color, anchor="middle", bold=True)

    # Arrows between boxes
    arrows = [
        (225, 125, 325, 125),   # start → load
        (475, 125, 575, 125),   # load → merge
        (650, 125, 650, 172),   # merge → memory (down)
        (575, 228, 475, 228),  # memory → save (left)
        (400, 292, 400, 347),   # save → browser (down)
        (300, 347, 75, 347),   # browser → right? no, just draw
        (225, 347, 150, 347),   # browser left
    ]

    # Draw flow arrows manually
    s.arrow(225, 110, 310, 110)
    s.arrow(475, 110, 560, 110)
    s.arrow(650, 125, 650, 185)
    s.arrow(560, 235, 490, 235)

    # Memory → Save (down then left)
    s.polyline([(650, 255), (650, 280), (475, 280), (475, 290)], stroke="#455a64", sw=1.5, fill="none")
    # arrowhead at end
    s.arrow(400, 290, 400, 305)
    s.arrow(310, 347, 240, 347)
    s.arrow(75, 347, 75, 290)  # browser back to start? hmm

    # Browser → App Start (loop back)
    s.polyline([(75, 347), (75, 115), (110, 115)], stroke="#455a64", sw=1.3, fill="none", dash="6,4")

    # Labels on arrows
    s.text(268, 102, "defaults if\nfile missing", size=9, fill="#888", anchor="middle")
    s.text(518, 102, "saved fields\noverride defaults", size=9, fill="#888", anchor="middle")
    s.text(660, 155, "mu.Lock()\nthread-safe", size=9, fill="#888")
    s.text(560, 225, "auto-save on\nsearch & download", size=9, fill="#888")
    s.text(310, 355, "GET /api/config", size=9, fill="#888", anchor="middle")

    s.text(130, 395, "Config file: ~/.FetchTubeWeb_config.json  (JSON format, human-readable)", size=10, fill="#999")

    s.save()


# ============================================================
# Main
# ============================================================

OUT_DIR = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "pic")

def main():
    print("FetchTubeWeb Diagram Generator")
    print(f"Output directory: {OUT_DIR}")
    print()

    diagram_architecture()
    diagram_request_flow()
    diagram_state_machine()
    diagram_ws_flow()
    diagram_directory()
    diagram_config_cycle()

    print()
    print(f"All diagrams generated in: {OUT_DIR}")

if __name__ == "__main__":
    main()
