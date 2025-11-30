package pxe

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/opnlaas/opnlaas/config"
	"github.com/opnlaas/opnlaas/db"
)

// handleDHCP processes an incoming DHCP request and sends an appropriate response.
func (s *Service) handleDHCP(conn net.PacketConn, peer net.Addr, req *dhcpv4.DHCPv4) (err error) {
	if req == nil {
		err = fmt.Errorf("nil request")
		return
	}

	var (
		mac     string             = req.ClientHWAddr.String()
		msgType dhcpv4.MessageType = req.MessageType()
		reply   *dhcpv4.DHCPv4
	)

	if !isPXEClient(req) {
		return
	}

	s.log.Basicf("DHCP request type=%s mac=%s xid=%d\n", msgType, mac, req.TransactionID)

	switch msgType {
	case dhcpv4.MessageTypeDiscover:
		reply, err = s.buildDHCPOffer(req)
	case dhcpv4.MessageTypeRequest:
		if s.proxyDHCP {
			return
		}

		reply, err = s.buildDHCPAck(req)
	default:
		return
	}

	if err != nil || reply == nil {
		return
	}

	err = sendDHCP(conn, peer, reply)
	return
}

// buildDHCPOffer constructs a DHCP Offer message in response to a DHCP Discover.
func (s *Service) buildDHCPOffer(req *dhcpv4.DHCPv4) (offer *dhcpv4.DHCPv4, err error) {
	var (
		mac     string = req.ClientHWAddr.String()
		profile *db.HostPXEProfile
	)

	if profile, err = s.profileForMAC(mac); err != nil {
		return
	} else if profile == nil {
		err = fmt.Errorf("no PXE profile available for %s", mac)
		return
	}

	var resp *dhcpv4.DHCPv4
	if resp, err = dhcpv4.NewReplyFromRequest(req); err != nil {
		return
	}

	var ip net.IP
	if ip, err = s.leaseIPForProfile(mac, profile); err != nil {
		return
	}

	resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeOffer))
	if ip != nil {
		resp.YourIPAddr = ip
	}

	s.decorateReply(resp, profile)
	offer = resp
	return
}

// buildDHCPAck constructs a DHCP Acknowledgment message in response to a DHCP Request.
func (s *Service) buildDHCPAck(req *dhcpv4.DHCPv4) (response *dhcpv4.DHCPv4, err error) {
	if s.proxyDHCP {
		return
	}

	var (
		mac     string = req.ClientHWAddr.String()
		profile *db.HostPXEProfile
	)

	if profile, err = s.profileForMAC(mac); err != nil {
		return
	} else if profile == nil {
		err = fmt.Errorf("no PXE profile available for %s", mac)
		return
	}

	var resp *dhcpv4.DHCPv4
	if resp, err = dhcpv4.NewReplyFromRequest(req); err != nil {
		return
	}

	var ip net.IP
	if ip, err = s.leaseIPForProfile(mac, profile); err != nil {
		return
	}

	resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))
	if ip != nil {
		resp.YourIPAddr = ip
	}

	s.decorateReply(resp, profile)
	if ip != nil {
		s.leases.Set(mac, ip)
	}
	response = resp
	return
}

// leaseIPForProfile determines and leases an IP address for the given MAC and PXE profile.
func (s *Service) leaseIPForProfile(mac string, profile *db.HostPXEProfile) (lease net.IP, err error) {
	if s.proxyDHCP {
		return
	}

	if profile == nil {
		err = fmt.Errorf("nil profile")
		return
	}

	if lease = parseIPv4(profile.IPv4Address); lease != nil {
		if err = s.ensureIPWithinRange(lease); err != nil {
			lease = nil
			return
		}

		return
	}

	if mac != "" {
		var leased net.IP
		if leased = s.leases.Get(mac); leased != nil {
			if err = s.ensureIPWithinRange(leased); err != nil {
				lease = nil
				return
			}

			lease = leased
			return
		}
	}

	lease, err = s.allocateDynamicIP(mac)
	return
}

// ensureIPWithinRange checks if the given IP is within the configured DHCP range.
func (s *Service) ensureIPWithinRange(ip net.IP) (err error) {
	if ip == nil {
		err = fmt.Errorf("nil ip address")
		return
	}

	if s.ipRangeStart == nil || s.ipRangeEnd == nil {
		return
	}

	if compareIPv4(ip, s.ipRangeStart) < 0 || compareIPv4(ip, s.ipRangeEnd) > 0 {
		err = fmt.Errorf("ip %s outside allowed DHCP range %s-%s", ip.String(), s.ipRangeStart.String(), s.ipRangeEnd.String())
		return
	}

	return
}

// allocateDynamicIP finds and allocates an available IP address from the DHCP range.
func (s *Service) allocateDynamicIP(mac string) (ip net.IP, err error) {
	// if s.ipRangeStart == nil || s.ipRangeEnd == nil {
	// 	return nil, fmt.Errorf("no DHCP IP range configured")
	// }

	// s.leaseMu.Lock()
	// defer s.leaseMu.Unlock()

	// if s.leaseCursor == nil {
	// 	s.leaseCursor = cloneIPv4(s.ipRangeStart)
	// }

	// start := cloneIPv4(s.leaseCursor)
	// for {
	// 	candidate := cloneIPv4(s.leaseCursor)
	// 	s.advanceLeaseCursor()
	// 	if !s.leases.InUse(candidate) {
	// 		if mac != "" {
	// 			s.leases.Set(mac, candidate)
	// 		}
	// 		return candidate, nil
	// 	}
	// 	if compareIPv4(s.leaseCursor, start) == 0 {
	// 		break
	// 	}
	// }
	// return nil, fmt.Errorf("no available DHCP addresses in range %s-%s", s.ipRangeStart, s.ipRangeEnd)

	if s.ipRangeStart == nil || s.ipRangeEnd == nil {
		err = fmt.Errorf("no DHCP IP range configured")
		return
	}

	s.leaseMu.Lock()
	defer s.leaseMu.Unlock()

	if s.leaseCursor == nil {
		s.leaseCursor = cloneIPv4(s.ipRangeStart)
	}

	var start net.IP = cloneIPv4(s.leaseCursor)
	for {
		var candidate net.IP = cloneIPv4(s.leaseCursor)
		s.advanceLeaseCursor()
		if !s.leases.InUse(candidate) {
			if mac != "" {
				s.leases.Set(mac, candidate)
			}

			ip = candidate
			return
		}

		if compareIPv4(s.leaseCursor, start) == 0 {
			break
		}
	}

	err = fmt.Errorf("no available DHCP addresses in range %s-%s", s.ipRangeStart, s.ipRangeEnd)
	return
}

