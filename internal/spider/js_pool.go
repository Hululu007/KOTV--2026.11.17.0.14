package spider

import (
	"context"

	"github.com/bobo/KOTV/internal/hostclient"
)

// jsPool 同站 QuickJS Runtime 池：每个 slot 独立 Runtime；跨租户靠 jsPyKey 分桶。
type jsPool struct {
	key, api, ext, jar string
	scriptSlotPool[*jsSpider]
}

func newJsPool(key, api, ext, jar string) *jsPool {
	p := &jsPool{key: key, api: api, ext: ext, jar: jar}
	p.max = scriptPoolSize()
	p.spawn = func(int) *jsSpider {
		return newJsSpiderWorker(p.key, p.api, p.ext, p.jar)
	}
	return p
}

func (p *jsPool) retarget(key, ext, jar string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed.Load() {
		return
	}
	p.key, p.ext, p.jar = key, ext, jar
	for _, s := range p.slots {
		s.siteMu.Lock()
		s.key, s.ext, s.jar = key, ext, jar
		s.siteMu.Unlock()
	}
}

func newJsSpiderWorker(key, api, ext, jar string) *jsSpider {
	return &jsSpider{
		key:      key,
		api:      api,
		ext:      ext,
		jar:      jar,
		ownerUID: hostclient.RuntimeUserID(),
		reqCh:    make(chan jsReq, 8),
		quitCh:   make(chan struct{}),
		timers:   map[int32]context.CancelFunc{},
	}
}

// withStickySlot 同 slot 连续跑多段（如 home→homeVod），避免二次 acquire 冷启新 Runtime。
func (p *jsPool) withStickySlot(fn func(s *jsSpider) error) error {
	_, err := withPooled(&p.scriptSlotPool, func(s *jsSpider) (string, error) {
		return "", fn(s)
	})
	return err
}

func (p *jsPool) Init(ext string) error {
	p.ext = ext
	_, err := withPooled(&p.scriptSlotPool, func(s *jsSpider) (string, error) {
		s.ext = ext
		return "", s.Init(ext)
	})
	return err
}

// HomeBundle 同 slot 串行 home + homeVod（首页专用）。
func (p *jsPool) HomeBundle(filter bool) (home string, homeVod string, err error) {
	err = p.withStickySlot(func(s *jsSpider) error {
		var e error
		home, e = s.HomeContent(filter)
		if e != nil {
			return e
		}
		homeVod, e = s.HomeVideoContent()
		return e
	})
	return home, homeVod, err
}

func (p *jsPool) HomeContent(filter bool) (string, error) {
	return withPooled(&p.scriptSlotPool, func(s *jsSpider) (string, error) { return s.HomeContent(filter) })
}
func (p *jsPool) HomeVideoContent() (string, error) {
	return withPooled(&p.scriptSlotPool, func(s *jsSpider) (string, error) { return s.HomeVideoContent() })
}
func (p *jsPool) CategoryContent(tid, pg string, filter bool, extend map[string]string) (string, error) {
	return withPooled(&p.scriptSlotPool, func(s *jsSpider) (string, error) {
		return s.CategoryContent(tid, pg, filter, extend)
	})
}
func (p *jsPool) DetailContent(ids []string) (string, error) {
	return withPooled(&p.scriptSlotPool, func(s *jsSpider) (string, error) { return s.DetailContent(ids) })
}
func (p *jsPool) SearchContent(key string, quick bool, pg string) (string, error) {
	return withPooled(&p.scriptSlotPool, func(s *jsSpider) (string, error) { return s.SearchContent(key, quick, pg) })
}
func (p *jsPool) PlayerContent(flag, id string, vipFlags []string) (string, error) {
	return withPooled(&p.scriptSlotPool, func(s *jsSpider) (string, error) { return s.PlayerContent(flag, id, vipFlags) })
}
func (p *jsPool) LiveContent(url string) (string, error) {
	return withPooled(&p.scriptSlotPool, func(s *jsSpider) (string, error) { return s.LiveContent(url) })
}
func (p *jsPool) Proxy(params map[string]string) (int, string, []byte, map[string]string, error) {
	s, release, err := borrowSlot(&p.scriptSlotPool)
	if err != nil {
		return 0, "", nil, nil, err
	}
	defer release()
	return s.Proxy(params)
}
func (p *jsPool) Action(action string) (string, error) {
	return withPooled(&p.scriptSlotPool, func(s *jsSpider) (string, error) { return s.Action(action) })
}
func (p *jsPool) ManualVideoCheck() (bool, error) {
	return withPooled(&p.scriptSlotPool, func(s *jsSpider) (bool, error) { return s.ManualVideoCheck() })
}
func (p *jsPool) IsVideoFormat(u string) (bool, error) {
	return withPooled(&p.scriptSlotPool, func(s *jsSpider) (bool, error) { return s.IsVideoFormat(u) })
}
func (p *jsPool) Destroy() {
	p.shutdown(func(s *jsSpider) { s.Destroy() })
}
