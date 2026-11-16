package app

import (
	"fmt"
	"io"
	"log"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bobo/KOTV/internal/auth"
	"github.com/bobo/KOTV/internal/cast"
	"github.com/bobo/KOTV/internal/clientsession"
	"github.com/bobo/KOTV/internal/config"
	"github.com/bobo/KOTV/internal/database"
	"github.com/bobo/KOTV/internal/dlna"
	"github.com/bobo/KOTV/internal/download"
	"github.com/bobo/KOTV/internal/hostclient"
	"github.com/bobo/KOTV/internal/live"
	"github.com/bobo/KOTV/internal/model"
	"github.com/bobo/KOTV/internal/parse"
	"github.com/bobo/KOTV/internal/paths"
	"github.com/bobo/KOTV/internal/player"
	"github.com/bobo/KOTV/internal/player/embed"
	"github.com/bobo/KOTV/internal/remote"
	appruntime "github.com/bobo/KOTV/internal/runtime"
	"github.com/bobo/KOTV/internal/server"
	"github.com/bobo/KOTV/internal/service"
	"github.com/bobo/KOTV/internal/settings"
	"github.com/bobo/KOTV/internal/spider"
	"github.com/bobo/KOTV/internal/util"
)

// App 全局应用状态。
type App struct {
	DB     *database.DB
	Config *config.Manager
	Sites  *service.SiteService
	Live   *live.Service
	Server *server.Server

	// 本机点播配置的加载状态。启动时在后台加载，HTTP 不等它（见 New）。
	cfgMu     sync.RWMutex
	cfgReady  bool
	cfgErr    string
	cfgLoaded chan struct{} // 启动时那次加载结束后关闭

	sessions *clientsession.Hub
	presence *clientsession.Presence

	CurrentScreen    string
	PendingSearch    string
	RemoteSearchAuto bool

	mediaMu    sync.RWMutex
	mediaState string
	mediaTitle string
	mediaURL   string

	// castDevs 按发现顺序追加，toolCast 用下标选设备；castGen 区分新旧搜索。
	castMu        sync.Mutex
	castDevs      []cast.Device
	castGen       uint64
	castSearching bool
	castErr       string

	// APILiveLoad 序列化缓存：按 Scope+URL 键复用 DTO（含媒体钥匙，不可跨用户命中）。
	liveLoadMu        sync.Mutex
	liveLoadCachedKey string
	liveLoadCached    map[string]any
}

