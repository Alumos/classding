package app

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

const accessTokenTTL = 15 * time.Minute

// Argon2id parameters. Verification uses them too, so changing them would
// invalidate every stored password.
const (
	argonTime    = 2
	argonMemory  = 32 * 1024
	argonThreads = 2
	argonKeyLen  = 32
)

func random(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func id(prefix string) string { return prefix + "_" + random(12) }

func hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func sign(secret string, parts ...string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	for _, part := range parts {
		mac.Write([]byte(part))
	}
	return mac.Sum(nil)
}

func passwordHash(password string) string {
	salt := make([]byte, 16)
	rand.Read(salt)
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

func passwordMatches(password, encoded string) bool {
	p := strings.Split(encoded, "$")
	if len(p) != 6 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(p[4])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(p[5])
	if err != nil || len(expected) != argonKeyLen {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return subtle.ConstantTimeCompare(expected, actual) == 1
}

type claims struct {
	Subject string `json:"sub"`
	Expires int64  `json:"exp"`
	Version int    `json:"ver"`
	Epoch   string `json:"epoch,omitempty"` // Changes on restore, retiring every earlier token.
}

var jwtHeader = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

// access issues a short-lived HS256 bearer token.
func (a *App) access(u User) string {
	body, _ := json.Marshal(claims{Subject: u.ID, Expires: time.Now().Add(accessTokenTTL).Unix(), Version: u.TokenVersion, Epoch: a.authEpoch()})
	payload := jwtHeader + "." + base64.RawURLEncoding.EncodeToString(body)
	return payload + "." + base64.RawURLEncoding.EncodeToString(sign(a.config.JWTSecret, payload))
}

func (a *App) verify(token string) (claims, error) {
	var c claims
	p := strings.Split(token, ".")
	if len(p) != 3 {
		return c, errors.New("invalid token")
	}
	signature, err := base64.RawURLEncoding.DecodeString(p[2])
	if err != nil || !hmac.Equal(signature, sign(a.config.JWTSecret, p[0]+"."+p[1])) {
		return c, errors.New("invalid token")
	}
	body, err := base64.RawURLEncoding.DecodeString(p[1])
	if err != nil || json.Unmarshal(body, &c) != nil {
		return c, errors.New("invalid token")
	}
	if c.Expires <= time.Now().Unix() || c.Epoch != a.authEpoch() {
		return c, errors.New("expired token")
	}
	return c, nil
}

func (a *App) authEpoch() string { return a.state.Settings[authEpochKey] }

// randomCode draws n characters uniformly from alphabet.
func randomCode(alphabet string, n int) string {
	b := make([]byte, n)
	limit := big.NewInt(int64(len(alphabet)))
	for i := range b {
		k, _ := rand.Int(rand.Reader, limit)
		b[i] = alphabet[k.Int64()]
	}
	return string(b)
}

// activation returns a license code such as "K7MP-2XQR"; the alphabet leaves
// out characters that are easily confused when read aloud.
func activation() string {
	code := randomCode("ABCDEFGHJKMNPQRSTUVWXYZ23456789", 8)
	return code[:4] + "-" + code[4:]
}

func sixDigits() string { return randomCode("0123456789", 6) }
