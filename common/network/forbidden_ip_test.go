package network

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestIsForbiddenIPSpecialPurposeRanges proves the user-content address policy
// refuses every IANA special-purpose range that is not globally reachable, and
// every IPv6 form that embeds such an IPv4 address (mapped, compatible, NAT64,
// 6to4, Teredo), while public addresses, including public IPv4 reached through
// a DNS64/NAT64 prefix, stay allowed.
func TestIsForbiddenIPSpecialPurposeRanges(t *testing.T) {
	t.Parallel()

	forbidden := map[string]string{
		"0.1.2.3":         "this network (0.0.0.0/8)",
		"192.0.0.192":     "IETF protocol assignments (192.0.0.0/24)",
		"192.0.2.10":      "TEST-NET-1",
		"198.51.100.10":   "TEST-NET-2",
		"203.0.113.10":    "TEST-NET-3",
		"240.0.0.1":       "reserved (240.0.0.0/4)",
		"255.255.255.255": "limited broadcast",
		"::7f00:1":        "IPv4-compatible loopback",
		"::a9fe:a9fe":     "IPv4-compatible metadata address",
		"64:ff9b::7f00:1": "NAT64 well-known prefix embedding 127.0.0.1",
		"64:ff9b::a9fe:a9fe": "NAT64 well-known prefix embedding the " +
			"169.254.169.254 metadata address",
		"64:ff9b::a00:1":   "NAT64 well-known prefix embedding 10.0.0.1",
		"64:ff9b:1::a00:1": "NAT64 local-use prefix",
		"2002:7f00:1::1":   "6to4 embedding 127.0.0.1",
		"2002:808:808::1":  "6to4 (relay-routed, any embedded address)",
		"2001::1":          "Teredo / IETF protocol assignments (2001::/23)",
		"2001:db8::1":      "IPv6 documentation",
		"3fff::1":          "IPv6 documentation (RFC 9637)",
		"100::1":           "IPv6 discard-only",
		"fec0::1":          "deprecated site-local",
		"5f00::1":          "SRv6 SIDs",
		// Pre-existing coverage that must not regress.
		"127.0.0.1":                "loopback",
		"10.1.2.3":                 "private",
		"169.254.169.254":          "link-local metadata",
		"100.100.100.200":          "CGNAT metadata",
		"::1":                      "IPv6 loopback",
		"::":                       "IPv6 unspecified",
		"0.0.0.0":                  "IPv4 unspecified",
		"::ffff:127.0.0.1":         "IPv4-mapped loopback",
		"::ffff:169.254.169.254":   "IPv4-mapped metadata",
		"fd00:ec2::254":            "unique local (AWS IMDS IPv6)",
		"fe80::1":                  "IPv6 link-local",
		"ff02::1":                  "IPv6 multicast",
		"224.0.0.1":                "IPv4 multicast",
		"64:ff9b::ffff:ffff":       "NAT64 embedding limited broadcast",
		"64:ff9b::6440:1":          "NAT64 embedding CGNAT",
		"::ffff:0:0":               "IPv4-mapped unspecified",
		"64:ff9b::":                "NAT64 embedding 0.0.0.0",
		"2001:0:4136:e378:8000::1": "Teredo client address",
	}
	for raw, why := range forbidden {
		ip := net.ParseIP(raw)
		require.NotNil(t, ip, raw)
		require.True(t, IsForbiddenIP(ip), "%s (%s) must be forbidden", raw, why)
	}

	allowed := map[string]string{
		"8.8.8.8":              "public IPv4",
		"1.1.1.1":              "public IPv4",
		"2606:4700:4700::1111": "public IPv6",
		"::ffff:8.8.8.8":       "IPv4-mapped public address",
		"64:ff9b::808:808":     "DNS64-synthesized public IPv4 on an IPv6-only host",
		// Fake-IP DNS proxies (Clash, Surge, sing-box TUN) answer public names
		// with 198.18.0.0/15 addresses; refusing them would break every remote
		// image fetch on such hosts, so the benchmarking range stays allowed.
		"198.18.0.1": "fake-IP proxy address for a public name",
	}
	for raw, why := range allowed {
		ip := net.ParseIP(raw)
		require.NotNil(t, ip, raw)
		require.False(t, IsForbiddenIP(ip), "%s (%s) must stay allowed", raw, why)
	}

	require.True(t, IsForbiddenIP(nil), "an unparsed address must be refused")
}

// TestValidateExternalURLRejectsEmbeddedPrivateIPv4 drives the URL entry point
// with IPv6 literals that route to internal IPv4 services, which passed before
// the special-purpose ranges were enforced.
func TestValidateExternalURLRejectsEmbeddedPrivateIPv4(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, raw := range []string{
		"http://[64:ff9b::a9fe:a9fe]/latest/meta-data/",
		"http://[64:ff9b::7f00:1]:8080/",
		"http://[::7f00:1]/",
		"http://[2002:a00:1::1]/",
		"http://0.1.2.3/",
		"http://192.0.0.192/opc/v1/instance/",
	} {
		_, err := ValidateExternalURL(ctx, raw)
		require.Error(t, err, raw)
	}
	_, err := ValidateExternalURL(ctx, "http://[64:ff9b::808:808]/image.png")
	require.NoError(t, err, "a DNS64-synthesized public address must stay fetchable")
}