func New() (*App, error) {
	_ = os.MkdirAll(os.TempDir(), 0o755)
	setupFileLog()

	db, err := database.Open()
	if err != nil {
		return nil, fmt.Errorf("数据库初始化失败: %w", err)
	}
	settings.BindDB(db)
	auth.BindDB(db)
	if err := settings.Load(); err != nil {
		log.Printf("加载设置失败: %v", err)
	}
	settings.EnsureDeviceUUID()
	settings.EnsureSyncPairCode()
	if err := auth.Init(); err != nil {
		log.Printf("auth init: %v", err)
	}
	util.SetProxy(settings.Get(settings.Proxy))
	spider.SetUserProxy(settings.Get(settings.Proxy))

	cfg := config.NewManager(db)
	sites := service.NewSiteService(cfg)
	liveSvc := live.NewService(cfg)

	a := &App{
		DB:            db,
		Config:        cfg,
		Sites:         sites,
		Live:          liveSvc,
		sessions:      clientsession.NewHub(),
		CurrentScreen: "video",
		mediaState:    "idle",
	}
	a.presence = clientsession.NewPresence(func(userID string) {
		a.releaseUserRuntime(userID)
	})
	// Chromium 挂到统一取消/Kill 门面（避免 spider↔parse 循环依赖）。
	spider.SetRuntimeExtraHooks(spider.RuntimeExtraHooks{
		SoftCancelScope: parse.CancelSniffsForScope,
		SoftCancelAll:   parse.SoftCancelAllSniffs,
		KillUser:        parse.CancelUserSniffs,
		HardRestartShared: func() {
			parse.HardRestartSharedChromium()
			a.rehydrateAllAfterHardRestart()
		},
		Shutdown: parse.ShutdownAllChromium,
	})

	a.Server = server.New()
	a.Server.SetContentAPI(a)
	download.SetResolver(a)
	download.Start()
	a.Server.SetMediaProvider(func() map[string]string {
		return remote.SnapshotMedia()
	})
	a.Server.SetTvbusProvider(func() string {
		lv := a.scopeLive()
		if lv == nil {
			return server.ResolveCoreResp(config.Default().API().Lives, nil)
		}
		return server.ResolveCoreResp(append([]model.Live(nil), lv.Sources()...), lv.Current())
	})

	// HTTP 先起：http(s) 点播源要联网，断网或没外网时会卡到请求超时（约 30 秒），
	// 前端等不到端口就判定连不上本机后端。配置在后台加载，加载期间
	// 健康检查/配置接口报 loading；远端会话在 scope 里等加载结束再克隆配置，避免克隆到空配置。
	cfg.EnsureVodFromHistory()
	a.cfgLoaded = make(chan struct{})
	if err := a.Server.Start(); err != nil {
		_ = db.Close()
		return nil, err
	}
	go a.loadInitialConfig()
	syncH := server.NewAppSyncHandler(db, func(code string) bool {
		return code != "" && code == settings.Get(settings.SyncPairCode)
	})
	syncH.LoadConfig = a.loadVodURL
	a.Server.SetSyncHandler(syncH)
	a.Server.SetLibraryDB(db)

	go a.listenEvents()

	if settings.IsDLNARenderer() {
		a.startDLNARenderer()
	}

	st := appruntime.Status()
	log.Printf("运行时[%s] java=%s python=%s chromium=%s ffmpeg=%s bridge=%s",
		appruntime.Platform(), st["java"], st["python"], st["chromium"], st["ffmpeg"], st["bridge"])

	return a, nil
}

// scope 返回当前点播配置与 SiteService。
// 本机（非 Dedicated）：始终全局 App.Config，偏好落 setting(owner=”)。
// 远端 Dedicated：内存 ephemeral 会话，偏好落 setting(owner=userId)。
func (a *App) scope() (cfg *config.Manager, sites *service.SiteService, sess *clientsession.Session) {
	if !hostclient.DedicatedRuntime() {
		return a.Config, a.Sites, nil
	}
	cid := hostclient.ScopeID()
	if cid == "" {
		return a.Config, a.Sites, nil
	}
	a.waitConfigLoaded()
	sess = a.sessions.GetOrCreate(cid, func() *clientsession.Session {
		s := &clientsession.Session{ClientID: cid}
		s.SetRepoBaseline(a.siteRepoURLs())
		s.SetLiveBaseline(a.liveSourceURLs())
		a.useHostConfig(s)
		return s
	})
	if sess.Sites == nil || sess.Sites.Config() != sess.Cfg {
		log.Printf("session %s: rebound Sites to Cfg", cid)
		bindSessionConfig(sess, sess.Cfg)
	}
	a.bootstrapSession(sess)
	return sess.Cfg, sess.Sites, sess
}

// scopeReady 当前点播配置，以及它是否已加载就绪（远端会话看会话自己的状态）。
func (a *App) scopeReady() (*config.Manager, bool) {
	cfg, _, sess := a.scope()
	if sess != nil {
		return cfg, sess.Ready
	}
	ready, _ := a.configState()
	return cfg, ready
}

// loadInitialConfig 启动时加载点播配置；结束后关闭 cfgLoaded。
func (a *App) loadInitialConfig() {
	defer close(a.cfgLoaded)
	if err := a.Config.InitFromSettings(); err != nil {
		a.setConfigState(false, err.Error())
		log.Printf("配置加载: %v", err)
	} else {
		a.setConfigState(true, "")
	}
	a.Live.SyncFromConfig()
}

