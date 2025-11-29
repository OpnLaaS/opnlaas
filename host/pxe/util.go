package pxe

import (
	"fmt"
	"hash/crc32"
	"net"
	"strconv"
	"strings"
)

func cloneStringSlice(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	out := make([]string, 0, len(src))
	for _, v := range src {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func normalizeMAC(mac string) (string, error) {
	mac = strings.TrimSpace(mac)
	if mac == "" {
		return "", nil
	}
	cleaned := strings.ToLower(strings.ReplaceAll(mac, "-", ":"))
	parsed, err := net.ParseMAC(cleaned)
	if err != nil {
		return "", err
	}
	return strings.ToLower(parsed.String()), nil
}

func makeHostSlug(value string) string {
	cleaned := strings.TrimSpace(strings.ToLower(value))
	if cleaned == "" {
		return ""
	}
	replacer := strings.NewReplacer(".", "-", ":", "-")
	return replacer.Replace(cleaned)
}

func parsePXELinuxHexIP(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) != 8 {
		return "", false
	}
	var parts [4]byte
	for i := 0; i < 4; i++ {
		chunk := value[i*2 : i*2+2]
		num, err := strconv.ParseUint(chunk, 16, 8)
		if err != nil {
			return "", false
		}
		parts[i] = byte(num)
	}
	return fmt.Sprintf("%d.%d.%d.%d", parts[0], parts[1], parts[2], parts[3]), true
}

func parseMask(value string) net.IPMask {
	if value == "" {
		return nil
	}
	ip := net.ParseIP(value)
	if ip == nil {
		return nil
	}
	ip = ip.To4()
	if ip == nil {
		return nil
	}
	return net.IPv4Mask(ip[0], ip[1], ip[2], ip[3])
}

func parseIP(value string) net.IP {
	if value == "" {
		return nil
	}
	return net.ParseIP(value)
}

func parseIPv4(value string) net.IP {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return nil
	}
	return ip.To4()
}

func compareIPv4(a, b net.IP) int {
	if len(a) < net.IPv4len || len(b) < net.IPv4len {
		return 0
	}
	for i := 0; i < net.IPv4len; i++ {
		if a[i] == b[i] {
			continue
		}
		if a[i] < b[i] {
			return -1
		}
		return 1
	}
	return 0
}

func cloneIPv4(ip net.IP) net.IP {
	if ip == nil {
		return nil
	}
	ip = ip.To4()
	if ip == nil {
		return nil
	}
	out := make(net.IP, net.IPv4len)
	copy(out, ip)
	return out
}

func makeArtifactDirName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "iso"
	}
	base := makeSlug(name)
	if base == "" {
		base = "iso"
	}
	sum := crc32.ChecksumIEEE([]byte(name))
	return fmt.Sprintf("%s-%08x", base, sum)
}

func makeSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	var lastDash bool
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
