package clientsession

import "strings"

// 会话离开时清理「本会话新写入共享列表」的仓/直播 URL。
// NoteRepo / NoteLive 为对外薄封装，内部同一套 baseline/added 逻辑。

// SetRepoBaseline 记下会话创建时已经在库里的仓库地址。之后再写入的才算本会话新增。
func (s *Session) SetRepoBaseline(urls map[string]struct{}) {
	s.setSourceBaseline(true, urls)
}

// NoteRepo 记录本会话新写入、且创建时还不存在的仓库地址。
func (s *Session) NoteRepo(url string) {
	s.noteSourceURL(true, url)
}

// AddedRepos 返回本会话新增的仓库地址副本。
func (s *Session) AddedRepos() map[string]struct{} {
	return s.addedSourceURLs(true)
}

// SetLiveBaseline 记下会话创建时已在库里的直播源地址。之后再写入的才算本会话新增。
func (s *Session) SetLiveBaseline(urls map[string]struct{}) {
	s.setSourceBaseline(false, urls)
}

// NoteLive 记录本会话新写入、且创建时还不存在的直播源地址。
func (s *Session) NoteLive(url string) {
	s.noteSourceURL(false, url)
}

// AddedLives 返回本会话新增的直播源地址副本。
func (s *Session) AddedLives() map[string]struct{} {
	return s.addedSourceURLs(false)
}

func (s *Session) setSourceBaseline(repo bool, urls map[string]struct{}) {
	if s == nil {
		return
	}
	if repo {
		s.repoMu.Lock()
		defer s.repoMu.Unlock()
		s.repoAtStart = urls
		if s.addedRepos == nil {
			s.addedRepos = map[string]struct{}{}
		}
		return
	}
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	s.liveAtStart = urls
	if s.addedLives == nil {
		s.addedLives = map[string]struct{}{}
	}
}

func (s *Session) noteSourceURL(repo bool, url string) {
	if s == nil {
		return
	}
	url = strings.TrimSpace(url)
	if url == "" {
		return
	}
	if repo {
		s.repoMu.Lock()
		defer s.repoMu.Unlock()
		if _, ok := s.repoAtStart[url]; ok {
			return
		}
		if s.addedRepos == nil {
			s.addedRepos = map[string]struct{}{}
		}
		s.addedRepos[url] = struct{}{}
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

func (s *Session) addedSourceURLs(repo bool) map[string]struct{} {
	if s == nil {
		return nil
	}
	if repo {
		s.repoMu.Lock()
		defer s.repoMu.Unlock()
		out := make(map[string]struct{}, len(s.addedRepos))
		for u := range s.addedRepos {
			out[u] = struct{}{}
		}
		return out
	}
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	out := make(map[string]struct{}, len(s.addedLives))
	for u := range s.addedLives {
		out[u] = struct{}{}
	}
	return out
}

// URLsAddedBySession 挑出离开时应删除的地址：本会话新增、不是宿主当前源、别的会话也没记下。
func URLsAddedBySession(added map[string]struct{}, keep string, stillHeld map[string]struct{}) []string {
	keep = strings.TrimSpace(keep)
	var out []string
	for u := range added {
		u = strings.TrimSpace(u)
		if u == "" || u == keep {
			continue
		}
		if _, ok := stillHeld[u]; ok {
			continue
		}
		out = append(out, u)
	}
	return out
}