// waitConfigLoaded 等启动时那次配置加载结束（之后立即返回）。
func (a *App) waitConfigLoaded() {
	if a.cfgLoaded != nil {
		<-a.cfgLoaded
	}
}

// configLoading 启动时那次配置加载是否还在进行。
func (a *App) configLoading() bool {
	if a.cfgLoaded == nil {
		return false
	}
	select {
	case <-a.cfgLoaded:
		return false
	default:
		return true
	}
}

func (a *App) configState() (ready bool, errMsg string) {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return a.cfgReady, a.cfgErr
}

func (a *App) setConfigState(ready bool, errMsg string) {
	a.cfgMu.Lock()
	a.cfgReady, a.cfgErr = ready, errMsg
	a.cfgMu.Unlock()
}

// useHostConfig 会话改用主机当前点播配置的副本：来源、就绪状态都跟主机一致，之后随主机换源。
func (a *App) useHostConfig(sess *clientsession.Session) {
	bindSessionConfig(sess, a.Config.CloneEphemeral())
	sess.Source = strings.TrimSpace(a.Config.API().URL)
	sess.Ready, sess.ErrMsg = a.configState()
	sess.FollowHost = true
}

// loadSessionSource 会话加载用户自己的点播源。失败时主机配置可用就退回主机的，否则记下错误。
func (a *App) loadSessionSource(sess *clientsession.Session, src string) {
	if err := sess.Cfg.LoadFromSource(src); err != nil {
		if ready, _ := a.configState(); ready {
			a.useHostConfig(sess)
			return
		}
		sess.Ready, sess.ErrMsg = false, err.Error()
		return
	}
	sess.Source, sess.Ready, sess.ErrMsg, sess.FollowHost = src, true, "", false
	if sess.Live != nil {
		sess.Live.SyncFromConfig()
	}
}

// bindSessionConfig 替换会话配置时必须同步 Sites/Live，否则换源/换站只改 Cfg，
// 内容请求仍打旧空 Manager，表现为长期 Get "": unsupported protocol scheme ""。
func bindSessionConfig(sess *clientsession.Session, cfg *config.Manager) {
	if sess == nil || cfg == nil {
		return
	}
	sess.Cfg = cfg
	sess.Sites = service.NewSiteService(cfg)
	liveSvc := live.NewService(cfg)
	liveSvc.SyncFromConfig()
	sess.Live = liveSvc
	cfg.SetRepoWriteHook(sess.NoteRepo)
}

func (a *App) siteRepoURLs() map[string]struct{} {
	out := map[string]struct{}{}
	if a == nil || a.DB == nil {
		return out
	}
	cfgs, err := a.DB.ListConfigs(int64(database.ConfigTypeSite))
	if err != nil {
		return out
	}
	for _, c := range cfgs {
		u := strings.TrimSpace(c.URL)
		if u != "" {
			out[u] = struct{}{}
		}
	}
	return out
}

func (a *App) liveSourceURLs() map[string]struct{} {
	out := map[string]struct{}{}
	if a == nil || a.DB == nil {
		return out
	}
	cfgs, err := a.DB.ListConfigs(int64(database.ConfigTypeLive))
	if err != nil {
		return out
	}
	for _, c := range cfgs {
		u := strings.TrimSpace(c.URL)
		if u != "" {
			out[u] = struct{}{}
		}
	}
	return out
}

func sessionOwnedByUser(s *clientsession.Session, userID string) bool {
	if s == nil || userID == "" {
		return false
	}
	id := s.ClientID
	return id == "u:"+userID || id == userID || strings.HasSuffix(id, ":"+userID)
}

