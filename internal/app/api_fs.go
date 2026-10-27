package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bobo/KOTV/internal/paths"
)

// APIFsRoots 引擎本机可浏览根（下载目录等）。
func (a *App) APIFsRoots() map[string]any {
	roots := paths.BrowseRoots()
	items := make([]any, 0, len(roots))
	for _, r := range roots {
		items = append(items, map[string]any{
			"name": r.Name,
			"path": r.Path,
		})
	}
	return map[string]any{"ok": true, "roots": items}
}

// APIFsList 列出引擎本机目录。path 空则返回浏览根列表。
func (a *App) APIFsList(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		roots := paths.BrowseRoots()
		entries := make([]any, 0, len(roots))
		for _, r := range roots {
			entries = append(entries, map[string]any{
				"name": r.Name,
				"path": r.Path,
				"dir":  true,
			})
		}
		return map[string]any{
			"ok":      true,
			"path":    "",
			"parent":  "",
			"entries": entries,
		}, nil
	}

	abs, err := filepath.Abs(filepath.Clean(raw))
	if err != nil {
		return nil, fmt.Errorf("无效路径")
	}
	if !paths.UnderBrowseRoot(abs) {
		return nil, fmt.Errorf("路径不在可浏览范围内")
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("目录不存在")
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("不是目录")
	}
	ents, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	type item struct {
		Name string
		Path string
		Dir  bool
	}
	list := make([]item, 0, len(ents))
	for _, e := range ents {
		name := e.Name()
		if name == "" || name == "." || name == ".." {
			continue
		}
		full := filepath.Join(abs, name)
		list = append(list, item{Name: name, Path: full, Dir: e.IsDir()})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Dir != list[j].Dir {
			return list[i].Dir
		}
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})
	entries := make([]any, 0, len(list))
	for _, it := range list {
		entries = append(entries, map[string]any{
			"name": it.Name,
			"path": it.Path,
			"dir":  it.Dir,
		})
	}
	return map[string]any{
		"ok":      true,
		"path":    abs,
		"parent":  paths.BrowseParent(abs),
		"entries": entries,
	}, nil
}
