package favicon

import (
	"net"
	"net/netip"
	"testing"
)

func TestIsPrivateAddr(t *testing.T) {
	cases := map[string]bool{
		// public
		"8.8.8.8":              false,
		"1.1.1.1":              false,
		"2606:4700:4700::1111": false,
		"64:ff9b::808:808":     false, // NAT64 of 8.8.8.8
		"100.63.255.255":       false,
		"100.128.0.0":          false,
		"198.17.255.255":       false,
		"198.20.0.0":           false,
		// private / non-public
		"10.0.0.1":          true,
		"172.16.0.1":        true,
		"192.168.1.1":       true,
		"127.0.0.1":         true,
		"169.254.1.1":       true,
		"0.0.0.0":           true,
		"0.1.2.3":           true,
		"100.64.0.1":        true, // CGNAT
		"100.100.100.100":   true, // Tailscale
		"100.127.255.255":   true,
		"198.18.0.1":        true, // benchmarking
		"198.19.255.255":    true,
		"224.0.0.1":         true, // multicast
		"239.255.255.250":   true,
		"255.255.255.255":   true,
		"::1":               true,
		"::":                true,
		"fc00::1":           true,
		"fe80::1":           true,
		"ff02::1":           true, // multicast
		"ff05::2":           true,
		"::ffff:10.0.0.1":   true, // IPv4-mapped
		"::ffff:100.64.0.1": true,
		"64:ff9b::a00:1":    true, // NAT64 of 10.0.0.1
		"64:ff9b::7f00:1":   true, // NAT64 of 127.0.0.1
		"64:ff9b::6440:1":   true, // NAT64 of 100.64.0.1
	}
	for raw, want := range cases {
		if got := isPrivateAddr(netip.MustParseAddr(raw)); got != want {
			t.Errorf("isPrivateAddr(%s) = %v, want %v", raw, got, want)
		}
	}
	if !isPrivateAddr(netip.Addr{}) {
		t.Error("zero addr must be private")
	}
}

// The resolver returns 16-byte IPv4 forms; the SSRF guards convert them with
// AddrFromSlice, which yields IPv4-mapped IPv6 that must still be private.
func TestIsPrivateAddrFromResolverSlice(t *testing.T) {
	for _, raw := range []string{"10.0.0.1", "100.64.0.1", "198.18.0.1"} {
		addr, ok := netip.AddrFromSlice(net.ParseIP(raw).To16())
		if !ok || !isPrivateAddr(addr) {
			t.Errorf("%s via 16-byte slice must be private", raw)
		}
	}
}
