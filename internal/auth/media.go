package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"time"
)

// 媒体钥匙：远端播放器、图片加载、电视投屏等带不了登录头的请求，用它访问引擎的媒体地址。
// 它放在地址里（/k/<钥匙>/proxy/…），只能读媒体，不能调用接口。
// 无状态：按周签发，当周和上一周的都有效；用引擎身份密钥派生的密钥签名，重启后照样有效。
const (
	mediaKeyPeriod = 7 * 24 * time.Hour
	mediaKeyValid  = 2 // 有效的签发周期数
	mediaMACBytes  = 16
)

// MediaKey 当前签发周期的媒体钥匙。
func MediaKey() string {
	return mediaKeyFor(mediaPeriod(time.Now()))
}

// ValidMediaKey 钥匙格式正确、签名正确且未过期。
func ValidMediaKey(key string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(key)
	if err != nil || len(raw) != 4+mediaMACBytes {
		return false
	}
	period := binary.BigEndian.Uint32(raw[:4])
	now := mediaPeriod(time.Now())
	if period > now || now-period >= mediaKeyValid {
		return false
	}
	want, err := base64.RawURLEncoding.DecodeString(mediaKeyFor(period))
	return err == nil && hmac.Equal(raw, want)
}

func mediaPeriod(t time.Time) uint32 {
	return uint32(t.Unix() / int64(mediaKeyPeriod/time.Second))
}

func mediaKeyFor(period uint32) string {
	priv, err := identity()
	if err != nil {
		return ""
	}
	secret := sha256.Sum256(append([]byte("kotv-media-key"), priv.Bytes()...))
	var head [4]byte
	binary.BigEndian.PutUint32(head[:], period)
	mac := hmac.New(sha256.New, secret[:])
	mac.Write(head[:])
	out := append(head[:], mac.Sum(nil)[:mediaMACBytes]...)
	return base64.RawURLEncoding.EncodeToString(out)
}
