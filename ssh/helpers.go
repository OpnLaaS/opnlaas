package ssh

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func MustLocalIP() string {
	var (
		output []byte
		err    error
	)

	if output, err = exec.Command("hostname", "-I").Output(); err == nil {
		return strings.TrimSpace(string(output))
	}

	var interfaces []net.Interface

	if interfaces, err = net.Interfaces(); err != nil {
		panic(err)
	}

	for _, iface := range interfaces {
		var addrs []net.Addr
		if addrs, err = iface.Addrs(); err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
				return strings.TrimSpace(ipNet.IP.String())
			}
		}
	}

	panic("no valid IP address found")
}

func PingHost(ipAddress string) (pinged bool) {
	var (
		cmd *exec.Cmd
		err error
	)

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("ping", "-n", "1", "-w", "2000", ipAddress)
	default:
		cmd = exec.Command("ping", "-c", "1", "-W", "2", ipAddress)
	}

	pinged = true
	if err = cmd.Run(); err != nil {
		pinged = false
	}

	return
}

func WaitOnline(ipAddress string) (err error) {
	var start time.Time = time.Now()

	for time.Since(start) < 3*time.Minute {
		if PingHost(ipAddress) {
			return
		}

		time.Sleep(time.Second * 3)
	}

	err = fmt.Errorf("host not available on %s within timeout", ipAddress)
	return
}

// Given a "cidr" string like "10.255.255.0/24", return the first & last usable addresses, the cidr block, and any error encountered.
func ParseSubnet(cidr string) (firstIP, lastIP net.IP, block *net.IPNet, err error) {
	var ip net.IP

	ip, block, err = net.ParseCIDR(cidr)
	if err != nil {
		return
	}

	firstIP = ip.Mask(block.Mask)
	lastIP = make(net.IP, len(firstIP))
	copy(lastIP, firstIP)

	for i := range len(lastIP) {
		lastIP[i] |= ^block.Mask[i]
	}

	firstIP = firstIP.To4()
	lastIP = lastIP.To4()

	if firstIP != nil {
		firstIP[3] += 1
		lastIP[3] -= 1
	}

	return
}

func GetSubnetRange(first, last net.IP) (ips []net.IP) {
	var nextIP = func(ip net.IP) (next net.IP) {
		ip = ip.To4()
		next = make(net.IP, len(ip))
		copy(next, ip)

		for i := len(next) - 1; i >= 0; i-- {
			next[i]++
			if next[i] != 0 {
				break
			}
		}

		return
	}

	var ip net.IP
	for ip = first; !ip.Equal(last); ip = nextIP(ip) {
		ips = append(ips, ip)
	}

	ips = append(ips, last)
	return
}

func FindOpenIPs(ips []net.IP, numNeeded int) (openIPs []net.IP, err error) {
	if numNeeded > len(ips) {
		err = fmt.Errorf("not enough IPs in set of %d to satisfy request for %d open IPs", len(ips), numNeeded)
		return
	}

	for _, ip := range ips {
		if PingHost(ip.String()) {
			continue
		}

		openIPs = append(openIPs, ip)
		if len(openIPs) >= numNeeded {
			break
		}
	}

	if len(openIPs) < numNeeded {
		err = fmt.Errorf("only found %d open IPs on set of %d, needed %d", len(openIPs), len(ips), numNeeded)
		return
	}

	return
}
