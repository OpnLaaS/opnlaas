package app

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/opnlaas/opnlaas/config"
	"github.com/opnlaas/opnlaas/db"
)

type bookingNetworkDefaults struct {
	Supernet          netip.Prefix
	BookingPrefix     int
	GatewayIPv4       netip.Addr
	HostNetworkPrefix int
	HostStartOffset   int
	DNSServers        []string
	DisableOtherNICs  bool
}

func loadBookingNetworkDefaults() (defaults bookingNetworkDefaults, err error) {
	defaults.BookingPrefix = config.Config.BookingNetworking.BookingPrefix
	defaults.HostNetworkPrefix = config.Config.BookingNetworking.HostNetworkPrefix
	defaults.HostStartOffset = config.Config.BookingNetworking.HostStartOffset
	defaults.DNSServers = append([]string(nil), config.Config.BookingNetworking.DNSServers...)
	defaults.DisableOtherNICs = config.Config.BookingNetworking.DisableOtherNICs

	if defaults.Supernet, err = parseIPv4Prefix(config.Config.BookingNetworking.SupernetCIDR); err != nil {
		return defaults, fmt.Errorf("invalid booking networking supernet: %w", err)
	}
	if defaults.GatewayIPv4, err = parseIPv4Address(config.Config.BookingNetworking.GatewayIPv4); err != nil {
		return defaults, fmt.Errorf("invalid booking networking gateway_ipv4: %w", err)
	}

	if defaults.BookingPrefix < defaults.Supernet.Bits() {
		return defaults, fmt.Errorf("booking prefix /%d cannot be larger than supernet prefix /%d", defaults.BookingPrefix, defaults.Supernet.Bits())
	}

	if defaults.BookingPrefix > 30 {
		return defaults, fmt.Errorf("booking prefix /%d too small for host assignment", defaults.BookingPrefix)
	}
	if defaults.HostNetworkPrefix < 1 || defaults.HostNetworkPrefix > 30 {
		return defaults, fmt.Errorf("host network prefix /%d is invalid", defaults.HostNetworkPrefix)
	}

	usableHosts := usableHostCount(defaults.BookingPrefix)
	if usableHosts <= 2 {
		return defaults, fmt.Errorf("booking prefix /%d does not leave enough host addresses", defaults.BookingPrefix)
	}

	if defaults.HostStartOffset <= 0 || defaults.HostStartOffset >= usableHosts+1 {
		return defaults, fmt.Errorf("host start offset %d must be within subnet host range", defaults.HostStartOffset)
	}

	return defaults, nil
}

func parseIPv4Prefix(raw string) (prefix netip.Prefix, err error) {
	if prefix, err = netip.ParsePrefix(strings.TrimSpace(raw)); err != nil {
		return prefix, err
	}

	if !prefix.Addr().Is4() {
		return prefix, fmt.Errorf("expected IPv4 CIDR")
	}

	return prefix.Masked(), nil
}

func parseIPv4Address(raw string) (addr netip.Addr, err error) {
	if addr, err = netip.ParseAddr(strings.TrimSpace(raw)); err != nil {
		return addr, err
	}

	if !addr.Is4() {
		return addr, fmt.Errorf("expected IPv4 address")
	}

	return addr, nil
}

func usableHostCount(prefixBits int) int {
	if prefixBits >= 31 {
		return 0
	}

	size := 1 << (32 - prefixBits)
	return size - 2
}

func prefixNetworkAndBroadcast(prefix netip.Prefix) (network netip.Addr, broadcast netip.Addr, err error) {
	var netUint uint32
	if netUint, err = ipv4ToUint32(prefix.Masked().Addr()); err != nil {
		return
	}

	blockSize := subnetBlockSize(prefix.Bits())
	if blockSize == 0 {
		err = fmt.Errorf("invalid block size for /%d", prefix.Bits())
		return
	}

	network, _ = uint32ToIPv4(netUint)
	broadcast, _ = uint32ToIPv4(netUint + blockSize - 1)
	return
}

func subnetBlockSize(prefixBits int) uint32 {
	if prefixBits < 0 || prefixBits > 32 {
		return 0
	}

	hostBits := 32 - prefixBits
	if hostBits >= 32 {
		return 0
	}

	return uint32(1) << hostBits
}

