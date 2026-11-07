package spider

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// unzipOne 从 zip 中抽出单个成员到 dest。
func unzipOne(zipPath, member, dest string) error {
	member = strings.TrimPrefix(member, "/")
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	var found *zip.File
	for _, f := range r.File {
		name := strings.TrimPrefix(f.Name, "/")
		if name == member {
			found = f
			break
		}
	}
	if found == nil {
		return fmt.Errorf("zip 中无 %s", member)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	rc, err := found.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	tmp := dest + ".partial"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, rc)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	_ = os.Remove(dest)
	return os.Rename(tmp, dest)
}
