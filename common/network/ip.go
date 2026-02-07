package network

import (
	"context"
	"net"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/songquanpeng/one-api/common/logger"
)

func splitSubnets(subnets string) []string {
	res := strings.Split(subnets, ",")
	for i := range res {
		res[i] = strings.TrimSpace(res[i])
	}
	return res
}

func isValidSubnet(subnet string) error {
	_, _, err := net.ParseCIDR(subnet)
	if err != nil {
		return errors.Wrapf(err, "failed to parse subnet: %s", subnet)
	}
	return nil
}

func isIpInSubnet(ctx context.Context, ip string, subnet string) bool {
	_, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		logger.Logger.Error("failed to parse subnet", zap.String("subnet", subnet), zap.Error(errors.Wrapf(err, "parse subnet: %s", subnet)))
		return false
	}
	return ipNet.Contains(net.ParseIP(ip))
}

// IsValidSubnets verifies that every comma-separated subnet string parses as a CIDR block.
func IsValidSubnets(subnets string) error {
	for _, subnet := range splitSubnets(subnets) {
		if err := isValidSubnet(subnet); err != nil {
			return errors.Wrapf(err, "invalid subnet in list: %s", subnet)
		}
	}
	return nil
}

// IsIpInSubnets evaluates whether the provided IP address belongs to any subnet in the comma-separated list.
func IsIpInSubnets(ctx context.Context, ip string, subnets string) bool {
	for _, subnet := range splitSubnets(subnets) {
		if isIpInSubnet(ctx, ip, subnet) {
			return true
		}
	}
	return false
}

// IsInternalIP returns true if the IP is a loopback, link-local, private, or unspecified address.
func IsInternalIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate() || ip.IsUnspecified()
}
