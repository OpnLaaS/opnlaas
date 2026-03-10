package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bougou/go-ipmi"
	"github.com/opnlaas/opnlaas/config"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
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
		Username: config.Config.Management.Username,
		Password: config.Config.Management.Password,
		Insecure: true,
		// High handshake timeout
		TLSHandshakeTimeout: 60,
	}); err != nil {
		return
	}

	c.redfishService = c.redfishClient.Service

	var chassisList []*schemas.Chassis
	if chassisList, err = c.redfishService.Chassis(); err != nil {
		return
	}

	if len(chassisList) == 0 {
		err = ErrNoChassisFound
		return
	}

	c.redfishPrimaryChassis = chassisList[0]

	var systemList []*schemas.ComputerSystem
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
	if c.ipmiClient, err = ipmi.NewClient(c.Host.ManagementIP, 623, config.Config.Management.Username, config.Config.Management.Password); err != nil {
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

func (c *HostManagementClient) refreshRedfishPrimarySystem() (err error) {
	if c.redfishService == nil {
		return ErrNotConnected
	}

	var systems []*schemas.ComputerSystem
	if systems, err = c.redfishService.Systems(); err != nil {
		return
	}

	if len(systems) == 0 || systems[0] == nil {
		return ErrNoSystemFound
	}

	c.redfishPrimarySystem = systems[0]
	return
}

func (c *HostManagementClient) ensureRedfishPrimarySystem(forceRefresh bool) (err error) {
	if forceRefresh || c.redfishPrimarySystem == nil {
		return c.refreshRedfishPrimarySystem()
	}

	return nil
}

func numberAnyToInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint:
		return int(n)
	case uint8:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		return int(n)
	case uint64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	case *int:
		if n != nil {
			return *n
		}
	case *int8:
		if n != nil {
			return int(*n)
		}
	case *int16:
		if n != nil {
			return int(*n)
		}
	case *int32:
		if n != nil {
			return int(*n)
		}
	case *int64:
		if n != nil {
			return int(*n)
		}
	case *uint:
		if n != nil {
			return int(*n)
		}
	case *uint8:
		if n != nil {
			return int(*n)
		}
	case *uint16:
		if n != nil {
			return int(*n)
		}
	case *uint32:
		if n != nil {
			return int(*n)
		}
	case *uint64:
		if n != nil {
			return int(*n)
		}
	case *float32:
		if n != nil {
			return int(*n)
		}
	case *float64:
		if n != nil {
			return int(*n)
		}
	}

	return 0
}

func numberAnyToInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int8:
		return int64(n)
	case int16:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case uint:
		return int64(n)
	case uint8:
		return int64(n)
	case uint16:
		return int64(n)
	case uint32:
		return int64(n)
	case uint64:
		return int64(n)
	case float32:
		return int64(n)
	case float64:
		return int64(n)
	case *int:
		if n != nil {
			return int64(*n)
		}
	case *int8:
		if n != nil {
			return int64(*n)
		}
	case *int16:
		if n != nil {
			return int64(*n)
		}
	case *int32:
		if n != nil {
			return int64(*n)
		}
	case *int64:
		if n != nil {
			return *n
		}
	case *uint:
		if n != nil {
			return int64(*n)
		}
	case *uint8:
		if n != nil {
			return int64(*n)
		}
	case *uint16:
		if n != nil {
			return int64(*n)
		}
	case *uint32:
		if n != nil {
			return int64(*n)
		}
	case *uint64:
		if n != nil {
			return int64(*n)
		}
	case *float32:
		if n != nil {
			return int64(*n)
		}
	case *float64:
		if n != nil {
			return int64(*n)
		}
	}

	return 0
}

func firstNonEmpty(values ...string) (out string) {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}

	return
}

// ---------- POWER MANAGEMENT ----------