// dropSessionRepos 删掉远端会话新写入、且不是本机当前 settings.VOD 的仓库行。本机会话留下。
func (a *App) dropSessionRepos(sess *clientsession.Session) {
	if a == nil || sess == nil || a.DB == nil || !strings.HasPrefix(sess.ClientID, "u:") {
		return
	}
	added := sess.AddedRepos()
	if len(added) == 0 {
		return
	}
	held := map[string]struct{}{}
	if a.sessions != nil {
		a.sessions.ForEach(func(other *clientsession.Session) {
			if other == nil || other.ClientID == sess.ClientID {
				return
			}
			for u := range other.AddedRepos() {
				held[u] = struct{}{}
			}
		})
	}
	keep := strings.TrimSpace(settings.Get(settings.VOD))
	for _, u := range clientsession.URLsAddedBySession(added, keep, held) {
		if err := a.DB.DeleteConfigByURL(u, database.ConfigTypeSite); err != nil {
			log.Printf("session %s: drop repo %s: %v", sess.ClientID, u, err)
		}
	}
}

// dropSessionLives 对齐 dropSessionRepos：清本会话新增且无人占用的直播源行。
func (a *App) dropSessionLives(sess *clientsession.Session) {
	if a == nil || sess == nil || a.DB == nil || !strings.HasPrefix(sess.ClientID, "u:") {
		return
	}
	added := sess.AddedLives()
	if len(added) == 0 {
		return
	}
	held := map[string]struct{}{}
	if a.sessions != nil {
		a.sessions.ForEach(func(other *clientsession.Session) {
			if other == nil || other.ClientID == sess.ClientID {
				return
			}
			for u := range other.AddedLives() {
				held[u] = struct{}{}
			}
		})
	}
	keep := strings.TrimSpace(settings.Get(settings.LIVE))
	for _, u := range clientsession.URLsAddedBySession(added, keep, held) {
		if err := a.DB.DeleteConfigByURL(u, database.ConfigTypeLive); err != nil {
			log.Printf("session %s: drop live %s: %v", sess.ClientID, u, err)
		}
	}
}

func (a *App) dropReposForUser(userID string) {
	userID = strings.TrimSpace(userID)
	if a == nil || userID == "" || a.sessions == nil {
		return
	}
	var list []*clientsession.Session
	a.sessions.ForEach(func(s *clientsession.Session) {
		if sessionOwnedByUser(s, userID) {
			list = append(list, s)
		}
	})
	for _, s := range list {
		a.dropSessionRepos(s)
		a.dropSessionLives(s)
	}
}

// releaseUserRuntime 释放该用户的会话仓与全部运行时（JVM/Py/JS/Node/Chromium；不删库 / TV）。
func (a *App) releaseUserRuntime(userID string) {
	userID = strings.TrimSpace(userID)
	if a == nil || userID == "" {
		return
	}
	a.dropReposForUser(userID)
	spider.KillUserRuntime(userID) // 含 Chromium（RuntimeExtraHooks）
	if a.sessions != nil {
		a.sessions.RemoveByUser(userID)
	}
}

// bootstrapSession 远端会话首次进入时恢复该用户上次选的点播源与首页站（scope 下 settings 已按用户分开）。
// 用户没选过就跟随主机。末尾再水合 bridge 侧 net/jar（KillUser 后跟主机路径不会走 LoadFromSource）。
func (a *App) bootstrapSession(sess *clientsession.Session) {
	if sess == nil || sess.Bootstrapped {
		return
	}
	sess.Bootstrapped = true
	if src := strings.TrimSpace(settings.Get(settings.VOD)); src != "" {
		if src == sess.Source {
			sess.FollowHost = false
		} else {
			a.loadSessionSource(sess, src)
		}
	}
	if homeKey := strings.TrimSpace(settings.Get(settings.Home)); homeKey != "" && sess.Cfg != nil {
		if site := sess.Cfg.GetSite(homeKey); site != nil {
			sess.Cfg.SetHome(*site)
		}
	}
	a.rehydrateSessionSpider(sess)
}

