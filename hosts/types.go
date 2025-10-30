package hosts

import (
	"encoding/json"
	"fmt"
)

type (
	VendorID       int
	FormFactor     int
	ManagementType int
	PowerState     int
	BootMode       int

	HostCPUSpecs struct {
		Sku     string `json:"sku"`
		Count   int    `json:"count"`
		Cores   int    `json:"cores"`
		Threads int    `json:"threads"`
	}

	HostMemorySpecs struct {
		NumDIMMs int `json:"num_dimms"`
		SizeGB   int `json:"size_gb"`
		SpeedMHz int `json:"speed_mhz"`
	}

	HostStorageSpecs struct {
		CapacityGB int    `json:"capacity_gb"`
		MediaType  string `json:"media_type"`
	}

	HostSpecs struct {
		Processor HostCPUSpecs       `json:"processor"`
		Memory    HostMemorySpecs    `json:"memory"`
		Storage   []HostStorageSpecs `json:"storage"`
	}

	Host struct {
		ManagementIP        string                `gomysql:"management_ip,primary,unique" json:"management_ip"`
		Vendor              VendorID              `gomysql:"vendor" json:"vendor"`
		FormFactor          FormFactor            `gomysql:"form_factor" json:"form_factor"`
		ManagementType      ManagementType        `gomysql:"management_type" json:"management_type"`
		Model               string                `gomysql:"model" json:"model"`
		LastKnownPowerState PowerState            `gomysql:"last_known_power_state" json:"last_known_power_state"`
		Specs               HostSpecs             `gomysql:"specs" json:"specs"`
		Management          *HostManagementClient `json:"-"`
	}
)

const (
	VendorOther VendorID = iota
	VendorDELL
	VendorHPE
	VendorLenovo
	VendorCisco
	VendorSupermicro
	VendorGigabyte
	VendorAsus
	VendorIntel

	FormFactorOther FormFactor = iota
	FormFactorRackmount
	FormFactorTower
	FormFactorBlade
	FormFactorMicroserver

	ManagementTypeNotSupported ManagementType = iota
	ManagementTypeIPMI
	ManagementTypeRedfish

	PowerStateUnknown PowerState = iota
	PowerStateOn
	PowerStateOff

	BootModeUEFI BootMode = iota
	BootModeLegacy
)

var (
	VendorNames = map[VendorID]string{
		VendorOther:      "Other",
		VendorDELL:       "Dell",
		VendorHPE:        "HPE",
		VendorLenovo:     "Lenovo",
		VendorCisco:      "Cisco",
		VendorSupermicro: "Supermicro",
		VendorGigabyte:   "Gigabyte",
		VendorAsus:       "Asus",
		VendorIntel:      "Intel",
	}

	VendorNameReverses = map[string]VendorID{}

	FormFactorNames = map[FormFactor]string{
		FormFactorOther:       "Other",
		FormFactorRackmount:   "Rackmount",
		FormFactorTower:       "Tower",
		FormFactorBlade:       "Blade",
		FormFactorMicroserver: "Microserver",
	}

	FormFactorNameReverses = map[string]FormFactor{}

	ManagementTypeNames = map[ManagementType]string{
		ManagementTypeNotSupported: "Not Supported",
		ManagementTypeIPMI:         "IPMI",
		ManagementTypeRedfish:      "Redfish",
	}

	ManagementTypeNameReverses = map[string]ManagementType{}

	PowerStateNames = map[PowerState]string{
		PowerStateUnknown: "Unknown",
		PowerStateOn:      "On",
		PowerStateOff:     "Off",
	}

	PowerStateNameReverses = map[string]PowerState{}

	BootModeNames = map[BootMode]string{
		BootModeUEFI:   "UEFI",
		BootModeLegacy: "Legacy",
	}

	BootModeNameReverses = map[string]BootMode{}
)

func (v VendorID) String() string {
	if name, exists := VendorNames[v]; exists {
		return name
	}

	return "Other"
}

func (f FormFactor) String() string {
	if name, exists := FormFactorNames[f]; exists {
		return name
	}

	return "Other"
}

func (m ManagementType) String() string {
	if name, exists := ManagementTypeNames[m]; exists {
		return name
	}

	return "Not Supported"
}

func (p PowerState) String() string {
	if name, exists := PowerStateNames[p]; exists {
		return name
	}

	return "Unknown"
}

func (b BootMode) String() string {
	if name, exists := BootModeNames[b]; exists {
		return name
	}

	return "Legacy"
}

func (specs HostSpecs) String() string {
	var (
		specsBytes []byte
		err        error
	)

	if specsBytes, err = json.MarshalIndent(specs, "", "  "); err != nil {
		return fmt.Sprintf("{\"error\": \"failed to marshal specs: %v\"}", err)
	}

	return string(specsBytes)
}

func init() {
	for k, v := range VendorNames {
		VendorNameReverses[v] = k
	}

	for k, v := range FormFactorNames {
		FormFactorNameReverses[v] = k
	}

	for k, v := range ManagementTypeNames {
		ManagementTypeNameReverses[v] = k
	}

	for k, v := range PowerStateNames {
		PowerStateNameReverses[v] = k
	}

	for k, v := range BootModeNames {
		BootModeNameReverses[v] = k
	}
}
