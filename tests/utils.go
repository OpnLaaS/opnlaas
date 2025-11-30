package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/opnlaas/opnlaas/app"
	"github.com/opnlaas/opnlaas/config"
	"github.com/opnlaas/opnlaas/db"
)

func setup(t *testing.T) {
	var err error

	if err = config.Init("../config.toml"); err != nil {
		t.Fatalf("Failed to load config.toml: %v", err)
	}

	if config.Config.Management.Testing.Basic.Enabled {
		if len(config.Config.Management.Testing.Basic.IPs) == 0 {
			t.Fatalf("No management IPs configured in config.toml")
		} else if strings.TrimSpace(config.Config.Management.Testing.Basic.IPs[0]) == "" {
			t.Fatalf("First management IP is empty in config.toml")
		}
	}

	// Randomize the database file name to avoid conflicts
	config.Config.Database.File = fmt.Sprintf("test_db_%d.db", time.Now().UnixNano())

	if err = db.InitDB(); err != nil {
		t.Fatalf("Failed to initialize DB: %v", err)
	}
}

func cleanup(t *testing.T) {
	var err error

	if err = db.CloseDB(); err != nil {
		t.Fatalf("Failed to close DB: %v", err)
	}

	if err = os.Remove(db.DatabaseFilePath()); err != nil {
		t.Fatalf("Failed to remove test database file: %v", err)
	}
}

func setupAppServer(t *testing.T) (fiberApp *fiber.App) {
	fiberApp = app.CreateApp()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	config.Config.WebServer.Address = listener.Addr().String()

	go func(t *testing.T) {
		if err := fiberApp.Listener(listener); err != nil {
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

func makeHTTPGetRequestJSON(t *testing.T, url string) (body any, err error) {
	var (
		status  int
		bodyStr string
	)

	if status, bodyStr, err = makeHTTPGetRequest(t, url); err != nil {
		return
	}

	if status != http.StatusOK {
		err = fmt.Errorf("unexpected status code: %d", status)
		return
	}

	if err = json.Unmarshal([]byte(bodyStr), &body); err != nil {
		return
	}

	return
}

// Username and password as form login
func loginAndGetCookies(t *testing.T, username, password string) (cookies []*http.Cookie, err error) {
	var (
		request  *http.Request
		formData string = fmt.Sprintf("username=%s&password=%s", username, password)
	)

	if request, err = http.NewRequest("POST", fmt.Sprintf("http://%s/api/auth/login?no_redirect=1", config.Config.WebServer.Address), strings.NewReader(formData)); err != nil {
		return
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var client http.Client
	var response *http.Response
	if response, err = client.Do(request); err != nil {
		return
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		err = fmt.Errorf("login failed with status code: %d", response.StatusCode)
		return
	}

	cookies = response.Cookies()
	return
}

func makeHTTPPostRequest(t *testing.T, url, jsonData string, cookies []*http.Cookie) (statusCode int, body string, err error) {
	var request *http.Request
	if request, err = http.NewRequest("POST", url, strings.NewReader(jsonData)); err != nil {
		return
	}
	request.Header.Set("Content-Type", "application/json")

	for _, cookie := range cookies {
		request.AddCookie(cookie)
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

func makeHTTPDeleteRequest(t *testing.T, url string, cookies []*http.Cookie) (statusCode int, body string, err error) {
	var request *http.Request
	if request, err = http.NewRequest("DELETE", url, nil); err != nil {
		return
	}

	for _, cookie := range cookies {
		request.AddCookie(cookie)
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
