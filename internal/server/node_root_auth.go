package server

import (
	"net/http"
	"strings"

	"github.com/bobo/KOTV/internal/auth"
	"github.com/bobo/KOTV/internal/spider"
)

// nodeRootAuthCookie 根别名页内绝对路径 fetch 用的会话 Cookie（Path=/<别名>）。
const nodeRootAuthCookie = "kotv_node_root_auth"

// requestToken 登录令牌：Authorization: Bearer；
// /ui、/node/…、已登记根别名也可带 ?token=；根别名还可读 Cookie。
func requestToken(r *http.Request) string {
	if tok := auth.BearerFromHeader(r.Header.Get("Authorization")); tok != "" {
		return tok
	}
	path := r.URL.Path
	underNode := strings.HasPrefix(path, "/node/")
	alias := spider.DefaultNodeHost().MatchNodeRootAlias(path)
	if path == "/ui" || underNode || alias {
		if t := strings.TrimSpace(r.URL.Query().Get("token")); t != "" {
			return t
		}
	}
	if alias {
		if c, err := r.Cookie(nodeRootAuthCookie); err == nil {
			return strings.TrimSpace(c.Value)
		}
	}
	return ""
}

// wrapNodeRootAuthCookies 仅在 /node/… 或根别名响应上种 Cookie（供页内绝对路径 API）。
func wrapNodeRootAuthCookies(w http.ResponseWriter, r *http.Request, token string) http.ResponseWriter {
	token = strings.TrimSpace(token)
	if token == "" || r == nil {
		return w
	}
	path := r.URL.Path
	if !strings.HasPrefix(path, "/node/") && !spider.DefaultNodeHost().MatchNodeRootAlias(path) {
		return w
	}
	cookies := nodeRootAuthCookies(token)
	if len(cookies) == 0 {
		return w
	}
	return &nodeRootCookieWriter{ResponseWriter: w, cookies: cookies}
}

func nodeRootAuthCookies(token string) []*http.Cookie {
	aliases := spider.DefaultNodeHost().RootAliases()
	out := make([]*http.Cookie, 0, len(aliases))
	for _, a := range aliases {
		out = append(out, &http.Cookie{
			Name:     nodeRootAuthCookie,
			Value:    token,
			Path:     "/" + a,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}
	return out
}

// nodeRootCookieWriter 在 WriteHeader 时写入 Cookie，避免 ReverseProxy 冲掉提前 Set-Cookie。
type nodeRootCookieWriter struct {
	http.ResponseWriter
	cookies []*http.Cookie
	wrote   bool
}

func (w *nodeRootCookieWriter) WriteHeader(code int) {
	if !w.wrote {
		w.wrote = true
		for _, c := range w.cookies {
			http.SetCookie(w.ResponseWriter, c)
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *nodeRootCookieWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func stripQueryToken(r *http.Request) {
	if r == nil || r.URL == nil {
		return
	}
	q := r.URL.Query()
	if !q.Has("token") {
		return
	}
	q.Del("token")
	r.URL.RawQuery = q.Encode()
}
