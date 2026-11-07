package util

import (
	"net/url"
	"path"
	"strings"
)

// MaskURLCredentials 日志/界面展示时把 user:pass@ 里的密码打码。
func MaskURLCredentials(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return u
	}
	schemeIdx := strings.Index(u, "://")
	if schemeIdx < 0 {
		return u
	}
	rest := u[schemeIdx+3:]
	at := strings.Index(rest, "@")
	if at <= 0 {
		return u
	}
	cred := rest[:at]
	colon := strings.Index(cred, ":")
	if colon < 0 {
		return u
	}
	return u[:schemeIdx+3] + cred[:colon+1] + "***" + rest[at:]
}

// NodePackageBaseURL 去掉 index.js / index.js.md5，得到源包目录 URL（含尾斜杠）。
func NodePackageBaseURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return u
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	p := parsed.Path
	lower := strings.ToLower(p)
	switch {
	case strings.HasSuffix(lower, "/index.js.md5"):
		p = p[:len(p)-len("/index.js.md5")] + "/"
	case strings.HasSuffix(lower, "index.js.md5"):
		p = p[:len(p)-len("index.js.md5")]
	case strings.HasSuffix(lower, "/index.js"):
		p = p[:len(p)-len("/index.js")] + "/"
	case strings.HasSuffix(lower, "index.js"):
		p = p[:len(p)-len("index.js")]
	default:
		p = path.Dir(p)
		if !strings.HasSuffix(p, "/") {
			p += "/"
		}
	}
	parsed.Path = p
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}
