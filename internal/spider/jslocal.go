package spider

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"sync"

	"github.com/bobo/KOTV/internal/hostclient"
	"github.com/bobo/KOTV/internal/paths"
)

// Local：Prefers key = cache_{rule}_{key}（rule 空则 cache_{key}）。
// 可变态按用户落 UserJsLocalDir；JS 模块包体走全局 JsModuleCache。
var jsLocalMu sync.Mutex

// LocalPrefersKey — Cache.getKey / Local.getKey。
func LocalPrefersKey(rule, key string) string {
	if rule == "" {
		return "cache_" + key
	}
	return "cache_" + rule + "_" + key
}

func jsLocalPath(rule, key string) string {
	// Prefers 键名按原文语义；落盘用 base64url，避免非法文件名且不碰撞。
	enc := base64.RawURLEncoding.EncodeToString([]byte(LocalPrefersKey(rule, key)))
	return filepath.Join(paths.UserJsLocalDir(hostclient.RuntimeUserID()), enc+".dat")
}

func LocalGet(rule, key string) string {
	jsLocalMu.Lock()
	defer jsLocalMu.Unlock()
	b, err := os.ReadFile(jsLocalPath(rule, key))
	if err != nil {
		return ""
	}
	return string(b)
}

func LocalSet(rule, key, value string) {
	jsLocalMu.Lock()
	defer jsLocalMu.Unlock()
	path := jsLocalPath(rule, key)
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(value), 0o644)
}

func LocalDelete(rule, key string) {
	jsLocalMu.Lock()
	defer jsLocalMu.Unlock()
	_ = os.Remove(jsLocalPath(rule, key))
}

func jsLocalGet(rule, key string) string  { return LocalGet(rule, key) }
func jsLocalSet(rule, key, value string)  { LocalSet(rule, key, value) }
func jsLocalDelete(rule, key string)      { LocalDelete(rule, key) }
