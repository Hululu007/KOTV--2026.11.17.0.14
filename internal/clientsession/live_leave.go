package clientsession

import "strings"

// SetLiveBaseline 记下会话创建时已在库里的直播源地址。之后再写入的才算本会话新增。
func (s *Session) SetLiveBaseline(urls map[string]struct{}) {
	if s == nil {
		return
	}
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	s.liveAtStart = urls
	if s.addedLives == nil {
		s.addedLives = map[string]struct{}{}
	}
}

// NoteLive 记录本会话新写入、且创建时还不存在的直播源地址。
func (s *Session) NoteLive(url string) {
	if s == nil {
		return
	}
	url = strings.TrimSpace(url)
	if url == "" {
		return
	}
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	if _, ok := s.liveAtStart[url]; ok {
		return
	}
	if s.addedLives == nil {
		s.addedLives = map[string]struct{}{}
	}
	s.addedLives[url] = struct{}{}
}

// AddedLives 返回本会话新增的直播源地址副本。
func (s *Session) AddedLives() map[string]struct{} {
	if s == nil {
		return nil
	}
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	out := make(map[string]struct{}, len(s.addedLives))
	for u := range s.addedLives {
		out[u] = struct{}{}
	}
	return out
}
