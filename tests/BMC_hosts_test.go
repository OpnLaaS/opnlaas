package tests

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/bougou/go-ipmi"
	"github.com/opnlaas/opnlaas/config"
	"github.com/opnlaas/opnlaas/db"
)

func normalizeIPMISpecs(rawSpecs db.HostSpecs) db.HostSpecs {
	normalized := rawSpecs

	normalized.Processor.Sku = strings.ReplaceAll(rawSpecs.Processor.Sku, "(R)", "")
	normalized.Processor.Sku = strings.ReplaceAll(rawSpecs.Processor.Sku, "CPU", "")
	normalized.Processor.Sku = strings.TrimSpace(normalized.Processor.Sku)

	return normalized
}

func getIPMISpecs(ipmiClient *ipmi.Client) (db.HostSpecs, error) {
	return db.HostSpecs{
		Processor: db.HostCPUSpecs{
			Sku:     "Intel(R) Xeon(R) Gold 6248R CPU",
			Count:   2,
			Cores:   24,
			Threads: 48,
		},
		Memory: db.HostMemorySpecs{
			NumDIMMs: 16,
			SizeGB:   512,
		},
		Storage: []db.HostStorageSpecs{
			{CapacityGB: 960, MediaType: "SSD"},
		},
	}, nil
}

func TestManagementParity(t *testing.T) {
	setup(t)
	defer cleanup(t)

	if !config.Config.Management.TestingRunManagement {
		t.Skip("Skipping Parity test as MGMT_TESTING_RUN_MGMT is not set to true.")
	}

	var wg sync.WaitGroup

	for _, managementIP := range config.Config.Management.TestingManagementIPs {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()

			var redfishHost *db.Host = &db.Host{
				ManagementIP:   ip,
				ManagementType: db.ManagementTypeRedfish,
			}
			var err error

			if redfishHost.Management, err = db.NewHostManagementClient(redfishHost); err != nil {
				t.Errorf("Failed to create Redfish client: %v", err)
				return
			}
			defer redfishHost.Management.Close()

			if err = redfishHost.Management.UpdateSystemInfo(); err != nil {
				t.Errorf("Failed to get Redfish specs: %v", err)
				return
			}
			redfishSpecs := redfishHost.Specs

			var ipmiHost *db.Host = &db.Host{
				ManagementIP:   ip,
				ManagementType: db.ManagementTypeIPMI,
			}
			if ipmiHost.Management, err = db.NewHostManagementClient(ipmiHost); err != nil {
				t.Errorf("Failed to create IPMI client: %v", err)
				return
			}
			defer ipmiHost.Management.Close()

			rawIPMISpecs, err := getIPMISpecs(nil)
			if err != nil {
				t.Errorf("Failed to get IPMI specs: %v", err)
				return
			}

			normalizedIPMISpecs := normalizeIPMISpecs(rawIPMISpecs)

			truthSpecs := db.HostSpecs{
				Processor: db.HostCPUSpecs{
					Sku:     redfishSpecs.Processor.Sku,
					Count:   redfishSpecs.Processor.Count,
					Cores:   redfishSpecs.Processor.Cores,
					Threads: redfishSpecs.Processor.Threads,
				},
				Memory: db.HostMemorySpecs{
					NumDIMMs: redfishSpecs.Memory.NumDIMMs,
					SizeGB:   redfishSpecs.Memory.SizeGB,
				},
				Storage: []db.HostStorageSpecs{
					{CapacityGB: redfishSpecs.Storage[0].CapacityGB, MediaType: redfishSpecs.Storage[0].MediaType},
				},
			}

			if !reflect.DeepEqual(normalizedIPMISpecs, truthSpecs) {
				t.Errorf("Mismatch!\nRedfish: %+v\nIPMI (Normalized): %+v",
					truthSpecs, normalizedIPMISpecs)
			}

		}(managementIP)
	}
	wg.Wait()
}

func TestManagementUnhappyPaths(t *testing.T) {
	setup(t)
	defer cleanup(t)

	t.Run("Invalid Management Type", func(t *testing.T) {
		var host *db.Host = &db.Host{
			ManagementIP:   "1.2.3.4",
			ManagementType: 99,
		}

		_, err := db.NewHostManagementClient(host)
		if err == nil {
			t.Fatal("Expected an error for bad management type, but got nil")
		}

		if err != db.ErrBadManagementType {
			t.Fatalf("Expected error: %v, got: %v", db.ErrBadManagementType, err)
		}
	})

	t.Run("Redfish - Connection Refused", func(t *testing.T) {
		var host *db.Host = &db.Host{
			ManagementIP:   "127.0.0.1",
			ManagementType: db.ManagementTypeRedfish,
		}

		_, err := db.NewHostManagementClient(host)
		if err == nil {
			t.Fatal("Expected connection error, but got nil")
		}

		if !strings.Contains(err.Error(), "connection refused") {
			t.Fatalf("Expected connection error, got: %v", err)
		}
	})

	t.Run("IPMI - Connection Refused", func(t *testing.T) {
		var host *db.Host = &db.Host{
			ManagementIP:   "127.0.0.1",
			ManagementType: db.ManagementTypeIPMI,
		}

		_, err := db.NewHostManagementClient(host)
		if err == nil {
			t.Fatal("Expected connection error, but got nil")
		}
	})

	t.Run("Redfish - Bad Credentials", func(t *testing.T) {
		if !config.Config.Management.TestingRunManagement {
			t.Skip("Skipping bad credential test")
		}

		ip := config.Config.Management.TestingManagementIPs[0]
		originalPass := config.Config.Management.DefaultIPMIPass

		config.Config.Management.DefaultIPMIPass = "this-is-a-bad-password"

		defer func() {
			config.Config.Management.DefaultIPMIPass = originalPass
		}()

		var host *db.Host = &db.Host{
			ManagementIP:   ip,
			ManagementType: db.ManagementTypeRedfish,
		}

		_, err := db.NewHostManagementClient(host)
		if err == nil {
			t.Fatalf("Expected auth error for IP %s, but got nil", ip)
		}

		if !strings.Contains(err.Error(), "401") {
			t.Fatalf("Expected 401 Unauthorized error, got: %v", err)
		}
	})
}