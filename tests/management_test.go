package tests

import (
	"sync"
	"testing"

	"github.com/opnlaas/opnlaas/config"
	"github.com/opnlaas/opnlaas/db"
)

func TestRedfishManagementHosts(t *testing.T) {
	setup(t)
	defer cleanup(t)

	if !config.Config.Management.TestingRunManagement {
		t.Skip("Skipping Redfish management host tests as MGMT_TESTING_RUN_MGMT is not set to true.")
	}

	t.Log("Testing Redfish management host functionality in parallel.")

	var (
		wg                                                                  sync.WaitGroup
		totalHosts, totalMemoryGB, totalCores, totalThreads, totalStorageGB int
	)
	for _, managementIP := range config.Config.Management.TestingManagementIPs {
		wg.Add(1)

		go func(ip string) {
			defer wg.Done()

			t.Logf("Testing host with management IP: %s", ip)

			var (
				err  error
				host *db.Host = &db.Host{
					ManagementIP:   ip,
					ManagementType: db.ManagementTypeRedfish,
				}
			)

			if host.Management, err = db.NewHostManagementClient(host); err != nil {
				t.Errorf("Failed to create HostManagementClient for IP %s: %v", ip, err)
				return
			} else {
				defer host.Management.Close()
			}

			if host.LastKnownPowerState, err = host.Management.PowerState(); err != nil {
				t.Errorf("Failed to get Redfish power state for IP %s: %v", ip, err)
				return
			}

			if err = host.Management.UpdateSystemInfo(); err != nil {
				t.Errorf("Failed to update system info for IP %s: %v", ip, err)
				return
			}

			t.Logf("Host %s - Power State: %s, System Model: %s, System: %+v\n", ip, host.LastKnownPowerState.String(), host.Model, host)
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
