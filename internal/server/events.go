package server

import (
	"strings"
	"sync"
)

// Events HTTP 遥控事件总线。
type Events struct {
	mu          sync.RWMutex
	searchSubs  []chan searchEvent
	pushSubs    []chan string
	danmakuSubs []chan string
	settingSubs []chan settingEvent
	controlSubs []chan controlEvent
	refreshSubs []chan refreshEvent
	castSubs    []chan castEvent
	openSubs    []chan openEvent
	postMsgSubs []chan string
	// 按 Flutter clientId 分队列；无 clientId 的消息进 untagged（广播/兼容）。
	postMsgQ        map[string][]string
	postMsgUntagged []string
	postMsgSeen     map[string]struct{}
	deliver         func(clientID, msg string) bool
}

type settingEvent struct {
	Value string
	Name  string
}

type searchEvent struct {
	Word     string
	ClientID string
}

type controlEvent struct {
	Type     string
	SeekMs   int64
	ClientID string
}

type refreshEvent struct {
	Type string // subtitle / danmaku / detail / player / live
	Path string
}

type castEvent struct {
	Config  string
	Device  string
	History string
}

// openEvent 遥控片库点选：打开详情或直链播放。
type openEvent struct {
	Kind     string
	Site     string
	ID       string
	Name     string
	Pic      string
	Flag     string
	Remarks  string
	URL      string
	Position int64
	ClientID string
}

func NewEvents() *Events {
	return &Events{
		postMsgQ:    map[string][]string{},
		postMsgSeen: map[string]struct{}{},
	}
}

func (e *Events) SubscribeSearch() <-chan searchEvent   { return e.subscribeSearch(8) }
func (e *Events) SubscribePush() <-chan string          { return e.subscribe(&e.pushSubs, 8) }
func (e *Events) SubscribeDanmaku() <-chan string       { return e.subscribe(&e.danmakuSubs, 32) }
func (e *Events) SubscribeSetting() <-chan settingEvent { return e.subscribeSetting(4) }
func (e *Events) SubscribeControl() <-chan controlEvent { return e.subscribeControl(16) }
func (e *Events) SubscribeRefresh() <-chan refreshEvent { return e.subscribeRefresh(8) }
func (e *Events) SubscribeCast() <-chan castEvent       { return e.subscribeCast(4) }
func (e *Events) SubscribeOpen() <-chan openEvent       { return e.subscribeOpen(8) }

func (e *Events) subscribeSearch(cap int) <-chan searchEvent {
	e.mu.Lock()
	defer e.mu.Unlock()
	ch := make(chan searchEvent, cap)
	e.searchSubs = append(e.searchSubs, ch)
	return ch
}

func (e *Events) subscribeSetting(cap int) <-chan settingEvent {
	e.mu.Lock()
	defer e.mu.Unlock()
	ch := make(chan settingEvent, cap)
	e.settingSubs = append(e.settingSubs, ch)
	return ch
}

func (e *Events) subscribeControl(cap int) <-chan controlEvent {
	e.mu.Lock()
	defer e.mu.Unlock()
	ch := make(chan controlEvent, cap)
	e.controlSubs = append(e.controlSubs, ch)
	return ch
}

func (e *Events) subscribeRefresh(cap int) <-chan refreshEvent {
	e.mu.Lock()
	defer e.mu.Unlock()
	ch := make(chan refreshEvent, cap)
	e.refreshSubs = append(e.refreshSubs, ch)
	return ch
}

func (e *Events) subscribeCast(cap int) <-chan castEvent {
	e.mu.Lock()
	defer e.mu.Unlock()
	ch := make(chan castEvent, cap)
	e.castSubs = append(e.castSubs, ch)
	return ch
}

func (e *Events) subscribeOpen(cap int) <-chan openEvent {
	e.mu.Lock()
	defer e.mu.Unlock()
	ch := make(chan openEvent, cap)
	e.openSubs = append(e.openSubs, ch)
	return ch
}

func (e *Events) subscribe(subs *[]chan string, cap int) <-chan string {
	e.mu.Lock()
	defer e.mu.Unlock()
	ch := make(chan string, cap)
	*subs = append(*subs, ch)
	return ch
}

func (e *Events) EmitSearch(word, clientID string) {
	e.emitSearch(searchEvent{Word: word, ClientID: strings.TrimSpace(clientID)})
}
func (e *Events) EmitPush(url string)     { e.emit(&e.pushSubs, url) }
func (e *Events) EmitDanmaku(text string) { e.emit(&e.danmakuSubs, text) }
func (e *Events) EmitSetting(cfg, name string) {
	e.emitSetting(settingEvent{Value: cfg, Name: name})
}
func (e *Events) EmitControl(typ string, seekMs int64, clientID string) {
	e.emitControl(controlEvent{Type: typ, SeekMs: seekMs, ClientID: strings.TrimSpace(clientID)})
}
func (e *Events) EmitRefresh(typ, path string) {
	e.emitRefresh(refreshEvent{Type: typ, Path: path})
}
func (e *Events) EmitCast(config, device, history string) {
	e.emitCast(castEvent{Config: config, Device: device, History: history})
}

func (e *Events) EmitOpen(ev openEvent) {
	e.emitOpen(ev)
}

// EmitPostMsg 无目标客户端时入 untagged / 广播。
func (e *Events) EmitPostMsg(msg string) {
	e.EmitPostMsgTo(msg, "")
}

