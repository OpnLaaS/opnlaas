package db

import (
	"context"
	"fmt"

	"github.com/bougou/go-ipmi"
	"github.com/opnlaas/opnlaas/config"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
)

var (
	bg                   context.Context = context.Background()
	ErrBadManagementType                 = fmt.Errorf("bad management type for host/function")
	ErrNotConnected                      = fmt.Errorf("not connected to host management interface")
	ErrNoChassisFound                    = fmt.Errorf("no chassis found for host")
	ErrInvalidState                      = fmt.Errorf("invalid state input for host/function")
	ErrNoSystemFound                     = fmt.Errorf("no system found for host")
)

func NewHostManagementClient(host *Host) (client *HostManagementClient, err error) {
	client = &HostManagementClient{
		Host: host,
	}

	switch host.ManagementType {
	case ManagementTypeRedfish:
		if err = client.redfishInit(); err != nil {
			return
		}
	case ManagementTypeIPMI:
		if err = client.ipmiInit(); err != nil {
			return
		}
	default:
		err = ErrBadManagementType
		return
	}

	client.connected = true
	return
}

func (c *HostManagementClient) redfishInit() (err error) {
	if c.redfishClient, err = gofish.Connect(gofish.ClientConfig{
		Endpoint: "https://" + c.Host.ManagementIP,
		Username: config.Config.Management.DefaultIPMIUser,
		Password: config.Config.Management.DefaultIPMIPass,
		Insecure: true,
	}); err != nil {
		return
	}

	c.redfishService = c.redfishClient.Service

	var chassisList []*redfish.Chassis
	if chassisList, err = c.redfishService.Chassis(); err != nil {
		return
	}

	if len(chassisList) == 0 {
		err = ErrNoChassisFound
		return
	}

	c.redfishPrimaryChassis = chassisList[0]

	var systemList []*redfish.ComputerSystem
	if systemList, err = c.redfishService.Systems(); err != nil {
		return
	}

	if len(systemList) == 0 {
		err = ErrNoSystemFound
		return
	}

	c.redfishPrimarySystem = systemList[0]
	return
}

func (c *HostManagementClient) ipmiInit() (err error) {
	if c.ipmiClient, err = ipmi.NewClient(c.Host.ManagementIP, 623, config.Config.Management.DefaultIPMIUser, config.Config.Management.DefaultIPMIPass); err != nil {
		return
	}

	err = c.ipmiClient.Connect(context.Background())
	return
}

func (c *HostManagementClient) Close() {
	if c.redfishClient != nil {
		c.redfishClient.Logout()
	}

	if c.ipmiClient != nil {
		c.ipmiClient.Close(bg)
	}

	c.connected = false
}

// ---------- POWER MANAGEMENT ----------

func (c *HostManagementClient) redfishPowerState() PowerState {
	switch c.redfishPrimaryChassis.PowerState {
	case redfish.OnPowerState:
		return PowerStateOn
	case redfish.OffPowerState:
		return PowerStateOff
	default:
		return PowerStateUnknown
	}
}

func (c *HostManagementClient) ipmiPowerState() (state PowerState, err error) {
	var resp *ipmi.GetChassisStatusResponse
	if resp, err = c.ipmiClient.GetChassisStatus(bg); err != nil {
		return
	}

	if resp.PowerIsOn {
		state = PowerStateOn
	} else {
		state = PowerStateOff
	}

	return
}

func (c *HostManagementClient) PowerState() (state PowerState, err error) {
	if !c.connected {
		err = ErrNotConnected
		return
	}

	switch c.Host.ManagementType {
	case ManagementTypeRedfish:
		state = c.redfishPowerState()
	case ManagementTypeIPMI:
		state, err = c.ipmiPowerState()
	default:
		err = ErrBadManagementType
	}

	c.Host.LastKnownPowerState = state
	return
}

func (c *HostManagementClient) redfishSetPowerState(desiredState PowerState, force bool) (err error) {
	var action redfish.ResetType
	switch desiredState {
	case PowerStateOn:
		action = redfish.OnResetType
		if force {
			action = redfish.ForceOnResetType
		}
	case PowerStateOff:
		action = redfish.GracefulShutdownResetType
		if force {
			action = redfish.ForceOffResetType
		}
	default:
		err = ErrInvalidState
		return
	}

	err = c.redfishPrimaryChassis.Reset(action)
	return
}

func (c *HostManagementClient) ipmiSetPowerState(desiredState PowerState, force bool) (err error) {
	var action ipmi.ChassisControl

	switch desiredState {
	case PowerStateOn:
		action = ipmi.ChassisControlPowerUp
	case PowerStateOff:
		action = ipmi.ChassisControlSoftShutdown
		if force {
			action = ipmi.ChassisControlPowerDown
		}
	default:
		err = ErrInvalidState
		return
	}

	_, err = c.ipmiClient.ChassisControl(bg, action)
	return
}

