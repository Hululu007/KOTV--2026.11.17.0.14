package download

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bobo/KOTV/internal/paths"
	"github.com/bobo/KOTV/internal/thunder"
)

// PlayResolver 解析可下载地址（由 app 注入，避免循环依赖）。
type PlayResolver interface {
	ResolveDownload(site, vodID, flag, epURL string, epIndex int) (url string, headers map[string]string, format string, err error)
}

// EpisodeSpec 入队的一集。
type EpisodeSpec struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	URL   string `json:"url"`
}

// EnqueueReq 批量入队。
type EnqueueReq struct {
	Site     string        `json:"site"`
	VodID    string        `json:"vodId"`
	Title    string        `json:"title"`
	Flag     string        `json:"flag"`
	Pic      string        `json:"pic"`
	Episodes []EpisodeSpec `json:"episodes"`
}

type Manager struct {
	mu       sync.Mutex
	tasks    []*Task
	byID     map[string]*Task
	cancels  map[string]context.CancelFunc
	resolver PlayResolver
	workers  int
	queue    chan string
	started  bool
}

var defaultMgr = &Manager{
	byID:    map[string]*Task{},
	cancels: map[string]context.CancelFunc{},
	workers: 2,
	queue:   make(chan string, 256),
}

// Default 返回进程级下载管理器。
func Default() *Manager { return defaultMgr }

// SetResolver 注入播放地址解析。
func SetResolver(r PlayResolver) { defaultMgr.resolver = r }

// Start 启动 worker（幂等）。
func Start() { defaultMgr.start() }

func (m *Manager) start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return
	}
	m.started = true
	m.loadLocked()
	n := m.workers
	if n < 1 {
		n = 1
	}
	for i := 0; i < n; i++ {
		go m.loop()
	}
	for _, t := range m.tasks {
		if t.Status == StatusQueued || t.Status == StatusRunning {
			t.Status = StatusQueued
			t.Message = "等待中"
			select {
			case m.queue <- t.ID:
			default:
			}
		}
	}
}

func (m *Manager) loop() {
	for id := range m.queue {
		m.runOne(id)
	}
}

func (m *Manager) runOne(id string) {
	m.mu.Lock()
	t := m.byID[id]
	if t == nil || t.Status == StatusCancelled || t.Status == StatusDone {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancels[id] = cancel
	t.Status = StatusRunning
	t.Message = "开始下载"
	t.UpdatedAt = nowMS()
	snap := *t
	m.persistLocked()
	m.mu.Unlock()

	err := m.execute(ctx, &snap)
	cancel()

	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cancels, id)
	cur := m.byID[id]
	if cur == nil {
		return
	}
	if cur.Status == StatusCancelled {
		m.persistLocked()
		return
	}
	if err != nil {
		cur.Status = StatusError
		cur.Error = err.Error()
		cur.Message = err.Error()
	} else {
		cur.Status = StatusDone
		cur.Progress = 1
		cur.Path = snap.Path
		cur.Bytes = snap.Bytes
		cur.Total = snap.Total
		cur.Message = "完成"
		cur.Error = ""
	}
	cur.UpdatedAt = nowMS()
	m.persistLocked()
}

