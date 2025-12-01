package db

import (
	"fmt"
	"net"
	"strings"
)

// normalizeMACAddress standardizes MAC strings to lowercase colon-separated format.
func normalizeMACAddress(mac string) (string, error) {
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

// SavePXEProfile inserts or updates the PXE profile for a host.
func SavePXEProfile(profile *HostPXEProfile) error {
	if profile == nil {
		return fmt.Errorf("pxe profile cannot be nil")
	}

	normalized, err := normalizeMACAddress(profile.BootMACAddress)
	if err != nil {
		return fmt.Errorf("invalid boot mac address: %w", err)
	}

	profile.BootMACAddress = normalized
	return HostPXEProfiles.Insert(profile)
}

// DeletePXEProfile removes a PXE profile for the given management IP.
func DeletePXEProfile(managementIP string) error {
	if len(managementIP) == 0 {
		return fmt.Errorf("management ip is required")
	}

	return HostPXEProfiles.Delete(managementIP)
}

// PXEProfileByIP fetches the PXE profile bound to the management IP.
func PXEProfileByIP(managementIP string) (*HostPXEProfile, error) {
	if len(managementIP) == 0 {
		return nil, fmt.Errorf("management ip is required")
	}

	return HostPXEProfiles.Select(managementIP)
}

// PXEProfilesAll returns the full set of PXE profiles.
func PXEProfilesAll() ([]*HostPXEProfile, error) {
	return HostPXEProfiles.SelectAll()
}
