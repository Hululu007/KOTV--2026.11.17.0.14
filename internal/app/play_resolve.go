package app

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/bobo/KOTV/internal/localproxy"
	"github.com/bobo/KOTV/internal/model"
	"github.com/bobo/KOTV/internal/parse"
	"github.com/bobo/KOTV/internal/service"
	"github.com/bobo/KOTV/internal/settings"
	"github.com/bobo/KOTV/internal/thunder"
)

// playerMedia 为 playerContent + 二次解析后的媒体（尚未 PreparePlayback / 登记本地透传）。
type playerMedia struct {
	Site     model.Site
	EpURL    string
	PlayURL  string
	Headers  map[string]string
	Format   string
	Result   model.Result
	DidParse bool
	PlayMsg  string
}

func (a *App) resolvePlaySite(siteKey string) model.Site {
	cfg, _, _ := a.scope()
	var site model.Site
	if siteKey != "" {
		if s := cfg.GetSite(siteKey); s != nil {
			site = *s
		} else if !service.IsDirectPlayKey(siteKey) {
			site = cfg.Home()
		}
	} else {
		site = cfg.Home()
	}
	if site.Key == "" && !service.IsDirectPlayKey(siteKey) {
		site = cfg.Home()
	}
	return site
}

func (a *App) resolveEpisodeURL(vodID, flag, episodeURL string, episodeIndex int) (string, error) {
	_, sites, _ := a.scope()
	epURL := strings.TrimSpace(episodeURL)
	if epURL == "" {
		if u, ok := sites.EpisodeURLByIndex(vodID, flag, episodeIndex); ok {
			epURL = u
		}
	}
	if epURL == "" {
		return "", fmt.Errorf("empty episode url")
	}
	return epURL, nil
}

// episodeNameFromCache 从详情缓存取集名（供猫爪 getPlayInfo）。
func episodeNameFromCache(v model.Vod, flag string, episodeIndex int, epURL string) string {
	v.SetVodFlags()
	flag = strings.TrimSpace(flag)
	epURL = strings.TrimSpace(epURL)
	for _, f := range v.VodFlags {
		if flag != "" && f.Flag != flag && f.Show != flag {
			continue
		}
		if episodeIndex >= 0 && episodeIndex < len(f.Episodes) {
			if n := strings.TrimSpace(f.Episodes[episodeIndex].Name); n != "" {
				return n
			}
		}
		if epURL != "" {
			for _, ep := range f.Episodes {
				if strings.TrimSpace(ep.URL) == epURL {
					return strings.TrimSpace(ep.Name)
				}
			}
		}
	}
	return ""
}

func prepareDesktopDRM(result *model.Result) error {
	if runtime.GOOS == "android" || result == nil {
		return nil
	}
	prepared, err := model.PrepareForDesktop(result.Drm)
	if err != nil {
		return err
	}
	result.Drm = prepared
	return nil
}

// resolvePlayerMedia 跑站点 playerContent 与二次解析，返回 ConvertScheme 后的地址与爬虫/站源 Header。
// parseFallbackOK 在二次解析失败时判断直链候选是否仍可用；nil 时仅接受视频直链或磁力。
func (a *App) resolvePlayerMedia(siteKey string, site model.Site, flag, epURL string, qualIdx int, parseFallbackOK func(string) bool) (playerMedia, error) {
	out := playerMedia{Site: site, EpURL: epURL}
	if thunder.Match(epURL) {
		out.PlayURL = epURL
		return out, nil
	}

	cfg, sites, _ := a.scope()
	result, err := sites.PlayerContent(siteKey, site, flag, epURL)
	if err != nil {
		return out, err
	}
	if err := prepareDesktopDRM(&result); err != nil {
		return out, err
	}

	out.PlayMsg = result.EffectiveMsg()
	out.Format = strings.TrimSpace(result.Format)
	out.Headers = copyStringMap(map[string]string(result.Header))
	out.Result = result

	api := cfg.API()
	rules := parse.GetRules()
	if len(rules) == 0 {
		rules = api.Rules
	}
	out.DidParse = parse.NeedParse(result) || parse.IsUseParse(result, api.Flags, api.Parses)
	parsed, perr := parse.ResolveWithParses(result, parse.Options{
		Parses:    api.Parses,
		Flags:     api.Flags,
		Rules:     rules,
		Jar:       api.Spider,
		Flag:      flag,
		Click:     result.Click,
		SiteClick: site.Click,
		Prefer:    settings.Get(settings.PreferredParse),
		IsVideo: func(u string) bool {
			return sites.IsVideoFormat(site, u)
		},
		ManualVideo: sites.ManualVideoCheck(site),
	})
	if perr != nil {
		cand := apiResolvePlayURL("", result.PlayURL, result.URL.URLs, qualIdx)
		ok := parse.IsVideoFormat(cand) || thunder.Match(cand)
		if parseFallbackOK != nil {
			ok = parseFallbackOK(cand)
		}
		if cand != "" && ok {
			out.PlayURL = localproxy.ConvertScheme(cand)
			out.DidParse = false
			return out, nil
		}
		return out, parse.AnnotateParseErr(perr)
	}

	result = parsed
	if err := prepareDesktopDRM(&result); err != nil {
		return out, err
	}
	out.Result = result
	out.Headers = copyStringMap(map[string]string(result.Header))
	out.PlayURL = localproxy.ConvertScheme(apiResolvePlayURL("", result.PlayURL, result.URL.URLs, qualIdx))
	if s := strings.TrimSpace(result.Format); s != "" {
		out.Format = s
	}
	out.PlayMsg = result.EffectiveMsg()
	return out, nil
}