// rehydrateAllAfterHardRestart clearAll 卸全体 CL 后：本机 + 在线 Dedicated 会话再写根 jar。
func (a *App) rehydrateAllAfterHardRestart() {
	if a == nil {
		return
	}
	if a.Config != nil {
		api := a.Config.API()
		if sp := strings.TrimSpace(api.Spider); sp != "" {
			if err := spider.EnsureRootJar(sp, api.URL); err != nil {
				log.Printf("hard restart rehydrate shared jar: %v", err)
			}
		}
	}
	if a.sessions == nil {
		return
	}
	a.sessions.ForEach(func(sess *clientsession.Session) {
		if sess == nil {
			return
		}
		leave := bindSessionScope(sess)
		a.rehydrateSessionSpider(sess)
		leave()
	})
}

// rehydrateSessionSpider 在当前 Bind 下按会话配置重放 ConfigBase/NetConfig/EnsureJar。
// 失败只记日志，不杀唯一 JVM。
func (a *App) rehydrateSessionSpider(sess *clientsession.Session) {
	if sess == nil || sess.Cfg == nil {
		return
	}
	api := sess.Cfg.API()
	base := strings.TrimSpace(api.URL)
	if base != "" {
		spider.SetConfigBase(base)
	}
	spider.SetNetConfig(api.Headers, api.Proxy, api.Hosts, nil)
	sp := strings.TrimSpace(api.Spider)
	if sp == "" {
		return
	}
	if err := spider.EnsureRootJar(sp, base); err != nil {
		log.Printf("session %s: rehydrate jar: %v", sess.ClientID, err)
	}
}

// scopeLive 当前 Scope 的直播服务（有会话则用会话内选中源）。
func (a *App) scopeLive() *live.Service {
	_, _, sess := a.scope()
	if sess != nil && sess.Live != nil {
		return sess.Live
	}
	return a.Live
}

func (a *App) savedLiveSources() []model.Live {
	var out []model.Live
	seen := map[string]struct{}{}
	if a.DB != nil {
		cfgs, err := a.DB.ListConfigs(int64(database.ConfigTypeLive))
		if err == nil {
			for _, c := range cfgs {
				url := strings.TrimSpace(c.URL)
				if url == "" {
					continue
				}
				seen[url] = struct{}{}
				out = append(out, model.Live{Name: strings.TrimSpace(c.Name), URL: url})
			}
		}
	}
	current := strings.TrimSpace(settings.Get(settings.LIVE))
	if current != "" {
		if _, ok := seen[current]; !ok {
			a.persistLiveSource(current)
			out = append([]model.Live{{Name: "", URL: current}}, out...)
		}
	}
	return out
}

func (a *App) persistLiveSource(raw string) string {
	url := config.NormalizeSource(strings.TrimSpace(raw))
	if url == "" || a.DB == nil {
		return url
	}
	if _, err := a.DB.UpsertConfig(&database.Config{
		Type: database.ConfigTypeLive,
		URL:  url,
	}); err != nil {
		log.Printf("persist live source %s: %v", url, err)
	}
	if hostclient.DedicatedRuntime() {
		if _, _, sess := a.scope(); sess != nil {
			sess.NoteLive(url)
		}
	}
	return url
}

// syncLive 只同步当前 Scope 的 Live。远端 Bind 下禁止 SyncFromConfig 本机 a.Live（会把远端 LIVE 选择写进本机内存）。
func (a *App) syncLive() {
	if lv := a.scopeLive(); lv != nil {
		lv.SyncFromConfig()
	}
}

