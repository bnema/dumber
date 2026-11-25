// Package netaddr classifies IP addresses for SSRF and local network guards.
package netaddr

import "net/netip"

// Non-public ranges that netip.Addr has no predicate for.
var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),     // "this network"
	netip.MustParsePrefix("100.64.0.0/10"), // CGNAT, Tailscale
	netip.MustParsePrefix("192.0.0.0/24"),  // IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"), // benchmarking
	netip.MustParsePrefix("240.0.0.0/4"),   // reserved, broadcast
}

// nat64Prefix is the well-known NAT64 prefix; its last 32 bits embed IPv4.
var nat64Prefix = netip.MustParsePrefix("64:ff9b::/96")

// IsPrivate reports whether addr is not a public unicast address. It
// unmaps IPv4-mapped IPv6 and checks the IPv4 embedded in NAT64 addresses.
func IsPrivate(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() {
		return true
	}
	if addr.Is6() && nat64Prefix.Contains(addr) {
		b := addr.As16()
		return IsPrivate(netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}))
	}
	if addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() || addr.IsInterfaceLocalMulticast() ||
		addr.IsMulticast() || addr.IsUnspecified() {
		return true
	}
	for _, p := range nonPublicPrefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
