package main

import "testing"

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
