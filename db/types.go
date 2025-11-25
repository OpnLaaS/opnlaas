package db

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bougou/go-ipmi"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
)

type (
	VendorID               int
	FormFactor             int
	ManagementType         int
	PowerState             int
	BootMode               int
	PowerAction            int
	Architecture           string
	DistroType             int
	PreConfigureType       int
	BookingPermissionLevel int
	BookingStatus          int

	HostCPUSpecs struct {
		Manufacturer string `json:"manufacturer"`
		Sku          string `json:"sku"`
		Count        int    `json:"count"`
		Cores        int    `json:"cores"`
		Threads      int    `json:"threads"`
		BaseSpeedMHz int    `json:"base_speed_mhz"`
		MaxSpeedMHz  int    `json:"max_speed_mhz"`
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

	HostManagementClient struct {
		Host      *Host
		connected bool

		// Redfish stuff
		redfishClient         *gofish.APIClient
		redfishService        *gofish.Service
		redfishPrimaryChassis *redfish.Chassis
		redfishPrimarySystem  *redfish.ComputerSystem

		// IPMI stuff
		ipmiClient *ipmi.Client
	}

	Host struct {
		ManagementIP            string                `gomysql:"management_ip,primary,unique" json:"management_ip"`
		Vendor                  VendorID              `gomysql:"vendor" json:"vendor"`
		FormFactor              FormFactor            `gomysql:"form_factor" json:"form_factor"`
		ManagementType          ManagementType        `gomysql:"management_type" json:"management_type"`
		Model                   string                `gomysql:"model" json:"model"`
		LastKnownPowerState     PowerState            `gomysql:"last_known_power_state" json:"last_known_power_state"`
		LastKnownPowerStateTime time.Time             `gomysql:"last_known_power_state_time" json:"last_known_power_state_time"`
		Specs                   HostSpecs             `gomysql:"specs" json:"specs"`
		IsBooked                bool                  `gomysql:"is_booked" json:"is_booked"`
		ActiveBookingID         int                   `gomysql:"active_booking_id" json:"active_booking_id"`
		Management              *HostManagementClient `json:"-"`
	}

	StoredISOImage struct {
		Name         string           `gomysql:"name,primary,unique" json:"name"`
		DistroName   string           `gomysql:"distro_name" json:"distro_name"`
		Version      string           `gomysql:"version" json:"version"`
		Size         int64            `gomysql:"size" json:"size"`
		FullISOPath  string           `gomysql:"full_iso_path" json:"full_iso_path"`
		KernelPath   string           `gomysql:"kernel_path" json:"kernel_path"`
		InitrdPath   string           `gomysql:"initrd_path" json:"initrd_path"`
		Architecture Architecture     `gomysql:"architecture" json:"architecture"`
		DistroType   DistroType       `gomysql:"distro_type" json:"distro_type"`
		PreConfigure PreConfigureType `gomysql:"preconfigure_type" json:"preconfigure_type"`
	}

	BookingPerson struct {
		Username        string                 `json:"username"`
		PermissionLevel BookingPermissionLevel `json:"permission_level"`
	}

	BookingContainer struct {
		ProxmoxID           int        `gomysql:"proxmox_id,primary,unique" json:"proxmox_id"`
		BookingID           int        `gomysql:"booking_id" json:"booking_id"`
		Template            string     `gomysql:"template" json:"template"`
		Cores               int        `gomysql:"cores" json:"cores"`
		MemoryMB            int        `gomysql:"memory_mb" json:"memory_mb"`
		DiskGB              int        `gomysql:"disk_gb" json:"disk_gb"`
		NetworkAddresses    []string   `gomysql:"network_addresses,json" json:"network_addresses"`
		LastKnownPowerState PowerState `gomysql:"last_known_power_state" json:"last_known_power_state"`
		LastKnownPowerTime  time.Time  `gomysql:"last_known_power_time" json:"last_known_power_time"`
	}

	BookingVM struct {
		ProxmoxID       int    `gomysql:"proxmox_id,primary,unique" json:"proxmox_id"`
		BookingID       int    `gomysql:"booking_id" json:"booking_id"`
		OperatingSystem string `gomysql:"operating_system" json:"operating_system"`
		Cores           int    `gomysql:"cores" json:"cores"`
		MemoryMB        int    `gomysql:"memory_mb" json:"memory_mb"`
		DiskGB          int    `gomysql:"disk_gb" json:"disk_gb"`
	}

	Booking struct {
		ID                     int             `gomysql:"id,primary,auto_increment" json:"id"`
		Name                   string          `gomysql:"name" json:"name"`
		Description            string          `gomysql:"description" json:"description"`
		Status                 BookingStatus   `gomysql:"status" json:"status"`
		StartTime              time.Time       `gomysql:"start_time" json:"start_time"`
		EndTime                time.Time       `gomysql:"end_time" json:"end_time"`
		People                 []BookingPerson `gomysql:"people,json" json:"people"`
		OwnedHostManagementIPs []string        `gomysql:"owned_host_management_ips,json" json:"owned_host_management_ips"`
		OwnedBookingCTIDs      []int           `gomysql:"owned_booking_ctids,json" json:"owned_booking_ctids"`
		OwnedBookingVMIDs      []int           `gomysql:"owned_booking_vmids,json" json:"owned_booking_vmids"`
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
)

const (
	FormFactorOther FormFactor = iota
	FormFactorRackmount
	FormFactorTower
	FormFactorBlade
	FormFactorMicroserver
)

const (
	ManagementTypeNotSupported ManagementType = iota
	ManagementTypeIPMI
	ManagementTypeRedfish
)

const (
	PowerStateUnknown PowerState = iota
	PowerStateOn
	PowerStateOff
)

const (
	BootModeUEFI BootMode = iota
	BootModeLegacy
)

const (
	PowerActionPowerOn PowerAction = iota
	PowerActionPowerOff
	PowerActionGracefulShutdown
	PowerActionGracefulRestart
	PowerActionForceRestart
)

const (
	ArchitectureX86_64 Architecture = "x86_64"
	ArchitectureARM64  Architecture = "aarch64"
)

const (
	DistroTypeOther DistroType = iota
	DistroTypeDebianBased
	DistroTypeRedHatBased
	DistroTypeArchBased
	DistroTypeSUSEBased
	DistroTypeAlpineBased
	DistroTypeWindowsBased
)

const (
	PreConfigureTypeNone PreConfigureType = iota
	PreConfigureTypeCloudInit
	PreConfigureTypeKickstart
	PreConfigureTypePreseed
	PreConfigureTypeAutoYaST
	PreConfigureTypeArchInstallAuto
)

const (
	BookingPermissionLevelNone BookingPermissionLevel = iota
	BookingPermissionLevelUser
	BookingPermissionLevelOperator
	BookingPermissionLevelOwner
)

const (
	BookingStatusPendingApproval BookingStatus = iota
	BookingStatusApproved
	BookingStatusActive
	BookingStatusDone
	BookingStatusRejected
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

	PowerActionNames = map[PowerAction]string{
		PowerActionPowerOn:          "Power On",
		PowerActionPowerOff:         "Power Off",
		PowerActionGracefulShutdown: "Graceful Shutdown",
		PowerActionGracefulRestart:  "Graceful Restart",
		PowerActionForceRestart:     "Force Restart",
	}

	PowerActionNameReverses = map[string]PowerAction{}

	ArchitectureNames = map[Architecture]string{
		ArchitectureX86_64: "x86_64",
		ArchitectureARM64:  "arm64",
	}

	ArchitectureNameReverses = map[string]Architecture{}

	DistroTypeNames = map[DistroType]string{
		DistroTypeOther:        "Other",
		DistroTypeDebianBased:  "Debian-Based",
		DistroTypeRedHatBased:  "RedHat-Based",
		DistroTypeArchBased:    "Arch-Based",
		DistroTypeSUSEBased:    "SUSE-Based",
		DistroTypeAlpineBased:  "Alpine-Based",
		DistroTypeWindowsBased: "Windows-Based",
	}

	DistroTypeNameReverses = map[string]DistroType{}

	PreConfigureTypeNames = map[PreConfigureType]string{
		PreConfigureTypeNone:            "None",
		PreConfigureTypeCloudInit:       "Cloud-Init",
		PreConfigureTypeKickstart:       "Kickstart",
		PreConfigureTypePreseed:         "Preseed",
		PreConfigureTypeAutoYaST:        "AutoYaST",
		PreConfigureTypeArchInstallAuto: "Arch Install Auto",
	}

	PreConfigureTypeNameReverses = map[string]PreConfigureType{}

	BookingPermissionLevelNames = map[BookingPermissionLevel]string{
		BookingPermissionLevelNone:     "None",
		BookingPermissionLevelUser:     "User",
		BookingPermissionLevelOperator: "Operator",
		BookingPermissionLevelOwner:    "Owner",
	}

	BookingPermissionLevelNameReverses = map[string]BookingPermissionLevel{}

	BookingStatusNames = map[BookingStatus]string{
		BookingStatusPendingApproval: "Pending Approval",
		BookingStatusApproved:        "Approved",
		BookingStatusActive:          "Active",
		BookingStatusDone:            "Done",
		BookingStatusRejected:        "Rejected",
	}

	BookingStatusNameReverses = map[string]BookingStatus{}
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

func (p PowerAction) String() string {
	if name, exists := PowerActionNames[p]; exists {
		return name
	}

	return "Unknown Action"
}

func (a Architecture) String() string {
	if name, exists := ArchitectureNames[a]; exists {
		return name
	}

	return "Unknown Architecture"
}

func (d DistroType) String() string {
	if name, exists := DistroTypeNames[d]; exists {
		return name
	}

	return "Other"
}

func (p PreConfigureType) String() string {
	if name, exists := PreConfigureTypeNames[p]; exists {
		return name
	}

	return "None"
}

func (b BookingPermissionLevel) String() string {
	if name, exists := BookingPermissionLevelNames[b]; exists {
		return name
	}

	return "None"
}

func (b BookingStatus) String() string {
	if name, exists := BookingStatusNames[b]; exists {
		return name
	}

	return "Unknown Status"
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

	for k, v := range PowerActionNames {
		PowerActionNameReverses[v] = k
	}

	for k, v := range ArchitectureNames {
		ArchitectureNameReverses[v] = k
	}

	for k, v := range DistroTypeNames {
		DistroTypeNameReverses[v] = k
	}

	for k, v := range PreConfigureTypeNames {
		PreConfigureTypeNameReverses[v] = k
	}

	for k, v := range BookingPermissionLevelNames {
		BookingPermissionLevelNameReverses[v] = k
	}

	for k, v := range BookingStatusNames {
		BookingStatusNameReverses[v] = k
	}
}
