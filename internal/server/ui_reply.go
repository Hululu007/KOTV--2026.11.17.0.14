package server

import (
	"strings"
	"sync"
)

// uiReplyStore 按会话 id 排队保存宿主回传；支持 shown / 业务事件 / closed 连续握手。
type uiReplyStore struct {
	mu sync.Mutex
	m  map[string][]string
}

func newUIReplyStore() *uiReplyStore {
	return &uiReplyStore{m: map[string][]string{}}
}

func (s *uiReplyStore) put(id, value string) {
	id = strings.TrimSpace(id)
	if id == "" || value == "" {
		return
	}
	s.mu.Lock()
	s.m[id] = append(s.m[id], value)
	s.mu.Unlock()
}

func (s *uiReplyStore) drainAll() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.m) == 0 {
		return nil
	}
	var out []string
	for _, q := range s.m {
		out = append(out, q...)
	}
	s.m = map[string][]string{}
	return out
}
