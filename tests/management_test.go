package tests

import (
	"sync"
	"testing"

	"github.com/opnlaas/laas/config"
	"github.com/opnlaas/laas/hosts"
)

func TestRedfishManagementHosts(t *testing.T) {
	setup(t)
	defer cleanup(t)

	t.Log("Testing Redfish management host functionality in parallel.")

	var (
		wg                                                                  sync.WaitGroup
		totalHosts, totalMemoryGB, totalCores, totalThreads, totalStorageGB int
	)
	for _, managementIP := range config.Config.Management.ManagementIPs {
		wg.Add(1)

		go func(ip string) {
			defer wg.Done()

			t.Logf("Testing host with management IP: %s", ip)

			var (
				err  error
				host *hosts.Host = &hosts.Host{
					ManagementIP:   ip,
					ManagementType: hosts.ManagementTypeRedfish,
				}
			)

			if host.Management, err = hosts.NewHostManagementClient(host); err != nil {
				t.Errorf("Failed to create HostManagementClient for IP %s: %v", ip, err)
				return
			}

			if host.LastKnownPowerState, err = host.Management.PowerState(); err != nil {
				t.Errorf("Failed to get Redfish power state for IP %s: %v", ip, err)
				return
			}

			if err = host.Management.UpdateSystemInfo(); err != nil {
				t.Errorf("Failed to update system info for IP %s: %v", ip, err)
				return
			}

			t.Logf("Host %s - Power State: %s, System Model: %s\n", ip, host.LastKnownPowerState.String(), host.Model)
			totalHosts++
			totalMemoryGB += host.Specs.Memory.SizeGB
			totalCores += host.Specs.Processor.Cores
			totalThreads += host.Specs.Processor.Threads
			for _, disk := range host.Specs.Storage {
				totalStorageGB += disk.CapacityGB
			}

		}(managementIP)
	}

	wg.Wait()
	t.Log("Completed Redfish management host tests.")
	t.Logf("Total Hosts Tested: %d", totalHosts)
	t.Logf("Aggregate Specs - Memory: %d GB, CPU Cores: %d, CPU Threads: %d, Storage: %d GB",
		totalMemoryGB, totalCores, totalThreads, totalStorageGB)
}
