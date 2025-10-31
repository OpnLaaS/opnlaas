package tests

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/opnlaas/laas/app"
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

func setupAppServer(t *testing.T) (fiberApp *fiber.App) {
	fiberApp = app.CreateApp()

	go func(t *testing.T) {
		if err := fiberApp.Listen(config.Config.WebServer.Address); err != nil {
			t.Errorf("Failed to start app: %v", err)
			panic(err)
		}
	}(t)

	time.Sleep(255 * time.Millisecond) // Give the server a little time to start
	return
}

func cleanupAppServer(t *testing.T, fiberApp *fiber.App) {
	if err := fiberApp.Shutdown(); err != nil {
		t.Fatalf("Failed to shutdown app: %v", err)
	}
}

func makeHTTPGetRequest(t *testing.T, url string) (statusCode int, body string, err error) {
	var request *http.Request
	if request, err = http.NewRequest("GET", url, nil); err != nil {
		return
	}

	var client http.Client
	var response *http.Response
	if response, err = client.Do(request); err != nil {
		return
	}
	defer response.Body.Close()

	statusCode = response.StatusCode

	var bodyBytes []byte
	if bodyBytes, err = io.ReadAll(response.Body); err != nil {
		return
	}
	body = string(bodyBytes)

	return
}