func (a *App) listenEvents() {
	go func() {
		for ev := range a.Server.Events().SubscribeSearch() {
			a.PendingSearch = ev.Word
			a.CurrentScreen = "search"
			a.RemoteSearchAuto = true
			remote.NotifySearch(ev.Word, ev.ClientID)
		}
	}()
	go func() {
		for url := range a.Server.Events().SubscribePush() {
			a.openPushURL(url)
			remote.NotifyPush()
		}
	}()
	go func() {
		for ev := range a.Server.Events().SubscribeSetting() {
			a.applyRemoteSetting(ev.Name, ev.Value)
		}
	}()
	go func() {
		for ev := range a.Server.Events().SubscribeControl() {
			remote.NotifyControl(ev.Type, ev.SeekMs, ev.ClientID)
		}
	}()
	go func() {
		for text := range a.Server.Events().SubscribeDanmaku() {
			remote.DefaultQueue.PushLiveDanmaku(text, "")
		}
	}()
	go func() {
		for ev := range a.Server.Events().SubscribeRefresh() {
			switch strings.ToLower(ev.Type) {
			case "live":
				// 推到 Flutter：清内存表并重拉当前直播源频道列表。
				remote.DefaultQueue.PushRefresh("live", "", "")
			case "detail", "player", "category":
				remote.DefaultQueue.PushRefresh(strings.ToLower(ev.Type), "", "")
			case "subtitle":
				if enh, ok := embed.ActiveEnhanced(); ok && ev.Path != "" {
					_ = enh.AddSubtitleFile(ev.Path)
				}
				if ev.Path != "" {
					remote.DefaultQueue.PushRefresh("subtitle", ev.Path, "")
				}
			case "danmaku":
				path := ev.Path
				if path == "" {
					continue
				}
				// 前端收到路径后经 /api/v1/danmaku 让引擎读本机文件并解析。
				remote.DefaultQueue.PushRefresh("danmaku", path, "")
			}
		}
	}()
	go func() {
		for ev := range a.Server.Events().SubscribeCast() {
			a.ApplyRemoteCast(ev.Config, ev.Device, ev.History)
		}
	}()
	go func() {
		for ev := range a.Server.Events().SubscribeOpen() {
			kind := strings.ToLower(strings.TrimSpace(ev.Kind))
			if kind == "" {
				kind = "detail"
			}
			site := strings.TrimSpace(ev.Site)
			id := strings.TrimSpace(ev.ID)
			url := strings.TrimSpace(ev.URL)
			if kind == "play" {
				if url == "" {
					url = id
				}
				if url == "" {
					continue
				}
				if site == "" {
					site = service.PushAgentKey
				}
				if id == "" {
					id = url
				}
			} else if id == "" {
				continue
			}
			name := strings.TrimSpace(ev.Name)
			if name == "" {
				name = id
			}
			remote.DefaultQueue.PushOpen(remote.OpenCmd{
				Kind:     kind,
				Site:     site,
				ID:       id,
				Name:     name,
				Pic:      ev.Pic,
				Flag:     ev.Flag,
				Remarks:  ev.Remarks,
				URL:      url,
				Position: ev.Position,
			}, ev.ClientID)
		}
	}()
}

func (a *App) applyRemoteSetting(name, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "", "vod":
		settings.Set(settings.VOD, value)
	case "live":
		url := a.persistLiveSource(value)
		settings.Set(settings.LIVE, url)
	default:
		settings.Set(settings.Type(name), value)
	}
	_ = settings.Save()
	_ = a.ReloadConfig()
}

func (a *App) ReloadConfig() error {
	return a.reloadConfig(func() error { return a.Config.LoadSettings(false) })
}

// LoadVodSource 从 URL 或 JSON 正文加载点播配置。
func (a *App) LoadVodSource(source string) error {
	return a.reloadConfig(func() error { return a.Config.LoadVod(source, false) })
}

func (a *App) reloadConfig(load func() error) error {
	a.waitConfigLoaded()
	util.SetProxy(settings.Get(settings.Proxy))
	spider.SetUserProxy(settings.Get(settings.Proxy))
	// 换仓前先不可用，避免 Clear 窗口内仍 Ready 去 Get ""。
	a.setConfigState(false, "")
	a.Sites.DropContentCache()
	a.Config.Clear()
	if err := load(); err != nil {
		a.setConfigState(false, err.Error())
		a.switchConfigJars(model.Api{})
		return err
	}
	a.switchConfigJars(a.Config.API())
	a.setConfigState(true, "")
	a.Live.SyncFromConfig()
	a.followHostSessions()
	return nil
}

