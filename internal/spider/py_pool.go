package spider

import (
	"strings"
)

// pyPool 同站 Python 进程池：每个 slot 一个常驻 pySpider，多用户可并行打同一站。
type pyPool struct {
	key, api, ext, jar string
	scriptSlotPool[*pySpider]
}

func newPyPool(key, api, ext, jar string) *pyPool {
	p := &pyPool{key: key, api: api, ext: ext, jar: jar}
	p.max = scriptPoolSize()
	p.spawn = func(index int) *pySpider {
		return &pySpider{key: p.key, api: p.api, ext: p.ext, jar: p.jar, sessionSlot: index}
	}
	return p
}

func (p *pyPool) retarget(key, ext, jar string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed.Load() {
		return
	}
	p.key, p.ext, p.jar = key, ext, jar
	for _, s := range p.slots {
		s.mu.Lock()
		if s.key != key || s.ext != ext || s.jar != jar {
			s.key, s.ext, s.jar = key, ext, jar
			s.inited = false
		}
		s.mu.Unlock()
	}
}

func (p *pyPool) matchesClient(clientID string) bool {
	return pooledMatchesClient(&p.scriptSlotPool, clientID)
}

func (p *pyPool) activeClientIDs() []string {
	slots := p.snapshot()
	var out []string
	for _, s := range slots {
		if s == nil {
			continue
		}
		if id := strings.TrimSpace(s.activeClientID()); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func (p *pyPool) interrupt() {
	pooledInterrupt(&p.scriptSlotPool)
}

func (p *pyPool) Init(ext string) error {
	p.ext = ext
	_, err := withPooled(&p.scriptSlotPool, func(s *pySpider) (string, error) {
		s.ext = ext
		return "", s.Init(ext)
	})
	return err
}

func (p *pyPool) HomeContent(filter bool) (string, error) {
	return withPooled(&p.scriptSlotPool, func(s *pySpider) (string, error) { return s.HomeContent(filter) })
}
func (p *pyPool) HomeVideoContent() (string, error) {
	return withPooled(&p.scriptSlotPool, func(s *pySpider) (string, error) { return s.HomeVideoContent() })
}
func (p *pyPool) CategoryContent(tid, pg string, filter bool, extend map[string]string) (string, error) {
	return withPooled(&p.scriptSlotPool, func(s *pySpider) (string, error) {
		return s.CategoryContent(tid, pg, filter, extend)
	})
}
func (p *pyPool) DetailContent(ids []string) (string, error) {
	return withPooled(&p.scriptSlotPool, func(s *pySpider) (string, error) { return s.DetailContent(ids) })
}
func (p *pyPool) SearchContent(key string, quick bool, pg string) (string, error) {
	return withPooled(&p.scriptSlotPool, func(s *pySpider) (string, error) { return s.SearchContent(key, quick, pg) })
}
func (p *pyPool) PlayerContent(flag, id string, vipFlags []string) (string, error) {
	return withPooled(&p.scriptSlotPool, func(s *pySpider) (string, error) {
		return s.PlayerContent(flag, id, vipFlags)
	})
}
func (p *pyPool) LiveContent(url string) (string, error) {
	return withPooled(&p.scriptSlotPool, func(s *pySpider) (string, error) { return s.LiveContent(url) })
}
func (p *pyPool) Proxy(params map[string]string) (int, string, []byte, map[string]string, error) {
	s, idx, err := p.acquire()
	if err != nil {
		return 0, "", nil, nil, err
	}
	defer p.release(idx)
	return s.Proxy(params)
}
func (p *pyPool) Action(action string) (string, error) {
	return withPooled(&p.scriptSlotPool, func(s *pySpider) (string, error) { return s.Action(action) })
}
func (p *pyPool) ManualVideoCheck() (bool, error) {
	return withPooled(&p.scriptSlotPool, func(s *pySpider) (bool, error) { return s.ManualVideoCheck() })
}
func (p *pyPool) IsVideoFormat(u string) (bool, error) {
	return withPooled(&p.scriptSlotPool, func(s *pySpider) (bool, error) { return s.IsVideoFormat(u) })
}
func (p *pyPool) Destroy() {
	p.shutdown(func(s *pySpider) { s.Destroy() })
}
