package app

import (
	"fmt"
	"strings"

	"github.com/bobo/KOTV/internal/auth"
	"github.com/bobo/KOTV/internal/download"
	"github.com/bobo/KOTV/internal/hostclient"
	"github.com/bobo/KOTV/internal/localproxy"
	"github.com/bobo/KOTV/internal/parse"
	"github.com/bobo/KOTV/internal/source"
	"github.com/bobo/KOTV/internal/thunder"
)

// ResolveDownload 解析可下载地址：与起播共用 playerContent / 二次解析，保留爬虫与站源 Header。
// 不走 PreparePlayback / /proxy/play；网盘类 /proxy?url=&header= 展开成直链并合并鉴权头。
func (a *App) ResolveDownload(siteKey, vodID, flag, epURL string, epIndex int) (string, map[string]string, string, error) {
	site := a.resolvePlaySite(siteKey)
	ep, err := a.resolveEpisodeURL(vodID, flag, epURL, epIndex)
	if err != nil {
		return "", nil, "", err
	}
	if err := requireLocalCrawler(); err != nil && !(strings.HasPrefix(ep, "http") && parse.IsVideoFormat(ep)) && !thunder.Match(ep) {
		return "", nil, "", err
	}

	media, err := a.resolvePlayerMedia(siteKey, site, flag, ep, 0, func(u string) bool {
		return parse.IsVideoFormat(u) || thunder.Match(u) ||
			localproxy.IsSpiderProxyURL(u) || strings.HasPrefix(u, "proxy://")
	})
	if err != nil {
		return "", nil, "", err
	}

	playURL := strings.TrimSpace(media.PlayURL)
	headers := media.Headers
	format := media.Format
	if playURL == "" {
		if media.PlayMsg != "" {
			return "", nil, "", fmt.Errorf("%s", media.PlayMsg)
		}
		return "", nil, "", fmt.Errorf("未解析到可下载地址")
	}
	if apiLooksUnplayable(playURL) && !localproxy.IsSpiderProxyURL(playURL) {
		return "", nil, "", fmt.Errorf("未解析到可下载地址")
	}
	if thunder.Match(playURL) {
		return playURL, nil, format, nil
	}

	if source.Match(playURL) {
		info, err := source.FetchInfo(playURL, a.liveCoreJSON())
		if err != nil {
			return "", nil, "", fmt.Errorf("专用源解析失败: %w", err)
		}
		playURL = localproxy.ConvertScheme(strings.TrimSpace(info.URL))
	}

	// 网盘等爬虫 /proxy 地址不展开成 CDN 直链：下载在引擎本机执行，和本机播放器一样经
	// /proxy→jar 拉流，由爬虫处理 Cookie / 签名续期。直连 CDN 常因鉴权或时效失败。
	if strings.TrimSpace(playURL) == "" {
		return "", nil, "", fmt.Errorf("未解析到可下载地址")
	}
	return playURL, headers, format, nil
}

// downloadViewer 当前请求操作下载列表的身份：本机与管理员看全部，其他远端用户只看自己的。
func downloadViewer() download.Viewer {
	return download.Viewer{Owner: hostclient.DataOwner(), All: currentRequestIsAdmin()}
}

func (a *App) APIDownloadList() map[string]any {
	list := download.List(downloadViewer())
	items := make([]any, 0, len(list))
	for _, t := range list {
		items = append(items, t)
	}
	return map[string]any{"ok": true, "tasks": items}
}

func (a *App) APIDownloadEnqueue(body map[string]any) (map[string]any, error) {
	req := download.EnqueueReq{
		Site:  strings.TrimSpace(fmt.Sprint(body["site"])),
		VodID: strings.TrimSpace(fmt.Sprint(body["vodId"])),
		Title: strings.TrimSpace(fmt.Sprint(body["title"])),
		Flag:  strings.TrimSpace(fmt.Sprint(body["flag"])),
		Pic:   strings.TrimSpace(fmt.Sprint(body["pic"])),
		Owner: hostclient.DataOwner(),
	}
	if u := auth.GetUser(req.Owner); u != nil {
		req.OwnerName = u.Username
	}
	rawEps, _ := body["episodes"].([]any)
	for _, e := range rawEps {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		idx := 0
		switch v := m["index"].(type) {
		case float64:
			idx = int(v)
		case int:
			idx = v
		}
		req.Episodes = append(req.Episodes, download.EpisodeSpec{
			Index: idx,
			Name:  strings.TrimSpace(fmt.Sprint(m["name"])),
			URL:   strings.TrimSpace(fmt.Sprint(m["url"])),
		})
	}
	tasks, err := download.Enqueue(req)
	if err != nil {
		return nil, err
	}
	items := make([]any, 0, len(tasks))
	for _, t := range tasks {
		items = append(items, t)
	}
	return map[string]any{"ok": true, "tasks": items, "count": len(tasks)}, nil
}

func (a *App) APIDownloadCancel(id string) map[string]any {
	if err := download.Cancel(strings.TrimSpace(id), downloadViewer()); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true}
}

func (a *App) APIDownloadPause(id string) map[string]any {
	if err := download.Pause(strings.TrimSpace(id), downloadViewer()); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true}
}

func (a *App) APIDownloadResume(id string) map[string]any {
	if err := download.Resume(strings.TrimSpace(id), downloadViewer()); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true}
}

func (a *App) APIDownloadPauseAll() map[string]any {
	return map[string]any{"ok": true, "count": download.PauseAll(downloadViewer())}
}

func (a *App) APIDownloadResumeAll() map[string]any {
	return map[string]any{"ok": true, "count": download.ResumeAll(downloadViewer())}
}

func (a *App) APIDownloadRemove(id string, deleteFile bool) map[string]any {
	if err := download.Remove(strings.TrimSpace(id), deleteFile, downloadViewer()); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true}
}

func (a *App) APIDownloadClearFinished() map[string]any {
	n := download.ClearFinished(downloadViewer())
	return map[string]any{"ok": true, "removed": n}
}
