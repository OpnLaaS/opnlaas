package tests

import (
	"fmt"
	"testing"

	"github.com/opnlaas/laas/hosts"
)

func TestDB_CRUD_Basic(t *testing.T) {
	setup(t)
	defer cleanup(t)

	var (
		err      error
		testHost *hosts.Host = &hosts.Host{
			ManagementIP:   "10.0.1.17", // Primary Key
			ManagementType: hosts.ManagementTypeRedfish,
		}
	)

	// Create
	if err = hosts.Hosts.Insert(testHost); err != nil {
		t.Fatalf("Failed to create test host: %v", err)
	}

	// Read
	var fetchedHost *hosts.Host
	if fetchedHost, err = hosts.Hosts.Select(testHost.ManagementIP); err != nil {
		t.Fatalf("Failed to fetch test host by ID: %v", err)
	}

	if fetchedHost.ManagementType != testHost.ManagementType {
		t.Fatalf("Fetched host has incorrect ManagementType: got %v, want %v",
			fetchedHost.ManagementType, testHost.ManagementType)
	}

	// Update
	fetchedHost.ManagementType = hosts.ManagementTypeIPMI
	if err = hosts.Hosts.Update(fetchedHost); err != nil {
		t.Fatalf("Failed to update test host: %v", err)
	}

	var updatedHost *hosts.Host
	if updatedHost, err = hosts.Hosts.Select(testHost.ManagementIP); err != nil {
		t.Fatalf("Failed to fetch updated test host by ID: %v", err)
	}

	if updatedHost.ManagementType != hosts.ManagementTypeIPMI {
		t.Fatalf("Updated host has incorrect ManagementType: got %v, want %v",
			updatedHost.ManagementType, hosts.ManagementTypeIPMI)
	}

	// Delete
	if err = hosts.Hosts.Delete(updatedHost.ManagementIP); err != nil {
		t.Fatalf("Failed to delete test host: %v", err)
	}

	var deletedHost *hosts.Host
	if deletedHost, err = hosts.Hosts.Select(testHost.ManagementIP); err != nil {
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
		testHosts = [225]hosts.Host{}
	)

	for i := 0; i < 225; i++ {

		testHosts[i] = hosts.Host{

			ManagementIP:        fmt.Sprintf("10.0.1.%03d", i),
			Vendor:              hosts.VendorID(i % 8),
			FormFactor:          hosts.FormFactor(i % 5),
			ManagementType:      hosts.ManagementType(i % 3),
			Model:               fmt.Sprintf("Model #%03d", i),
			LastKnownPowerState: hosts.PowerState(i % 3),
		}
	}

	for _, testHost := range testHosts {
		if err = hosts.Hosts.Insert(&testHost); err != nil {
			t.Fatalf("Failed to create test host: %v", err)
		}

		// Read
		var fetchedHost *hosts.Host
		if fetchedHost, err = hosts.Hosts.Select(testHost.ManagementIP); err != nil {
			t.Fatalf("Failed to fetch test host by ID: %v", err)
		}

		if fetchedHost.ManagementType != testHost.ManagementType {
			t.Fatalf("Fetched host has incorrect ManagementType: got %v, want %v",
				fetchedHost.ManagementType, testHost.ManagementType)
		}
	}

	for _, testHost := range testHosts {

		// Update
		var fetchedHost *hosts.Host
		fetchedHost, _ = hosts.Hosts.Select(testHost.ManagementIP)

		fetchedHost.ManagementType = (fetchedHost.ManagementType + 1) % 3
		if err = hosts.Hosts.Update(fetchedHost); err != nil {
			t.Fatalf("Failed to update test host: %v", err)
		}

		var updatedHost *hosts.Host
		if updatedHost, err = hosts.Hosts.Select(testHost.ManagementIP); err != nil {
			t.Fatalf("Failed to fetch updated test host by ID: %v", err)
		}

		if updatedHost.ManagementType == testHost.ManagementType {
			t.Fatalf("Updated host has incorrect ManagementType: got %v, want %v",
				updatedHost.ManagementType, testHost.ManagementType)
		}
	}

	for i, testHost := range testHosts {

		// Delete
		if err = hosts.Hosts.Delete(testHost.ManagementIP); err != nil {
			t.Fatalf("Failed to delete test host: %v", err)
		}

		var deletedHost *hosts.Host
		if deletedHost, err = hosts.Hosts.Select(testHost.ManagementIP); err != nil {
			t.Fatalf("Expected no error when fetching deleted host, but got: %v", err)
		}

		if i < len(testHosts)-2 {
			var nextIp = fmt.Sprintf("%s%03d", testHost.ManagementIP[:len(testHost.ManagementIP)-3], i+1)

			if _, err = hosts.Hosts.Select(nextIp); err != nil {
				t.Fatalf("Deletion operation deleted the wrong host, should have deleted host %v but deleted %v", deletedHost.ManagementIP, nextIp)
			}
		}

		if deletedHost != nil {
			t.Fatalf("Expected deleted host to be nil, but got: %v", deletedHost)
		}
	}

}
