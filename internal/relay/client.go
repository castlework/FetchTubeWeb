// Package relay — VPS 中继 HTTP 客户端
package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// Client VPS 中继 HTTP 客户端
type Client struct {
	host    string
	port    int
	timeout time.Duration
}

// NewClient 创建新的中继客户端
func NewClient(host string, port int) *Client {
	return &Client{
		host:    host,
		port:    port,
		timeout: 10 * time.Second,
	}
}

func (c *Client) baseURL() string {
	return fmt.Sprintf("http://%s:%d", c.host, c.port)
}

func (c *Client) httpClient() *http.Client {
	return &http.Client{Timeout: c.timeout}
}

// TestConnection 测试连接
func (c *Client) TestConnection() (bool, error) {
	data, err := c.get("/api/health")
	if err != nil {
		return false, fmt.Errorf("无法连接到服务器 %s:%d", c.host, c.port)
	}
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	return result["status"] == "ok", nil
}

// SubmitDownload 下发下载任务
func (c *Client) SubmitDownload(videoURL, formatID, outputExt string) (map[string]interface{}, error) {
	data, err := c.post("/api/download", map[string]string{
		"url":        videoURL,
		"format_id":  formatID,
		"output_ext": outputExt,
	})
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	return result, nil
}

// ListTasks 获取任务列表
func (c *Client) ListTasks() ([]map[string]interface{}, error) {
	data, err := c.get("/api/tasks")
	if err != nil {
		return nil, err
	}
	var tasks []map[string]interface{}
	json.Unmarshal(data, &tasks)
	if tasks == nil {
		tasks = []map[string]interface{}{}
	}
	return tasks, nil
}

// ListFiles 获取文件列表
func (c *Client) ListFiles() ([]map[string]interface{}, error) {
	data, err := c.get("/api/files")
	if err != nil {
		return nil, err
	}
	var files []map[string]interface{}
	json.Unmarshal(data, &files)
	if files == nil {
		files = []map[string]interface{}{}
	}
	return files, nil
}

// DownloadFile 从 VPS 下载文件到本地
func (c *Client) DownloadFile(filename, saveDir string, progressCallback func(float64, float64, float64)) (string, error) {
	savePath := filepath.Join(saveDir, filename)

	req, err := http.NewRequest("GET",
		c.baseURL()+"/api/files/"+url.PathEscape(filename)+"/dl", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("下载请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("服务器返回 %d: %s", resp.StatusCode, string(body))
	}

	f, err := os.Create(savePath)
	if err != nil {
		return "", fmt.Errorf("创建文件失败: %w", err)
	}
	defer f.Close()

	contentLength := resp.ContentLength
	downloaded := int64(0)
	start := time.Now()
	buf := make([]byte, 64*1024)

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return "", err
			}
			downloaded += int64(n)

			if progressCallback != nil && contentLength > 0 {
				elapsed := time.Since(start).Seconds()
				if elapsed > 0 {
					speedMBps := (float64(downloaded) / (1024 * 1024)) / elapsed
					downloadedMB := float64(downloaded) / (1024 * 1024)
					totalMB := float64(contentLength) / (1024 * 1024)
					progressCallback(downloadedMB, totalMB, speedMBps)
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}

	return savePath, nil
}

// DeleteFile 删除 VPS 上的文件
func (c *Client) DeleteFile(filename string) error {
	req, err := http.NewRequest("DELETE",
		c.baseURL()+"/api/files/"+url.PathEscape(filename), nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("服务器返回 %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// ---- 内部方法 ----

func (c *Client) get(path string) ([]byte, error) {
	resp, err := c.httpClient().Get(c.baseURL() + path)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("服务器错误 (%d): %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (c *Client) post(path string, data interface{}) ([]byte, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient().Post(c.baseURL()+path, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("服务器错误 (%d): %s", resp.StatusCode, string(body))
	}

	return body, nil
}
