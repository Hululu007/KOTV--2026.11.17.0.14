package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/bobo/KOTV/internal/spider"
)

// handleNodeMsg 本机回调：猫爪 messageToDart → POST /node/msg 或 /msg。
func (s *Server) handleNodeMsg(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result := spider.HandleNodeMsg(body)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if result == nil {
		_, _ = w.Write([]byte("null"))
		return
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(result)
}

// handleNodeProxy 反代 /node/<pkg>/… → node 进程。
func (s *Server) handleNodeProxy(w http.ResponseWriter, r *http.Request) {
	stripQueryToken(r)
	path := r.URL.Path
	rest := strings.TrimPrefix(path, "/node/")
	if rest == "" || rest == path {
		http.NotFound(w, r)
		return
	}
	pkgID, sub, ok := strings.Cut(rest, "/")
	if !ok {
		pkgID, sub = rest, ""
	}
	pkgID = strings.TrimSpace(pkgID)
	if pkgID == "" || pkgID == "msg" {
		http.NotFound(w, r)
		return
	}
	pkg := spider.DefaultNodeHost().GetPkg(pkgID)
	if pkg == nil {
		http.Error(w, "未知 Node 源包", http.StatusNotFound)
		return
	}
	forward := "/" + sub
	if sub == "" {
		forward = "/"
	}
	pkg.Proxy(w, r, forward)
}
