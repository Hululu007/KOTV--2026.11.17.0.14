package spider

import (
	"os"
	"strings"

	"github.com/bobo/KOTV/internal/paths"
)

// ProxyBodyFileHeader 是内部标记头：当代理响应体被 bridge 溢写到磁盘文件时，
// 通过该头把文件路径传给 HTTP 层做流式回写，服务端写响应前会剥离它。
const ProxyBodyFileHeader = "X-KOTV-Body-File"

// ProxyBodyStreamHeader 是内部标记头：bridge 在 127.0.0.1 上开 TCP 泵流，
// 值为 "host:port"。Go 拨号后边读边写给客户端，避免整段视频溢写磁盘。
const ProxyBodyStreamHeader = "X-KOTV-Body-Stream"

// Spider CatVod 爬虫接口。
type Spider interface {
	Init(extend string) error
	HomeContent(filter bool) (string, error)
	HomeVideoContent() (string, error)
	CategoryContent(tid, pg string, filter bool, extend map[string]string) (string, error)
	DetailContent(ids []string) (string, error)
	SearchContent(key string, quick bool, pg string) (string, error)
	PlayerContent(flag, id string, vipFlags []string) (string, error)
	LiveContent(url string) (string, error)
	Proxy(params map[string]string) (status int, contentType string, body []byte, headers map[string]string, err error)
	Action(action string) (string, error)
	ManualVideoCheck() (bool, error)
	IsVideoFormat(url string) (bool, error)
	Destroy()
}

type noopSpider struct{}

func (noopSpider) Init(string) error                 { return nil }
func (noopSpider) HomeContent(bool) (string, error)  { return "{}", nil }
func (noopSpider) HomeVideoContent() (string, error) { return "{}", nil }
func (noopSpider) CategoryContent(string, string, bool, map[string]string) (string, error) {
	return "{}", nil
}
func (noopSpider) DetailContent([]string) (string, error)                 { return "{}", nil }
func (noopSpider) SearchContent(string, bool, string) (string, error)     { return "{}", nil }
func (noopSpider) PlayerContent(string, string, []string) (string, error) { return "{}", nil }
func (noopSpider) LiveContent(string) (string, error)                     { return "", nil }
func (noopSpider) Proxy(map[string]string) (int, string, []byte, map[string]string, error) {
	return 0, "", nil, nil, nil
}
func (noopSpider) Action(string) (string, error)      { return "", nil }
func (noopSpider) ManualVideoCheck() (bool, error)    { return false, nil }
func (noopSpider) IsVideoFormat(string) (bool, error) { return false, nil }
func (noopSpider) Destroy()                           {}

func isJS(api string) bool  { return strings.Contains(api, ".js") }
func isPy(api string) bool  { return strings.Contains(api, ".py") }
func isCSP(api string) bool { return strings.HasPrefix(api, "csp_") }

// isNode Node 站：api 形如 /spider/<key>/<type>。
func isNode(api string) bool {
	return strings.HasPrefix(strings.TrimSpace(api), "/spider/")
}

// IsNodeAPI 供配置/站点层判断是否 Node 爬虫 api（勿被 resolveSitePaths 改写成 http）。
func IsNodeAPI(api string) bool { return isNode(api) }

// KeepAPIRaw 爬虫 api 标识须原样保留，禁止当相对路径拼到配置根 URL 上。
func KeepAPIRaw(api string) bool {
	api = strings.TrimSpace(api)
	if api == "" {
		return false
	}
	return isNode(api) || isCSP(api) || isJS(api) || isPy(api)
}

// RunnableAPI 表示该 api 会进 jar / js / py / node。其余类型不会被当成空首页成功。
func RunnableAPI(api string) bool {
	return isPy(api) || isJS(api) || isCSP(api) || isNode(api)
}

// Get 根据站源类型获取爬虫实例。
func Get(key, api, ext, jar string) Spider {
	switch {
	case isPy(api):
		return newPySpider(key, api, ext, jar)
	case isJS(api):
		return newJsSpider(key, api, ext, jar)
	case isCSP(api):
		return newJarSpider(key, api, ext, jar)
	case isNode(api):
		return newNodeSpider(key, api, ext, jar)
	default:
		return noopSpider{}
	}
}

// SetRecent 每次选中站点时更新 recent，供全局 proxy 使用。
// api 同时含 .js 与 .py 时先记 js（getSpider 仍 py 优先）。
func SetRecent(key, api, ext, jar string) {
	switch {
	case isJS(api):
		setRecentJs(key, api, ext, jar)
	case isPy(api):
		setRecentPy(key, api, ext, jar)
	case isCSP(api):
		setRecentJar(jar)
	}
	ReleaseOtherScripts(api)
}

func Clear() {
	clearJar()
	clearJsPy()
	stopAllNodeHosts()
	clearScriptCaches()
	// JAR Prefers 等可变态随 Clear 一并擦掉（包体 JarCache 留给 cacheclean / 下次复用）。
	_, _ = paths.WipeUserRuntimeSubdirs("jvm")
}

// ResetScriptSpiders 换源/清缓存时销毁全部 JS/Python/Node 爬虫，避免旧进程状态污染。
func ResetScriptSpiders() {
	clearJsPy()
	clearJSMemoryCaches()
	stopAllNodeHosts()
}

// clearScriptCaches 清空 JS 内存缓存，擦共享 js/py/node 包体，并清各用户脚本可变态。
// Node 进程/索引由 stopAllNodeHosts 负责。
func clearScriptCaches() {
	clearJSMemoryCaches()
	for _, dir := range []string{paths.JsCache(), paths.PyCache(), paths.NodeCache()} {
		_ = os.RemoveAll(dir)
		_ = os.MkdirAll(dir, 0o755)
	}
	_, _ = paths.WipeUserRuntimeSubdirs("js-local", "node", "py")
}

// ClearJarDisk 清空 JAR 内存与 bridge（磁盘包体 / JVM 可变态由 cacheclean 擦）。
func ClearJarDisk() {
	clearJar()
}