// SetDeliver 在有界面套接字时直接推送。返回 false 的目标仍进队列，等下次连上再取。
func (e *Events) SetDeliver(fn func(clientID, msg string) bool) {
	e.mu.Lock()
	e.deliver = fn
	e.mu.Unlock()
}

// EmitPostMsgTo 将消息投递到指定 clientId；clientId 为空则发给已经出现过的客户端。
func (e *Events) EmitPostMsgTo(msg, clientID string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	clientID = strings.TrimSpace(clientID)
	e.mu.Lock()
	var targets []string
	queuedUntagged := false
	if clientID != "" {
		targets = []string{clientID}
	} else if len(e.postMsgSeen) == 0 {
		e.postMsgUntagged = append(e.postMsgUntagged, msg)
		e.trimUntaggedLocked()
		queuedUntagged = true
	} else {
		for cid := range e.postMsgSeen {
			targets = append(targets, cid)
		}
	}
	deliver := e.deliver
	e.mu.Unlock()
	if !queuedUntagged {
		var missed []string
		for _, cid := range targets {
			if deliver != nil && deliver(cid, msg) {
				continue
			}
			missed = append(missed, cid)
		}
		if len(missed) > 0 {
			e.mu.Lock()
			for _, cid := range missed {
				e.appendClientLocked(cid, msg)
			}
			e.mu.Unlock()
		}
	}
	e.emit(&e.postMsgSubs, msg)
}

func (e *Events) appendClientLocked(clientID, msg string) {
	if e.postMsgQ == nil {
		e.postMsgQ = map[string][]string{}
	}
	q := append(e.postMsgQ[clientID], msg)
	if len(q) > 64 {
		q = q[len(q)-64:]
	}
	e.postMsgQ[clientID] = q
}

func (e *Events) trimUntaggedLocked() {
	if len(e.postMsgUntagged) > 64 {
		e.postMsgUntagged = e.postMsgUntagged[len(e.postMsgUntagged)-64:]
	}
}

// DrainPostMsg 取出并清空指定客户端缓冲（界面套接字连上时先推完）。
// clientID 为空时只取 untagged（旧客户端兼容）。
func (e *Events) DrainPostMsg(clientID string) []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		out := e.postMsgUntagged
		e.postMsgUntagged = nil
		return out
	}
	if e.postMsgSeen == nil {
		e.postMsgSeen = map[string]struct{}{}
	}
	e.postMsgSeen[clientID] = struct{}{}
	out := e.postMsgQ[clientID]
	delete(e.postMsgQ, clientID)
	if len(e.postMsgUntagged) > 0 {
		out = append(append([]string{}, e.postMsgUntagged...), out...)
		e.postMsgUntagged = nil
	}
	return out
}

func (e *Events) emitSearch(msg searchEvent) {
	e.mu.RLock()
	snapshot := append([]chan searchEvent(nil), e.searchSubs...)
	e.mu.RUnlock()
	for _, ch := range snapshot {
		select {
		case ch <- msg:
		default:
			go func(c chan searchEvent, m searchEvent) { c <- m }(ch, msg)
		}
	}
}

func (e *Events) emitSetting(msg settingEvent) {
	e.mu.RLock()
	snapshot := append([]chan settingEvent(nil), e.settingSubs...)
	e.mu.RUnlock()
	for _, ch := range snapshot {
		select {
		case ch <- msg:
		default:
			go func(c chan settingEvent, m settingEvent) { c <- m }(ch, msg)
		}
	}
}

func (e *Events) emitControl(msg controlEvent) {
	e.mu.RLock()
	snapshot := append([]chan controlEvent(nil), e.controlSubs...)
	e.mu.RUnlock()
	for _, ch := range snapshot {
		select {
		case ch <- msg:
		default:
			go func(c chan controlEvent, m controlEvent) { c <- m }(ch, msg)
		}
	}
}

func (e *Events) emitRefresh(msg refreshEvent) {
	e.mu.RLock()
	snapshot := append([]chan refreshEvent(nil), e.refreshSubs...)
	e.mu.RUnlock()
	for _, ch := range snapshot {
		select {
		case ch <- msg:
		default:
			go func(c chan refreshEvent, m refreshEvent) { c <- m }(ch, msg)
		}
	}
}

func (e *Events) emitCast(msg castEvent) {
	e.mu.RLock()
	snapshot := append([]chan castEvent(nil), e.castSubs...)
	e.mu.RUnlock()
	for _, ch := range snapshot {
		select {
		case ch <- msg:
		default:
			go func(c chan castEvent, m castEvent) { c <- m }(ch, msg)
		}
	}
}

func (e *Events) emitOpen(msg openEvent) {
	e.mu.RLock()
	snapshot := append([]chan openEvent(nil), e.openSubs...)
	e.mu.RUnlock()
	for _, ch := range snapshot {
		select {
		case ch <- msg:
		default:
			go func(c chan openEvent, m openEvent) { c <- m }(ch, msg)
		}
	}
}

func (e *Events) emit(subs *[]chan string, msg string) {
	e.mu.RLock()
	snapshot := append([]chan string(nil), (*subs)...)
	e.mu.RUnlock()
	for _, ch := range snapshot {
		select {
		case ch <- msg:
		default:
			go func(c chan string, m string) { c <- m }(ch, msg)
		}
	}
}
