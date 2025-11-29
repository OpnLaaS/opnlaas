package pxe

import (
	"fmt"
	"hash/crc32"
	"maps"
	"net"
	"strconv"
	"strings"
)

// cloneStringSlice creates a copy of the given string slice.
func cloneStringSlice(src []string) (dst []string) {
	if len(src) == 0 {
		dst = nil
		return
	}

	dst = make([]string, len(src))
	copy(dst, src)
	return
}

// cloneMap creates a copy of the given string map.
func cloneMap(src map[string]string) (out map[string]string) {
	if len(src) == 0 {
		return
	}

	out = make(map[string]string, len(src))
	maps.Copy(out, src)
	return
}

// normalizeMAC normalizes a MAC address to lowercase colon-separated format.
func normalizeMAC(mac string) (normalized string, err error) {
	mac = strings.TrimSpace(mac)
	if mac == "" {
		return
	}

	var addr net.HardwareAddr
	normalized = strings.ToLower(strings.ReplaceAll(mac, "-", ":"))
	if addr, err = net.ParseMAC(normalized); err != nil {
		return
	}

	normalized = strings.ToLower(addr.String())
	return
}

// makeHostSlug creates a slug from the given value suitable for hostnames.
func makeHostSlug(value string) (slug string) {
	slug = strings.ToLower(strings.TrimSpace(value))
	if slug == "" {
		return
	}

	var replacer = strings.NewReplacer(".", "-", ":", "-")
	slug = replacer.Replace(slug)
	return
}

// parsePXELinuxHexIP parses a PXELinux-style hexadecimal IP address string.
func parsePXELinuxHexIP(value string) (parsed string, ok bool) {
	value = strings.TrimSpace(value)
	if len(value) != 8 {
		ok = false
		return
	}

	var (
		parts [4]byte
		num   uint64
		err   error
	)

	for i := range 4 {
		var chunk = value[i*2 : i*2+2]
		if num, err = strconv.ParseUint(chunk, 16, 8); err != nil {
			ok = false
			return
		}

		parts[i] = byte(num)
	}

	parsed = fmt.Sprintf("%d.%d.%d.%d", parts[0], parts[1], parts[2], parts[3])
	ok = true
	return
}

// parseMask parses a string representation of an IPv4 netmask.
func parseMask(value string) (mask net.IPMask) {
	if value == "" {
		return
	}

	var ip net.IP = net.ParseIP(value)
	if ip == nil {
		return
	}

	ip = ip.To4()
	if ip == nil {
		return
	}

	mask = net.IPv4Mask(ip[0], ip[1], ip[2], ip[3])
	return
}

// parseIPv4 parses a string representation of an IPv4 address.
func parseIPv4(value string) (ip net.IP) {
	if ip = net.ParseIP(strings.TrimSpace(value)); ip != nil {
		ip = ip.To4()
	}

	return
}

// compareIPv4 compares two IPv4 addresses.
func compareIPv4(a, b net.IP) (cmp int) {
	a = a.To4()
	b = b.To4()

	if a == nil || b == nil || len(a) != net.IPv4len || len(b) != net.IPv4len {
		return 0
	}

	for i := range net.IPv4len {
		if a[i] < b[i] {
			cmp = -1
			return
		}

		if a[i] > b[i] {
			cmp = 1
			return
		}
	}

	cmp = 0
	return
}

// cloneIPv4 creates a copy of the given IPv4 address.
func cloneIPv4(ip net.IP) (clone net.IP) {
	if ip == nil {
		return
	}

	if ip = ip.To4(); ip == nil {
		return
	}

	clone = make(net.IP, net.IPv4len)
	copy(clone, ip)
	return
}

// makeArtifactDirName creates a unique directory name for the given artifact name.
func makeArtifactDirName(name string) (dir string) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "iso"
	}

	if dir = makeSlug(name); dir == "" {
		dir = "iso"
	}

	var sum = crc32.ChecksumIEEE([]byte(name))
	dir = fmt.Sprintf("%s-%08x", dir, sum)
	return
}

// makeSlug creates a slug from the given value.
func makeSlug(value string) (slug string) {
	if value = strings.ToLower(strings.TrimSpace(value)); value == "" {
		return
	}

	var (
		builder  strings.Builder
		lastDash bool
	)

	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastDash = false
			continue
		}

		if !lastDash && builder.Len() > 0 {
			builder.WriteRune('-')
			lastDash = true
		}
	}

	slug = strings.Trim(builder.String(), "-")
	return
}
