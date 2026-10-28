package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/bobo/KOTV/internal/paths"
)

// 账号密码的加密传输：客户端用临时 X25519 密钥与引擎身份公钥协商，
// HKDF-SHA256 派生请求、响应各一把 AES-256-GCM 密钥，附加数据为接口路径。
// 抓包或转发请求的一方没有临时私钥，解不开密码，也解不开返回的登录令牌。
const sealInfo = "kotv-seal-v1"

// Sealed 加密后的请求/响应体。Key 为客户端临时公钥，只出现在请求里。
type Sealed struct {
	Key  string `json:"key,omitempty"`
	IV   string `json:"iv"`
	Data string `json:"data"`
}

// SealReply 用同一次协商的响应密钥加密回复。
type SealReply struct {
	aead cipher.AEAD
	aad  []byte
}

var (
	identityOnce sync.Once
	identityPriv *ecdh.PrivateKey
	identityErr  error
)

// identity 引擎身份密钥，首次使用时生成并保存在数据目录，重启后不变。
func identity() (*ecdh.PrivateKey, error) {
	identityOnce.Do(func() {
		identityPriv, identityErr = loadIdentity(filepath.Join(paths.Data(), "identity.key"))
	})
	return identityPriv, identityErr
}

func loadIdentity(path string) (*ecdh.PrivateKey, error) {
	if raw, err := os.ReadFile(path); err == nil {
		if k, err := ecdh.X25519().NewPrivateKey(raw); err == nil {
			return k, nil
		}
	}
	k, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, k.Bytes(), 0o600); err != nil {
		return nil, err
	}
	return k, nil
}

// IdentityKey 引擎身份公钥（base64）。远端客户端据此加密，并记住它来认出同一台后端。
func IdentityKey() string {
	k, err := identity()
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(k.PublicKey().Bytes())
}

// OpenSealed 解密请求体；返回明文和加密回复用的 SealReply。aad 为接口路径。
func OpenSealed(s Sealed, aad string) ([]byte, *SealReply, error) {
	priv, err := identity()
	if err != nil {
		return nil, nil, err
	}
	epkRaw, err1 := base64.StdEncoding.DecodeString(s.Key)
	iv, err2 := base64.StdEncoding.DecodeString(s.IV)
	data, err3 := base64.StdEncoding.DecodeString(s.Data)
	if err1 != nil || err2 != nil || err3 != nil {
		return nil, nil, fmt.Errorf("加密数据格式错误")
	}
	epk, err := ecdh.X25519().NewPublicKey(epkRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("加密数据格式错误")
	}
	shared, err := priv.ECDH(epk)
	if err != nil {
		return nil, nil, fmt.Errorf("加密数据格式错误")
	}
	salt := append(append([]byte{}, epkRaw...), priv.PublicKey().Bytes()...)
	keys, err := hkdf.Key(sha256.New, shared, salt, sealInfo, 64)
	if err != nil {
		return nil, nil, err
	}
	reqAEAD, err := newGCM(keys[:32])
	if err != nil {
		return nil, nil, err
	}
	respAEAD, err := newGCM(keys[32:])
	if err != nil {
		return nil, nil, err
	}
	if len(iv) != reqAEAD.NonceSize() {
		return nil, nil, fmt.Errorf("加密数据格式错误")
	}
	plain, err := reqAEAD.Open(nil, iv, data, []byte(aad))
	if err != nil {
		return nil, nil, fmt.Errorf("加密数据校验失败")
	}
	return plain, &SealReply{aead: respAEAD, aad: []byte(aad)}, nil
}

// Seal 加密回复明文。
func (r *SealReply) Seal(plain []byte) Sealed {
	iv := make([]byte, r.aead.NonceSize())
	_, _ = rand.Read(iv)
	return Sealed{
		IV:   base64.StdEncoding.EncodeToString(iv),
		Data: base64.StdEncoding.EncodeToString(r.aead.Seal(nil, iv, plain, r.aad)),
	}
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
