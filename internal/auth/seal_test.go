package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

// sealAsClient 按客户端的做法加密 plain，返回请求体和解回复用的 AEAD。
func sealAsClient(t *testing.T, serverPub []byte, plain []byte, aad string) (Sealed, cipher.AEAD) {
	t.Helper()
	eph, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	spk, err := ecdh.X25519().NewPublicKey(serverPub)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := eph.ECDH(spk)
	if err != nil {
		t.Fatal(err)
	}
	salt := append(append([]byte{}, eph.PublicKey().Bytes()...), serverPub...)
	keys, err := hkdf.Key(sha256.New, shared, salt, sealInfo, 64)
	if err != nil {
		t.Fatal(err)
	}
	gcm := func(k []byte) cipher.AEAD {
		b, _ := aes.NewCipher(k)
		a, _ := cipher.NewGCM(b)
		return a
	}
	req := gcm(keys[:32])
	iv := make([]byte, 12)
	_, _ = rand.Read(iv)
	enc := base64.StdEncoding.EncodeToString
	return Sealed{
		Key:  enc(eph.PublicKey().Bytes()),
		IV:   enc(iv),
		Data: enc(req.Seal(nil, iv, plain, []byte(aad))),
	}, gcm(keys[32:])
}

func TestSealRoundTrip(t *testing.T) {
	t.Setenv("KOTV_DATA_DIR", t.TempDir())
	pub, err := base64.StdEncoding.DecodeString(IdentityKey())
	if err != nil || len(pub) != 32 {
		t.Fatalf("identity key: %v len=%d", err, len(pub))
	}
	const path = "/api/v1/auth/login"
	body, resp := sealAsClient(t, pub, []byte(`{"username":"a","password":"secret"}`), path)

	plain, reply, err := OpenSealed(body, path)
	if err != nil || string(plain) != `{"username":"a","password":"secret"}` {
		t.Fatalf("open: %v %q", err, plain)
	}
	out := reply.Seal([]byte(`{"token":"t"}`))
	iv, _ := base64.StdEncoding.DecodeString(out.IV)
	data, _ := base64.StdEncoding.DecodeString(out.Data)
	got, err := resp.Open(nil, iv, data, []byte(path))
	if err != nil || string(got) != `{"token":"t"}` {
		t.Fatalf("reply: %v %q", err, got)
	}

	if _, _, err := OpenSealed(body, "/api/v1/auth/register"); err == nil {
		t.Fatal("other path must not decrypt")
	}
	tampered := body
	raw, _ := base64.StdEncoding.DecodeString(body.Data)
	raw[0] ^= 1
	tampered.Data = base64.StdEncoding.EncodeToString(raw)
	if _, _, err := OpenSealed(tampered, path); err == nil {
		t.Fatal("tampered data must fail")
	}
}
