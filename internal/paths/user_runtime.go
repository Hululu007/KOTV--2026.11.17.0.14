package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// 缓存分两类（勿混用）：
//
//  1. 内容寻址包体（全局共享，多用户只下一次）：
//     JarCache / PyCache / JsModuleCache / NodeCache/<pkgId>/
//  2. 可变运行时态（按 RuntimeUserID 隔离，空=shared）：
//     UserRuntimeDir = cache/run/<uid|shared>/{jvm,py,chromium,node,js-local}
//
// 进程是否也按用户拆开（与落盘配合）：
//   - JVM：远端每用户一进程（Prefers/spider 实例/recentJar 在 bridge 内未按 user 隔离）
//   - Python / Node：每用户独立进程与 workDir/dataDir
//   - QuickJS：同引擎进程内分槽（jsPyKey 含 RuntimeUserID），无第二 OS 进程
//   - 嗅探浏览器：每用户独立进程 + user-data-dir（Android 为 WebView，不走本目录）

// RuntimeOwnerLabel 运行时落盘子目录名：远端用户用 uid，本机共享用 shared。
func RuntimeOwnerLabel(userID string) string {
	if strings.TrimSpace(userID) == "" {
		return "shared"
	}
	return strings.TrimSpace(userID)
}

// UserRuntimeDir 可变运行时态根：cache/run/<uid|shared>/。
func UserRuntimeDir(userID string) string {
	return ensure(filepath.Join(CacheRoot(), "run", RuntimeOwnerLabel(userID)))
}

// UserRuntimeRoot 全部用户可变态的父目录 cache/run（需要落盘时 ensure）。
func UserRuntimeRoot() string {
	return ensure(userRuntimeRootPath())
}

// userRuntimeRootPath cache/run 路径（不 create，供清理扫描）。
func userRuntimeRootPath() string {
	return filepath.Join(CacheRoot(), "run")
}

// UserJVMDir 该用户 JVM Prefers/cache/files 根（-Dkotv.cache.dir）。
func UserJVMDir(userID string) string {
	return ensure(filepath.Join(UserRuntimeDir(userID), "jvm"))
}

// UserPyWorkDir Python 进程可写工作目录（脚本包体仍读全局 PyCache）。
func UserPyWorkDir(userID string) string {
	return ensure(filepath.Join(UserRuntimeDir(userID), "py"))
}

// UserChromiumDir Chromium user-data-dir。
func UserChromiumDir(userID string) string {
	return ensure(filepath.Join(UserRuntimeDir(userID), "chromium"))
}

// UserNodeDataDir 该用户某 Node 源包的可写 dataDir（包体仍在 NodeCache/<pkgId>/）。
func UserNodeDataDir(userID, pkgID string) string {
	pkgID = strings.TrimSpace(pkgID)
	return ensure(filepath.Join(UserRuntimeDir(userID), "node", pkgID, "data"))
}

// UserJsLocalDir QuickJS Prefers/Local 落盘（可变；模块包体见 JsModuleCache）。
func UserJsLocalDir(userID string) string {
	return ensure(filepath.Join(UserRuntimeDir(userID), "js-local"))
}

// JsModuleCache 远程 JS 模块磁盘缓存（内容寻址，全局共享）。
func JsModuleCache() string {
	return ensure(filepath.Join(JsCache(), "mod"))
}

// RemoveUserRuntimeDir 删除某远端用户的全部可变运行时目录。空 userID 拒绝（避免误删 shared）。
func RemoveUserRuntimeDir(userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	return os.RemoveAll(filepath.Join(userRuntimeRootPath(), RuntimeOwnerLabel(userID)))
}

// WipeUserRuntimeSubdirs 清理 cache/run/<各用户>/<names…>（保留 run 与用户目录本身；不 create run）。
func WipeUserRuntimeSubdirs(names ...string) (freed int64, err error) {
	root := userRuntimeRootPath()
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return 0, nil
		}
		return 0, readErr
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		owner := filepath.Join(root, e.Name())
		for _, name := range names {
			n, e2 := wipeDirContents(filepath.Join(owner, name))
			freed += n
			if e2 != nil && err == nil {
				err = e2
			}
		}
	}
	return freed, err
}

// wipeDirContents 删除目录内全部子项，保留目录本身；目录不存在视为 0。
func wipeDirContents(dir string) (int64, error) {
	st, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if !st.IsDir() {
		n := st.Size()
		return n, os.Remove(dir)
	}
	var freed int64
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.IsDir() {
			n, _ := dirSize(p)
			if rmErr := os.RemoveAll(p); rmErr == nil {
				freed += n
			}
		} else {
			n := info.Size()
			if rmErr := os.Remove(p); rmErr == nil {
				freed += n
			}
		}
	}
	return freed, nil
}

func dirSize(root string) (int64, error) {
	var n int64
	err := filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			n += info.Size()
		}
		return nil
	})
	return n, err
}
