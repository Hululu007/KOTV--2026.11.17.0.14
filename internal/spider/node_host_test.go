package spider

import (
	"os"
	"path/filepath"
	"testing"

	appruntime "github.com/bobo/KOTV/internal/runtime"
)

func TestNodeHostStart_SkipIfNoNode(t *testing.T) {
	if appruntime.Node() == "" {
		t.Skip("本机没有 Node 运行时")
	}
	// 最小 CJS 包：监听 DEV_HTTP_PORT，触发 catServerFactory listening。
	dir := t.TempDir()
	index := filepath.Join(dir, "index.js")
	src := `
const http = require('http');
module.exports = {
  start: async function() {
    const server = catServerFactory((req, res) => {
      if (req.url === '/config') {
        res.setHeader('Content-Type', 'application/json');
        res.end(JSON.stringify({video:{sites:[{key:'nodejs_t',api:'/spider/t/3',type:3}]}}));
        return;
      }
      res.statusCode = 404;
      res.end();
    });
    const port = parseInt(process.env.DEV_HTTP_PORT || '0', 10) || 0;
    server.listen(port, process.env.HOST || '127.0.0.1');
  }
};
`
	if err := os.WriteFile(index, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	// 用 file:// 走 Ensure 会走下载逻辑；这里直接测 start 路径。
	h := DefaultNodeHost()
	if err := h.ensureRunner(); err != nil {
		t.Fatal(err)
	}
	pkg := &nodePkg{
		id:       "testlocal",
		pkgURL:   "file://" + index,
		dir:      dir,
		dataDir:  filepath.Join(dir, "data"),
		indexJS:  index,
		configJS: filepath.Join(dir, "index.config.js"),
		backoff:  nodeRestartBackoff,
	}
	_ = os.MkdirAll(pkg.dataDir, 0o755)
	h.mu.Lock()
	h.pkgs[pkg.id] = pkg
	h.mu.Unlock()
	t.Cleanup(func() {
		pkg.Stop()
		h.mu.Lock()
		delete(h.pkgs, pkg.id)
		h.mu.Unlock()
	})
	if err := pkg.start(h); err != nil {
		t.Fatal(err)
	}
	if pkg.Port() <= 0 {
		t.Fatal("no port")
	}
	cfg, err := pkg.FetchConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !h.KnowsAPI("/spider/t/3") {
		t.Fatalf("api not registered, cfg=%s", cfg)
	}
}
