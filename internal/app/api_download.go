package app

import (
	"fmt"
	"strings"

	"github.com/bobo/KOTV/internal/download"
)

func (a *App) ResolveDownload(site, vodID, flag, epURL string, epIndex int) (string, map[string]string, string, error) {
	out, err := a.APIPlay(site, vodID, flag, epURL, epIndex, 0)
	if err != nil {
		return "", nil, "", err
	}
	u := strings.TrimSpace(fmt.Sprint(out["url"]))
	if u == "" {
		u = strings.TrimSpace(fmt.Sprint(out["media"]))
	}
	format := strings.TrimSpace(fmt.Sprint(out["format"]))
	headers := map[string]string{}
	if h, ok := out["headers"].(map[string]string); ok {
		for k, v := range h {
			headers[k] = v
		}
	} else if hm, ok := out["headers"].(map[string]any); ok {
		for k, v := range hm {
			headers[k] = fmt.Sprint(v)
		}
	}
	return u, headers, format, nil
}

func (a *App) APIDownloadList() map[string]any {
	list := download.List()
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
	if err := download.Cancel(strings.TrimSpace(id)); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true}
}

func (a *App) APIDownloadPause(id string) map[string]any {
	if err := download.Pause(strings.TrimSpace(id)); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true}
}

func (a *App) APIDownloadResume(id string) map[string]any {
	if err := download.Resume(strings.TrimSpace(id)); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true}
}

func (a *App) APIDownloadPauseAll() map[string]any {
	return map[string]any{"ok": true, "count": download.PauseAll()}
}

func (a *App) APIDownloadResumeAll() map[string]any {
	return map[string]any{"ok": true, "count": download.ResumeAll()}
}

func (a *App) APIDownloadRemove(id string, deleteFile bool) map[string]any {
	if err := download.Remove(strings.TrimSpace(id), deleteFile); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true}
}

func (a *App) APIDownloadClearFinished() map[string]any {
	n := download.ClearFinished()
	return map[string]any{"ok": true, "removed": n}
}
