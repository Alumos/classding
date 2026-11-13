package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"runtime"
	"strings"
)

// Argon2id takes tens of milliseconds and 32 MiB per call. preparePasswords
// does that work before App.mu is taken, so one sign-in never stalls every
// other request and classroom socket. Handlers then read the results with
// freshHash and checkPassword, which fall back to computing inline.

type passwordWork struct {
	hashes  map[string]string // Plain text → new encoded hash.
	matches map[string]bool   // Plain text + "\x00" + encoded hash → verified.
}

type passwordKey struct{}

// argonSlots bounds concurrent hashing so a burst of sign-ins cannot exhaust memory.
var argonSlots = make(chan struct{}, max(2, min(runtime.NumCPU(), 8)))

// dummyPasswordHash is verified against when an account does not exist, so a
// response takes the same time whether or not the username is registered.
var dummyPasswordHash = passwordHash("classding-unknown-account-0")

func withArgon[T any](fn func() T) T {
	argonSlots <- struct{}{}
	defer func() { <-argonSlots }()
	return fn()
}

func (a *App) preparePasswords(r *http.Request) *http.Request {
	if r.Body == nil || r.Body == http.NoBody || r.Method == http.MethodGet {
		return r
	}
	raw, err := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil || !bytes.Contains(raw, []byte("password")) {
		return r
	}
	var in struct {
		Username string  `json:"username"`
		Password *string `json:"password"`
		Current  *string `json:"current_password"`
		New      *string `json:"new_password"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return r
	}
	login := strings.HasSuffix(r.URL.Path, "/auth/login")
	var fresh []string
	if !login && in.Password != nil && validPassword(*in.Password) {
		fresh = append(fresh, *in.Password)
	}
	if in.New != nil && validPassword(*in.New) {
		fresh = append(fresh, *in.New)
	}
	type check struct{ plain, encoded string }
	var checks []check
	a.mu.Lock()
	switch {
	case login && in.Password != nil:
		username := normalizeUsername(in.Username)
		if a.loginWait(a.clientIP(r), username) == 0 {
			encoded := dummyPasswordHash
			if u, found := a.userByUsername(username); found {
				encoded = u.PasswordHash
			}
			checks = append(checks, check{*in.Password, encoded})
		}
	case in.Current != nil:
		if u, err := a.authenticate(r); err == nil {
			checks = append(checks, check{*in.Current, u.PasswordHash})
		}
	}
	a.mu.Unlock()
	if len(fresh) == 0 && len(checks) == 0 {
		return r
	}
	work := &passwordWork{hashes: map[string]string{}, matches: map[string]bool{}}
	for _, plain := range fresh {
		work.hashes[plain] = withArgon(func() string { return passwordHash(plain) })
	}
	for _, c := range checks {
		work.matches[c.plain+"\x00"+c.encoded] = withArgon(func() bool { return passwordMatches(c.plain, c.encoded) })
	}
	return r.WithContext(context.WithValue(r.Context(), passwordKey{}, work))
}

func requestPasswords(r *http.Request) *passwordWork {
	work, _ := r.Context().Value(passwordKey{}).(*passwordWork)
	return work
}

// freshHash returns the hash prepared for plain, or computes one.
func freshHash(r *http.Request, plain string) string {
	if work := requestPasswords(r); work != nil {
		if encoded, exists := work.hashes[plain]; exists {
			return encoded
		}
	}
	return passwordHash(plain)
}

// checkPassword returns the prepared verification of plain against encoded, or computes it.
func checkPassword(r *http.Request, plain, encoded string) bool {
	if work := requestPasswords(r); work != nil {
		if matched, exists := work.matches[plain+"\x00"+encoded]; exists {
			return matched
		}
	}
	return passwordMatches(plain, encoded)
}
