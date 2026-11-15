// Package cacheclean 清理爬虫/磁力/日志等用户缓存（不删设置与数据库）。
// 「全部」时扫数据根：保留 db/、ui/client.json、TV/ 根凭证与 s_u_<用户名>；清掉 s_c_* 与其它残留。
package cacheclean

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bobo/KOTV/internal/paths"
	"github.com/bobo/KOTV/internal/spider"
	"github.com/bobo/KOTV/internal/thunder"
)

// Options 选择清理项。
type Options struct {
	Script bool // JS / Python 落盘
	Jar    bool // JAR 包与 bridge 侧 cache/jar
	Magnet bool // 磁力种子与下载缓冲
	Logs   bool // 引擎/桥接日志
	Other  bool // http/epg/sub/pic 等杂项缓存
}

// Result 清理结果摘要。
type Result struct {
	FreedBytes int64             `json:"freedBytes"`
	FreedHuman string            `json:"freedHuman"`
	Cleared    []string          `json:"cleared"`
	Errors     []string          `json:"errors,omitempty"`
	Details    map[string]string `json:"details,omitempty"`
}

// All 默认全选。
func All() Options {
	return Options{Script: true, Jar: true, Magnet: true, Logs: true, Other: true}
}

// Run 执行清理。运行中可调用；磁力会关闭 BT 客户端。
func Run(opt Options) Result {
	res := Result{Details: map[string]string{}, Cleared: []string{}}
	add := func(label, path string, n int64, err error) {
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", label, err))
			return
		}
		res.FreedBytes += n
		res.Cleared = append(res.Cleared, label)
		res.Details[label] = fmt.Sprintf("%s (%s)", path, humanBytes(n))
	}

	// 清脚本或 JAR 前先软取消四引擎 in-flight（含仅勾 JAR、未勾脚本的情况）。
	if opt.Script || opt.Jar {
		spider.SoftCancelAll()
	}

	if opt.Script {
		spider.ResetScriptSpiders()
		// 公共包体 cache/{js,py,node}（CacheRoot 会先收拢历史误放）。
		_ = paths.CacheRoot()
		if n2, err2 := paths.WipeDirRemove(filepath.Join(paths.CacheRoot(), "js", "pkg")); n2 > 0 || err2 != nil {
			add("Node误嵌残留", filepath.Join(paths.CacheRoot(), "js", "pkg"), n2, err2)
		}
		n, err := paths.WipeDirKeepRoot(paths.JsCache())
		add("JS", paths.JsCache(), n, err)
		n, err = paths.WipeDirKeepRoot(paths.PyCache())
		add("Python", paths.PyCache(), n, err)
		n, err = paths.WipeDirKeepRoot(paths.NodeCache())
		add("Node", paths.NodeCache(), n, err)
		// 每用户可变态（js-local / node data / py work）
		n, err = paths.WipeUserRuntimeSubdirs("js-local", "node", "py")
		if n > 0 || err != nil {
			add("脚本运行时", filepath.Join(paths.CacheRoot(), "run"), n, err)
		}
	}

	if opt.Jar {
		spider.ClearJarDisk()
		n, err := paths.WipeDirKeepRoot(paths.JarCache())
		add("JAR", paths.JarCache(), n, err)
		n, err = paths.WipeUserRuntimeSubdirs("jvm")
		if n > 0 || err != nil {
			add("JVM运行时", filepath.Join(paths.CacheRoot(), "run"), n, err)
		}
		n, err = paths.WipeDirKeepRoot(filepath.Join(paths.CacheRoot(), "jpa"))
		if n > 0 || err != nil {
			add("JPA", filepath.Join(paths.CacheRoot(), "jpa"), n, err)
		}
		n, err = paths.WipeDirKeepRoot(filepath.Join(paths.CacheRoot(), "proxy"))
		if n > 0 || err != nil {
			add("代理溢写", filepath.Join(paths.CacheRoot(), "proxy"), n, err)
		}
	}

	if opt.Magnet {
		n, err := thunder.ClearStorage()
		add("磁力下载", thunder.DataDir(), n, err)
		n2, err2 := paths.WipeDirRemove(filepath.Join(paths.Root(), "bt"))
		if n2 > 0 || (err2 == nil && dirExists(filepath.Join(paths.Root(), "bt"))) {
			add("BT遗留", filepath.Join(paths.Root(), "bt"), n2, err2)
		}
		// 旧版遗留路径一并清掉。
		for _, legacy := range []string{
			filepath.Join(paths.Root(), "thunder"),
			filepath.Join(paths.Root(), "cache", "thunder"),
		} {
			n3, err3 := paths.WipeDirKeepRoot(legacy)
			if n3 > 0 {
				add("磁力遗留", legacy, n3, err3)
			}
		}
	}

	if opt.Logs {
		n, err := clearLogs(paths.LogDir())
		add("日志", paths.LogDir(), n, err)
	}

	if opt.Other {
		for _, item := range []struct{ label, path string }{
			{"HTTP缓存", paths.HttpCache()},
			{"EPG", paths.EpgCache()},
			{"字幕", paths.SubCache()},
			{"封面", paths.PicCache()},
			{"更新包", filepath.Join(paths.Root(), "update")},
			// 旧版套娃目录：…/data/cache/*（清理一次即可）
			{"旧HTTP缓存", filepath.Join(paths.Data(), "cache", "http")},
			{"旧EPG", filepath.Join(paths.Data(), "cache", "epg")},
			{"旧字幕", filepath.Join(paths.Data(), "cache", "sub")},
			{"旧封面", filepath.Join(paths.Data(), "cache", "pic")},
			{"旧JS", filepath.Join(paths.Data(), "cache", "js")},
			{"旧JAR", filepath.Join(paths.Data(), "cache", "jar")},
			{"旧PY", filepath.Join(paths.Data(), "cache", "py")},
		} {
			n, err := paths.WipeDirKeepRoot(item.path)
			if n > 0 || err != nil {
				add(item.label, item.path, n, err)
			}
		}
		// 先关全部嗅探浏览器，再擦 user-data-dir（进程仍占目录时删不干净）。
		spider.ShutdownSniffBrowsers()
		n, err := paths.WipeUserRuntimeSubdirs("chromium")
		if n > 0 || err != nil {
			add("Chromium运行时", filepath.Join(paths.CacheRoot(), "run"), n, err)
		}
	}

	// 全选：再扫一遍数据根，删掉 db 以外的可再生目录与文件。
	if opt.Script && opt.Jar && opt.Magnet && opt.Logs && opt.Other {
		n, err := sweepRootKeepConfigAndDB(paths.Root())
		if n > 0 || err != nil {
			add("数据根残留", paths.Root(), n, err)
		}
	}

	res.FreedHuman = humanBytes(res.FreedBytes)
	return res
}

