package util

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bobo/KOTV/internal/paths"
)

// AppendLog 同时打到 stderr 和 LogDir 下的追加文件。
type AppendLog struct {
	mu     sync.Mutex
	file   *os.File
	Name   string
	Opened string
}

func (l *AppendLog) Printf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Print(msg)
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := l.openLocked()
	if err != nil || f == nil {
		return
	}
	_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05.000"), msg)
	_ = f.Sync()
}

func (l *AppendLog) openLocked() (*os.File, error) {
	if l.file != nil {
		return l.file, nil
	}
	dir := paths.LogDir()
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, l.Name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	l.file = f
	_, _ = fmt.Fprintf(f, "---- %s %s ----\n", l.Opened, time.Now().Format(time.RFC3339))
	return f, nil
}

// Preview 截断诊断用的长字符串。max 小于等于 0 时按 240。
func Preview(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 {
		max = 240
	}
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("…(%d bytes)", len(s))
}