func (m *Manager) execute(ctx context.Context, t *Task) error {
	onProg := func(p float64, bytes, total int64, msg string) {
		m.mu.Lock()
		defer m.mu.Unlock()
		cur := m.byID[t.ID]
		if cur == nil || cur.Status != StatusRunning {
			return
		}
		cur.Progress = p
		cur.Bytes = bytes
		cur.Total = total
		if msg != "" {
			cur.Message = msg
		}
		cur.UpdatedAt = nowMS()
	}

	epURL := strings.TrimSpace(t.EpURL)
	if thunder.Match(epURL) {
		t.Kind = KindMagnet
		dest := ResolvePath(t.Title, t.Flag, t.EpName, "mp4")
		t.Path = dest
		onProg(0.01, 0, 0, "磁力下载中…")
		err := thunder.SaveToFile(ctx, epURL, dest, func(bytes, total int64, msg string) {
			p := 0.0
			if total > 0 {
				p = float64(bytes) / float64(total)
			}
			onProg(p, bytes, total, msg)
			t.Bytes, t.Total = bytes, total
		})
		if err != nil {
			return err
		}
		if st, e := os.Stat(dest); e == nil {
			t.Bytes, t.Total = st.Size(), st.Size()
		}
		return nil
	}

	if m.resolver == nil {
		return fmt.Errorf("下载解析器未就绪")
	}
	playURL, headers, format, err := m.resolver.ResolveDownload(t.Site, t.VodID, t.Flag, epURL, t.EpIndex)
	if err != nil {
		return err
	}
	playURL = strings.TrimSpace(playURL)
	if playURL == "" {
		return fmt.Errorf("未解析到可下载地址")
	}
	if thunder.Match(playURL) || thunder.IsLocalStream(playURL) {
		// 解析结果又变磁力：走 thunder；本地流地址则 HTTP 拉满。
		if thunder.Match(playURL) {
			t.Kind = KindMagnet
			dest := ResolvePath(t.Title, t.Flag, t.EpName, "mp4")
			t.Path = dest
			return thunder.SaveToFile(ctx, playURL, dest, func(bytes, total int64, msg string) {
				p := 0.0
				if total > 0 {
					p = float64(bytes) / float64(total)
				}
				onProg(p, bytes, total, msg)
				t.Bytes, t.Total = bytes, total
			})
		}
	}

	if isHLSURL(playURL, format) {
		t.Kind = KindHLS
		dest := ResolvePath(t.Title, t.Flag, t.EpName, "mp4")
		t.Path = dest
		err := downloadHLS(ctx, playURL, headers, dest, func(p float64, bytes, total int64, msg string) {
			onProg(p, bytes, total, msg)
			t.Bytes, t.Total = bytes, total
		})
		return err
	}

	t.Kind = KindHTTP
	ext := extFromURL(playURL, "mp4")
	dest := ResolvePath(t.Title, t.Flag, t.EpName, ext)
	t.Path = dest
	n, total, err := downloadHTTP(ctx, playURL, headers, dest, func(bytes, total int64) {
		p := 0.0
		if total > 0 {
			p = float64(bytes) / float64(total)
		}
		onProg(p, bytes, total, "下载中")
		t.Bytes, t.Total = bytes, total
	})
	if err != nil {
		return err
	}
	t.Bytes, t.Total = n, total
	return nil
}

func downloadHTTP(ctx context.Context, raw string, headers map[string]string, dest string, onProg func(bytes, total int64)) (int64, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return 0, 0, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 KOTV")
	}
	cli := &http.Client{Timeout: 0}
	resp, err := cli.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	total := resp.ContentLength
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, 0, err
	}
	var written int64
	buf := make([]byte, 256*1024)
	for {
		if ctx.Err() != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return written, total, ctx.Err()
		}
		nr, er := resp.Body.Read(buf)
		if nr > 0 {
			nw, ew := f.Write(buf[:nr])
			written += int64(nw)
			if onProg != nil {
				onProg(written, total)
			}
			if ew != nil {
				_ = f.Close()
				_ = os.Remove(tmp)
				return written, total, ew
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return written, total, er
		}
	}
	_ = f.Close()
	_ = os.Remove(dest)
	if err := os.Rename(tmp, dest); err != nil {
		return written, total, err
	}
	return written, total, nil
}

