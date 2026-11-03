package app

import (
	"net"
	"net/http"
	"strings"
)

// clientIP only uses forwarding headers when the connection comes from a
// configured proxy. The first untrusted hop from the right is the client;
// anything before that hop may have been supplied by the client itself.
func (a *App) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer := net.ParseIP(host)
	zone := ""
	if peer == nil {
		// Link-local IPv6 peers can include an interface zone. Keep it in the
		// rate-limit key, but match the address against configured networks.
		if i := strings.LastIndexByte(host, '%'); i > 0 && i < len(host)-1 {
			if scoped := net.ParseIP(host[:i]); scoped != nil && scoped.To4() == nil {
				peer = scoped
				zone = host[i:]
			}
		}
	}
	if peer == nil {
		return r.RemoteAddr
	}
	direct := peer.String() + zone
	if !a.isTrustedProxy(peer) {
		return direct
	}

	forwarded := r.Header.Values("X-Forwarded-For")
	for i := len(forwarded) - 1; i >= 0; i-- {
		hops := strings.Split(forwarded[i], ",")
		for j := len(hops) - 1; j >= 0; j-- {
			address := net.ParseIP(strings.TrimSpace(hops[j]))
			if address == nil {
				return direct
			}
			if !a.isTrustedProxy(address) {
				return address.String()
			}
		}
	}
	return direct
}

func (a *App) isTrustedProxy(address net.IP) bool {
	for _, value := range a.config.TrustedProxies {
		value = strings.TrimSpace(value)
		if trustedIP := net.ParseIP(value); trustedIP != nil {
			if trustedIP.Equal(address) {
				return true
			}
			continue
		}
		if _, network, err := net.ParseCIDR(value); err == nil && network.Contains(address) {
			return true
		}
	}
	return false
}
