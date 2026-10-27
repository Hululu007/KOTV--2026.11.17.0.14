package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// PathUnderRoot 判断 abs 是否等于 root 或其子路径（含 Windows 盘符根如 C:\）。
func PathUnderRoot(abs, root string) bool {
	abs = filepath.Clean(strings.TrimSpace(abs))
	root = filepath.Clean(strings.TrimSpace(root))
	if abs == "" || root == "" {
		return false
	}
	if runtime.GOOS == "windows" {
		if strings.EqualFold(abs, root) {
			return true
		}
	} else if abs == root {
		return true
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}
