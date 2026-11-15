package spider

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// writeFileAtomic 先确保父目录存在，再 .tmp → rename。
// jar / js / py / node 包体落盘共用，避免缓存子目录未建时 WriteFile 报 no such file。
func writeFileAtomic(dest string, data []byte) error {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return fmt.Errorf("empty dest")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}
