package app

import (
	"github.com/bobo/KOTV/internal/auth"
	"github.com/bobo/KOTV/internal/hostclient"
)

// currentRequestIsAdmin 当前请求用户是否为管理员（本机无 userId 视为本机管理）。
func currentRequestIsAdmin() bool {
	uid := hostclient.DataOwner()
	if uid == "" {
		return true
	}
	u := auth.GetUser(uid)
	return u != nil && u.Role == auth.RoleAdmin && u.Enabled
}
