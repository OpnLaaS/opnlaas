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
	msgType := req.MessageType()
	s.log.Basicf("DHCP request type=%s mac=%s xid=%d\n", msgType, mac, req.TransactionID)

	var reply *dhcpv4.DHCPv4
	var err error

	switch msgType {
	case dhcpv4.MessageTypeDiscover:
		reply, err = s.buildDHCPOffer(req)
	case dhcpv4.MessageTypeRequest:
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
	profile, err := s.profileForMAC(req.ClientHWAddr.String())
	if err != nil || profile == nil {
		return nil, err
	}
	resp, err := dhcpv4.NewReplyFromRequest(req)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(profile.IPv4Address)
	if ip == nil {
		return nil, fmt.Errorf("profile %s missing IPv4", profile.ManagementIP)
	}
	resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeOffer))
	resp.YourIPAddr = ip
	s.decorateReply(resp, profile)
	return resp, nil
}

func (s *Service) buildDHCPAck(req *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, error) {
	profile, err := s.profileForMAC(req.ClientHWAddr.String())
	if err != nil || profile == nil {
		return nil, err
	}
	resp, err := dhcpv4.NewReplyFromRequest(req)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(profile.IPv4Address)
	if ip == nil {
		return nil, fmt.Errorf("profile %s missing IPv4", profile.ManagementIP)
	}
	resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))
	resp.YourIPAddr = ip
	s.decorateReply(resp, profile)
	s.leases.Set(req.ClientHWAddr.String(), ip)
	return resp, nil
}

func (s *Service) decorateReply(resp *dhcpv4.DHCPv4, profile *db.HostPXEProfile) {
	if serverIP := parseIP(s.cfg.TFTP.DHCP_ServerIP); serverIP != nil {
		resp.Options.Update(dhcpv4.OptServerIdentifier(serverIP))
	}
	if mask := parseMask(profile.SubnetMask); mask != nil {
		resp.Options.Update(dhcpv4.Option{Code: dhcpv4.OptionSubnetMask, Value: dhcpv4.IPMask(mask)})
	}
	if router := parseIP(profile.Gateway); router != nil {
		resp.Options.Update(dhcpv4.Option{Code: dhcpv4.OptionRouter, Value: dhcpv4.IP(router)})
	}
	if len(profile.DNSServers) > 0 {
		var dns []net.IP
		for _, entry := range profile.DNSServers {
			if ip := parseIP(entry); ip != nil {
				dns = append(dns, ip)
			}
		}
		if len(dns) > 0 {
			resp.Options.Update(dhcpv4.OptDNS(dns...))
		}
	}
	if profile.DomainName != "" {
		resp.Options.Update(dhcpv4.OptDomainName(profile.DomainName))
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
	} else if ip := parseIP(s.cfg.TFTP.DHCP_ServerIP); ip != nil {
		resp.ServerIPAddr = ip
	}
	lease := time.Duration(s.cfg.TFTP.DHCP_LeaseSeconds) * time.Second
	resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(lease))
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
