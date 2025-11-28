package pxe

import (
	"fmt"
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

func parseTemplateDataPairs(entries []string) map[string]string {
	if len(entries) == 0 {
		return nil
	}
	parsed := make(map[string]string, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "=", 2)
		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}
		val := ""
		if len(parts) > 1 {
			val = strings.TrimSpace(parts[1])
		}
		parsed[key] = val
	}
	return parsed
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