func ipv4ToUint32(addr netip.Addr) (value uint32, err error) {
	if !addr.Is4() {
		return 0, fmt.Errorf("address %s is not IPv4", addr.String())
	}

	bytes := addr.As4()
	value = (uint32(bytes[0]) << 24) | (uint32(bytes[1]) << 16) | (uint32(bytes[2]) << 8) | uint32(bytes[3])
	return
}

func uint32ToIPv4(value uint32) (addr netip.Addr, err error) {
	addr = netip.AddrFrom4([4]byte{
		byte(value >> 24),
		byte(value >> 16),
		byte(value >> 8),
		byte(value),
	})
	return
}

func bookingCIDROverlapsAny(candidate netip.Prefix, excludeBookingID int) (overlap bool, err error) {
	var bookings []*db.Booking
	if bookings, err = db.BookingList(); err != nil {
		return
	}

	for _, booking := range bookings {
		if booking == nil || booking.ID == excludeBookingID {
			continue
		}

		cidr := strings.TrimSpace(booking.CIDRBlock)
		if cidr == "" {
			continue
		}

		var existing netip.Prefix
		if existing, err = parseIPv4Prefix(cidr); err != nil {
			continue
		}

		if prefixesOverlap(candidate, existing) {
			return true, nil
		}
	}

	return false, nil
}

func prefixesOverlap(a, b netip.Prefix) bool {
	return a.Contains(b.Addr()) || b.Contains(a.Addr())
}

func suggestNextBookingCIDR(defaults bookingNetworkDefaults) (cidr netip.Prefix, err error) {
	var superStart uint32
	if superStart, err = ipv4ToUint32(defaults.Supernet.Masked().Addr()); err != nil {
		return
	}

	superSize := subnetBlockSize(defaults.Supernet.Bits())
	bookingSize := subnetBlockSize(defaults.BookingPrefix)
	if bookingSize == 0 || superSize == 0 || bookingSize > superSize {
		return cidr, fmt.Errorf("invalid booking/supernet size relationship")
	}

	limit := superStart + superSize
	for base := superStart; base+bookingSize <= limit; base += bookingSize {
		var addr netip.Addr
		if addr, err = uint32ToIPv4(base); err != nil {
			return
		}

		candidate := netip.PrefixFrom(addr, defaults.BookingPrefix).Masked()
		if !defaults.Supernet.Contains(candidate.Addr()) {
			continue
		}

		var overlap bool
		if overlap, err = bookingCIDROverlapsAny(candidate, 0); err != nil {
			return
		}

		if !overlap {
			return candidate, nil
		}
	}

	return cidr, fmt.Errorf("no available booking subnet in %s", defaults.Supernet.String())
}

func validateBookingCIDR(defaults bookingNetworkDefaults, raw string, excludeBookingID int) (cidr netip.Prefix, gateway netip.Addr, err error) {
	if cidr, err = parseIPv4Prefix(raw); err != nil {
		return cidr, gateway, fmt.Errorf("invalid booking subnet: %w", err)
	}

	if cidr.Bits() != defaults.BookingPrefix {
		return cidr, gateway, fmt.Errorf("booking subnet must be /%d", defaults.BookingPrefix)
	}

	network, _, netErr := prefixNetworkAndBroadcast(cidr)
	if netErr != nil {
		return cidr, gateway, netErr
	}

	if !defaults.Supernet.Contains(network) {
		return cidr, gateway, fmt.Errorf("booking subnet %s is outside allowed supernet %s", cidr.String(), defaults.Supernet.String())
	}

	var overlap bool
	if overlap, err = bookingCIDROverlapsAny(cidr, excludeBookingID); err != nil {
		return
	}

	if overlap {
		return cidr, gateway, fmt.Errorf("booking subnet %s overlaps an existing booking", cidr.String())
	}

	if !gatewayReachableFromSubnet(cidr, defaults.GatewayIPv4, defaults.HostNetworkPrefix) {
		return cidr, gateway, fmt.Errorf(
			"gateway %s is not reachable from booking subnet %s with host network prefix /%d",
			defaults.GatewayIPv4.String(),
			cidr.String(),
			defaults.HostNetworkPrefix,
		)
	}

	gateway = defaults.GatewayIPv4
	return
}

