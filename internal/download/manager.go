package download

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	running  int
	wake     chan struct{}
	started  bool
}

var defaultMgr = &Manager{
	byID:    map[string]*Task{},
	cancels: map[string]context.CancelFunc{},
	wake:    make(chan struct{}, 8),
}

// Default 返回进程级下载管理器。
func Default() *Manager { return defaultMgr }

// SetResolver 注入播放地址解析。
func SetResolver(r PlayResolver) { defaultMgr.resolver = r }

// Start 启动调度（幂等）。
func Start() { defaultMgr.start() }

func (m *Manager) start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return
	}
	m.started = true
	m.loadLocked()
	for _, t := range m.tasks {
		if t.Status == StatusRunning {
			t.Status = StatusQueued
			t.Message = "等待中"
		}
	}
	m.persistLocked()
	go m.dispatcher()
	m.signalLocked()
}

func (m *Manager) signalLocked() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Manager) signal() {
	m.mu.Lock()
	m.signalLocked()
	m.mu.Unlock()
}

func (m *Manager) dispatcher() {
	for range m.wake {
		for m.tryStartOne() {
		}
	}
}

func (m *Manager) tryStartOne() bool {
	m.mu.Lock()
	limit := concurrencyLimit()
	if m.running >= limit {
		m.mu.Unlock()
		return false
	}
	var id string
	for _, t := range m.tasks {
		if t.Status == StatusQueued {
			id = t.ID
			t.Status = StatusRunning
			t.Message = "开始下载"
			t.UpdatedAt = nowMS()
			m.running++
			m.persistLocked()
			break
		}
	}
	m.mu.Unlock()
	if id == "" {
		return false
	}
	go m.runOne(id)
	return true
}

func (m *Manager) runOne(id string) {
	m.mu.Lock()
	t := m.byID[id]
	if t == nil || t.Status != StatusRunning {
		m.running--
		if m.running < 0 {
			m.running = 0
		}
		m.signalLocked()
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancels[id] = cancel
	snap := *t
	m.mu.Unlock()

	err := m.execute(ctx, &snap)
	cancel()

	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cancels, id)
	m.running--
	if m.running < 0 {
		m.running = 0
	}
	cur := m.byID[id]
	if cur == nil {
		m.signalLocked()
		return
	}
	switch cur.Status {
	case StatusCancelled, StatusPaused:
		cur.Progress = snap.Progress
		cur.Bytes = snap.Bytes
		cur.Total = snap.Total
		cur.Path = snap.Path
		if cur.Status == StatusPaused {
			cur.Message = "已暂停"
		}
		cur.UpdatedAt = nowMS()
		m.persistLocked()
		m.signalLocked()
		return
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			// 外部取消已改状态；否则记失败。
			if cur.Status == StatusRunning {
				cur.Status = StatusError
				cur.Error = "已中断"
				cur.Message = "已中断"
			}
		} else {
			cur.Status = StatusError
			cur.Error = err.Error()
			cur.Message = err.Error()
		}
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
	m.signalLocked()
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
		t.Progress, t.Bytes, t.Total = p, bytes, total
	}
	threads := threadLimit()

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
		})
	}

	if isHLSURL(playURL, format) {
		t.Kind = KindHLS
		dest := ResolvePath(t.Title, t.Flag, t.EpName, "mp4")
		t.Path = dest
		return downloadHLS(ctx, playURL, headers, dest, threads, onProg)
	}

	t.Kind = KindHTTP
	ext := extFromURL(playURL, "mp4")
	dest := ResolvePath(t.Title, t.Flag, t.EpName, ext)
	t.Path = dest
	n, total, err := downloadHTTP(ctx, playURL, headers, dest, threads, func(bytes, total int64) {
		p := 0.0
		if total > 0 {
			p = float64(bytes) / float64(total)
		}
		onProg(p, bytes, total, "下载中")
	})
	if err != nil {
		return err
	}
	t.Bytes, t.Total = n, total
	return nil
}

func (m *Manager) hasEpisodeLocked(site, vodID, flag string, epIndex int) bool {
	for _, t := range m.tasks {
		if t == nil {
			continue
		}
		if t.Site == site && t.VodID == vodID && t.Flag == flag && t.EpIndex == epIndex {
			return true
		}
	}
	return false
}

// Enqueue 批量入队；同站同片同线路同集若已有任务则跳过（删记录后才可再入队）。
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
	site := strings.TrimSpace(req.Site)
	vodID := strings.TrimSpace(req.VodID)
	flag := strings.TrimSpace(req.Flag)
	out := make([]*Task, 0, len(req.Episodes))
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ep := range req.Episodes {
		if m.hasEpisodeLocked(site, vodID, flag, ep.Index) {
			continue
		}
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
			Site:      site,
			VodID:     vodID,
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
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("所选剧集已在下载列表中")
	}
	m.persistLocked()
	m.signalLocked()
	return out, nil
}

// List 按状态优先级排序后返回快照。
func List() []*Task {
	m := defaultMgr
	m.start()
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		out = append(out, cloneTask(t))
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := statusRank(out[i].Status), statusRank(out[j].Status)
		if ri != rj {
			return ri < rj
		}
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out
}

// Pause 暂停排队或运行中任务。
func Pause(id string) error {
	m := defaultMgr
	m.start()
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.byID[id]
	if t == nil {
		return fmt.Errorf("任务不存在")
	}
	switch t.Status {
	case StatusDone, StatusCancelled, StatusPaused:
		return nil
	case StatusQueued:
		t.Status = StatusPaused
		t.Message = "已暂停"
		t.UpdatedAt = nowMS()
		m.persistLocked()
		return nil
	case StatusRunning:
		t.Status = StatusPaused
		t.Message = "正在暂停…"
		t.UpdatedAt = nowMS()
		if c := m.cancels[id]; c != nil {
			c()
		}
		m.persistLocked()
		return nil
	default:
		t.Status = StatusPaused
		t.Message = "已暂停"
		t.UpdatedAt = nowMS()
		m.persistLocked()
		return nil
	}
}

// Resume 继续暂停或失败任务。
func Resume(id string) error {
	m := defaultMgr
	m.start()
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.byID[id]
	if t == nil {
		return fmt.Errorf("任务不存在")
	}
	switch t.Status {
	case StatusPaused, StatusError, StatusCancelled:
		t.Status = StatusQueued
		t.Error = ""
		t.Message = "排队中"
		t.UpdatedAt = nowMS()
		m.persistLocked()
		m.signalLocked()
		return nil
	default:
		return nil
	}
}

// PauseAll 暂停全部可暂停任务。
func PauseAll() int {
	m := defaultMgr
	m.start()
	ids := []string{}
	m.mu.Lock()
	for _, t := range m.tasks {
		if t.Status == StatusQueued || t.Status == StatusRunning {
			ids = append(ids, t.ID)
		}
	}
	m.mu.Unlock()
	n := 0
	for _, id := range ids {
		if Pause(id) == nil {
			n++
		}
	}
	return n
}

// ResumeAll 恢复全部暂停/失败。
func ResumeAll() int {
	m := defaultMgr
	m.start()
	ids := []string{}
	m.mu.Lock()
	for _, t := range m.tasks {
		if t.Status == StatusPaused || t.Status == StatusError || t.Status == StatusCancelled {
			ids = append(ids, t.ID)
		}
	}
	m.mu.Unlock()
	n := 0
	for _, id := range ids {
		if Resume(id) == nil {
			n++
		}
	}
	return n
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
	m.signalLocked()
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
	m.signalLocked()
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