// sweepRootKeepConfigAndDB 保留 db/、ui/client.json、TV/ 根凭证与 s_u_<用户名>；删除遗留 TV 桶与其它残留。
func sweepRootKeepConfigAndDB(root string) (int64, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var freed int64
	for _, e := range entries {
		name := e.Name()
		low := strings.ToLower(name)
		p := filepath.Join(root, name)
		switch low {
		case "db":
			continue
		case "ui":
			n, err := scrubDirKeepFileNames(p, "client.json")
			freed += n
			if err != nil {
				return freed, err
			}
			continue
		case "tv":
			n, err := removeLegacyClientTVDirs(p)
			freed += n
			if err != nil {
				return freed, err
			}
			continue
		}
		if e.IsDir() {
			n, err := paths.WipeDirRemove(p)
			freed += n
			if err != nil {
				return freed, err
			}
			continue
		}
		info, err := e.Info()
		sz := int64(0)
		if err == nil {
			sz = info.Size()
		}
		if err := os.Remove(p); err == nil {
			freed += sz
		}
	}
	return freed, nil
}

// scrubDirKeepFileNames 删除目录内除 keep 以外的条目（大小写不敏感文件名）。
func scrubDirKeepFileNames(dir string, keep ...string) (int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	allow := map[string]struct{}{}
	for _, k := range keep {
		allow[strings.ToLower(k)] = struct{}{}
	}
	var freed int64
	for _, e := range entries {
		if _, ok := allow[strings.ToLower(e.Name())]; ok {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if e.IsDir() {
			n, err := paths.WipeDirRemove(p)
			freed += n
			if err != nil {
				return freed, err
			}
			continue
		}
		info, err := e.Info()
		sz := int64(0)
		if err == nil {
			sz = info.Size()
		}
		if err := os.Remove(p); err == nil {
			freed += sz
		}
	}
	return freed, nil
}

// removeLegacyClientTVDirs 删除 paths.IsLegacyTVDir（s_c_*、旧 s_u_<uuid>）；保留根凭证与 s_u_<用户名>。
func removeLegacyClientTVDirs(tvRoot string) (int64, error) {
	entries, err := os.ReadDir(tvRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var freed int64
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !paths.IsLegacyTVDir(name) {
			continue
		}
		n, err := paths.WipeDirRemove(filepath.Join(tvRoot, name))
		freed += n
		if err != nil {
			return freed, err
		}
	}
	return freed, nil
}

func clearLogs(dir string) (int64, error) {
	_ = os.MkdirAll(dir, 0o755)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	var freed int64
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		sz := info.Size()
		// kotv.log 可能仍被进程占用：截断而不是删除。
		if strings.EqualFold(e.Name(), "kotv.log") || strings.HasSuffix(strings.ToLower(e.Name()), ".log") {
			if err := truncateFile(p); err != nil {
				_ = os.Remove(p)
			}
			freed += sz
			continue
		}
		if e.IsDir() {
			n, _ := paths.WipeDirRemove(p)
			freed += n
			continue
		}
		if err := os.Remove(p); err == nil {
			freed += sz
		}
	}
	return freed, nil
}

func truncateFile(p string) error {
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func humanBytes(n int64) string {
	if n <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	v := float64(n)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d %s", n, units[i])
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}
