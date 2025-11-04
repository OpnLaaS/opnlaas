package tests

import (
	"encoding/json"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/opnlaas/opnlaas/config"
	"github.com/opnlaas/opnlaas/db"
)

func TestEnumAPIVendorIDs(t *testing.T) {
	setup(t)
	defer cleanup(t)

	var app *fiber.App = setupAppServer(t)
	defer cleanupAppServer(t, app)

	if status, body, err := makeHTTPGetRequest(t, "http://"+config.Config.WebServer.Address+"/api/enums/vendors"); err != nil {
		t.Fatalf("HTTP GET request failed: %v", err)
	} else if status != 200 {
		t.Fatalf("Expected status code 200, got %d", status)
	} else if body == "" {
		t.Fatalf("Expected non-empty response body")
	} else {
		var result map[string]db.VendorID
		if err = json.Unmarshal([]byte(body), &result); err != nil {
			t.Fatalf("Failed to parse JSON response: %v", err)
		}

		if len(result) != len(db.VendorNameReverses) {
			t.Fatalf("Expected %d vendors, got %d", len(db.VendorNameReverses), len(result))
		}

		for name, id := range db.VendorNameReverses {
			if result[name] != id {
				t.Fatalf("Expected vendor %s to have ID %d, got %d", name, id, result[name])
			}
		}
	}
}

func TestEnumAPIFormFactorNames(t *testing.T) {
	setup(t)
	defer cleanup(t)

	var app *fiber.App = setupAppServer(t)
	defer cleanupAppServer(t, app)

	if status, body, err := makeHTTPGetRequest(t, "http://"+config.Config.WebServer.Address+"/api/enums/form-factors"); err != nil {
		t.Fatalf("HTTP GET request failed: %v", err)
	} else if status != 200 {
		t.Fatalf("Expected status code 200, got %d", status)
	} else if body == "" {
		t.Fatalf("Expected non-empty response body")
	} else {
		var result map[string]db.FormFactor
		if err = json.Unmarshal([]byte(body), &result); err != nil {
			t.Fatalf("Failed to parse JSON response: %v", err)
		}

		if len(result) != len(db.FormFactorNameReverses) {
			t.Fatalf("Expected %d vendors, got %d", len(db.FormFactorNameReverses), len(result))
		}

		for name, id := range db.FormFactorNameReverses {
			if result[name] != id {
				t.Fatalf("Expected vendor %s to have ID %d, got %d", name, id, result[name])
			}
		}
	}
}

func TestEnumAPIFormManagementTypes(t *testing.T) {
	setup(t)
	defer cleanup(t)

	var app *fiber.App = setupAppServer(t)
	defer cleanupAppServer(t, app)

	if status, body, err := makeHTTPGetRequest(t, "http://"+config.Config.WebServer.Address+"/api/enums/management-types"); err != nil {
		t.Fatalf("HTTP GET request failed: %v", err)
	} else if status != 200 {
		t.Fatalf("Expected status code 200, got %d", status)
	} else if body == "" {
		t.Fatalf("Expected non-empty response body")
	} else {
		var result map[string]db.ManagementType
		if err = json.Unmarshal([]byte(body), &result); err != nil {
			t.Fatalf("Failed to parse JSON response: %v", err)
		}

		if len(result) != len(db.ManagementTypeNameReverses) {
			t.Fatalf("Expected %d vendors, got %d", len(db.ManagementTypeNameReverses), len(result))
		}

		for name, id := range db.ManagementTypeNameReverses {
			if result[name] != id {
				t.Fatalf("Expected vendor %s to have ID %d, got %d", name, id, result[name])
			}
		}
	}
}

func TestEnumAPIFormPowerStates(t *testing.T) {
	setup(t)
	defer cleanup(t)

	var app *fiber.App = setupAppServer(t)
	defer cleanupAppServer(t, app)

	if status, body, err := makeHTTPGetRequest(t, "http://"+config.Config.WebServer.Address+"/api/enums/power-states"); err != nil {
		t.Fatalf("HTTP GET request failed: %v", err)
	} else if status != 200 {
		t.Fatalf("Expected status code 200, got %d", status)
	} else if body == "" {
		t.Fatalf("Expected non-empty response body")
	} else {
		var result map[string]db.PowerState
		if err = json.Unmarshal([]byte(body), &result); err != nil {
			t.Fatalf("Failed to parse JSON response: %v", err)
		}

		if len(result) != len(db.PowerStateNameReverses) {
			t.Fatalf("Expected %d vendors, got %d", len(db.PowerStateNameReverses), len(result))
		}

		for name, id := range db.PowerStateNameReverses {
			if result[name] != id {
				t.Fatalf("Expected vendor %s to have ID %d, got %d", name, id, result[name])
			}
		}
	}
}

func TestEnumAPIFormBootModes(t *testing.T) {
	setup(t)
	defer cleanup(t)

	var app *fiber.App = setupAppServer(t)
	defer cleanupAppServer(t, app)

	if status, body, err := makeHTTPGetRequest(t, "http://"+config.Config.WebServer.Address+"/api/enums/boot-modes"); err != nil {
		t.Fatalf("HTTP GET request failed: %v", err)
	} else if status != 200 {
		t.Fatalf("Expected status code 200, got %d", status)
	} else if body == "" {
		t.Fatalf("Expected non-empty response body")
	} else {
		var result map[string]db.BootMode
		if err = json.Unmarshal([]byte(body), &result); err != nil {
			t.Fatalf("Failed to parse JSON response: %v", err)
		}

		if len(result) != len(db.BootModeNameReverses) {
			t.Fatalf("Expected %d vendors, got %d", len(db.BootModeNameReverses), len(result))
		}

		for name, id := range db.BootModeNameReverses {
			if result[name] != id {
				t.Fatalf("Expected vendor %s to have ID %d, got %d", name, id, result[name])
			}
		}
	}
}
