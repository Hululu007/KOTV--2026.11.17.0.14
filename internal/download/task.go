package download

import "time"

// 任务状态。
const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusPaused    = "paused"
	StatusDone      = "done"
	StatusError     = "error"
	StatusCancelled = "cancelled"
)

// Kind 下载类型。
const (
	KindHTTP   = "http"
	KindHLS    = "hls"
	KindMagnet = "magnet"
)

// Task 一条下载任务。
type Task struct {
	ID        string  `json:"id"`
	Site      string  `json:"site"`
	VodID     string  `json:"vodId"`
	Title     string  `json:"title"`
	Flag      string  `json:"flag"`
	EpIndex   int     `json:"epIndex"`
	EpName    string  `json:"epName"`
	EpURL     string  `json:"epUrl"`
	Pic       string  `json:"pic,omitempty"`
	Kind      string  `json:"kind"`
	Status    string  `json:"status"`
	Progress  float64 `json:"progress"` // 0..1
	Bytes     int64   `json:"bytes"`
	Total     int64   `json:"total"`
	Path      string  `json:"path,omitempty"`
	Error     string  `json:"error,omitempty"`
	Message   string  `json:"message,omitempty"`
	CreatedAt int64   `json:"createdAt"`
	UpdatedAt int64   `json:"updatedAt"`
}

func nowMS() int64 { return time.Now().UnixMilli() }

// statusRank 越小越靠前：下载中 > 等待 > 暂停 > 失败 > 取消 > 完成。
func statusRank(s string) int {
	switch s {
	case StatusRunning:
		return 0
	case StatusQueued:
		return 1
	case StatusPaused:
		return 2
	case StatusError:
		return 3
	case StatusCancelled:
		return 4
	case StatusDone:
		return 5
	default:
		return 9
	}
}
