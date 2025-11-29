package pxe

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/opnlaas/opnlaas/db"
)

func (s *Service) handleDHCP(conn net.PacketConn, peer net.Addr, req *dhcpv4.DHCPv4) error {
	if req == nil {
		return fmt.Errorf("nil request")
	}

	mac := req.ClientHWAddr.String()
	if !isPXEClient(req) {
		return nil
	}
	msgType := req.MessageType()
	s.log.Basicf("DHCP request type=%s mac=%s xid=%d\n", msgType, mac, req.TransactionID)

	var reply *dhcpv4.DHCPv4
	var err error

	switch msgType {
	case dhcpv4.MessageTypeDiscover:
		reply, err = s.buildDHCPOffer(req)
	case dhcpv4.MessageTypeRequest:
		if s.proxyDHCP {
			return nil
		}
		reply, err = s.buildDHCPAck(req)
	default:
		return nil
	}
	if err != nil || reply == nil {
		return err
	}

	return sendDHCP(conn, peer, reply)
}

func (s *Service) buildDHCPOffer(req *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, error) {
	mac := req.ClientHWAddr.String()
	profile, err := s.profileForMAC(mac)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, fmt.Errorf("no PXE profile available for %s", mac)
	}
	resp, err := dhcpv4.NewReplyFromRequest(req)
	if err != nil {
		return nil, err
	}
	ip, err := s.leaseIPForProfile(mac, profile)
	if err != nil {
		return nil, err
	}
	resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeOffer))
	if ip != nil {
		resp.YourIPAddr = ip
	}
	s.decorateReply(resp, profile)
	return resp, nil
}

func (s *Service) buildDHCPAck(req *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, error) {
	if s.proxyDHCP {
		return nil, nil
	}
	mac := req.ClientHWAddr.String()
	profile, err := s.profileForMAC(mac)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, fmt.Errorf("no PXE profile available for %s", mac)
	}
	resp, err := dhcpv4.NewReplyFromRequest(req)
	if err != nil {
		return nil, err
	}
	ip, err := s.leaseIPForProfile(mac, profile)
	if err != nil {
		return nil, err
	}
	resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))
	if ip != nil {
		resp.YourIPAddr = ip
	}
	s.decorateReply(resp, profile)
	if ip != nil {
		s.leases.Set(mac, ip)
	}
	return resp, nil
}

func (s *Service) leaseIPForProfile(mac string, profile *db.HostPXEProfile) (net.IP, error) {
	if s.proxyDHCP {
		return nil, nil
	}
	if profile == nil {
		return nil, fmt.Errorf("nil profile")
	}
	if ip := parseIPv4(profile.IPv4Address); ip != nil {
		if err := s.ensureIPWithinRange(ip); err != nil {
			return nil, err
		}
		return ip, nil
	}
	if mac != "" {
		if leased := s.leases.Get(mac); leased != nil {
			if err := s.ensureIPWithinRange(leased); err != nil {
				return nil, err
			}
			return leased, nil
		}
	}

	ip, err := s.allocateDynamicIP(mac)
	if err != nil {
		return nil, err
	}
	return ip, nil
}

func (s *Service) ensureIPWithinRange(ip net.IP) error {
	if ip == nil {
		return fmt.Errorf("nil ip address")
	}
	if s.ipRangeStart == nil || s.ipRangeEnd == nil {
		return nil
	}
	if compareIPv4(ip, s.ipRangeStart) < 0 || compareIPv4(ip, s.ipRangeEnd) > 0 {
		return fmt.Errorf("ip %s outside allowed DHCP range %s-%s", ip.String(), s.ipRangeStart.String(), s.ipRangeEnd.String())
	}
	return nil
}

func (s *Service) allocateDynamicIP(mac string) (net.IP, error) {
	if s.ipRangeStart == nil || s.ipRangeEnd == nil {
		return nil, fmt.Errorf("no DHCP IP range configured")
	}

	s.leaseMu.Lock()
	defer s.leaseMu.Unlock()

	if s.leaseCursor == nil {
		s.leaseCursor = cloneIPv4(s.ipRangeStart)
	}

	start := cloneIPv4(s.leaseCursor)
	for {
		candidate := cloneIPv4(s.leaseCursor)
		s.advanceLeaseCursor()
		if !s.leases.InUse(candidate) {
			if mac != "" {
				s.leases.Set(mac, candidate)
			}
			return candidate, nil
		}
		if compareIPv4(s.leaseCursor, start) == 0 {
			break
		}
	}
	return nil, fmt.Errorf("no available DHCP addresses in range %s-%s", s.ipRangeStart, s.ipRangeEnd)
}

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

func (s *Service) decorateReply(resp *dhcpv4.DHCPv4, profile *db.HostPXEProfile) {
	if !s.proxyDHCP {
		if serverIP := s.serverIdentifierIP(); serverIP != nil {
			resp.Options.Update(dhcpv4.OptServerIdentifier(serverIP))
		}
	}
	if !s.proxyDHCP {
		subnetMask := profile.SubnetMask
		if strings.TrimSpace(subnetMask) == "" {
			subnetMask = s.cfg.PXE.DHCPServer.SubnetMask
		}
		if mask := parseMask(subnetMask); mask != nil {
			resp.Options.Update(dhcpv4.Option{Code: dhcpv4.OptionSubnetMask, Value: dhcpv4.IPMask(mask)})
		}
		routerIP := profile.Gateway
		if strings.TrimSpace(routerIP) == "" {
			routerIP = s.cfg.PXE.DHCPServer.Router
		}
		if router := parseIP(routerIP); router != nil {
			resp.Options.Update(dhcpv4.Option{Code: dhcpv4.OptionRouter, Value: dhcpv4.IP(router)})
		}
		dnsEntries := profile.DNSServers
		if len(dnsEntries) == 0 {
			dnsEntries = s.cfg.PXE.DHCPServer.DNSServers
		}
		var dns []net.IP
		for _, entry := range dnsEntries {
			if ip := parseIP(entry); ip != nil {
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
	if next := parseIP(profile.NextServer); next != nil {
		resp.ServerIPAddr = next
	} else if ip := parseIP(s.cfg.PXE.DHCPServer.ServerPublicAddress); ip != nil {
		resp.ServerIPAddr = ip
	}
	if !s.proxyDHCP {
		lease := time.Duration(s.cfg.PXE.DHCPServer.LeaseSeconds) * time.Second
		resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(lease))
	}
}

func (s *Service) profileForMAC(mac string) (*db.HostPXEProfile, error) {
	mac = strings.TrimSpace(mac)
	if mac == "" {
		return s.buildDefaultProfile(nil, ""), nil
	}
	if prof, err := s.profileCache.ByMAC(mac); err == nil && prof != nil {
		return prof, nil
	}
	host, err := s.hostCache.ByMAC(mac)
	if err != nil {
		return nil, err
	}
	return s.buildDefaultProfile(host, mac), nil
}

func sendDHCP(conn net.PacketConn, peer net.Addr, resp *dhcpv4.DHCPv4) error {
	if resp == nil {
		return nil
	}
	_, err := conn.WriteTo(resp.ToBytes(), peer)
	return err
}

func isPXEClient(req *dhcpv4.DHCPv4) bool {
	if req == nil {
		return false
	}
	opt := req.Options.Get(dhcpv4.OptionClassIdentifier)
	if len(opt) == 0 {
		return false
	}
	return strings.Contains(strings.ToUpper(string(opt)), "PXECLIENT")
}
