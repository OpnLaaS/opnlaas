package pxe

import (
	"net"
	"strings"

	"github.com/opnlaas/opnlaas/db"
)

// handlePXELinux processes a PXELinux configuration file request.
func (s *Service) handlePXELinux(filename string, req *tftpRequestContext) (data []byte, flag bool, err error) {
	var lower string = strings.ToLower(strings.TrimPrefix(filename, "/"))
	if !strings.HasPrefix(lower, "pxelinux.cfg") {
		flag = false
		return
	}

	var name string = strings.Trim(strings.TrimPrefix(lower, "pxelinux.cfg"), "/")
	if name == "" {
		name = "default"
	}

	var (
		host    *db.Host
		profile *db.HostPXEProfile
	)

	if host, profile, err = s.lookupProfileForPXELinux(name, req); err != nil {
		flag = true
		return
	} else if profile == nil {
		err = nil
		flag = true
		return
	}

	var iso *db.StoredISOImage
	if iso, err = db.StoredISOImages.Select(profile.ISOName); err != nil {
		flag = true
		return
	} else if iso == nil {
		err = nil
		flag = true
		return
	}

	var ctx *TemplateContext = s.buildTemplateContext(host, profile, iso)
	if data, err = s.renderTemplate(templateKeyPXELinux, ctx); err == nil {
		s.log.Basicf("PXE served pxelinux config=%s host=%s iso=%s\n", filename, ctx.Identifiers.Slug, iso.Name)
		s.log.Basicf("PXE config for %s:\n%s\n", ctx.Identifiers.Slug, string(data))
	}

	flag = true
	return
}

// lookupProfileForPXELinux finds the host and PXE profile based on the given PXELinux filename.
func (s *Service) lookupProfileForPXELinux(name string, req *tftpRequestContext) (host *db.Host, profile *db.HostPXEProfile, err error) {
	name = strings.ToLower(name)

	var bootMAC string

	if strings.HasPrefix(name, "01-") && len(name) > 3 {
		var mac string = strings.ReplaceAll(name[3:], "-", ":")
		bootMAC = mac

		if profile = s.overrideProfileForMAC(mac); profile != nil {
			if host, err = s.hostCache.ByMAC(mac); err != nil {
				return
			}

			return
		}

		var candidate *db.Host
		if candidate, err = s.hostCache.ByMAC(mac); err != nil {
			return
		}

		if candidate != nil {
			host = candidate
			if profile, err = s.profileCache.ByIP(candidate.ManagementIP); err != nil {
				return
			}

			if profile == nil {
				profile = s.buildDefaultProfile(candidate, mac)
			}
		}

		if profile != nil {
			return
		}
	}

	var (
		ip string
		ok bool
	)

	if profile == nil {
		if ip, ok = parsePXELinuxHexIP(name); ok {
			if host == nil {
				if host, err = s.hostCache.ByIP(ip); err != nil {
					return
				}
			}

			if host != nil {
				if profile = s.overrideProfileForHost(host); profile == nil {
					if profile, err = s.profileCache.ByIP(host.ManagementIP); err != nil {
						return
					}
				}

				if profile == nil {
					profile = s.buildDefaultProfile(host, "")
				}
			}

			if profile != nil {
				return
			}
		}
	}

	if profile == nil && req != nil && req.clientMAC != "" {
		if profile, err = s.profileForMAC(req.clientMAC); err != nil {
			return
		}

		if profile != nil {
			if host == nil {
				if host, err = s.hostCache.ByMAC(req.clientMAC); err != nil {
					return
				}
			}

			return
		}
	}

	if profile == nil && name == "default" && req != nil && req.remoteAddr != nil {
		var hostIP string
		if hostIP = req.remoteAddr.IP.String(); hostIP != "" {
			if host == nil {
				if host, err = s.hostCache.ByIP(hostIP); err != nil {
					return
				}
			}

			if host != nil {
				if profile = s.overrideProfileForHost(host); profile == nil {
					if profile, err = s.profileCache.ByIP(host.ManagementIP); err != nil {
						return
					}
				}

				if profile == nil {
					profile = s.buildDefaultProfile(host, "")
				}
			}
		}
	}

	if profile == nil && host != nil {
		profile = s.buildDefaultProfile(host, "")
	}

	if profile == nil {
		profile = s.buildDefaultProfile(host, bootMAC)
	}

	return
}

// tftpRequestContext holds context information about a TFTP request.
type tftpRequestContext struct {
	remoteAddr *net.UDPAddr
	filename   string
	clientMAC  string
}
