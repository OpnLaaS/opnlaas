package pxe

import (
	"fmt"
	"strings"

	"github.com/opnlaas/opnlaas/db"
)

// ProfileOverride describes PXE configuration overrides that should be applied
// when matching MAC addresses request DHCP/TFTP assets.
type ProfileOverride struct {
	MACAddresses []string

	ISOName      string
	BootFilename string
	KernelParams []string
	InitrdParams []string
	TemplateData map[string]string

	IPv4Address string
	SubnetMask  string
	Gateway     string
	DNSServers  []string
	DomainName  string
	NextServer  string
}

// ApplyHostProfileOverride registers PXE overrides for the provided host. When
// MAC addresses are not explicitly included in the override payload, all known
// network interface MAC addresses from the host record are used. Overrides take
// effect immediately for DHCP and PXELinux requests without requiring a restart.
func ApplyHostProfileOverride(host *db.Host, override ProfileOverride) (err error) {
	if instance == nil {
		return fmt.Errorf("pxe service not initialized")
	}

	err = instance.applyHostProfileOverride(host, override)
	return
}

// ClearHostProfileOverride drops any registered overrides for the provided host.
func ClearHostProfileOverride(host *db.Host) (err error) {
	if instance == nil {
		return fmt.Errorf("pxe service not initialized")
	}

	instance.clearOverridesForHost(host)
	return
}

// ClearProfileOverridesByMAC removes overrides for the provided MAC addresses.
func ClearProfileOverridesByMAC(macs ...string) {
	if instance == nil || len(macs) == 0 {
		return
	}

	instance.clearOverridesForMACs(macs)
}

func (s *Service) applyHostProfileOverride(host *db.Host, override ProfileOverride) (err error) {
	if host == nil {
		return fmt.Errorf("pxe override host cannot be nil")
	}

	var macs []string = override.MACAddresses
	if len(macs) == 0 {
		for _, nic := range host.NetworkInterfaces {
			if strings.TrimSpace(nic.MACAddress) != "" {
				macs = append(macs, nic.MACAddress)
			}
		}
	}

	if len(macs) == 0 {
		return fmt.Errorf("host %s missing MAC addresses for PXE override", host.ManagementIP)
	}

	return s.applyProfileOverride(macs, host, override)
}

func (s *Service) applyProfileOverride(macs []string, host *db.Host, override ProfileOverride) (err error) {
	var normalized []string = normalizeMACList(macs)
	if len(normalized) == 0 {
		return fmt.Errorf("no valid MAC addresses provided for PXE override")
	}

	var base *db.HostPXEProfile = s.buildDefaultProfile(host, "")
	if base == nil {
		return fmt.Errorf("no default PXE profile available")
	}

	if err = override.validate(); err != nil {
		return
	}

	override.applyToProfile(base)

	s.overrideMu.Lock()
	defer s.overrideMu.Unlock()

	for _, mac := range normalized {
		var profile *db.HostPXEProfile = clonePXEProfile(base)
		profile.BootMACAddress = mac
		s.overrideProfiles[mac] = profile
	}

	var hostID string
	if host != nil && host.ManagementIP != "" {
		hostID = host.ManagementIP
	}

	s.log.Basicf("PXE override registered host=%s macs=%v iso=%s\n", hostID, normalized, base.ISOName)
	return
}

func (s *Service) clearOverridesForHost(host *db.Host) {
	if host == nil {
		return
	}

	var macs []string
	for _, nic := range host.NetworkInterfaces {
		if strings.TrimSpace(nic.MACAddress) != "" {
			macs = append(macs, nic.MACAddress)
		}
	}

	s.clearOverridesForMACs(macs)
}

func (s *Service) clearOverridesForMACs(macs []string) {
	var normalized []string = normalizeMACList(macs)
	if len(normalized) == 0 {
		return
	}

	s.overrideMu.Lock()
	defer s.overrideMu.Unlock()
	for _, mac := range normalized {
		delete(s.overrideProfiles, mac)
	}
}

func (s *Service) overrideProfileForMAC(mac string) (profile *db.HostPXEProfile) {
	var norm, err = normalizeMAC(mac)
	if err != nil || norm == "" {
		return
	}

	s.overrideMu.RLock()
	defer s.overrideMu.RUnlock()

	if base, ok := s.overrideProfiles[norm]; ok && base != nil {
		profile = clonePXEProfile(base)
	}

	return
}

func (s *Service) overrideProfileForHost(host *db.Host) (profile *db.HostPXEProfile) {
	if host == nil {
		return
	}

	for _, nic := range host.NetworkInterfaces {
		if strings.TrimSpace(nic.MACAddress) == "" {
			continue
		}

		if profile = s.overrideProfileForMAC(nic.MACAddress); profile != nil {
			return
		}
	}

	return
}

func (o ProfileOverride) validate() (err error) {
	if strings.TrimSpace(o.ISOName) == "" {
		return
	}

	var iso *db.StoredISOImage
	if iso, err = db.StoredISOImages.Select(o.ISOName); err != nil {
		err = fmt.Errorf("lookup ISO %s: %w", o.ISOName, err)
		return
	} else if iso == nil {
		err = fmt.Errorf("iso %s not found", o.ISOName)
		return
	}

	return
}

func (o ProfileOverride) applyToProfile(profile *db.HostPXEProfile) {
	if profile == nil {
		return
	}

	if strings.TrimSpace(o.ISOName) != "" {
		profile.ISOName = o.ISOName
	}

	if strings.TrimSpace(o.BootFilename) != "" {
		profile.BootFilename = o.BootFilename
	}

	if len(o.KernelParams) > 0 {
		profile.KernelParams = cloneStringSlice(o.KernelParams)
	}

	if len(o.InitrdParams) > 0 {
		profile.InitrdParams = cloneStringSlice(o.InitrdParams)
	}

	if o.TemplateData != nil {
		profile.TemplateData = cloneMap(o.TemplateData)
	}

	if strings.TrimSpace(o.IPv4Address) != "" {
		profile.IPv4Address = o.IPv4Address
	}

	if strings.TrimSpace(o.SubnetMask) != "" {
		profile.SubnetMask = o.SubnetMask
	}

	if strings.TrimSpace(o.Gateway) != "" {
		profile.Gateway = o.Gateway
	}

	if len(o.DNSServers) > 0 {
		profile.DNSServers = cloneStringSlice(o.DNSServers)
	}

	if strings.TrimSpace(o.DomainName) != "" {
		profile.DomainName = o.DomainName
	}

	if strings.TrimSpace(o.NextServer) != "" {
		profile.NextServer = o.NextServer
	}
}

func normalizeMACList(values []string) (normalized []string) {
	if len(values) == 0 {
		return
	}

	var seen map[string]struct{} = make(map[string]struct{})
	for _, raw := range values {
		var norm, err = normalizeMAC(raw)
		if err != nil || norm == "" {
			continue
		}

		if _, ok := seen[norm]; ok {
			continue
		}

		seen[norm] = struct{}{}
		normalized = append(normalized, norm)
	}

	return
}
