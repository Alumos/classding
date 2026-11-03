package main

import (
	"net"
	"reflect"
	"testing"
)

func TestLANIPv4(t *testing.T) {
	addresses := []net.Addr{
		&net.IPNet{IP: net.ParseIP("127.0.0.1")},
		&net.IPNet{IP: net.ParseIP("fe80::1")},
		&net.IPNet{IP: net.ParseIP("169.254.10.2")},
		&net.IPNet{IP: net.ParseIP("203.0.113.9")},
		&net.IPNet{IP: net.ParseIP("192.168.1.23")},
		&net.IPAddr{IP: net.ParseIP("192.168.1.23")},
		&net.IPNet{IP: net.ParseIP("10.0.0.5")},
	}
	got := lanIPv4(addresses)
	want := []string{"192.168.1.23", "10.0.0.5", "203.0.113.9"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("lanIPv4 = %v, want %v", got, want)
	}
}
