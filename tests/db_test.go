package tests

import (
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
