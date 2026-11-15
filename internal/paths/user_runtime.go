package paths

import (
	"path/filepath"
	"strings"
)

// RuntimeOwnerLabel 运行时落盘子目录名：远端用户用 uid，本机共享用 shared。
func RuntimeOwnerLabel(userID string) string {
	if strings.TrimSpace(userID) == "" {
		return "shared"
	}
	return strings.TrimSpace(userID)
}

// UserRuntimeDir 可变运行时态根：cache/run/<uid|shared>/。
// 不可变内容寻址缓存（jar/py/js 包体）仍用 JarCache/PyCache 等全局目录。
func UserRuntimeDir(userID string) string {
	return ensure(filepath.Join(CacheRoot(), "run", RuntimeOwnerLabel(userID)))
}

// UserJVMDir 该用户 JVM Prefers/cache/files 根（-Dkotv.cache.dir）。
func UserJVMDir(userID string) string {
	return ensure(filepath.Join(UserRuntimeDir(userID), "jvm"))
}

// UserPyWorkDir Python 进程可写工作目录（脚本包体仍读全局 PyCache）。
func UserPyWorkDir(userID string) string {
	return ensure(filepath.Join(UserRuntimeDir(userID), "py"))
}

// UserChromiumDir Chromium user-data-dir（按用户隔离 cookie/storage）。
func UserChromiumDir(userID string) string {
	return ensure(filepath.Join(UserRuntimeDir(userID), "chromium"))
}
