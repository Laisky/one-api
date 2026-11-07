package network

import (
	"net"
	"net/netip"
)

// nat64WellKnownPrefix is the RFC 6052 NAT64 well-known prefix. A NAT64
// gateway translates 64:ff9b::a.b.c.d to the IPv4 address a.b.c.d, including
// private and link-local ones (AWS NAT gateways reach in-VPC IPv4 this way), so
// an address in it is judged by the IPv4 address it embeds.
var nat64WellKnownPrefix = netip.MustParsePrefix("64:ff9b::/96")

// forbiddenPrefixes lists the IANA special-purpose IPv4 and IPv6 ranges that
// are not globally reachable, beyond what the netip predicates in
// IsForbiddenIP already cover. 198.18.0.0/15 (benchmarking) is deliberately
// absent: fake-IP DNS proxies (Clash, Surge, sing-box TUN) answer public names
// with addresses from it, and refusing it would break every remote fetch on
// such hosts.
var forbiddenPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this network"
	netip.MustParsePrefix("100.64.0.0/10"),   // shared address space (CGNAT, cloud metadata such as 100.100.100.200)
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("192.88.99.0/24"),  // deprecated 6to4 relay anycast
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved, including 255.255.255.255
	netip.MustParsePrefix("::/96"),           // deprecated IPv4-compatible addresses
	netip.MustParsePrefix("64:ff9b:1::/48"),  // local-use NAT64, operator-chosen embedding
	netip.MustParsePrefix("100::/64"),        // discard-only
	netip.MustParsePrefix("2001::/23"),       // IETF protocol assignments, including Teredo
	netip.MustParsePrefix("2001:db8::/32"),   // documentation
	netip.MustParsePrefix("2002::/16"),       // 6to4, which embeds an arbitrary IPv4 address
	netip.MustParsePrefix("3fff::/20"),       // documentation (RFC 9637)
	netip.MustParsePrefix("5f00::/16"),       // SRv6 SIDs
	netip.MustParsePrefix("fec0::/10"),       // deprecated site-local
}

// IsForbiddenIP reports whether ip must not be dialed for user-supplied
// content: loopback, private, link-local, multicast, unspecified, or any other
// IANA special-purpose address that is not globally reachable. IPv4-mapped
// addresses are judged as IPv4 and NAT64 well-known-prefix addresses by the
// IPv4 address they embed, so a public IPv4 host stays reachable through
// DNS64 on an IPv6-only network. Parameters: ip is the resolved address; nil
// or malformed input is forbidden. Returns true when the address is refused.
func IsForbiddenIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	addr = addr.Unmap()
	if nat64WellKnownPrefix.Contains(addr) {
		embedded := addr.As16()
		addr = netip.AddrFrom4([4]byte(embedded[12:]))
	}
	return isForbiddenAddr(addr)
}

// isForbiddenAddr applies the address policy to an unmapped address.
// Parameters: addr is an IPv4 or native IPv6 address. Returns true when it is
// not a globally reachable unicast address.
func isForbiddenAddr(addr netip.Addr) bool {
	if !addr.IsValid() || addr.IsUnspecified() || addr.IsLoopback() || addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsInterfaceLocalMulticast() ||
		addr.IsMulticast() {
		return true
	}
	for _, prefix := range forbiddenPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
