package util

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// RedirectOrigins 记录 302 Location（绝对地址）对应的发起地址，两套客户端各持一份。
type RedirectOrigins struct {
	mu  sync.Mutex
	m   map[string]string
	ord []string
	max int
}

func NewRedirectOrigins(max int) *RedirectOrigins {
	return &RedirectOrigins{m: map[string]string{}, max: max}
}

func (r *RedirectOrigins) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m = map[string]string{}
	r.ord = nil
}

func (r *RedirectOrigins) Remember(from, location string) {
	from = strings.TrimSpace(from)
	location = strings.TrimSpace(location)
	if from == "" || location == "" {
		return
	}
	abs := AbsHTTPRef(from, location)
	if abs == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.m[abs]; !ok {
		r.ord = append(r.ord, abs)
	}
	r.m[abs] = from
	for len(r.m) > r.max && len(r.ord) > 0 {
		victim := r.ord[0]
		r.ord = r.ord[1:]
		delete(r.m, victim)
	}
}

func (r *RedirectOrigins) Lookup(requestURL string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.m[strings.TrimSpace(requestURL)]
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

func (r *RedirectOrigins) Track(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return http.ErrUseLastResponse
	}
	if len(via) > 0 && req != nil && req.URL != nil {
		prev := via[len(via)-1]
		if prev != nil && prev.URL != nil {
			r.Remember(prev.URL.String(), req.URL.String())
		}
	}
	return nil
}

// AbsHTTPRef 把相对地址补成绝对 http(s)。已是 http(s) 或解析失败时原样返回。
func AbsHTTPRef(base, ref string) string {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	bu, err := url.Parse(base)
	if err != nil {
		return ref
	}
	ru, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return bu.ResolveReference(ru).String()
}

func IsHTTPRedirect(code int) bool {
	switch code {
	case 301, 302, 303, 307, 308:
		return true
	default:
		return false
	}
}