// followHostSessions 主机配置变了：跟随主机的远端会话换成主机的新配置；选过自己源的会话不动。
func (a *App) followHostSessions() {
	if a.sessions == nil {
		return
	}
	a.sessions.ForEach(func(sess *clientsession.Session) {
		if sess == nil || !sess.FollowHost {
			return
		}
		leave := bindSessionScope(sess)
		a.useHostConfig(sess)
		a.rehydrateSessionSpider(sess)
		leave()
	})
}

// bindSessionScope 把 goroutine 绑到该远端会话的 Dedicated 身份（LoadJar/Net 写对用户桶）。
func bindSessionScope(sess *clientsession.Session) func() {
	if sess == nil {
		return func() {}
	}
	uid := hostclient.UserIDFromScope(sess.ClientID)
	return hostclient.Bind(hostclient.Request{
		ClientID:  sess.ClientID,
		UserID:    uid,
		Dedicated: uid != "",
	})
}

type sessionSourceSnap struct {
	sess *clientsession.Session
	src  string
	home string
}

// snapshotSessionSources 记下选过自己源的远端会话（清理爬虫包后按此重拉；跟随主机的随主机重载）。
func (a *App) snapshotSessionSources() []sessionSourceSnap {
	if a == nil || a.sessions == nil {
		return nil
	}
	var out []sessionSourceSnap
	a.sessions.ForEach(func(sess *clientsession.Session) {
		if sess == nil || sess.FollowHost {
			return
		}
		src := strings.TrimSpace(sess.Source)
		if src == "" {
			return
		}
		home := ""
		if sess.Cfg != nil {
			home = strings.TrimSpace(sess.Cfg.Home().Key)
		}
		out = append(out, sessionSourceSnap{
			sess: sess,
			src:  src,
			home: home,
		})
	})
	return out
}

