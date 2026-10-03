// Package totp implements RFC 6238 TOTP (HMAC-SHA1, 6 digits, 30s window)
// with base32 secrets, plus AES-GCM sealing for stored secrets.
package totp

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// Period is the TOTP time step in seconds (RFC 6238 default).
const Period = 30

// GenerateSecret returns a random base32 secret (without padding).
func GenerateSecret() string {
	buf := make([]byte, 20)
	_, _ = rand.Read(buf)
	return strings.TrimRight(base32.StdEncoding.EncodeToString(buf), "=")
}

// codeAt computes the 6-digit TOTP code for the given time.
func codeAt(secret string, t time.Time) (string, error) {
	key, err := DecodeSecret(secret)
	if err != nil {
		return "", err
	}
	counter := uint64(t.Unix() / Period)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(buf)
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := (uint32(sum[off])&0x7f)<<24 |
		uint32(sum[off+1])<<16 |
		uint32(sum[off+2])<<8 |
		uint32(sum[off+3])
	return fmt.Sprintf("%06d", code%1_000_000), nil
}

// Verify checks a user-supplied code against the secret, allowing ±skew time
// steps of clock drift. Codes are compared as strings to avoid the classic
// leading-zero bug.
func Verify(secret, code string, skew int) bool {
	if code == "" {
		return false
	}
	if skew < 0 {
		skew = 0
	}
	now := time.Now()
	for i := -skew; i <= skew; i++ {
		want, err := codeAt(secret, now.Add(time.Duration(i)*Period*time.Second))
		if err != nil {
			return false
		}
		if hmac.Equal([]byte(want), []byte(code)) {
			return true
		}
	}
	return false
}

// ProvisioningURL builds the otpauth:// style URL for QR rendering.
func ProvisioningURL(issuer, account, secret string) string {
	return fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&period=%d&digits=6&algorithm=SHA1",
		urlEscape(issuer), urlEscape(account), secret, urlEscape(issuer), Period)
}

func urlEscape(s string) string {
	r := strings.NewReplacer(
		":", "%3A", "/", "%2F", "?", "%3F", "&", "%26",
		"=", "%3D", " ", "%20", "@", "%40",
	)
	return r.Replace(s)
}

// DecodeSecret normalizes a base32 secret (accepting optional padding).
func DecodeSecret(secret string) ([]byte, error) {
	s := strings.ToUpper(strings.TrimSpace(secret))
	s = strings.TrimRight(s, "=")
	pad := (8 - len(s)%8) % 8
	return base32.StdEncoding.DecodeString(s + strings.Repeat("=", pad))
}

// ---------------------------------------------------------------------------
// Sealing: stored TOTP secrets are AES-256-GCM encrypted with a key derived
// from the platform JWT secret, so the DB never carries plaintext secrets.
// ---------------------------------------------------------------------------

// Cipher encrypts/decrypts TOTP secrets.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher derives an AES-256-GCM key from the platform secret.
func NewCipher(platformSecret string) (*Cipher, error) {
	if platformSecret == "" {
		return nil, fmt.Errorf("totp: platform secret required")
	}
	key := sha256.Sum256([]byte("aiiot-totp:" + platformSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Seal encrypts the secret. Output: nonce || ciphertext (base64-free hex).
func (c *Cipher) Seal(secret string) (string, error) {
	if c == nil || c.aead == nil {
		return "", fmt.Errorf("totp cipher not initialized")
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := c.aead.Seal(nonce, nonce, []byte(secret), nil)
	return fmt.Sprintf("%x", out), nil
}

// Open decrypts a sealed secret.
func (c *Cipher) Open(sealed string) (string, error) {
	if c == nil || c.aead == nil || sealed == "" {
		return "", fmt.Errorf("totp cipher not initialized or empty")
	}
	raw := make([]byte, len(sealed)/2)
	if _, err := fmt.Sscanf(sealed, "%x", &raw); err != nil {
		return "", fmt.Errorf("totp: decode sealed secret: %w", err)
	}
	if len(raw) < c.aead.NonceSize() {
		return "", fmt.Errorf("totp: sealed secret too short")
	}
	nonce, ct := raw[:c.aead.NonceSize()], raw[c.aead.NonceSize():]
	plain, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("totp: seal verify failed")
	}
	return string(plain), nil
}