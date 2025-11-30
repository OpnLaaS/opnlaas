package tests

import (
	"strings"
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
		if !config.Config.Management.Testing.Basic.Enabled {
			t.Skip("Skipping bad credential test")
		}

		ip := config.Config.Management.Testing.Basic.IPs[0]
		originalPass := config.Config.Management.Password

		config.Config.Management.Password = "this-is-a-bad-password"

		defer func() {
			config.Config.Management.Password = originalPass
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
