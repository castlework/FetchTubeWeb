# YouTube 视频下载器 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建一个基于 tkinter 的桌面 GUI 应用，用户可以粘贴 YouTube URL、搜索视频信息、选择分辨率、下载为 mp4/webm/mkv 格式。

**Architecture:** 三层分离 — `gui.py`（tkinter UI）、`downloader.py`（yt-dlp 封装）、`models.py`（数据类）。GUI 通过后台线程调用 downloader，用队列将结果/进度传回主线程更新 UI。

**Tech Stack:** Python 3.13, tkinter, yt-dlp

---

## 文件清单

| 操作 | 路径 | 职责 |
|------|------|------|
| 创建 | `requirements.txt` | 依赖声明 |
| 创建 | `src/__init__.py` | 包标记 |
| 创建 | `src/models.py` | VideoInfo, FormatOption 数据类 |
| 创建 | `src/downloader.py` | Downloader 类，封装 yt-dlp |
| 创建 | `src/gui.py` | tkinter 完整界面 |
| 创建 | `main.py` | 程序入口 |

---

### Task 1: 项目初始化

**Files:**
- Create: `requirements.txt`
- Create: `src/__init__.py`

- [ ] **Step 1: 创建 requirements.txt**

```python
# requirements.txt
yt-dlp>=2024.0.0
```

- [ ] **Step 2: 创建 src 包**

创建空文件 `src/__init__.py`。

- [ ] **Step 3: 安装依赖**

```bash
pip install -r requirements.txt
```

---

### Task 2: 数据模型

**Files:**
- Create: `src/models.py`

- [ ] **Step 1: 实现 FormatOption 和 VideoInfo 数据类**

```python
"""YouTube 视频下载器 — 数据模型"""
from dataclasses import dataclass, field


@dataclass
class FormatOption:
    """单个可下载的格式（视频 + 音频）"""
    format_id: str          # yt-dlp 格式 ID
    resolution: str         # "1920x1080"
    fps: int                # 帧率
    codec: str              # 视频编码
    audio_codec: str        # 音频编码
    file_size_mb: float     # 文件大小（MB），未知为 0.0
    ext: str                # 文件扩展名
    note: str               # 备注


@dataclass
class VideoInfo:
    """视频完整元信息"""
    title: str
    url: str
    thumbnail_url: str
    duration_seconds: int
    duration_str: str       # 格式化时长 "14:25"
    uploader: str
    view_count: int
    like_count: int
    description: str
    formats: list[FormatOption] = field(default_factory=list)
```

- [ ] **Step 2: 验证模型可以正常导入**

```bash
python -c "from src.models import FormatOption, VideoInfo; print('OK')"
```

---

### Task 3: 下载引擎 — 信息提取

**Files:**
- Create: `src/downloader.py`

- [ ] **Step 1: 实现 Downloader 类和 fetch_info 方法**

