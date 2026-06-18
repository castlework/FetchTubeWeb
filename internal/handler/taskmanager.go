package handler

import (
	"encoding/base32"
	"fmt"
	"sync"
	"time"

	"FetchTubeWeb/internal/ytdlp"
)

const maxConcurrent = 3

// TaskManager 管理多个并发下载任务
type TaskManager struct {
	mu        sync.RWMutex
	tasks     map[string]*DownloadTask
	semaphore chan struct{}
}

// DownloadTask 单个下载任务
type DownloadTask struct {
	ID      string
	Manager *ytdlp.DownloadManager
	Opts    ytdlp.DownloadOptions
	Info    TaskInfo
}

// TaskInfo 任务状态（API 响应用）
type TaskInfo struct {
	TaskID       string  `json:"task_id"`
	URL          string  `json:"url"`
	Title        string  `json:"title"`
	Status       string  `json:"status"`
	Percent      float64 `json:"percent"`
	SpeedMBps    float64 `json:"speed_mbps"`
	TotalMB      float64 `json:"total_mb"`
	ETASeconds   int     `json:"eta_seconds"`
	ErrorMessage string  `json:"error_message,omitempty"`
	SaveDir      string  `json:"save_dir,omitempty"`
}

// NewTaskManager 创建任务管理器
func NewTaskManager() *TaskManager {
	return &TaskManager{
		tasks:     make(map[string]*DownloadTask),
		semaphore: make(chan struct{}, maxConcurrent),
	}
}

// Enqueue 将下载任务加入队列，立即返回 taskID，异步执行
// cleanup 在任务结束时调用（可用于删除临时文件），可为 nil
func (tm *TaskManager) Enqueue(opts ytdlp.DownloadOptions, title string, broadcast func(ytdlp.ProgressData), cleanup func()) string {
	taskID := genTaskID()

	task := &DownloadTask{
		ID:      taskID,
		Manager: ytdlp.NewDownloadManager(),
		Opts:    opts,
		Info: TaskInfo{
			TaskID:  taskID,
			URL:     opts.URL,
			Title:   title,
			Status:  "queued",
			SaveDir: opts.SaveDir,
		},
	}

	tm.mu.Lock()
	tm.tasks[taskID] = task
	tm.mu.Unlock()

	// 广播队列状态
	broadcast(ytdlp.ProgressData{
		TaskID:  taskID,
		Status:  "queued",
		Title:   title,
		URL:     opts.URL,
		SaveDir: opts.SaveDir,
	})

	go tm.run(task, broadcast, cleanup)

	return taskID
}

// run 在 goroutine 中执行下载任务
func (tm *TaskManager) run(task *DownloadTask, broadcast func(ytdlp.ProgressData), cleanup func()) {
	if cleanup != nil {
		defer cleanup()
	}

	// 获取并发槽位
	tm.semaphore <- struct{}{}
	defer func() { <-tm.semaphore }()

	tm.updateStatus(task.ID, "downloading", 0, 0, 0, 0, "")
	broadcast(ytdlp.ProgressData{
		TaskID: task.ID,
		Status: "starting",
		Title:  task.Info.Title,
	})

	// 执行下载
	_ = task.Manager.Download(task.Opts, func(data ytdlp.ProgressData) {
		data.TaskID = task.ID
		data.Title = task.Info.Title
		data.URL = task.Info.URL
		data.SaveDir = task.Info.SaveDir
		broadcast(data)

		// 同步更新 TaskInfo
		tm.updateStatus(task.ID, data.Status, data.Percent, data.SpeedMBps, data.TotalMB, data.ETASeconds, data.ErrorMessage)
	})
}

// Cancel 取消指定任务
func (tm *TaskManager) Cancel(taskID string) error {
	tm.mu.RLock()
	task, ok := tm.tasks[taskID]
	tm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("任务不存在: %s", taskID)
	}
	task.Manager.Cancel()
	return nil
}

// Remove 从任务列表中移除指定任务（不取消正在进行的下载）
func (tm *TaskManager) Remove(taskID string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if _, ok := tm.tasks[taskID]; !ok {
		return fmt.Errorf("任务不存在: %s", taskID)
	}
	delete(tm.tasks, taskID)
	return nil
}

// RemoveBatch 批量移除任务
func (tm *TaskManager) RemoveBatch(taskIDs []string) int {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	count := 0
	for _, id := range taskIDs {
		if _, ok := tm.tasks[id]; ok {
			delete(tm.tasks, id)
			count++
		}
	}
	return count
}

// Get 获取指定任务
func (tm *TaskManager) Get(taskID string) (*DownloadTask, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	task, ok := tm.tasks[taskID]
	return task, ok
}

// List 返回所有任务的 TaskInfo 列表
func (tm *TaskManager) List() []TaskInfo {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	result := make([]TaskInfo, 0, len(tm.tasks))
	for _, t := range tm.tasks {
		result = append(result, t.Info)
	}
	return result
}

// updateStatus 更新任务状态（线程安全）
func (tm *TaskManager) updateStatus(taskID, status string, percent, speed, totalMB float64, eta int, errMsg string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if task, ok := tm.tasks[taskID]; ok {
		task.Info.Status = status
		task.Info.Percent = percent
		task.Info.SpeedMBps = speed
		task.Info.TotalMB = totalMB
		task.Info.ETASeconds = eta
		if errMsg != "" {
			task.Info.ErrorMessage = errMsg
		}
	}
}

// genTaskID 生成唯一任务 ID（时间戳 base32）
func genTaskID() string {
	var buf [8]byte
	ns := uint64(time.Now().UnixNano())
	buf[0] = byte(ns >> 56)
	buf[1] = byte(ns >> 48)
	buf[2] = byte(ns >> 40)
	buf[3] = byte(ns >> 32)
	buf[4] = byte(ns >> 24)
	buf[5] = byte(ns >> 16)
	buf[6] = byte(ns >> 8)
	buf[7] = byte(ns)
	return "ts_" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf[:])
}
