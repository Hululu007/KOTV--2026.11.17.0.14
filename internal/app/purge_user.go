package app

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/bobo/KOTV/internal/parse"
	"github.com/bobo/KOTV/internal/paths"
	"github.com/bobo/KOTV/internal/spider"
)

// PurgeUserData 删除远端用户的全部用户态（owner 为空即本机，不可调用）。
// username 用于删 /TV/s_u_<用户名>/；可空（仅清库与运行时）。
func (a *App) PurgeUserData(userID, username string) {
	userID = strings.TrimSpace(userID)
	username = strings.TrimSpace(username)
	if a == nil || userID == "" {
		return
	}
	if a.DB != nil {
		if err := a.DB.ClearHistory(userID); err != nil {
			log.Printf("purge user %s history: %v", userID, err)
		}
		if err := a.DB.ClearKeep(userID); err != nil {
			log.Printf("purge user %s keep: %v", userID, err)
		}
		if err := a.DB.DeleteSettingsOwner(userID); err != nil {
			log.Printf("purge user %s settings: %v", userID, err)
		}
		_ = a.DB.DeleteAuthTokensForUser(userID)
	}
	a.dropReposForUser(userID)
	if a.sessions != nil {
		a.sessions.RemoveByUser(userID)
	}
	spider.KillUserRuntime(userID)
	parse.CancelUserSniffs(userID)
	paths.RemoveTVUserDirs(username)
	// 清理迁移前残留目录
	_ = os.RemoveAll(filepath.Join(paths.Data(), "users", userID))
}
