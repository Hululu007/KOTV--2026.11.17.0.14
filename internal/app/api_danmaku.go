package app

import (
	"context"
	"strings"

	"github.com/bobo/KOTV/internal/danmaku"
	"github.com/bobo/KOTV/internal/hostclient"
	"github.com/bobo/KOTV/internal/model"
	"github.com/bobo/KOTV/internal/settings"
)

// danmakuSearchAPI 弹幕搜索接口：设置里的 danmakuApi 优先，否则用配置顶层 danmaku。
func (a *App) danmakuSearchAPI() string {
	if api := strings.TrimSpace(settings.Get(settings.DanmakuAPI)); api != "" {
		return api
	}
	cfg, _, _ := a.scope()
	return strings.TrimSpace(cfg.API().Danmaku)
}

// APIDanmakuLoad 拉取并解析一个弹幕源（视频页、弹幕地址或引擎本机弹幕文件）；durationMs 为片长，未知传 0。
func (a *App) APIDanmakuLoad(ctx context.Context, src string, durationMs int64) (map[string]any, error) {
	items, err := danmaku.Load(ctx, src, durationMs)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []danmaku.Item{}
	}
	return map[string]any{"ok": true, "items": items}, nil
}

// APIDanmakuSearch 手动搜索弹幕源。
func (a *App) APIDanmakuSearch(ctx context.Context, name, episode string) (map[string]any, error) {
	found, err := danmaku.Search(ctx, hostclient.ScopeID(), a.danmakuSearchAPI(), name, episode)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "sources": danmakuSourcesDTO(found)}, nil
}

// APIDanmakuAuto 起播时决定弹幕源：片源自带的加上自动搜到的第一条，返回列表和默认选中项。
// 关了「加载弹幕」返回空；本站 danmaku=0 或关了自动搜索只用片源弹幕；搜索失败不影响片源弹幕。
func (a *App) APIDanmakuAuto(ctx context.Context, siteKey, name, episode string, spiderSources []model.DanmakuItem) map[string]any {
	if !settings.IsDanmakuLoad() {
		return map[string]any{"ok": true, "sources": []any{}, "selected": 0}
	}
	sources, selected := spiderSources, 0
	if api := a.danmakuSearchAPI(); api != "" && settings.IsDanmakuAuto() && a.siteAutoDanmaku(siteKey) {
		if found, err := danmaku.Search(ctx, hostclient.ScopeID(), api, name, episode); err == nil && len(found) > 0 {
			add := settings.IsDanmakuSpiderFirst() && len(spiderSources) > 0
			sources, selected = danmaku.Merge(spiderSources, found[0], add)
		}
	}
	return map[string]any{"ok": true, "sources": danmakuSourcesDTO(sources), "selected": selected}
}

func (a *App) siteAutoDanmaku(siteKey string) bool {
	cfg, _, _ := a.scope()
	if site := cfg.GetSite(siteKey); site != nil {
		return site.AutoDanmaku()
	}
	return true
}

func danmakuSourcesDTO(items []model.DanmakuItem) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]any{"name": it.Name, "url": it.URL})
	}
	return out
}
