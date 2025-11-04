package tests

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/opnlaas/opnlaas/db"
)

func TestDB_CRUD_Basic(t *testing.T) {
	setup(t)
	defer cleanup(t)

	var (
		err      error
		testHost *db.Host = &db.Host{
			ManagementIP:   "10.0.1.17", // Primary Key
			ManagementType: db.ManagementTypeRedfish,
		}
	)

	// Create
	if err = db.Hosts.Insert(testHost); err != nil {
		t.Fatalf("Failed to create test host: %v", err)
	}

	// Read
	var fetchedHost *db.Host
	if fetchedHost, err = db.Hosts.Select(testHost.ManagementIP); err != nil {
		t.Fatalf("Failed to fetch test host by ID: %v", err)
	}

	if fetchedHost.ManagementType != testHost.ManagementType {
		t.Fatalf("Fetched host has incorrect ManagementType: got %v, want %v",
			fetchedHost.ManagementType, testHost.ManagementType)
	}

	// Update
	fetchedHost.ManagementType = db.ManagementTypeIPMI
	if err = db.Hosts.Update(fetchedHost); err != nil {
		t.Fatalf("Failed to update test host: %v", err)
	}

	var updatedHost *db.Host
	if updatedHost, err = db.Hosts.Select(testHost.ManagementIP); err != nil {
		t.Fatalf("Failed to fetch updated test host by ID: %v", err)
	}

	if updatedHost.ManagementType != db.ManagementTypeIPMI {
		t.Fatalf("Updated host has incorrect ManagementType: got %v, want %v",
			updatedHost.ManagementType, db.ManagementTypeIPMI)
	}

	// Delete
	if err = db.Hosts.Delete(updatedHost.ManagementIP); err != nil {
		t.Fatalf("Failed to delete test host: %v", err)
	}

	var deletedHost *db.Host
	if deletedHost, err = db.Hosts.Select(testHost.ManagementIP); err != nil {
		t.Fatalf("Expected no error when fetching deleted host, but got: %v", err)
	}

	if deletedHost != nil {
		t.Fatalf("Expected deleted host to be nil, but got: %v", deletedHost)
	}
}

func TestDB_CRUD_Many(t *testing.T) {
	setup(t)
	defer cleanup(t)

	var (
		err       error
		testHosts = [225]db.Host{}
	)

	for i := 0; i < 225; i++ {

		testHosts[i] = db.Host{

			ManagementIP:        fmt.Sprintf("10.0.1.%03d", i),
			Vendor:              db.VendorID(i % 8),
			FormFactor:          db.FormFactor(i % 5),
			ManagementType:      db.ManagementType(i % 3),
			Model:               fmt.Sprintf("Model #%03d", i),
			LastKnownPowerState: db.PowerState(i % 3),
		}
	}

	for _, testHost := range testHosts {
		if err = db.Hosts.Insert(&testHost); err != nil {
			t.Fatalf("Failed to create test host: %v", err)
		}

		// Read
		var fetchedHost *db.Host
		if fetchedHost, err = db.Hosts.Select(testHost.ManagementIP); err != nil {
			t.Fatalf("Failed to fetch test host by ID: %v", err)
		}

		if !reflect.DeepEqual(fetchedHost, &testHost) {
			t.Fatalf("Fetched host does not match expected: got %v, want %v",
				fetchedHost.ManagementType, testHost.ManagementType)
		}
	}

	for _, testHost := range testHosts {

		// Update
		var fetchedHost *db.Host
		fetchedHost, _ = db.Hosts.Select(testHost.ManagementIP)

		fetchedHost.ManagementType = (fetchedHost.ManagementType + 1) % 3
		if err = db.Hosts.Update(fetchedHost); err != nil {
			t.Fatalf("Failed to update test host: %v", err)
		}

		var updatedHost *db.Host
		if updatedHost, err = db.Hosts.Select(testHost.ManagementIP); err != nil {
			t.Fatalf("Failed to fetch updated test host by ID: %v", err)
		}

		if updatedHost.ManagementType == testHost.ManagementType {
			t.Fatalf("Updated host has incorrect ManagementType: got %v, want %v",
				updatedHost.ManagementType, testHost.ManagementType)
		}
	}

	for i, testHost := range testHosts {

		// Delete
		if err = db.Hosts.Delete(testHost.ManagementIP); err != nil {
			t.Fatalf("Failed to delete test host: %v", err)
		}

		var deletedHost *db.Host
		if deletedHost, err = db.Hosts.Select(testHost.ManagementIP); err != nil {
			t.Fatalf("Expected no error when fetching deleted host, but got: %v", err)
		}

		if i < len(testHosts)-2 {
			var nextIp = fmt.Sprintf("%s%03d", testHost.ManagementIP[:len(testHost.ManagementIP)-3], i+1)

			if _, err = db.Hosts.Select(nextIp); err != nil {
				t.Fatalf("Deletion operation deleted the wrong host, should have deleted host %v but deleted %v", deletedHost.ManagementIP, nextIp)
			}
		}

		if deletedHost != nil {
			t.Fatalf("Expected deleted host to be nil, but got: %v", deletedHost)
		}
	}
}

func TestDB_CRUD_Complex(t *testing.T) {
	setup(t)
	defer cleanup(t)

	var (
		err      error
		testHost *db.Host = &db.Host{
			ManagementIP:        "10.0.0.1",
			Vendor:              db.VendorAsus,
			FormFactor:          db.FormFactorBlade,
			ManagementType:      db.ManagementTypeIPMI,
			Model:               "s3rver",
			LastKnownPowerState: db.PowerStateOn,
			Specs: db.HostSpecs{
				Processor: db.HostCPUSpecs{
					Sku:     "adjklasd",
					Count:   4,
					Cores:   16,
					Threads: 8,
				},
			},
		}
	)

	// Create
	if err = db.Hosts.Insert(testHost); err != nil {
		t.Fatalf("Failed to create test host: %v", err)
	}

	// Read
	var fetchedHost *db.Host
	if fetchedHost, err = db.Hosts.Select(testHost.ManagementIP); err != nil {
		t.Fatalf("Failed to fetch test host by ID: %v", err)
	}

	if !reflect.DeepEqual(&fetchedHost, &testHost) {
		t.Fatalf("Fetched host is incorrect: got %v, want %v",
			fetchedHost, testHost)
	}

	// Update
	fetchedHost.ManagementType = db.ManagementTypeIPMI
	if err = db.Hosts.Update(fetchedHost); err != nil {
		t.Fatalf("Failed to update test host: %v", err)
	}

	var updatedHost *db.Host
	if updatedHost, err = db.Hosts.Select(testHost.ManagementIP); err != nil {
		t.Fatalf("Failed to fetch updated test host by ID: %v", err)
	}

	if !reflect.DeepEqual(&fetchedHost, &updatedHost) {
		t.Fatalf("Updated host is incorrect: got %v, want %v",
			updatedHost, fetchedHost)
	}

	// Delete
	if err = db.Hosts.Delete(updatedHost.ManagementIP); err != nil {
		t.Fatalf("Failed to delete test host: %v", err)
	}

	var deletedHost *db.Host
	if deletedHost, err = db.Hosts.Select(testHost.ManagementIP); err != nil {
		t.Fatalf("Expected no error when fetching deleted host, but got: %v", err)
	}

	if deletedHost != nil {
		t.Fatalf("Expected deleted host to be nil, but got: %v", deletedHost)
	}

}
