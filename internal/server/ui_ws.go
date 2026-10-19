package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/bobo/KOTV/internal/hostclient"
)

var uiUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

type uiSock struct {
	conn *websocket.Conn
	send chan []byte
	done chan struct{}
}

func newUISock(conn *websocket.Conn) *uiSock {
	return &uiSock{
		conn: conn,
		send: make(chan []byte, 128),
		done: make(chan struct{}),
	}
}

func (c *uiSock) sendText(msg string) bool {
	select {
	case <-c.done:
		return false
	default:
	}
	select {
	case c.send <- []byte(msg):
		return true
	case <-c.done:
		return false
	default:
		return false
	}
}

func (c *uiSock) sendWait(msg string) bool {
	select {
	case c.send <- []byte(msg):
		return true
	case <-c.done:
		return false
	}
}

func (c *uiSock) writeLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	defer close(c.done)
	defer c.conn.Close()
	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *uiSock) readLoop(onText func(string)) {
	c.conn.SetReadLimit(2 << 20)
	c.conn.SetReadDeadline(time.Now().Add(70 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(70 * time.Second))
		return nil
	})
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		c.conn.SetReadDeadline(time.Now().Add(70 * time.Second))
		onText(string(data))
	}
}

type uiHub struct {
	mu  sync.Mutex
	ui  map[string][]*uiSock
	src []*uiSock
}

func newUIHub() *uiHub {
	return &uiHub{ui: map[string][]*uiSock{}}
}

func (h *uiHub) addUI(clientID string, sock *uiSock) {
	h.mu.Lock()
	h.ui[clientID] = append(h.ui[clientID], sock)
	h.mu.Unlock()
}

func (h *uiHub) removeUI(clientID string, sock *uiSock) {
	h.mu.Lock()
	list := h.ui[clientID]
	kept := list[:0]
	for _, item := range list {
		if item != sock {
			kept = append(kept, item)
		}
	}
	if len(kept) == 0 {
		delete(h.ui, clientID)
	} else {
		h.ui[clientID] = kept
	}
	h.mu.Unlock()
}

func (h *uiHub) addSrc(sock *uiSock) {
	h.mu.Lock()
	h.src = append(h.src, sock)
	h.mu.Unlock()
}

func (h *uiHub) removeSrc(sock *uiSock) {
	h.mu.Lock()
	kept := h.src[:0]
	for _, item := range h.src {
		if item != sock {
			kept = append(kept, item)
		}
	}
	h.src = kept
	h.mu.Unlock()
}

func (h *uiHub) pushUI(clientID, msg string) bool {
	h.mu.Lock()
	list := append([]*uiSock(nil), h.ui[clientID]...)
	h.mu.Unlock()
	if len(list) == 0 {
		return false
	}
	ok := false
	for _, sock := range list {
		if sock.sendText(msg) {
			ok = true
		}
	}
	return ok
}

func (h *uiHub) pushSrc(msg string) bool {
	h.mu.Lock()
	list := append([]*uiSock(nil), h.src...)
	h.mu.Unlock()
	if len(list) == 0 {
		return false
	}
	ok := false
	for _, sock := range list {
		if sock.sendText(msg) {
			ok = true
		}
	}
	return ok
}

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	role := strings.TrimSpace(r.URL.Query().Get("role"))
	scope := hostclient.ScopeID()
	if scope == "" {
		scope = hostclient.ResolveScopeKey(
			r.URL.Query().Get("userId"),
			r.URL.Query().Get("scopeId"),
			first(r.URL.Query().Get("clientId"), r.Header.Get("X-Kotv-Client-Id")),
		)
	}
	conn, err := uiUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	if s.ui == nil {
		s.ui = newUIHub()
	}
	sock := newUISock(conn)
	go sock.writeLoop()
	if role == "src" {
		s.ui.addSrc(sock)
		if s.uiReply != nil {
			for _, raw := range s.uiReply.drainAll() {
				sock.sendWait(raw)
			}
		}
		sock.readLoop(func(text string) { s.acceptSrcFrame(scope, text) })
		s.ui.removeSrc(sock)
		_ = conn.Close()
		return
	}
	s.ui.addUI(scope, sock)
	for _, msg := range s.events.DrainPostMsg(scope) {
		sock.sendWait(msg)
	}
	sock.readLoop(func(text string) { s.acceptUIReply(text) })
	s.ui.removeUI(scope, sock)
	_ = conn.Close()
}

func (s *Server) acceptSrcFrame(scope, text string) {
	text = strings.TrimSpace(text)
	if text == "" || s.events == nil {
		return
	}
	msg := text
	clientID := scope
	if strings.HasPrefix(text, "{") {
		var env struct {
			Msg      string `json:"msg"`
			UserID   string `json:"userId"`
			ScopeID  string `json:"scopeId"`
			ClientID string `json:"clientId"`
		}
		if json.Unmarshal([]byte(text), &env) == nil && strings.TrimSpace(env.Msg) != "" {
			msg = env.Msg
			if id := hostclient.ResolveScopeKey(env.UserID, env.ScopeID, env.ClientID); id != "" {
				clientID = id
			}
		}
	}
	s.events.EmitPostMsgTo(msg, clientID)
}

func (s *Server) acceptUIReply(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	var reply struct {
		ID string `json:"id"`
	}
	if json.Unmarshal([]byte(text), &reply) != nil {
		return
	}
	id := strings.TrimSpace(reply.ID)
	if id == "" {
		return
	}
	if s.ui != nil && s.ui.pushSrc(text) {
		return
	}
	if s.uiReply == nil {
		s.uiReply = newUIReplyStore()
	}
	s.uiReply.put(id, text)
}