// restoreSessionSources 按清理前记下的会话源重新拉取。http(s) 不走库内 JSON。
// 每个会话在 Dedicated Bind 下重载，避免 LoadJar 写到 shared 桶。
func (a *App) restoreSessionSources(snaps []sessionSourceSnap) error {
	var errs []string
	for _, s := range snaps {
		if s.sess == nil || s.sess.Cfg == nil {
			errs = append(errs, "会话配置未就绪")
			continue
		}
		leave := bindSessionScope(s.sess)
		err := s.sess.Cfg.ReloadFresh(s.src)
		if err != nil {
			leave()
			errs = append(errs, err.Error())
			continue
		}
		if s.home != "" {
			if site := s.sess.Cfg.GetSite(s.home); site != nil {
				s.sess.Cfg.SetHome(*site)
			}
		}
		s.sess.Source = s.src
		s.sess.Ready = true
		s.sess.ErrMsg = ""
		if s.sess.Live != nil {
			s.sess.Live.SyncFromConfig()
		}
		if s.sess.Sites != nil {
			s.sess.Sites.InvalidateHomeOnly()
		}
		a.rehydrateSessionSpider(s.sess)
		leave()
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func (a *App) OpenPushURL(url string) {
	a.openPushURL(url)
}

func (a *App) openPushURL(url string) {
	url = strings.TrimSpace(url)
	url = strings.TrimPrefix(url, "file://")
	if !server.MatchPushURL(url) {
		return
	}
	name := "推送播放"
	play := url
	if filepath.IsAbs(url) || (!strings.Contains(url, "://") && strings.Contains(url, string(os.PathSeparator))) {
		if abs, err := filepath.Abs(url); err == nil {
			play = abs
			name = filepath.Base(abs)
		}
	} else if u, err := neturl.Parse(url); err == nil && u.Scheme == "" {
		name = filepath.Base(url)
	} else if u, err := neturl.Parse(url); err == nil && u.Path != "" {
		if base := filepath.Base(u.Path); base != "" && base != "/" && base != "." {
			name = base
		}
	}
	remote.DefaultQueue.PushOpen(remote.OpenCmd{
		Kind: "play",
		Site: service.PushAgentKey,
		ID:   play,
		Name: name,
		URL:  play,
	}, "")
}

func (a *App) Shutdown() {
	if a.presence != nil {
		a.presence.Stop()
	}
	spider.ShutdownAllRuntimes()
	dlna.StopRenderer()
	player.Stop()
	a.Server.Stop()
	_ = settings.Save()
	_ = a.DB.Close()
}

// SyncDLNARenderer 按设置启停被投端。
func (a *App) SyncDLNARenderer() {
	if settings.IsDLNARenderer() {
		a.startDLNARenderer()
		return
	}
	dlna.StopRenderer()
}

func (a *App) startDLNARenderer() {
	err := dlna.StartRendererHooks(dlna.RendererHooks{
		OnURI: func(uri string) {
			a.OpenPushURL(uri)
			remote.NotifyPush()
		},
		OnStop: func() {
			remote.NotifyControl("stop", 0, "")
		},
		OnPause: func(pause bool) {
			if pause {
				remote.NotifyControl("pause", 0, "")
			} else {
				remote.NotifyControl("play", 0, "")
			}
		},
		OnSeek: func(ms int64) {
			remote.NotifyControl("seek", ms, "")
		},
		OnNext: func() {
			remote.NotifyControl("next", 0, "")
		},
		State: func() dlna.MediaState {
			st := dlna.MediaState{URI: a.MediaURL(), State: "NO_MEDIA_PRESENT"}
			eng := embed.Active()
			st.PosMs = eng.PositionMs()
			st.DurMs = eng.DurationMs()
			switch {
			case eng.IsPlaying():
				st.State = "PLAYING"
			case st.DurMs > 0 || st.PosMs > 0:
				st.State = "PAUSED_PLAYBACK"
			case st.URI != "":
				st.State = "STOPPED"
			}
			return st
		},
	})
	if err != nil {
		log.Printf("DLNA DMR 启动失败: %v", err)
	}
}

func (a *App) SetMediaPlaying(title, url string) {
	_, _, sess := a.scope()
	if sess != nil {
		sess.SetMediaPlaying(title, url)
	} else {
		a.mediaMu.Lock()
		a.mediaState = "playing"
		a.mediaTitle = title
		a.mediaURL = url
		a.mediaMu.Unlock()
	}
	remote.SetMediaStore(map[string]string{
		"state": "playing",
		"title": title,
		"url":   url,
	})
}

func (a *App) SetMediaIdle() {
	_, _, sess := a.scope()
	if sess != nil {
		sess.SetMediaIdle()
	} else {
		a.mediaMu.Lock()
		a.mediaState = "idle"
		a.mediaTitle = ""
		a.mediaURL = ""
		a.mediaMu.Unlock()
	}
	remote.SetMediaStore(map[string]string{"state": "idle", "title": "未播放"})
}

func (a *App) MediaURL() string {
	_, _, sess := a.scope()
	if sess != nil {
		return sess.MediaURL()
	}
	a.mediaMu.RLock()
	defer a.mediaMu.RUnlock()
	return a.mediaURL
}

func (a *App) MediaTitle() string {
	_, _, sess := a.scope()
	if sess != nil {
		return sess.MediaTitle()
	}
	a.mediaMu.RLock()
	defer a.mediaMu.RUnlock()
	return a.mediaTitle
}

// PlayURLWithHistory 播放并绑定历史 key 以便续播写入。
func PlayURLWithHistory(url, histKey string) error {
	return player.Play(url, histKey)
}

// setupFileLog 同时写 stderr 与 {Root}/data/log/kotv.log，方便排障。
func setupFileLog() {
	dir := paths.LogDir()
	f, err := os.OpenFile(filepath.Join(dir, "kotv.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("无法打开日志文件: %v", err)
		return
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("日志文件: %s", filepath.Join(dir, "kotv.log"))
}
