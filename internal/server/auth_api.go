package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/bobo/KOTV/internal/auth"
)

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"remoteAuth":    auth.RemoteAuthEnabled(),
		"allowRegister": auth.AllowRegister(),
		"authRequired":  loginRequired(r),
		"remoteAccess":  remoteAccessAllowed(r),
		"adminRequired": !isLoopbackRequest(r) && auth.RemoteAuthEnabled(),
		"loopback":      isLoopbackRequest(r),
	})
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	body, reply, ok := readLoginForm(w, r)
	if !ok {
		return
	}
	tok, user, err := auth.Login(body.Username, body.Password)
	if !isLoopbackRequest(r) {
		result := "成功"
		if err != nil {
			result = "失败"
		}
		log.Printf("远端登录%s: 用户 %s，来自 %s（%s）", result, strings.TrimSpace(body.Username), r.RemoteAddr, clientLabel(r))
	}
	if err != nil {
		writeAPIError(w, http.StatusUnauthorized, err.Error())
		return
	}
	writeSealed(w, reply, map[string]any{
		"ok":    true,
		"token": tok,
		"user":  user.Public(),
	})
}

func (s *Server) handleAuthRegister(w http.ResponseWriter, r *http.Request) {
	body, reply, ok := readLoginForm(w, r)
	if !ok {
		return
	}
	user, err := auth.Register(body.Username, body.Password)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeSealed(w, reply, map[string]any{"ok": true, "user": user.Public()})
}

type loginForm struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func readLoginForm(w http.ResponseWriter, r *http.Request) (loginForm, *auth.SealReply, bool) {
	var body loginForm
	if rejectUnlessPost(w, r) {
		return body, nil, false
	}
	if !remoteAccessAllowed(r) {
		writeAPIError(w, http.StatusForbidden, "未开启远端鉴权")
		return body, nil, false
	}
	reply, ok := readCredentials(w, r, &body)
	return body, reply, ok
}

// readCredentials 读带密码的请求体。只收客户端用引擎身份公钥加密的 {"sealed":…}，
// 返回的 SealReply 供 writeSealed 加密回复。
func readCredentials(w http.ResponseWriter, r *http.Request, dst any) (*auth.SealReply, bool) {
	var env struct {
		Sealed *auth.Sealed `json:"sealed"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&env); err != nil || env.Sealed == nil {
		writeAPIError(w, http.StatusBadRequest, "账号密码需加密传输，请升级客户端")
		return nil, false
	}
	plain, reply, err := auth.OpenSealed(*env.Sealed, r.URL.Path)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	if err := json.Unmarshal(plain, dst); err != nil {
		writeAPIError(w, http.StatusBadRequest, "请求格式错误")
		return nil, false
	}
	return reply, true
}

// writeSealed 加密回复 readCredentials 读到的请求。
func writeSealed(w http.ResponseWriter, reply *auth.SealReply, v map[string]any) {
	plain, _ := json.Marshal(v)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sealed": reply.Seal(plain)})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if rejectUnlessPost(w, r) {
		return
	}
	auth.Logout(auth.BearerFromHeader(r.Header.Get("Authorization")))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	u := authUserFrom(r.Context())
	if u == nil {
		// 本机未登录：机主视图，供用户管理免登录。
		if isLoopbackRequest(r) {
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":    true,
				"local": true,
				"user": map[string]any{
					"id":                 "",
					"username":           "本机",
					"role":               auth.RoleAdmin,
					"mustChangePassword": false,
				},
			})
			return
		}
		writeAPIError(w, http.StatusUnauthorized, "需要登录")
		return
	}
	out := map[string]any{"ok": true, "user": u.Public()}
	if !isLoopbackRequest(r) {
		// 远端客户端自己拼引擎媒体地址（海报相对路径等）时带上它。
		out["mediaKey"] = auth.MediaKey()
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAuthPassword(w http.ResponseWriter, r *http.Request) {
	if rejectUnlessPost(w, r) {
		return
	}
	u := authUserFrom(r.Context())
	if u == nil {
		writeAPIError(w, http.StatusUnauthorized, "需要登录")
		return
	}
	var body struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	reply, ok := readCredentials(w, r, &body)
	if !ok {
		return
	}
	keep := auth.BearerFromHeader(r.Header.Get("Authorization"))
	if err := auth.ChangePassword(u.ID, keep, body.OldPassword, body.NewPassword); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeSealed(w, reply, map[string]any{"ok": true})
}

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list := auth.ListUsers()
		out := make([]map[string]any, 0, len(list))
		for _, u := range list {
			out = append(out, u.Public())
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":            true,
			"users":         out,
			"remoteAuth":    auth.RemoteAuthEnabled(),
			"allowRegister": auth.AllowRegister(),
		})
	case http.MethodPost:
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		reply, ok := readCredentials(w, r, &body)
		if !ok {
			return
		}
		u, err := auth.CreateUser(body.Username, body.Password, body.Role)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeSealed(w, reply, map[string]any{"ok": true, "user": u.Public()})
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleAdminUserAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/users/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeAPIError(w, http.StatusBadRequest, "missing user id")
		return
	}
	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	switch {
	case r.Method == http.MethodDelete && action == "":
		if err := auth.DeleteUser(id); err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		if s.contentAPI != nil {
			s.contentAPI.PurgeUserData(id)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case r.Method == http.MethodPost && action == "enable":
		if err := auth.SetEnabled(id, true); err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case r.Method == http.MethodPost && action == "disable":
		if err := auth.SetEnabled(id, false); err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case r.Method == http.MethodPost && action == "password":
		var body struct {
			Password string `json:"password"`
		}
		reply, ok := readCredentials(w, r, &body)
		if !ok {
			return
		}
		if err := auth.ResetPassword(id, body.Password); err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeSealed(w, reply, map[string]any{"ok": true})
	default:
		writeAPIError(w, http.StatusBadRequest, "unknown action")
	}
}

func (s *Server) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	if rejectUnlessPost(w, r) {
		return
	}
	var body struct {
		RemoteAuth    *bool `json:"remoteAuth"`
		AllowRegister *bool `json:"allowRegister"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body)
	if body.RemoteAuth != nil {
		_ = auth.SetRemoteAuth(*body.RemoteAuth)
	}
	if body.AllowRegister != nil {
		_ = auth.SetAllowRegister(*body.AllowRegister)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"remoteAuth":    auth.RemoteAuthEnabled(),
		"allowRegister": auth.AllowRegister(),
	})
}