```python
"""YouTube 视频下载器 — 下载引擎"""
import threading
from typing import Callable
import yt_dlp

from src.models import VideoInfo, FormatOption


class DownloadError(Exception):
    """下载相关错误"""
    pass


class Downloader:
    """封装 yt-dlp，提供视频信息提取和下载功能"""

    @staticmethod
    def _format_duration(seconds: int | None) -> str:
        if seconds is None:
            return "未知"
        m, s = divmod(int(seconds), 60)
        h, m = divmod(m, 60)
        if h:
            return f"{h}:{m:02d}:{s:02d}"
        return f"{m}:{s:02d}"

    @staticmethod
    def _get_resolution_sorter_key(fmt: FormatOption) -> tuple[int, float]:
        """用于排序：分辨率从高到低，同分辨率按文件大小从大到小"""
        try:
            h = int(fmt.resolution.split("x")[1]) if "x" in fmt.resolution else 0
        except (ValueError, IndexError):
            h = 0
        return (-h, -fmt.file_size_mb)

    def fetch_info(self, url: str) -> VideoInfo:
        """提取视频信息（不下载）"""
        opts = {
            "quiet": True,
            "no_warnings": True,
            "extract_flat": False,
        }
        try:
            with yt_dlp.YoutubeDL(opts) as ydl:
                info = ydl.extract_info(url, download=False)
        except Exception as e:
            raise DownloadError(f"无法获取视频信息：{e}") from e

        if info is None:
            raise DownloadError("无法获取视频信息，请检查链接是否有效")

        # 解析格式列表
        formats: list[FormatOption] = []
        seen_keys: set[tuple[int, str]] = set()  # 用于去重

        for f in info.get("formats", []):
            vcodec = f.get("vcodec", "none") or "none"
            acodec = f.get("acodec", "none") or "none"

            # 只保留同时有视频和音频的格式
            if vcodec == "none" or acodec == "none":
                continue

            width = f.get("width") or 0
            height = f.get("height") or 0
            resolution = f"{width}x{height}" if width and height else "未知"

            fps_val = f.get("fps") or 0
            try:
                fps = int(fps_val)
            except (ValueError, TypeError):
                fps = 0

            filesize = f.get("filesize") or f.get("filesize_approx") or 0
            file_size_mb = round(filesize / (1024 * 1024), 1) if filesize else 0.0

            ext = f.get("ext", "mp4")
            note = f.get("format_note", "")
            format_id = f.get("format_id", "")

            # 去重：同分辨率 + 同编码保留文件大小最大的
            key = (height, vcodec)
            if key in seen_keys:
                continue
            seen_keys.add(key)

            formats.append(FormatOption(
                format_id=format_id,
                resolution=resolution,
                fps=fps,
                codec=vcodec,
                audio_codec=acodec,
                file_size_mb=file_size_mb,
                ext=ext,
                note=note or f"{height}p",
            ))

        # 排序：分辨率从高到低
        formats.sort(key=self._get_resolution_sorter_key)

        # 处理简介：截取前 200 字
        raw_desc = info.get("description", "") or ""
        description = raw_desc[:200] + ("..." if len(raw_desc) > 200 else "")

        duration_sec = info.get("duration") or 0

        return VideoInfo(
            title=info.get("title", "未知标题"),
            url=url,
            thumbnail_url=info.get("thumbnail", ""),
            duration_seconds=duration_sec,
            duration_str=self._format_duration(duration_sec),
            uploader=info.get("uploader", "未知"),
            view_count=info.get("view_count") or 0,
            like_count=info.get("like_count") or 0,
            description=description,
            formats=formats,
        )
```

- [ ] **Step 2: 验证 fetch_info 可用（快速测试）**

```bash
python -c "
from src.downloader import Downloader
d = Downloader()
info = d.fetch_info('https://www.youtube.com/watch?v=lpDZwAxVkc4')
print(f'标题: {info.title}')
print(f'上传者: {info.uploader}')
print(f'时长: {info.duration_str}')
print(f'格式数: {len(info.formats)}')
for f in info.formats:
    print(f'  {f.note:8s} {f.resolution:12s} {f.codec:6s}+{f.audio_codec:6s} ~{f.file_size_mb:6.1f}MB ({f.ext})')
"
```

Expected output: 标题、上传者、时长、至少 3 个带音频的格式列表。

---

### Task 4: 下载引擎 — 下载功能

**Files:**
- Modify: `src/downloader.py`

- [ ] **Step 1: 添加 download 方法**

在 `Downloader` 类中追加以下方法：

