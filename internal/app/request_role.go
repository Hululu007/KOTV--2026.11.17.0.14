package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bobo/KOTV/internal/auth"
	"github.com/bobo/KOTV/internal/config"
	"github.com/bobo/KOTV/internal/database"
	"github.com/bobo/KOTV/internal/hostclient"
)

// ErrNeedAdmin 远端普通用户试图新增共享仓/直播源或清共享缓存。
var ErrNeedAdmin = errors.New("需要管理员")

// currentRequestIsAdmin 当前请求用户是否为管理员（本机无 userId 视为本机管理）。
func currentRequestIsAdmin() bool {
	uid := hostclient.DataOwner()
	if uid == "" {
		return true
	}
	u := auth.GetUser(uid)
	return u != nil && u.Role == auth.RoleAdmin && u.Enabled
}

// errUnlessSharedSourceAllowed 远端非管理员只能选用已在共享表中的源，不能新增。
func (a *App) errUnlessSharedSourceAllowed(typ int64, raw string) error {
	if a == nil || !hostclient.DedicatedRuntime() || currentRequestIsAdmin() {
		return nil
	}
	url := config.NormalizeSource(strings.TrimSpace(raw))
	if url == "" {
		return nil
	}
	var known map[string]struct{}
	if typ == database.ConfigTypeLive {
		known = a.liveSourceURLs()
	} else {
		known = a.siteRepoURLs()
	}
	if _, ok := known[url]; ok {
		return nil
	}
	for u := range known {
		if config.NormalizeSource(strings.TrimSpace(u)) == url {
			return nil
		}
	}
	return fmt.Errorf("%w", ErrNeedAdmin)
}