// advanceLeaseCursor moves the lease cursor to the next IP address in the DHCP range.
func (s *Service) advanceLeaseCursor() {
	if s.ipRangeStart == nil || s.ipRangeEnd == nil {
		return
	}

	if s.leaseCursor == nil {
		s.leaseCursor = cloneIPv4(s.ipRangeStart)
		return
	}

	for i := len(s.leaseCursor) - 1; i >= 0; i-- {
		s.leaseCursor[i]++
		if s.leaseCursor[i] != 0 {
			break
		}
	}

	if compareIPv4(s.leaseCursor, s.ipRangeEnd) > 0 {
		s.leaseCursor = cloneIPv4(s.ipRangeStart)
	}
}

// decorateReply adds necessary options to the DHCP reply based on the PXE profile.
func (s *Service) decorateReply(resp *dhcpv4.DHCPv4, profile *db.HostPXEProfile) {
	if !s.proxyDHCP {
		var serverIP net.IP = s.serverIdentifierIP()
		if serverIP != nil {
			resp.Options.Update(dhcpv4.OptServerIdentifier(serverIP))
		}
	}

	if !s.proxyDHCP {
		var subnetMask string = profile.SubnetMask
		if strings.TrimSpace(subnetMask) == "" {
			subnetMask = config.Config.PXE.DHCPServer.SubnetMask
		}

		var mask net.IPMask
		if mask = parseMask(subnetMask); mask != nil {
			resp.Options.Update(dhcpv4.Option{Code: dhcpv4.OptionSubnetMask, Value: dhcpv4.IPMask(mask)})
		}

		var routerIP string = profile.Gateway
		if strings.TrimSpace(routerIP) == "" {
			routerIP = config.Config.PXE.DHCPServer.Router
		}

		var router net.IP
		if router = net.ParseIP(routerIP); router != nil {
			resp.Options.Update(dhcpv4.Option{Code: dhcpv4.OptionRouter, Value: dhcpv4.IP(router)})
		}

		var dnsEntries []string = profile.DNSServers
		if len(dnsEntries) == 0 {
			dnsEntries = config.Config.PXE.DHCPServer.DNSServers
		}

		var dns []net.IP
		for _, entry := range dnsEntries {
			if ip := net.ParseIP(entry); ip != nil {
				dns = append(dns, ip)
			}
		}

		if len(dns) > 0 {
			resp.Options.Update(dhcpv4.OptDNS(dns...))
		}

		if profile.DomainName != "" {
			resp.Options.Update(dhcpv4.OptDomainName(profile.DomainName))
		}
	}

	if profile.BootFilename != "" {
		resp.BootFileName = profile.BootFilename
	} else if s.defaultProfile.BootFilename != "" {
		resp.BootFileName = s.defaultProfile.BootFilename
	} else {
		resp.BootFileName = "pxelinux.0"
	}

	var next net.IP
	if next = net.ParseIP(profile.NextServer); next != nil {
		resp.ServerIPAddr = next
	} else if ip := net.ParseIP(config.Config.PXE.DHCPServer.ServerPublicAddress); ip != nil {
		resp.ServerIPAddr = ip
	}

	if !s.proxyDHCP {
		var lease time.Duration = time.Duration(config.Config.PXE.DHCPServer.LeaseSeconds) * time.Second
		resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(lease))
	}
}

// profileForMAC retrieves the PXE profile associated with the given MAC address.
func (s *Service) profileForMAC(mac string) (profile *db.HostPXEProfile, err error) {
	if mac = strings.TrimSpace(mac); mac == "" {
		profile = s.buildDefaultProfile(nil, "")
		return
	}

	if profile = s.overrideProfileForMAC(mac); profile != nil {
		return
	}

	var prof *db.HostPXEProfile
	if prof, err = s.profileCache.ByMAC(mac); err == nil && prof != nil {
		profile = prof
		return
	}

	var host *db.Host
	if host, err = s.hostCache.ByMAC(mac); err != nil {
		return
	}

	profile = s.buildDefaultProfile(host, mac)
	return
}

// sendDHCP sends the DHCP response to the specified peer address.
func sendDHCP(conn net.PacketConn, peer net.Addr, resp *dhcpv4.DHCPv4) (err error) {
	if resp != nil {
		_, err = conn.WriteTo(resp.ToBytes(), peer)
	}

	return
}

// isPXEClient checks if the DHCP request is from a PXE client.
func isPXEClient(req *dhcpv4.DHCPv4) (isPXE bool) {
	if req == nil {
		isPXE = false
		return
	}

	var opt []byte = req.Options.Get(dhcpv4.OptionClassIdentifier)
	if len(opt) == 0 {
		isPXE = false
		return
	}

	isPXE = strings.Contains(strings.ToUpper(string(opt)), "PXECLIENT")
	return
}
