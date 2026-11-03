package app

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/crypto/argon2"
	"strings"
	"time"
	"unicode"
)

func random(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func id(prefix string) string { return prefix + "_" + random(12) }
func hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func passwordHash(password string) string {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		panic(err)
	}
	key := argon2.IDKey([]byte(password), salt, 2, 32*1024, 2, 32)
	return fmt.Sprintf("$argon2id$v=19$m=32768,t=2,p=2$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
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
	if err != nil || len(expected) != 32 {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, 2, 32*1024, 2, 32)
	return subtle.ConstantTimeCompare(expected, actual) == 1
}
func validPassword(s string) bool {
	var letter, digit bool
	for _, r := range s {
		letter = letter || unicode.IsLetter(r)
		digit = digit || unicode.IsDigit(r)
	}
	return len(s) >= 10 && len(s) <= 128 && letter && digit
}

func validUsername(username string) bool {
	if len(username) < 3 || len(username) > 32 {
		return false
	}
	for _, ch := range username {
		if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-' || ch == '.') {
			return false
		}
	}
	return true
}

type claims struct {
	Subject string `json:"sub"`
	Expires int64  `json:"exp"`
	Version int    `json:"ver"`
	Epoch   string `json:"epoch,omitempty"`
}

func (a *App) access(u User) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	body, _ := json.Marshal(claims{Subject: u.ID, Expires: time.Now().Add(15 * time.Minute).Unix(), Version: u.TokenVersion, Epoch: a.state.Settings["_auth_epoch"]})
	payload := header + "." + base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, []byte(a.config.JWTSecret))
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (a *App) verify(token string) (claims, error) {
	var c claims
	p := strings.Split(token, ".")
	if len(p) != 3 {
		return c, errors.New("invalid token")
	}
	signature, err := base64.RawURLEncoding.DecodeString(p[2])
	if err != nil {
		return c, err
	}
	mac := hmac.New(sha256.New, []byte(a.config.JWTSecret))
	mac.Write([]byte(p[0] + "." + p[1]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return c, errors.New("invalid token")
	}
	body, err := base64.RawURLEncoding.DecodeString(p[1])
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(body, &c); err != nil || c.Expires <= time.Now().Unix() || c.Epoch != a.state.Settings["_auth_epoch"] {
		return c, errors.New("expired token")
	}
	return c, nil
}
func activation() string {
	const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	b := make([]byte, 8)
	for i := range b {
		for {
			var sample [1]byte
			rand.Read(sample[:])
			if int(sample[0]) < 256-256%len(alphabet) {
				b[i] = alphabet[int(sample[0])%len(alphabet)]
				break
			}
		}
	}
	return string(b[:4]) + "-" + string(b[4:])
}
func sixDigits() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("%06d", binary.BigEndian.Uint32(b)%1000000)
}
