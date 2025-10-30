package tests

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/opnlaas/laas/config"
	"github.com/opnlaas/laas/hosts"
)

func setup(t *testing.T) {
	var err error

	if err = config.InitEnv("../.test.env"); err != nil {
		t.Fatalf("Failed to load .env: %v", err)
	}

	if config.Config.Management.TestingRunManagement {
		if len(config.Config.Management.TestingManagementIPs) == 0 {
			t.Fatalf("No management IPs configured in test.env")
		} else if strings.TrimSpace(config.Config.Management.TestingManagementIPs[0]) == "" {
			t.Fatalf("First management IP is empty in test.env")
		}
	}

	// Randomize the database file name to avoid conflicts
	config.Config.Database.File = fmt.Sprintf("test_db_%d.db", time.Now().UnixNano())

	if err = hosts.InitDB(); err != nil {
		t.Fatalf("Failed to initialize DB: %v", err)
	}
}

func cleanup(t *testing.T) {
	var err error

	if err = hosts.CloseDB(); err != nil {
		t.Fatalf("Failed to close DB: %v", err)
	}

	if err = os.Remove(hosts.DatabaseFilePath()); err != nil {
		t.Fatalf("Failed to remove test database file: %v", err)
	}
}