```python
    def download(
        self,
        url: str,
        format_id: str,
        output_ext: str,
        save_dir: str,
        progress_callback: Callable[[dict], None],
    ) -> None:
        """
        下载视频。
        progress_callback 接收 dict:
          {"status": "downloading"|"finished"|"error",
           "percent": float, "speed_mbps": float,
           "downloaded_mb": float, "total_mb": float,
           "eta_seconds": int, "elapsed_seconds": int,
           "filename": str}
        """
        result = {"status": "downloading"}

        def _progress_hook(d: dict) -> None:
            nonlocal result
            status = d.get("status", "")

            if status == "downloading":
                downloaded = d.get("downloaded_bytes", 0) or 0
                total = d.get("total_bytes") or d.get("total_bytes_estimate") or 0
                speed = d.get("speed") or 0
                elapsed = d.get("elapsed", 0) or 0

                percent = (downloaded / total * 100) if total else 0.0
                speed_mbps = speed / (1024 * 1024) if speed else 0.0
                downloaded_mb = downloaded / (1024 * 1024)
                total_mb = total / (1024 * 1024)

                eta = d.get("eta") or 0

                result.update({
                    "status": "downloading",
                    "percent": percent,
                    "speed_mbps": speed_mbps,
                    "downloaded_mb": downloaded_mb,
                    "total_mb": total_mb,
                    "eta_seconds": int(eta),
                    "elapsed_seconds": int(elapsed),
                    "filename": d.get("filename", ""),
                })
                progress_callback(result)

            elif status == "finished":
                result.update({
                    "status": "finished",
                    "percent": 100.0,
                    "filename": d.get("filename", ""),
                })
                progress_callback(result)

            elif status == "error":
                result.update({
                    "status": "error",
                    "filename": "",
                })
                progress_callback(result)

        ydl_opts = {
            "format": format_id,
            "outtmpl": f"{save_dir}/%(title)s.%(ext)s",
            "merge_output_format": output_ext,
            "progress_hooks": [_progress_hook],
            "quiet": True,
            "no_warnings": True,
        }

        try:
            with yt_dlp.YoutubeDL(ydl_opts) as ydl:
                ydl.download([url])
        except Exception as e:
            result.update({
                "status": "error",
                "percent": 0.0,
                "filename": str(e),
            })
            progress_callback(result)
```

- [ ] **Step 2: 验证 download 方法可被导入**

```bash
python -c "from src.downloader import Downloader; d = Downloader(); print('download 方法:', hasattr(d, 'download'))"
```

---

### Task 5: GUI — 主窗口和 URL 输入区

**Files:**
- Create: `src/gui.py`

- [ ] **Step 1: 创建 GUI 骨架，实现 URL 输入区**

