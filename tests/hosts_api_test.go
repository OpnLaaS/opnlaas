package tests

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/opnlaas/opnlaas/auth"
	"github.com/opnlaas/opnlaas/config"
	"github.com/opnlaas/opnlaas/hosts"
)

func TestHostsAPI(t *testing.T) {
	setup(t)
	defer cleanup(t)

	var app *fiber.App = setupAppServer(t)
	defer cleanupAppServer(t, app)

	// Setup
	auth.AddUserInjection("alice", "alice", auth.AuthPermsAdministrator)
	auth.AddUserInjection("bob", "bob", auth.AuthPermsUser)
	auth.AddUserInjection("charlie", "charlie", auth.AuthPermsNone)

	t.Run("Empty hosts list to start", func(t *testing.T) {
		var (
			hostsObject any
			err         error
		)

		if hostsObject, err = makeHTTPGetRequestJSON(t, fmt.Sprintf("http://%s/api/hosts", config.Config.WebServer.Address)); err != nil {
			t.Fatalf("Failed to get hosts: %v", err)
		} else if hostsObject != nil {
			if hostsSlice, ok := hostsObject.([]*hosts.Host); !ok {
				t.Fatalf("Expected hosts list to be a slice, got %T", hostsObject)
			} else if len(hostsSlice) != 0 {
				t.Fatalf("Expected hosts list to be empty, got %d hosts", len(hostsSlice))
			}
		}
	})

	t.Run("Add a host", func(t *testing.T) {
		if !config.Config.Management.TestingRunManagement {
			t.Skip("Host management is disabled; skipping host addition test")
		}

		var (
			newHost *hosts.Host = &hosts.Host{ManagementIP: config.Config.Management.TestingManagementIPs[0], ManagementType: hosts.ManagementTypeRedfish}
			user    string      = "alice"
		)

		if cookies, err := loginAndGetCookies(t, user, user); err != nil {
			t.Fatalf("Failed to login as %s: %v", user, err)
		} else {
			if status, resp, err := makeHTTPPostRequest(t, fmt.Sprintf("http://%s/api/hosts", config.Config.WebServer.Address), func() string {
				if str, err := json.Marshal(newHost); err != nil {
					t.Fatalf("Failed to marshal new host JSON: %v", err)
					return ""
				} else {
					return string(str)
				}
			}(), cookies); err != nil {
				t.Fatalf("Failed to create host: %v", err)
			} else if status != fiber.StatusOK {
				t.Fatalf("Expected status %d, got %d: %s", fiber.StatusOK, status, resp)
			} else {
				var createdHost hosts.Host
				if err := json.Unmarshal([]byte(resp), &createdHost); err != nil {
					t.Fatalf("Failed to unmarshal created host JSON: %v", err)
				} else if createdHost.ManagementIP != config.Config.Management.TestingManagementIPs[0] {
					t.Fatalf("Expected created host ManagementIP to be '%s', got '%s'", config.Config.Management.TestingManagementIPs[0], createdHost.ManagementIP)
				}
			}
		}
	})

	t.Run("Hosts list has one host", func(t *testing.T) {
		var (
			hostsObject any
			err         error
		)

		if hostsObject, err = makeHTTPGetRequestJSON(t, fmt.Sprintf("http://%s/api/hosts", config.Config.WebServer.Address)); err != nil {
			t.Fatalf("Failed to get hosts: %v", err)
		} else {
			if hostsSlice, ok := hostsObject.([]interface{}); !ok {
				t.Fatalf("Expected hosts list to be a slice, got %T", hostsObject)
			} else if len(hostsSlice) != 1 {
				t.Fatalf("Expected hosts list to have 1 host, got %d hosts", len(hostsSlice))
			} else {
				// Confrm it's a host object
				if _, ok := hostsSlice[0].(map[string]interface{}); !ok {
					t.Fatalf("Expected host object to be a map, got %T", hostsSlice[0])
				} else {
					var target hosts.Host
					hostJSON, _ := json.Marshal(hostsSlice[0])
					if err := json.Unmarshal(hostJSON, &target); err != nil {
						t.Fatalf("Failed to unmarshal host JSON: %v", err)
					} else if target.ManagementIP != config.Config.Management.TestingManagementIPs[0] {
						t.Fatalf("Expected host ManagementIP to be '%s', got '%s'", config.Config.Management.TestingManagementIPs[0], target.ManagementIP)
					}
				}
			}
		}
	})

	t.Run("Get host by management IP", func(t *testing.T) {
		var (
			hostObject any
			err        error
		)

		if hostObject, err = makeHTTPGetRequestJSON(t, fmt.Sprintf("http://%s/api/hosts/%s", config.Config.WebServer.Address, config.Config.Management.TestingManagementIPs[0])); err != nil {
			t.Fatalf("Failed to get host by management IP: %v", err)
		} else {
			if hostMap, ok := hostObject.(map[string]interface{}); !ok {
				t.Fatalf("Expected host object to be a map, got %T", hostObject)
			} else {
				var target hosts.Host
				hostJSON, _ := json.Marshal(hostMap)
				if err := json.Unmarshal(hostJSON, &target); err != nil {
					t.Fatalf("Failed to unmarshal host JSON: %v", err)
				} else if target.ManagementIP != config.Config.Management.TestingManagementIPs[0] {
					t.Fatalf("Expected host ManagementIP to be '%s', got '%s'", config.Config.Management.TestingManagementIPs[0], target.ManagementIP)
				}
			}
		}
	})

	t.Run("Delete host", func(t *testing.T) {
		if !config.Config.Management.TestingRunManagement {
			t.Skip("Host management is disabled; skipping host deletion test")
		}

		var (
			user string = "alice"
		)

		if cookies, err := loginAndGetCookies(t, user, user); err != nil {
			t.Fatalf("Failed to login as %s: %v", user, err)
		} else {
			if status, resp, err := makeHTTPDeleteRequest(t, fmt.Sprintf("http://%s/api/hosts/%s", config.Config.WebServer.Address, config.Config.Management.TestingManagementIPs[0]), cookies); err != nil {
				t.Fatalf("Failed to delete host: %v", err)
			} else if status != fiber.StatusOK {
				t.Fatalf("Expected status %d, got %d: %s", fiber.StatusOK, status, resp)
			}
		}
	})

	t.Run("Hosts list is empty again", func(t *testing.T) {
		var (
			hostsObject any
			err         error
		)

		if hostsObject, err = makeHTTPGetRequestJSON(t, fmt.Sprintf("http://%s/api/hosts", config.Config.WebServer.Address)); err != nil {
			t.Fatalf("Failed to get hosts: %v", err)
		} else if hostsObject != nil {
			if hostsSlice, ok := hostsObject.([]*hosts.Host); !ok {
				t.Fatalf("Expected hosts list to be a slice, got %T", hostsObject)
			} else if len(hostsSlice) != 0 {
				t.Fatalf("Expected hosts list to be empty, got %d hosts", len(hostsSlice))
			}
		}
	})
}
