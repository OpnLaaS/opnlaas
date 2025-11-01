package iso

import (
	"fmt"
	"net"
	"os/exec"
	"strings"

	"github.com/z46-dev/go-logger"
)

func myLocalIP() (string, error) {
	ifaces, err := net.Interfaces()

	if err != nil {
		return "", err
	}

	var bestChoice string
	for _, i := range ifaces {
		addrs, err := i.Addrs()

		if err != nil {
			return "", err
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if bestChoice == "" {
				bestChoice = ip.String()
			} else if !(strings.HasPrefix(bestChoice, "10.") || strings.HasPrefix(bestChoice, "192.168.")) && (strings.HasPrefix(ip.String(), "10.") || strings.HasPrefix(ip.String(), "192.168.")) {
				bestChoice = ip.String()
			}
		}
	}

	return bestChoice, nil
}

func runCommand(name string, args ...string) (_ string, err error) {
	var (
		cmd    *exec.Cmd
		stdout []byte
	)

	cmd = exec.Command(name, args...)

	if stdout, err = cmd.Output(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("failed to run command: %s", string(exitErr.Stderr))
		}

		return "", fmt.Errorf("failed to run command: %s", err)
	}

	return strings.TrimSpace(string(stdout)), nil
}

func cleanUpMount() error {
	if _, err := runCommand("umount", "/mnt/iso"); err != nil {
		return fmt.Errorf("failed to unmount /mnt/iso: %s", err)
	}

	if _, err := runCommand("rm", "-rf", "/mnt/iso"); err != nil {
		return fmt.Errorf("failed to remove /mnt/iso: %s", err)
	}

	return nil
}

func stringPtr(s string) *string {
	return &s
}

func logColorBasedOnName(name string) logger.ColorCode {
	var lowerName = strings.ToLower(name)

	if strings.Contains(lowerName, "ubuntu") {
		return logger.BoldRed
	}

	if strings.Contains(lowerName, "fedora") {
		return logger.BoldBlue
	}

	if strings.Contains(lowerName, "debian") {
		return logger.BoldYellow
	}

	if strings.Contains(lowerName, "centos") {
		return logger.BoldCyan
	}

	if strings.Contains(lowerName, "arch") {
		return logger.BoldPurple
	}

	if strings.Contains(lowerName, "suse") {
		return logger.BoldGreen
	}

	return logger.BoldWhite
}

func distroTypeBasedOnName(name string) distroType {
	var lowerName = strings.ToLower(name)

	if strings.Contains(lowerName, "ubuntu") {
		return UBUNTU
	}

	if strings.Contains(lowerName, "fedora") ||
		strings.Contains(lowerName, "redhat") ||
		strings.Contains(lowerName, "rhel") ||
		strings.Contains(lowerName, "centos") {
		return REDHAT
	}

	if strings.Contains(lowerName, "suse") {
		return SUSE
	}

	return UNKNOWN
}