```python
"""YouTube 视频下载器 — tkinter GUI"""
import tkinter as tk
from tkinter import ttk, messagebox, filedialog
import threading
import queue
import time

from src.downloader import Downloader, DownloadError
from src.models import VideoInfo, FormatOption


class YouTubeDownloaderApp:
    """主应用窗口"""

    WINDOW_WIDTH = 800
    WINDOW_HEIGHT = 700
    WINDOW_MIN_WIDTH = 600
    WINDOW_MIN_HEIGHT = 500

    def __init__(self, root: tk.Tk):
        self.root = root
        self.root.title("YouTube 视频下载器")
        self.root.geometry(f"{self.WINDOW_WIDTH}x{self.WINDOW_HEIGHT}")
        self.root.minsize(self.WINDOW_MIN_WIDTH, self.WINDOW_MIN_HEIGHT)

        self.downloader = Downloader()
        self.video_info: VideoInfo | None = None
        self.selected_format: FormatOption | None = None
        self.result_queue: queue.Queue = queue.Queue()
        self.download_thread: threading.Thread | None = None

        self._build_ui()
        self._poll_queue()

    def _build_ui(self) -> None:
        """构建全部 UI 组件"""
        # 主内容区（带 padding）
        main_frame = ttk.Frame(self.root, padding="16 12 16 12")
        main_frame.pack(fill=tk.BOTH, expand=True)

        # ---- ① URL 输入区 ----
        url_label = ttk.Label(
            main_frame,
            text="请输入 YouTube 视频链接",
            font=("", 13, "bold"),
        )
        url_label.pack(anchor=tk.W, pady=(0, 4))

        url_row = ttk.Frame(main_frame)
        url_row.pack(fill=tk.X, pady=(0, 12))

        self.url_var = tk.StringVar()
        self.url_entry = ttk.Entry(
            url_row,
            textvariable=self.url_var,
            font=("", 11),
        )
        self.url_entry.pack(side=tk.LEFT, fill=tk.X, expand=True, ipady=3)
        self.url_entry.bind("<Return>", lambda e: self._on_search())

        self.search_btn = ttk.Button(
            url_row,
            text="🔍 搜索",
            command=self._on_search,
            width=10,
        )
        self.search_btn.pack(side=tk.LEFT, padx=(8, 0))

        # ---- ② 视频信息区 ----
        self.info_frame = ttk.LabelFrame(main_frame, text=" 视频信息 ", padding="8")
        self.info_frame.pack(fill=tk.X, pady=(0, 12))

        # 占位提示
        self.info_placeholder = ttk.Label(
            self.info_frame,
            text="输入链接并点击「搜索」以查看视频信息",
            foreground="gray",
        )
        self.info_placeholder.pack(pady=8)

        # 信息展示区（初始隐藏）
        self.info_content = ttk.Frame(self.info_frame)

        self.title_var = tk.StringVar(value="标题：")
        self.uploader_var = tk.StringVar(value="上传者：")
        self.duration_var = tk.StringVar(value="时长：")
        self.views_var = tk.StringVar(value="播放：")
        self.likes_var = tk.StringVar(value="点赞：")
        self.desc_var = tk.StringVar(value="简介：")

        ttk.Label(self.info_content, textvariable=self.title_var, wraplength=720).pack(anchor=tk.W)
        detail_row = ttk.Frame(self.info_content)
        detail_row.pack(fill=tk.X, pady=(4, 0))
        ttk.Label(detail_row, textvariable=self.uploader_var).pack(side=tk.LEFT)
        ttk.Label(detail_row, textvariable=self.duration_var).pack(side=tk.LEFT, padx=(16, 0))
        ttk.Label(detail_row, textvariable=self.views_var).pack(side=tk.LEFT, padx=(16, 0))
        ttk.Label(detail_row, textvariable=self.likes_var).pack(side=tk.LEFT, padx=(16, 0))
        ttk.Label(self.info_content, textvariable=self.desc_var, wraplength=720, foreground="gray").pack(anchor=tk.W, pady=(4, 0))

        # ---- ③ 分辨率选择区 ----
        self.format_frame = ttk.LabelFrame(main_frame, text=" 选择分辨率 ", padding="8")

        columns = ("分辨率", "编码", "音频编码", "大小", "扩展名")
        self.format_tree = ttk.Treeview(
            self.format_frame,
            columns=columns,
            show="headings",
            height=6,
            selectmode="browse",
        )
        self.format_tree.heading("分辨率", text="分辨率")
        self.format_tree.heading("编码", text="视频编码")
        self.format_tree.heading("音频编码", text="音频编码")
        self.format_tree.heading("大小", text="大小 (MB)")
        self.format_tree.heading("扩展名", text="扩展名")
        self.format_tree.column("分辨率", width=100, anchor=tk.CENTER)
        self.format_tree.column("编码", width=100, anchor=tk.CENTER)
        self.format_tree.column("音频编码", width=100, anchor=tk.CENTER)
        self.format_tree.column("大小", width=80, anchor=tk.CENTER)
        self.format_tree.column("扩展名", width=70, anchor=tk.CENTER)
        self.format_tree.bind("<<TreeviewSelect>>", self._on_format_selected)

        format_scrollbar = ttk.Scrollbar(self.format_frame, orient=tk.VERTICAL, command=self.format_tree.yview)
        self.format_tree.configure(yscrollcommand=format_scrollbar.set)

        # ---- ④ 格式下拉框 ----
        fmt_row = ttk.Frame(main_frame)
        fmt_row.pack(fill=tk.X, pady=(0, 8))

        ttk.Label(fmt_row, text="输出格式：").pack(side=tk.LEFT)
        self.format_var = tk.StringVar(value="mp4")
        self.format_combo = ttk.Combobox(
            fmt_row,
            textvariable=self.format_var,
            values=["mp4", "webm", "mkv"],
            state="readonly",
            width=8,
        )
        self.format_combo.pack(side=tk.LEFT, padx=(4, 0))

        # ---- ⑤ 下载按钮 ----
        self.download_btn = ttk.Button(
            main_frame,
            text="⬇ 开始下载",
            command=self._on_download,
        )
        self.download_btn.pack(fill=tk.X, ipady=6, pady=(0, 12))

        # ---- ⑥ 进度展示区（初始隐藏） ----
        self.progress_frame = ttk.LabelFrame(main_frame, text=" 下载进度 ", padding="8")

        self.progress_bar = ttk.Progressbar(self.progress_frame, mode="determinate", length=400)
        self.progress_bar.pack(fill=tk.X)

        self.percent_var = tk.StringVar(value="0%")
        ttk.Label(self.progress_frame, textvariable=self.percent_var, font=("", 12, "bold")).pack(anchor=tk.E, pady=(2, 4))

        self.speed_var = tk.StringVar(value="速度：-- MB/s")
        self.size_var = tk.StringVar(value="已下载：-- / -- MB")
        self.time_var = tk.StringVar(value="耗时：-- | 剩余：--")
        self.path_var = tk.StringVar(value="保存至：--")
        self.status_var = tk.StringVar(value="就绪")

        ttk.Label(self.progress_frame, textvariable=self.speed_var).pack(anchor=tk.W)
        ttk.Label(self.progress_frame, textvariable=self.size_var).pack(anchor=tk.W)
        ttk.Label(self.progress_frame, textvariable=self.time_var).pack(anchor=tk.W)
        ttk.Label(self.progress_frame, textvariable=self.path_var, wraplength=720).pack(anchor=tk.W)
        ttk.Label(self.progress_frame, textvariable=self.status_var, foreground="blue").pack(anchor=tk.W, pady=(4, 0))

    # ----- 后续 Task 中添加的方法占位 -----

    def _on_search(self) -> None:
        """点击搜索按钮"""
        pass

    def _on_format_selected(self, event) -> None:
        """分辨率选中事件"""
        pass

    def _on_download(self) -> None:
        """点击下载按钮"""
        pass

    def _poll_queue(self) -> None:
        """定期轮询后台线程结果队列"""
        pass
```