func (c *HostManagementClient) redfishPowerState(forcePoll bool) (state PowerState, err error) {
	if err = c.ensureRedfishPrimarySystem(forcePoll); err != nil {
		return
	}

	switch c.redfishPrimarySystem.PowerState {
	case schemas.OnPowerState:
		state = PowerStateOn
	case schemas.OffPowerState:
		state = PowerStateOff
	default:
		state = PowerStateUnknown
	}

	return
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

func (c *HostManagementClient) PowerState(forcePoll bool) (state PowerState, err error) {
	if !c.connected {
		err = ErrNotConnected
		return
	}

	switch c.Host.ManagementType {
	case ManagementTypeRedfish:
		state, err = c.redfishPowerState(forcePoll)
	case ManagementTypeIPMI:
		// IPMI calls are always live; forcePoll is a no-op.
		state, err = c.ipmiPowerState()
	default:
		err = ErrBadManagementType
	}

	c.Host.LastKnownPowerState = state
	c.Host.LastKnownPowerStateTime = time.Now()
	return
}

func (c *HostManagementClient) redfishSetPowerState(desiredState PowerState, force bool) (err error) {
	if err = c.ensureRedfishPrimarySystem(false); err != nil {
		return
	}

	var action schemas.ResetType
	switch desiredState {
	case PowerStateOn:
		action = schemas.OnResetType
	case PowerStateOff:
		action = schemas.GracefulShutdownResetType
		if force {
			action = schemas.ForceOffResetType
		}
	default:
		err = ErrInvalidState
		return
	}

	_, err = c.redfishPrimarySystem.Reset(action)
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
	var state PowerState
	if state, err = c.redfishPowerState(true); err != nil {
		return
	}

	if state == PowerStateOff {
		_, err = c.redfishPrimarySystem.Reset(schemas.OnResetType)
		return
	}

	if force {
		if _, err = c.redfishPrimarySystem.Reset(schemas.ForceRestartResetType); err == nil {
			return nil
		}

		_, err = c.redfishPrimarySystem.Reset(schemas.PowerCycleResetType)
		return
	}

	if _, err = c.redfishPrimarySystem.Reset(schemas.GracefulRestartResetType); err == nil {
		return nil
	}

	_, err = c.redfishPrimarySystem.Reset(schemas.PowerCycleResetType)
	return
}

func (c *HostManagementClient) ipmiResetPowerState(force bool) (err error) {
	var action ipmi.ChassisControl
	var state PowerState
	if state, err = c.ipmiPowerState(); err != nil {
		return
	}

	if state == PowerStateOff {
		action = ipmi.ChassisControlPowerUp
	} else if force {
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
	if err = c.ensureRedfishPrimarySystem(false); err != nil {
		return
	}

	var bootType schemas.BootSourceOverrideMode
	switch bootMode {
	case BootModeUEFI:
		bootType = schemas.UEFIBootSourceOverrideMode
	case BootModeLegacy:
		bootType = schemas.LegacyBootSourceOverrideMode
	default:
		err = ErrInvalidState
		return
	}

	// Best-effort clear of any stale/persistent override from earlier runs.
	_ = c.redfishPrimarySystem.SetBoot(&schemas.Boot{
		BootSourceOverrideTarget:  schemas.NoneBootSource,
		BootSourceOverrideEnabled: schemas.DisabledBootSourceOverrideEnabled,
		BootSourceOverrideMode:    bootType,
	})

	err = c.redfishPrimarySystem.SetBoot(&schemas.Boot{
		BootSourceOverrideTarget:  schemas.PxeBootSource, //PxeBootSourceOverrideTarget,
		BootSourceOverrideEnabled: schemas.OnceBootSourceOverrideEnabled,
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

	// Best-effort clear of any stale/persistent override from earlier runs.
	_ = c.ipmiClient.SetBootDevice(bg, ipmi.BootDeviceSelectorNoOverride, bootType, false)

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

func (c *HostManagementClient) redfishClearBootOverride() (err error) {
	if err = c.ensureRedfishPrimarySystem(false); err != nil {
		return
	}

	err = c.redfishPrimarySystem.SetBoot(&schemas.Boot{
		BootSourceOverrideTarget:  schemas.NoneBootSource,
		BootSourceOverrideEnabled: schemas.DisabledBootSourceOverrideEnabled,
	})

	return
}

func (c *HostManagementClient) ipmiClearBootOverride() (err error) {
	errLegacy := c.ipmiClient.SetBootDevice(bg, ipmi.BootDeviceSelectorNoOverride, ipmi.BIOSBootTypeLegacy, false)
	errEFI := c.ipmiClient.SetBootDevice(bg, ipmi.BootDeviceSelectorNoOverride, ipmi.BIOSBootTypeEFI, false)
	if errLegacy == nil || errEFI == nil {
		return nil
	}

	return errLegacy
}

func (c *HostManagementClient) ClearBootOverride() (err error) {
	if !c.connected {
		err = ErrNotConnected
		return
	}

	switch c.Host.ManagementType {
	case ManagementTypeRedfish:
		err = c.redfishClearBootOverride()
	case ManagementTypeIPMI:
		err = c.ipmiClearBootOverride()
	default:
		err = ErrBadManagementType
	}

	return
}

// ---------- DATA COLLECTION ----------

func (c *HostManagementClient) redfishUpdateSystemInfo() (err error) {
	if c == nil || c.Host == nil {
		return fmt.Errorf("nil host management context")
	}

	if err = c.ensureRedfishPrimarySystem(true); err != nil {
		return
	}

	system := c.redfishPrimarySystem
	if system == nil {
		return ErrNoSystemFound
	}

	c.Host.Specs = HostSpecs{}
	c.Host.NetworkInterfaces = nil

	c.Host.Specs.Processor.Sku = strings.TrimSpace(system.ProcessorSummary.Model)

	for vID, vName := range VendorNames {
		if strings.Contains(strings.ToLower(system.Manufacturer), strings.ToLower(vName)) {
			c.Host.Vendor = vID
		}
	}

	count := numberAnyToInt(system.ProcessorSummary.Count)
	logicalCount := numberAnyToInt(system.ProcessorSummary.LogicalProcessorCount)
	if count > 0 {
		c.Host.Specs.Processor.Count = count
	}
	if logicalCount > 0 {
		c.Host.Specs.Processor.Threads = logicalCount
	}
	if count > 0 && logicalCount > 0 {
		c.Host.Specs.Processor.Cores = logicalCount / count
	}

	if processorsList, procErr := system.Processors(); procErr == nil && len(processorsList) > 0 && processorsList[0] != nil {
		c.Host.Specs.Processor.Manufacturer = string(processorsList[0].Manufacturer)
		c.Host.Specs.Processor.BaseSpeedMHz = numberAnyToInt(processorsList[0].OperatingSpeedMHz)
		c.Host.Specs.Processor.MaxSpeedMHz = numberAnyToInt(processorsList[0].MaxSpeedMHz)
	}

	c.Host.Specs.Memory.SizeGB = numberAnyToInt(system.MemorySummary.TotalSystemMemoryGiB)

	if memoryList, memErr := system.Memory(); memErr == nil {
		c.Host.Specs.Memory.NumDIMMs = len(memoryList)
		for _, memory := range memoryList {
			if memory == nil {
				continue
			}

			if speed := numberAnyToInt(memory.OperatingSpeedMhz); speed > 0 {
				c.Host.Specs.Memory.SpeedMHz = speed
				break
			}
		}
	}

	c.Host.Model = firstNonEmpty(system.Model, c.Host.Model)

	if services, storErr := system.Storage(); storErr == nil {
		for _, service := range services {
			if service == nil {
				continue
			}

			volumes, volErr := service.Volumes()
			if volErr != nil {
				continue
			}

			for _, volume := range volumes {
				if volume == nil {
					continue
				}

				capacityBytes := numberAnyToInt64(volume.CapacityBytes)
				c.Host.Specs.Storage = append(c.Host.Specs.Storage, HostStorageSpecs{
					CapacityGB: int(capacityBytes / (1024 * 1024 * 1024)),
					MediaType:  string(volume.VolumeType),
				})
			}
		}
	}

	if interfaces, ifErr := system.EthernetInterfaces(); ifErr == nil {
		for _, iface := range interfaces {
			if iface == nil {
				continue
			}

			c.Host.NetworkInterfaces = append(c.Host.NetworkInterfaces, HostNetworkInterface{
				Name:       firstNonEmpty(iface.ID, iface.Name),
				MACAddress: iface.MACAddress,
				SpeedMbps:  numberAnyToInt(iface.SpeedMbps),
			})
		}
	}

	return nil
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

func (c *HostManagementClient) redfishWaitSystemPowerState(desiredState PowerState, timeoutSeconds int) (err error) {
	var currentState PowerState
	for range timeoutSeconds {
		if currentState, err = c.redfishPowerState(true); err != nil {
			return
		}

		if currentState == desiredState {
			return
		}

		time.Sleep(1 * time.Second)
	}

	err = fmt.Errorf("timeout waiting for desired power state %v", desiredState)
	return
}

func (c *HostManagementClient) ipmiWaitSystemPowerState(desiredState PowerState, timeoutSeconds int) (err error) {
	var currentState PowerState
	for range timeoutSeconds {
		if currentState, err = c.ipmiPowerState(); err != nil {
			return
		}

		if currentState == desiredState {
			return
		}

		time.Sleep(1 * time.Second)
	}

	err = fmt.Errorf("timeout waiting for desired power state %v", desiredState)
	return
}

func (c *HostManagementClient) WaitSystemPowerState(desiredState PowerState, timeoutSeconds int) (err error) {
	if !c.connected {
		err = ErrNotConnected
		return
	}

	switch c.Host.ManagementType {
	case ManagementTypeRedfish:
		err = c.redfishWaitSystemPowerState(desiredState, timeoutSeconds)
	case ManagementTypeIPMI:
		err = c.ipmiWaitSystemPowerState(desiredState, timeoutSeconds)
	default:
		err = ErrBadManagementType
	}

	return
}
