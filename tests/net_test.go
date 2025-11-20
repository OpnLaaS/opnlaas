package tests

import (
	"net"
	"testing"

	"github.com/opnlaas/opnlaas/ssh"
)

func TestCIDRParse(t *testing.T) {
	var (
		correctFirstIP, correctLastIP net.IP     = net.ParseIP("10.255.255.1"), net.ParseIP("10.255.255.254")
		correctBlock                  *net.IPNet = &net.IPNet{
			IP:   net.ParseIP("10.255.255.0"),
			Mask: net.CIDRMask(24, 32),
		}
		firstIP, lastIP net.IP
		block           *net.IPNet
		err             error
	)

	if firstIP, lastIP, block, err = ssh.ParseSubnet("10.255.255.0/24"); err != nil {
		t.Fatalf("Failed to parse CIDR: %v", err)
	}

	if !firstIP.Equal(correctFirstIP) {
		t.Fatalf("First IP incorrect, expected %v, got %v", correctFirstIP, firstIP)
	}

	if !lastIP.Equal(correctLastIP) {
		t.Fatalf("Last IP incorrect, expected %v, got %v", correctLastIP, lastIP)
	}

	if !block.IP.Equal(correctBlock.IP) || block.Mask.String() != correctBlock.Mask.String() {
		t.Fatalf("Block incorrect, expected %v, got %v", correctBlock, block)
	}
}

func TestCIDRParseLargeSubnet(t *testing.T) {
	var (
		correctFirstIP, correctLastIP net.IP     = net.ParseIP("10.255.252.1"), net.ParseIP("10.255.255.254")
		correctBlock                  *net.IPNet = &net.IPNet{
			IP:   net.ParseIP("10.255.252.0"),
			Mask: net.CIDRMask(22, 32),
		}
		firstIP, lastIP net.IP
		block           *net.IPNet
		err             error
	)

	if firstIP, lastIP, block, err = ssh.ParseSubnet("10.255.252.0/22"); err != nil {
		t.Fatalf("Failed to parse CIDR: %v", err)
	}

	if !firstIP.Equal(correctFirstIP) {
		t.Fatalf("First IP incorrect, expected %v, got %v", correctFirstIP, firstIP)
	}

	if !lastIP.Equal(correctLastIP) {
		t.Fatalf("Last IP incorrect, expected %v, got %v", correctLastIP, lastIP)
	}

	if !block.IP.Equal(correctBlock.IP) || block.Mask.String() != correctBlock.Mask.String() {
		t.Fatalf("Block incorrect, expected %v, got %v", correctBlock, block)
	}
}

func TestCIDRParseInvalid(t *testing.T) {
	var err error

	if _, _, _, err = ssh.ParseSubnet("invalid-cidr"); err == nil {
		t.Fatalf("Expected error when parsing invalid CIDR, got nil")
	}

	if _, _, _, err = ssh.ParseSubnet("10.255.255.0/33"); err == nil {
		t.Fatalf("Expected error when parsing CIDR with invalid mask, got nil")
	}

	if _, _, _, err = ssh.ParseSubnet("300.255.255.0/24"); err == nil {
		t.Fatalf("Expected error when parsing CIDR with invalid IP, got nil")
	}
}
