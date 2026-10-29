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
	// Owner 发起下载的远端用户 id，本机为空；OwnerName 是其用户名，远端下载落在下载目录下以它命名的子目录。
	Owner     string `json:"owner,omitempty"`
	OwnerName string `json:"ownerName,omitempty"`
}

// destPath 这一集的落盘路径。
func (t *Task) destPath(ext string) string {
	return ResolvePath(t.OwnerName, t.Title, t.Flag, t.EpName, ext)
}

// Viewer 操作下载列表的身份：All 看全部（本机、管理员），否则只看 Owner 自己的任务。
type Viewer struct {
	Owner string
	All   bool
}

func (v Viewer) sees(t *Task) bool {
	return t != nil && (v.All || t.Owner == v.Owner)
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
