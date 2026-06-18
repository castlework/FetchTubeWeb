package handler

import (
	"encoding/base32"
	"fmt"
	"sync"
	"time"

	"FetchTubeWeb/internal/ytdlp"
)

const maxConcurrent = 3

// TaskManager manages multiple concurrent download tasks
type TaskManager struct {
	mu        sync.RWMutex
	tasks     map[string]*DownloadTask
	semaphore chan struct{}
}

// DownloadTask a single download task
type DownloadTask struct {
	ID      string
	Manager *ytdlp.DownloadManager
	Opts    ytdlp.DownloadOptions
	Info    TaskInfo
}

// TaskInfo task status (for API responses)
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

// NewTaskManager creates a task manager
func NewTaskManager() *TaskManager {
	return &TaskManager{
		tasks:     make(map[string]*DownloadTask),
		semaphore: make(chan struct{}, maxConcurrent),
	}
}

// Enqueue adds a download task to the queue, returns taskID immediately, executes async
// cleanup is called when the task ends (can be used to delete temp files), may be nil
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

	// Broadcast queued status
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

// run executes the download task in a goroutine
func (tm *TaskManager) run(task *DownloadTask, broadcast func(ytdlp.ProgressData), cleanup func()) {
	if cleanup != nil {
		defer cleanup()
	}

	// Acquire concurrency slot
	tm.semaphore <- struct{}{}
	defer func() { <-tm.semaphore }()

	tm.updateStatus(task.ID, "downloading", 0, 0, 0, 0, "")
	broadcast(ytdlp.ProgressData{
		TaskID: task.ID,
		Status: "starting",
		Title:  task.Info.Title,
	})

	// Execute download
	_ = task.Manager.Download(task.Opts, func(data ytdlp.ProgressData) {
		data.TaskID = task.ID
		data.Title = task.Info.Title
		data.URL = task.Info.URL
		data.SaveDir = task.Info.SaveDir
		broadcast(data)

		// Sync update TaskInfo
		tm.updateStatus(task.ID, data.Status, data.Percent, data.SpeedMBps, data.TotalMB, data.ETASeconds, data.ErrorMessage)
	})
}

// Cancel cancels the specified task
func (tm *TaskManager) Cancel(taskID string) error {
	tm.mu.RLock()
	task, ok := tm.tasks[taskID]
	tm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("Task not found: %s", taskID)
	}
	task.Manager.Cancel()
	return nil
}

// Remove removes the specified task from the list (does not cancel active downloads)
func (tm *TaskManager) Remove(taskID string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if _, ok := tm.tasks[taskID]; !ok {
		return fmt.Errorf("Task not found: %s", taskID)
	}
	delete(tm.tasks, taskID)
	return nil
}

// RemoveBatch batch removes tasks
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

// Get retrieves the specified task
func (tm *TaskManager) Get(taskID string) (*DownloadTask, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	task, ok := tm.tasks[taskID]
	return task, ok
}

// List returns all tasks' TaskInfo list
func (tm *TaskManager) List() []TaskInfo {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	result := make([]TaskInfo, 0, len(tm.tasks))
	for _, t := range tm.tasks {
		result = append(result, t.Info)
	}
	return result
}

// updateStatus updates task status (thread-safe)
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

// genTaskID generates a unique task ID (timestamp base32)
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
