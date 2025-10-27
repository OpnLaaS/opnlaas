package hosts

type (
	VendorID       int
	FormFactor     int
	ManagementType int
	PowerState     int
	BootMode       int

	HostCPUSpecs struct {
		Sku          string  `json:"sku"`
		Count        int     `json:"count"`
		Cores        int     `json:"cores"`
		Threads      int     `json:"threads"`
	}

	HostMemorySpecs struct {
		NumDIMMs int `json:"num_dimms"`
		SizeGB   int `json:"size_gb"`
		SpeedMHz int `json:"speed_mhz"`
	}

	HostSpecs struct {
		Processor HostCPUSpecs    `json:"processor"`
		Memory    HostMemorySpecs `json:"memory"`
	}

	Host struct {
		ManagementIP        string         `gomysql:"management_ip,primary,unique"`
		Vendor              VendorID       `gomysql:"vendor"`
		FormFactor          FormFactor     `gomysql:"form_factor"`
		ManagementType      ManagementType `gomysql:"management_type"`
		Model               string         `gomysql:"model"`
		LastKnownPowerState PowerState     `gomysql:"last_known_power_state"`
		Specs               HostSpecs      `gomysql:"specs"`
		Management          *HostManagementClient
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

	FormFactorNames = map[FormFactor]string{
		FormFactorOther:       "Other",
		FormFactorRackmount:   "Rackmount",
		FormFactorTower:       "Tower",
		FormFactorBlade:       "Blade",
		FormFactorMicroserver: "Microserver",
	}

	ManagementTypeNames = map[ManagementType]string{
		ManagementTypeNotSupported: "Not Supported",
		ManagementTypeIPMI:         "IPMI",
		ManagementTypeRedfish:      "Redfish",
	}

	PowerStateNames = map[PowerState]string{
		PowerStateUnknown: "Unknown",
		PowerStateOn:      "On",
		PowerStateOff:     "Off",
	}

	BootModeNames = map[BootMode]string{
		BootModeUEFI:   "UEFI",
		BootModeLegacy: "Legacy",
	}
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
