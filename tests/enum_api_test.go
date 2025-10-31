package tests

import (
	"encoding/json"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/opnlaas/laas/config"
	"github.com/opnlaas/laas/hosts"
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
		var result map[string]hosts.VendorID
		if err = json.Unmarshal([]byte(body), &result); err != nil {
			t.Fatalf("Failed to parse JSON response: %v", err)
		}

		if len(result) != len(hosts.VendorNameReverses) {
			t.Fatalf("Expected %d vendors, got %d", len(hosts.VendorNameReverses), len(result))
		}

		for name, id := range hosts.VendorNameReverses {
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
		var result map[string]hosts.FormFactor
		if err = json.Unmarshal([]byte(body), &result); err != nil {
			t.Fatalf("Failed to parse JSON response: %v", err)
		}

		if len(result) != len(hosts.FormFactorNameReverses) {
			t.Fatalf("Expected %d vendors, got %d", len(hosts.FormFactorNameReverses), len(result))
		}

		for name, id := range hosts.FormFactorNameReverses {
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
		var result map[string]hosts.ManagementType
		if err = json.Unmarshal([]byte(body), &result); err != nil {
			t.Fatalf("Failed to parse JSON response: %v", err)
		}

		if len(result) != len(hosts.ManagementTypeNameReverses) {
			t.Fatalf("Expected %d vendors, got %d", len(hosts.ManagementTypeNameReverses), len(result))
		}

		for name, id := range hosts.ManagementTypeNameReverses {
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
		var result map[string]hosts.PowerState
		if err = json.Unmarshal([]byte(body), &result); err != nil {
			t.Fatalf("Failed to parse JSON response: %v", err)
		}

		if len(result) != len(hosts.PowerStateNameReverses) {
			t.Fatalf("Expected %d vendors, got %d", len(hosts.PowerStateNameReverses), len(result))
		}

		for name, id := range hosts.PowerStateNameReverses {
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
		var result map[string]hosts.BootMode
		if err = json.Unmarshal([]byte(body), &result); err != nil {
			t.Fatalf("Failed to parse JSON response: %v", err)
		}

		if len(result) != len(hosts.BootModeNameReverses) {
			t.Fatalf("Expected %d vendors, got %d", len(hosts.BootModeNameReverses), len(result))
		}

		for name, id := range hosts.BootModeNameReverses {
			if result[name] != id {
				t.Fatalf("Expected vendor %s to have ID %d, got %d", name, id, result[name])
			}
		}
	}
}