func (c *HostManagementClient) SetPowerState(desiredState PowerState, force bool) (err error) {
	if !c.connected {
		err = ErrNotConnected
		return
	}

	switch c.Host.ManagementType {
	case ManagementTypeRedfish:
		err = c.redfishSetPowerState(desiredState, force)
	case ManagementTypeIPMI:
		err = c.ipmiSetPowerState(desiredState, force)
	default:
		err = ErrBadManagementType
	}

	return
}

func (c *HostManagementClient) redfishResetPowerState(force bool) (err error) {
	var action redfish.ResetType
	if force {
		action = redfish.ForceRestartResetType
	} else {
		action = redfish.PowerCycleResetType
	}

	err = c.redfishPrimaryChassis.Reset(action)
	return
}

func (c *HostManagementClient) ipmiResetPowerState(force bool) (err error) {
	var action ipmi.ChassisControl
	if force {
		action = ipmi.ChassisControlHardReset
	} else {
		action = ipmi.ChassisControlPowerCycle
	}

	_, err = c.ipmiClient.ChassisControl(bg, action)
	return
}

func (c *HostManagementClient) ResetPowerState(force bool) (err error) {
	if !c.connected {
		err = ErrNotConnected
		return
	}

	switch c.Host.ManagementType {
	case ManagementTypeRedfish:
		err = c.redfishResetPowerState(force)
	case ManagementTypeIPMI:
		err = c.ipmiResetPowerState(force)
	default:
		err = ErrBadManagementType
	}

	return
}

// ---------- BOOT MANAGEMENT ----------

func (c *HostManagementClient) redfishSetPXEBoot(bootMode BootMode) (err error) {
	var bootType redfish.BootSourceOverrideMode
	switch bootMode {
	case BootModeUEFI:
		bootType = redfish.UEFIBootSourceOverrideMode
	case BootModeLegacy:
		bootType = redfish.LegacyBootSourceOverrideMode
	default:
		err = ErrInvalidState
		return
	}

	err = c.redfishPrimarySystem.SetBoot(redfish.Boot{
		BootSourceOverrideTarget:  redfish.PxeBootSourceOverrideTarget,
		BootSourceOverrideEnabled: redfish.OnceBootSourceOverrideEnabled,
		BootSourceOverrideMode:    bootType,
	})

	return
}

func (c *HostManagementClient) ipmiSetPXEBoot(bootMode BootMode) (err error) {
	var bootType ipmi.BIOSBootType

	switch bootMode {
	case BootModeUEFI:
		bootType = ipmi.BIOSBootTypeEFI
	case BootModeLegacy:
		bootType = ipmi.BIOSBootTypeLegacy
	default:
		err = ErrInvalidState
		return
	}

	err = c.ipmiClient.SetBootDevice(bg, ipmi.BootDeviceSelectorForcePXE, bootType, false)
	return
}

func (c *HostManagementClient) SetPXEBoot(bootMode BootMode) (err error) {
	if !c.connected {
		err = ErrNotConnected
		return
	}

	switch c.Host.ManagementType {
	case ManagementTypeRedfish:
		err = c.redfishSetPXEBoot(bootMode)
	case ManagementTypeIPMI:
		err = c.ipmiSetPXEBoot(bootMode)
	default:
		err = ErrBadManagementType
	}

	return
}

// ---------- DATA COLLECTION ----------

func (c *HostManagementClient) redfishUpdateSystemInfo() (err error) {
	c.Host.Specs.Processor = HostCPUSpecs{
		Sku:     c.redfishPrimarySystem.ProcessorSummary.Model,
		Count:   c.redfishPrimarySystem.ProcessorSummary.Count,
		Cores:   c.redfishPrimarySystem.ProcessorSummary.LogicalProcessorCount / c.redfishPrimarySystem.ProcessorSummary.Count,
		Threads: c.redfishPrimarySystem.ProcessorSummary.LogicalProcessorCount,
	}

	c.Host.Specs.Memory = HostMemorySpecs{
		SizeGB: int(c.redfishPrimarySystem.MemorySummary.TotalSystemMemoryGiB),
	}

	c.Host.Model = c.redfishPrimarySystem.Model

	services, _ := c.redfishPrimarySystem.Storage()

	for _, service := range services {
		volumes, _ := service.Volumes()
		for _, volume := range volumes {
			c.Host.Specs.Storage = append(c.Host.Specs.Storage, HostStorageSpecs{
				CapacityGB: int(volume.CapacityBytes / (1024 * 1024 * 1024)),
				MediaType:  string(volume.VolumeType),
			})
		}
	}

	return
}

func (c *HostManagementClient) UpdateSystemInfo() (err error) {
	if !c.connected {
		err = ErrNotConnected
		return
	}

	switch c.Host.ManagementType {
	case ManagementTypeRedfish:
		err = c.redfishUpdateSystemInfo()
	default:
		err = ErrBadManagementType
	}

	return
}
