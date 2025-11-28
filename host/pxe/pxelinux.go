package pxe

import (
	"fmt"
	"net"
	"strings"

	"github.com/opnlaas/opnlaas/db"
)

func (s *Service) handlePXELinux(filename string, req *tftpRequestContext) ([]byte, bool, error) {
	lower := strings.ToLower(strings.TrimPrefix(filename, "/"))
	if !strings.HasPrefix(lower, "pxelinux.cfg") {
		return nil, false, nil
	}

	name := strings.Trim(strings.TrimPrefix(lower, "pxelinux.cfg"), "/")
	if name == "" {
		name = "default"
	}

	host, profile, err := s.lookupProfileForPXELinux(name, req)
	if err != nil {
		return nil, true, err
	}
	if profile == nil {
		return nil, true, fmt.Errorf("no PXE profile available for %s", filename)
	}

	iso, err := db.StoredISOImages.Select(profile.ISOName)
	if err != nil {
		return nil, true, err
	}
	if iso == nil {
		return nil, true, fmt.Errorf("iso %s not found", profile.ISOName)
	}

	ctx := s.buildTemplateContext(host, profile, iso)
	data, err := s.renderTemplate(templateKeyPXELinux, ctx)
	if err == nil {
		s.log.Basicf("PXE served pxelinux config=%s host=%s iso=%s\n", filename, ctx.Identifiers.Slug, iso.Name)
	}
	return data, true, err
}

func (s *Service) lookupProfileForPXELinux(name string, req *tftpRequestContext) (*db.Host, *db.HostPXEProfile, error) {
	name = strings.ToLower(name)
	var (
		host    *db.Host
		profile *db.HostPXEProfile
		err     error
	)

	if strings.HasPrefix(name, "01-") && len(name) > 3 {
		mac := strings.ReplaceAll(name[3:], "-", ":")
		host, err = s.hostCache.ByMAC(mac)
		if err != nil {
			return nil, nil, err
		}
		if host != nil {
			profile, err = s.profileCache.ByIP(host.ManagementIP)
			if err != nil {
				return nil, nil, err
			}
		}
		if profile == nil {
			profile = s.buildDefaultProfile(host, mac)
		}
		return host, profile, nil
	}

	if ip, ok := parsePXELinuxHexIP(name); ok {
		host, err = s.hostCache.ByIP(ip)
		if err != nil {
			return nil, nil, err
		}
		if host != nil {
			profile, err = s.profileCache.ByIP(host.ManagementIP)
			if err != nil {
				return nil, nil, err
			}
			if profile == nil {
				profile = s.buildDefaultProfile(host, "")
			}
		}
		return host, profile, nil
	}

	if name == "default" && req != nil && req.remoteAddr != nil {
		if hostIP := req.remoteAddr.IP.String(); hostIP != "" {
			host, err = s.hostCache.ByIP(hostIP)
			if err != nil {
				return nil, nil, err
			}
			if host != nil {
				profile, err = s.profileCache.ByIP(host.ManagementIP)
				if err != nil {
					return nil, nil, err
				}
				if profile == nil {
					profile = s.buildDefaultProfile(host, "")
				}
			}
		}
	}

	if profile == nil {
		profile = s.buildDefaultProfile(host, "")
	}
	return host, profile, nil
}

type tftpRequestContext struct {
	remoteAddr *net.UDPAddr
	filename   string
}
