package spider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomic_CreatesParentDir(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "node", "7a5dddfd7b57", "index.js")
	// 父目录故意不存在：复现清缓存后 Node 写 index.js.tmp 失败。
	if _, err := os.Stat(filepath.Dir(dest)); !os.IsNotExist(err) {
		t.Fatalf("precondition: parent should be missing, err=%v", err)
	}
	if err := writeFileAtomic(dest, []byte("module.exports={}")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "module.exports={}" {
		t.Fatalf("content=%q", b)
	}
	if _, err := os.Stat(dest + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("tmp should be renamed away")
	}
}