- [ ] **Step 2: 验证窗口可以启动**

```bash
python -c "
import tkinter as tk
from src.gui import YouTubeDownloaderApp
root = tk.Tk()
app = YouTubeDownloaderApp(root)
print('GUI 创建成功，窗口即将打开...')
root.after(2000, root.destroy)
root.mainloop()
print('窗口正常关闭')
"
```

Expected: 窗口显示 2 秒后关闭，无错误。

---

### Task 6: GUI — 搜索功能和信息展示

**Files:**
- Modify: `src/gui.py`

- [ ] **Step 1: 实现 _on_search 方法**

替换 `_on_search` 方法：

```python
    def _on_search(self) -> None:
        """点击搜索按钮"""
        url = self.url_var.get().strip()
        if not url:
            messagebox.showwarning("提示", "请输入 YouTube 链接")
            return

        self.search_btn.configure(text="⏳ 搜索中...", state=tk.DISABLED)
        self.root.update()

        def _do_search() -> None:
            try:
                info = self.downloader.fetch_info(url)
                self.result_queue.put(("search_ok", info))
            except Exception as e:
                self.result_queue.put(("search_error", str(e)))

        threading.Thread(target=_do_search, daemon=True).start()
```

- [ ] **Step 2: 实现 _handle_search_ok 和 _handle_search_error**

在类中追加：

```python
    def _handle_search_ok(self, info: VideoInfo) -> None:
        """搜索成功，更新 UI"""
        self.video_info = info
        self.search_btn.configure(text="🔍 搜索", state=tk.NORMAL)

        # 更新视频信息
        self.info_placeholder.pack_forget()
        self.info_content.pack(fill=tk.X)

        self.title_var.set(f"📌 标题：{info.title}")
        self.uploader_var.set(f"👤 上传者：{info.uploader}")
        self.duration_var.set(f"⏱ 时长：{info.duration_str}")
        self.views_var.set(f"👁 播放：{self._fmt_count(info.view_count)}")
        self.likes_var.set(f"❤ 点赞：{self._fmt_count(info.like_count)}")
        self.desc_var.set(f"📝 简介：{info.description}" if info.description else "")

        # 更新分辨率列表
        self._populate_format_list(info.formats)

        # 显示分辨率选择区
        self.format_frame.pack(fill=tk.X, pady=(0, 8))
        # 确保 tree 和 scrollbar 在 frame 内
        self.format_tree.pack(side=tk.LEFT, fill=tk.BOTH, expand=True)
        for child in self.format_frame.winfo_children():
            if isinstance(child, ttk.Scrollbar):
                child.pack_forget()
        format_scrollbar = ttk.Scrollbar(self.format_frame, orient=tk.VERTICAL, command=self.format_tree.yview)
        self.format_tree.configure(yscrollcommand=format_scrollbar.set)
        format_scrollbar.pack(side=tk.RIGHT, fill=tk.Y)

    @staticmethod
    def _fmt_count(n: int) -> str:
        """格式化播放量/点赞数"""
        if n >= 10000:
            return f"{n/10000:.1f}万"
        return str(n)

    def _populate_format_list(self, formats: list[FormatOption]) -> None:
        """填充分辨率列表"""
        for item in self.format_tree.get_children():
            self.format_tree.delete(item)
        for f in formats:
            self.format_tree.insert("", tk.END, values=(
                f.note,
                f.codec,
                f.audio_codec,
                f"{f.file_size_mb:.1f}" if f.file_size_mb else "未知",
                f.ext,
            ))

    def _handle_search_error(self, error_msg: str) -> None:
        """搜索失败"""
        self.search_btn.configure(text="🔍 搜索", state=tk.NORMAL)
        messagebox.showerror("搜索失败", error_msg)
```