func gatewayReachableFromSubnet(cidr netip.Prefix, gateway netip.Addr, hostPrefix int) bool {
	if !gateway.Is4() || hostPrefix < 1 || hostPrefix > 32 {
		return false
	}

	network := netip.PrefixFrom(cidr.Masked().Addr(), hostPrefix).Masked()
	return network.Contains(gateway)
}

func hostOffsetToIPv4(cidr netip.Prefix, hostOffset int) (addr netip.Addr, err error) {
	if hostOffset <= 0 {
		return addr, fmt.Errorf("host offset must be positive")
	}

	network, broadcast, err := prefixNetworkAndBroadcast(cidr)
	if err != nil {
		return addr, err
	}

	networkUint, err := ipv4ToUint32(network)
	if err != nil {
		return addr, err
	}
	broadcastUint, err := ipv4ToUint32(broadcast)
	if err != nil {
		return addr, err
	}

	candidateUint := networkUint + uint32(hostOffset)
	if candidateUint >= broadcastUint {
		return addr, fmt.Errorf("host offset %d is outside subnet %s usable range", hostOffset, cidr.String())
	}

	addr, _ = uint32ToIPv4(candidateUint)
	return
}

func validateAndAssignHostIPs(
	cidr netip.Prefix,
	gateway netip.Addr,
	hosts []db.BookingRequestHost,
	startOffset int,
) (updated []db.BookingRequestHost, err error) {
	updated = make([]db.BookingRequestHost, 0, len(hosts))
	if len(hosts) == 0 {
		return
	}

	network, broadcast, err := prefixNetworkAndBroadcast(cidr)
	if err != nil {
		return nil, err
	}

	netUint, _ := ipv4ToUint32(network)
	used := map[string]string{}

	for _, host := range hosts {
		assigned := strings.TrimSpace(host.AssignedIPv4)
		if assigned == "" {
			continue
		}

		addr, addrErr := parseIPv4Address(assigned)
		if addrErr != nil {
			return nil, fmt.Errorf("host %s has invalid assigned ip %q", host.ManagementIP, assigned)
		}

		if !cidr.Contains(addr) {
			return nil, fmt.Errorf("host %s assigned ip %s is outside subnet %s", host.ManagementIP, addr.String(), cidr.String())
		}
		if addr == network || addr == broadcast {
			return nil, fmt.Errorf("host %s assigned ip %s cannot be network or broadcast address", host.ManagementIP, addr.String())
		}
		if addr == gateway {
			return nil, fmt.Errorf("host %s assigned ip %s conflicts with gateway", host.ManagementIP, addr.String())
		}

		key := addr.String()
		if existingHost, exists := used[key]; exists && existingHost != host.ManagementIP {
			return nil, fmt.Errorf("duplicate assigned ip %s for hosts %s and %s", key, existingHost, host.ManagementIP)
		}

		used[key] = host.ManagementIP
	}

	hostSize := subnetBlockSize(cidr.Bits())
	maxOffset := int(hostSize) - 2
	nextOffset := startOffset
	if nextOffset < 2 {
		nextOffset = 2
	}

	gatewayUint, _ := ipv4ToUint32(gateway)
	for _, host := range hosts {
		if strings.TrimSpace(host.AssignedIPv4) != "" {
			updated = append(updated, host)
			continue
		}

		for {
			if nextOffset > maxOffset {
				return nil, fmt.Errorf("no available IPs left in subnet %s", cidr.String())
			}

			candidateUint := netUint + uint32(nextOffset)
			nextOffset++
			if candidateUint == gatewayUint {
				continue
			}

			candidateAddr, _ := uint32ToIPv4(candidateUint)
			candidate := candidateAddr.String()
			if _, exists := used[candidate]; exists {
				continue
			}

			host.AssignedIPv4 = candidate
			used[candidate] = host.ManagementIP
			updated = append(updated, host)
			break
		}
	}

	sort.Slice(updated, func(i, j int) bool { return updated[i].ManagementIP < updated[j].ManagementIP })
	return
}

func prefixMaskString(prefix netip.Prefix) (mask string) {
	mask = prefixMaskStringFromBits(prefix.Bits())
	return
}

func prefixMaskStringFromBits(bits int) (mask string) {
	var value uint32
	if bits == 0 {
		value = 0
	} else {
		value = ^uint32(0) << (32 - bits)
	}

	addr, _ := uint32ToIPv4(value)
	mask = addr.String()
	return
}
