package tests

import (
	"testing"

	"github.com/opnlaas/opnlaas/hosts"
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