// Enqueue 批量入队，返回新建任务。
func Enqueue(req EnqueueReq) ([]*Task, error) {
	m := defaultMgr
	m.start()
	if len(req.Episodes) == 0 {
		return nil, fmt.Errorf("未选择剧集")
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "未命名"
	}
	flag := strings.TrimSpace(req.Flag)
	out := make([]*Task, 0, len(req.Episodes))
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ep := range req.Episodes {
		name := strings.TrimSpace(ep.Name)
		if name == "" {
			name = fmt.Sprintf("%d", ep.Index+1)
		}
		kind := KindHTTP
		if thunder.Match(ep.URL) {
			kind = KindMagnet
		}
		t := &Task{
			ID:        newID(),
			Site:      req.Site,
			VodID:     req.VodID,
			Title:     title,
			Flag:      flag,
			EpIndex:   ep.Index,
			EpName:    name,
			EpURL:     ep.URL,
			Pic:       req.Pic,
			Kind:      kind,
			Status:    StatusQueued,
			Message:   "排队中",
			CreatedAt: nowMS(),
			UpdatedAt: nowMS(),
		}
		m.tasks = append(m.tasks, t)
		m.byID[t.ID] = t
		out = append(out, cloneTask(t))
		select {
		case m.queue <- t.ID:
		default:
		}
	}
	m.persistLocked()
	return out, nil
}

// List 返回任务快照（新在前）。
func List() []*Task {
	m := defaultMgr
	m.start()
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Task, 0, len(m.tasks))
	for i := len(m.tasks) - 1; i >= 0; i-- {
		out = append(out, cloneTask(m.tasks[i]))
	}
	return out
}

// Cancel 取消排队或运行中任务。
func Cancel(id string) error {
	m := defaultMgr
	m.start()
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.byID[id]
	if t == nil {
		return fmt.Errorf("任务不存在")
	}
	if t.Status == StatusDone || t.Status == StatusCancelled {
		return nil
	}
	t.Status = StatusCancelled
	t.Message = "已取消"
	t.UpdatedAt = nowMS()
	if c := m.cancels[id]; c != nil {
		c()
	}
	m.persistLocked()
	return nil
}

// Remove 删除任务记录（可选删文件）。
func Remove(id string, deleteFile bool) error {
	m := defaultMgr
	m.start()
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.byID[id]
	if t == nil {
		return fmt.Errorf("任务不存在")
	}
	if c := m.cancels[id]; c != nil {
		c()
	}
	path := t.Path
	delete(m.byID, id)
	for i, x := range m.tasks {
		if x.ID == id {
			m.tasks = append(m.tasks[:i], m.tasks[i+1:]...)
			break
		}
	}
	m.persistLocked()
	if deleteFile && path != "" {
		_ = os.Remove(path)
	}
	return nil
}

// ClearFinished 清除已完成/失败/取消。
func ClearFinished() int {
	m := defaultMgr
	m.start()
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	kept := m.tasks[:0]
	for _, t := range m.tasks {
		if t.Status == StatusDone || t.Status == StatusError || t.Status == StatusCancelled {
			delete(m.byID, t.ID)
			n++
			continue
		}
		kept = append(kept, t)
	}
	m.tasks = kept
	m.persistLocked()
	return n
}

func cloneTask(t *Task) *Task {
	c := *t
	return &c
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:]) + fmt.Sprintf("%d", time.Now().UnixNano()%1e6)
}

func storePath() string {
	return filepath.Join(paths.Ensure(filepath.Join(paths.Root(), "download")), "tasks.json")
}

func (m *Manager) persistLocked() {
	type file struct {
		Tasks []*Task `json:"tasks"`
	}
	b, err := json.MarshalIndent(file{Tasks: m.tasks}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(storePath(), b, 0o644)
}

func (m *Manager) loadLocked() {
	b, err := os.ReadFile(storePath())
	if err != nil {
		return
	}
	var f struct {
		Tasks []*Task `json:"tasks"`
	}
	if json.Unmarshal(b, &f) != nil {
		return
	}
	m.tasks = nil
	m.byID = map[string]*Task{}
	for _, t := range f.Tasks {
		if t == nil || t.ID == "" {
			continue
		}
		m.tasks = append(m.tasks, t)
		m.byID[t.ID] = t
	}
}