- [ ] **Step 3: 实现 _on_format_selected**

```python
    def _on_format_selected(self, event) -> None:
        """分辨率选中事件"""
        selection = self.format_tree.selection()
        if not selection or self.video_info is None:
            self.selected_format = None
            return
        idx = self.format_tree.index(selection[0])
        if idx < len(self.video_info.formats):
            self.selected_format = self.video_info.formats[idx]
```

- [ ] **Step 4: 实现 _poll_queue**

替换 `_poll_queue` 方法：

```python
    def _poll_queue(self) -> None:
        """定期轮询后台线程结果队列，每 100ms 检查一次"""
        try:
            while True:
                msg = self.result_queue.get_nowait()
                msg_type = msg[0]

                if msg_type == "search_ok":
                    self._handle_search_ok(msg[1])
                elif msg_type == "search_error":
                    self._handle_search_error(msg[1])
                elif msg_type == "download_progress":
                    self._handle_download_progress(msg[1])
                elif msg_type == "download_finished":
                    self._handle_download_finished(msg[1])
        except queue.Empty:
            pass
        self.root.after(100, self._poll_queue)
```

---

### Task 7: GUI — 下载功能和进度展示

**Files:**
- Modify: `src/gui.py`

- [ ] **Step 1: 实现 _on_download 方法**

```python
    def _on_download(self) -> None:
        """点击下载按钮"""
        if self.video_info is None or self.selected_format is None:
            messagebox.showwarning("提示", "请先搜索视频并选择分辨率")
            return

        # 弹出文件夹选择对话框
        save_dir = filedialog.askdirectory(
            title="选择保存目录",
            mustexist=True,
        )
        if not save_dir:
            return  # 用户取消

        self.download_btn.configure(text="⏳ 下载中...", state=tk.DISABLED)
        self._show_progress()
        self._reset_progress(save_dir)

        def _do_download() -> None:
            self.downloader.download(
                url=self.video_info.url,
                format_id=self.selected_format.format_id,
                output_ext=self.format_var.get(),
                save_dir=save_dir,
                progress_callback=lambda p: self.result_queue.put(("download_progress", p)),
            )

        threading.Thread(target=_do_download, daemon=True).start()

    def _show_progress(self) -> None:
        """显示进度区"""
        self.progress_frame.pack(fill=tk.X)

    def _hide_progress(self) -> None:
        """隐藏进度区"""
        self.progress_frame.pack_forget()

    def _reset_progress(self, save_dir: str) -> None:
        """重置进度显示"""
        self.progress_bar.configure(mode="determinate", value=0)
        self.percent_var.set("0%")
        self.speed_var.set("速度：-- MB/s")
        self.size_var.set("已下载：-- / -- MB")
        self.time_var.set("耗时：-- | 剩余：--")
        self.path_var.set(f"保存至：{save_dir}")
        self.status_var.set("正在下载...")
```

- [ ] **Step 2: 实现进度和完成处理**

