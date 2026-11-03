package main

import (
	"net"
	"sort"
)

// GetLocalAddresses reports this computer's LAN IPv4 addresses so the
// administrator can find the classroom screen on the school network.
func (a *App) GetLocalAddresses() ([]string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var addresses []net.Addr
	for _, item := range interfaces {
		if item.Flags&net.FlagUp == 0 || item.Flags&net.FlagLoopback != 0 {
			continue
		}
		list, err := item.Addrs()
		if err != nil {
			continue
		}
		addresses = append(addresses, list...)
	}
	return lanIPv4(addresses), nil
}

func lanIPv4(addresses []net.Addr) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, address := range addresses {
		var ip net.IP
		switch value := address.(type) {
		case *net.IPNet:
			ip = value.IP
		case *net.IPAddr:
			ip = value.IP
		}
		ip = ip.To4()
		// Link-local (169.254.x.x) addresses appear on disconnected adapters.
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || seen[ip.String()] {
			continue
		}
		seen[ip.String()] = true
		result = append(result, ip.String())
	}
	// Private school-network addresses come first.
	sort.SliceStable(result, func(i, j int) bool {
		return net.ParseIP(result[i]).IsPrivate() && !net.ParseIP(result[j]).IsPrivate()
	})
	if len(result) > 8 {
		result = result[:8]
	}
	return result
}
