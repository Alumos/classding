package main

import (
	"net/http"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestServerURLValidation(t *testing.T) {
	for _, value := range []string{"http://example.com", "https://example.com:443/", "http://[::1]:8080"} {
		if _, err := parseServer(value); err != nil {
			t.Errorf("valid URL %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{"", "example.com", "ftp://example.com", "https://user:pass@example.com", "https://example.com/path", "https://example.com/?", "https://example.com/#", "https://example.com/%2f", "https://example.com\\evil"} {
		if _, err := parseServer(value); err == nil {
			t.Errorf("invalid URL %q accepted", value)
		}
	}
}
