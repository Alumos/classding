package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name      string
		trusted   []string
		remote    string
		forwarded []string
		realIP    string
		want      string
	}{
		{name: "default ignores forwarding headers", remote: "203.0.113.8:1234", forwarded: []string{"198.51.100.9"}, realIP: "198.51.100.10", want: "203.0.113.8"},
		{name: "untrusted peer cannot spoof", trusted: []string{"10.0.0.0/8"}, remote: "203.0.113.8:1234", forwarded: []string{"198.51.100.9"}, want: "203.0.113.8"},
		{name: "single trusted IP", trusted: []string{" 10.0.0.1 "}, remote: "10.0.0.1:1234", forwarded: []string{"198.51.100.9"}, want: "198.51.100.9"},
		{name: "trusted subnet", trusted: []string{"10.0.0.0/8"}, remote: "10.0.0.1:1234", forwarded: []string{"198.51.100.9"}, want: "198.51.100.9"},
		{name: "several trusted hops", trusted: []string{"10.0.0.0/8", "172.16.0.3"}, remote: "10.0.0.1:1234", forwarded: []string{"198.51.100.9, 172.16.0.3, 10.0.0.2"}, want: "198.51.100.9"},
		{name: "separate header fields", trusted: []string{"10.0.0.0/8"}, remote: "10.0.0.1:1234", forwarded: []string{"198.51.100.9", " 10.0.0.2, 10.0.0.3 "}, want: "198.51.100.9"},
		{name: "stop at first untrusted hop", trusted: []string{"10.0.0.0/8"}, remote: "10.0.0.1:1234", forwarded: []string{"192.0.2.99, 198.51.100.9, 10.0.0.2"}, want: "198.51.100.9"},
		{name: "ignore malformed values beyond untrusted hop", trusted: []string{"10.0.0.0/8"}, remote: "10.0.0.1:1234", forwarded: []string{"invalid, 198.51.100.9, 10.0.0.2"}, want: "198.51.100.9"},
		{name: "invalid rightmost hop", trusted: []string{"10.0.0.0/8"}, remote: "10.0.0.1:1234", forwarded: []string{"198.51.100.9, invalid"}, want: "10.0.0.1"},
		{name: "invalid hop after trusted hops", trusted: []string{"10.0.0.0/8"}, remote: "10.0.0.1:1234", forwarded: []string{"198.51.100.9, invalid, 10.0.0.2"}, want: "10.0.0.1"},
		{name: "empty rightmost hop", trusted: []string{"10.0.0.0/8"}, remote: "10.0.0.1:1234", forwarded: []string{"198.51.100.9, "}, want: "10.0.0.1"},
		{name: "empty header", trusted: []string{"10.0.0.0/8"}, remote: "10.0.0.1:1234", forwarded: []string{""}, want: "10.0.0.1"},
		{name: "all hops trusted", trusted: []string{"10.0.0.0/8"}, remote: "10.0.0.1:1234", forwarded: []string{"10.0.0.4, 10.0.0.3"}, want: "10.0.0.1"},
		{name: "no forwarded header", trusted: []string{"10.0.0.0/8"}, remote: "10.0.0.1:1234", realIP: "198.51.100.9", want: "10.0.0.1"},
		{name: "invalid trusted configuration", trusted: []string{"", "0.0.0.0/invalid", "10.0.0.0/33"}, remote: "10.0.0.1:1234", forwarded: []string{"198.51.100.9"}, want: "10.0.0.1"},
		{name: "ipv6 peers and chain", trusted: []string{"2001:db8:1::/48"}, remote: "[2001:db8:1::2]:443", forwarded: []string{"2001:db8:ffff::2, 2001:db8:1::3"}, want: "2001:db8:ffff::2"},
		{name: "scoped ipv6 peer ignores untrusted header", remote: "[fe80::1234%en0]:443", forwarded: []string{"198.51.100.9"}, want: "fe80::1234%en0"},
		{name: "scoped ipv6 trusted by network", trusted: []string{"fe80::/10"}, remote: "[fe80::1234%en0]:443", forwarded: []string{"198.51.100.9"}, want: "198.51.100.9"},
		{name: "mapped peer matches IPv4 subnet", trusted: []string{"192.0.2.0/24"}, remote: "[::ffff:192.0.2.10]:443", forwarded: []string{"::ffff:198.51.100.7"}, want: "198.51.100.7"},
		{name: "mapped trusted IP matches IPv4 peer", trusted: []string{"::ffff:192.0.2.10"}, remote: "192.0.2.10:443", forwarded: []string{"198.51.100.7"}, want: "198.51.100.7"},
		{name: "mapped subnet matches IPv4 peer", trusted: []string{"::ffff:192.0.2.0/120"}, remote: "192.0.2.10:443", forwarded: []string{"198.51.100.7"}, want: "198.51.100.7"},
		{name: "remote IP without port", trusted: []string{"2001:db8::1"}, remote: "2001:db8::1", forwarded: []string{"198.51.100.9"}, want: "198.51.100.9"},
		{name: "malformed peer stays unchanged", trusted: []string{"0.0.0.0/0", "::/0"}, remote: "not-an-ip:1234", forwarded: []string{"198.51.100.9"}, want: "not-an-ip:1234"},
		{name: "port in forwarded IP rejected", trusted: []string{"10.0.0.0/8"}, remote: "10.0.0.1:1234", forwarded: []string{"198.51.100.9:1234"}, want: "10.0.0.1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := &App{config: Config{TrustedProxies: tc.trusted}}
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remote
			for _, value := range tc.forwarded {
				r.Header.Add("X-Forwarded-For", value)
			}
			if tc.realIP != "" {
				r.Header.Set("X-Real-IP", tc.realIP)
			}
			if got := a.clientIP(r); got != tc.want {
				t.Errorf("clientIP() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProxyClientIPRateLimits(t *testing.T) {
	endpoints := []struct {
		name     string
		path     string
		max      int
		response int
	}{
		{name: "registration", path: "/api/v1/auth/register", max: 10, response: http.StatusBadRequest},
		{name: "login", path: "/api/v1/auth/login", max: 8, response: http.StatusUnauthorized},
		{name: "device binding", path: "/api/v1/device/bind", max: 5, response: http.StatusBadRequest},
	}
	for _, endpoint := range endpoints {
		for _, trusted := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/trusted=%t", endpoint.name, trusted), func(t *testing.T) {
				config := Config{DataDir: t.TempDir(), JWTSecret: strings.Repeat("s", 32)}
				if trusted {
					config.TrustedProxies = []string{"10.0.0.0/8"}
				}
				a, err := New(config)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := a.Close(); err != nil {
						t.Error(err)
					}
				})
				request := func(forwarded string) int {
					r := httptest.NewRequest(http.MethodPost, endpoint.path, strings.NewReader(`{}`))
					r.RemoteAddr = "10.0.0.1:1234"
					r.Header.Set("X-Forwarded-For", forwarded)
					w := httptest.NewRecorder()
					commitState(a)
					a.Handler().ServeHTTP(w, r)
					return w.Code
				}
				for i := 0; i < endpoint.max; i++ {
					forwarded := "198.51.100.1"
					if !trusted {
						forwarded = fmt.Sprintf("198.51.100.%d", i+1)
					}
					if got := request(forwarded); got != endpoint.response {
						t.Fatalf("request %d returned %d, want %d", i+1, got, endpoint.response)
					}
				}
				if got := request("198.51.100.99"); trusted {
					if got != endpoint.response {
						t.Fatalf("different forwarded client returned %d, want %d", got, endpoint.response)
					}
					if got = request("198.51.100.1"); got != http.StatusTooManyRequests {
						t.Fatalf("limited forwarded client returned %d, want %d", got, http.StatusTooManyRequests)
					}
				} else if got != http.StatusTooManyRequests {
					t.Fatalf("spoofed forwarded client returned %d, want %d", got, http.StatusTooManyRequests)
				}
			})
		}
	}
}