```python
    def _handle_download_progress(self, progress: dict) -> None:
        """处理下载进度"""
        status = progress.get("status", "")

        if status == "downloading":
            percent = progress.get("percent", 0)
            speed_mbps = progress.get("speed_mbps", 0)
            downloaded_mb = progress.get("downloaded_mb", 0)
            total_mb = progress.get("total_mb", 0)
            eta_sec = progress.get("eta_seconds", 0)
            elapsed_sec = progress.get("elapsed_seconds", 0)
            filename = progress.get("filename", "")

            # 更新进度条
            if total_mb > 0:
                self.progress_bar.configure(mode="determinate", maximum=100, value=percent)
                self.size_var.set(f"已下载：{downloaded_mb:.1f} / {total_mb:.1f} MB")
            else:
                self.progress_bar.configure(mode="indeterminate")
                self.progress_bar.start(10)
                self.size_var.set(f"已下载：{downloaded_mb:.1f} MB")

            self.percent_var.set(f"{percent:.1f}%")
            self.speed_var.set(f"速度：{speed_mbps:.1f} MB/s")
            self.time_var.set(f"耗时：{self._fmt_time(elapsed_sec)} | 剩余：{self._fmt_time(eta_sec)}")
            if filename:
                self.path_var.set(f"保存至：{filename}")
            self.status_var.set("正在下载...")

        elif status == "finished":
            self.result_queue.put(("download_finished", progress))

        elif status == "error":
            self.progress_bar.stop()
            self.progress_bar.configure(mode="determinate", value=0)
            err_msg = progress.get("filename", "未知错误")
            self.status_var.set(f"下载失败：{err_msg}")
            self.download_btn.configure(text="⬇ 开始下载", state=tk.NORMAL)

    @staticmethod
    def _fmt_time(seconds: int) -> str:
        """格式化时间"""
        if seconds <= 0:
            return "--:--"
        m, s = divmod(int(seconds), 60)
        h, m = divmod(m, 60)
        if h:
            return f"{h:02d}:{m:02d}:{s:02d}"
        return f"{m:02d}:{s:02d}"

    def _handle_download_finished(self, progress: dict) -> None:
        """下载完成"""
        self.progress_bar.stop()
        self.progress_bar.configure(mode="determinate", value=100)
        self.percent_var.set("100%")
        self.status_var.set("下载完成 ✓")
        self.speed_var.set("速度：-- MB/s")
        self.time_var.set(f"耗时：-- | 剩余：--")

        # 按钮恢复
        self.download_btn.configure(text="✅ 下载完成", state=tk.NORMAL)
        self.root.after(2000, lambda: self.download_btn.configure(text="⬇ 开始下载"))

        messagebox.showinfo("下载完成", f"视频已保存至：\n{progress.get('filename', '')}")
```

---

### Task 8: 程序入口

**Files:**
- Create: `main.py`

- [ ] **Step 1: 创建 main.py**

```python
"""YouTube 视频下载器 — 程序入口"""
import tkinter as tk

from src.gui import YouTubeDownloaderApp


def main() -> None:
    root = tk.Tk()
    _ = YouTubeDownloaderApp(root)
    root.mainloop()


if __name__ == "__main__":
    main()
```

---

### Task 9: 端到端测试

**Files:**
- 无新建文件

- [ ] **Step 1: 启动应用**

```bash
python main.py
```

- [ ] **Step 2: 测试搜索**

在打开的窗口中：
1. 输入 URL：`https://www.youtube.com/watch?v=lpDZwAxVkc4`
2. 点击「🔍 搜索」按钮
3. 验证：视频信息区显示标题、上传者、时长、播放量等
4. 验证：分辨率列表中出现多个选项

- [ ] **Step 3: 测试下载**

1. 在分辨率列表中单击选中一个（如 720p）
2. 确认输出格式为 mp4
3. 点击「⬇ 开始下载」
4. 在弹出对话框中选择保存目录
5. 验证：进度条滚动、速度显示、百分比增长
6. 验证：完成后弹出"下载完成"提示
7. 验证：指定目录中出现下载的 mp4 文件

- [ ] **Step 4: 测试错误场景**

1. 空 URL → 点搜索 → 弹出警告
2. 无效 URL（如 `https://youtube.com/watch?v=invalid123`）→ 弹出错误提示
3. 未选分辨率直接点下载 → 弹出警告提示
