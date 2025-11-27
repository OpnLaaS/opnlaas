package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/opnlaas/opnlaas/app"
	"github.com/opnlaas/opnlaas/auth"
	"github.com/opnlaas/opnlaas/db"
)

func TestBookingAPIFlow(t *testing.T) {
	setup(t)
	defer cleanup(t)

	var appServer *fiber.App = app.CreateApp()

	auth.AddUserInjection("alice", "alice", auth.AuthPermsAdministrator)
	auth.AddUserInjection("bob", "bob", auth.AuthPermsUser)

	testHostIP := "10.0.0.1"
	if err := db.Hosts.Insert(&db.Host{
		ManagementIP:   testHostIP,
		ManagementType: db.ManagementTypeIPMI,
	}); err != nil {
		t.Fatalf("failed to insert test host: %v", err)
	}

	doRequest := func(method, target, body, contentType string, cookies []*http.Cookie) (int, string, []*http.Cookie, error) {
		req, err := http.NewRequest(method, target, strings.NewReader(body))
		if err != nil {
			return 0, "", nil, err
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if len(cookies) > 0 {
			var parts []string
			for _, c := range cookies {
				if c != nil && c.Name != "" && c.Value != "" {
					parts = append(parts, fmt.Sprintf("%s=%s", c.Name, c.Value))
					if strings.EqualFold(c.Name, "Authorization") {
						req.Header.Set("Authorization", "Bearer "+c.Value)
					}
				}
			}
			if len(parts) > 0 {
				req.Header.Set("Cookie", strings.Join(parts, "; "))
			}
		}

		resp, err := appServer.Test(req, -1)
		if err != nil {
			return 0, "", nil, err
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return resp.StatusCode, "", resp.Cookies(), err
		}

		return resp.StatusCode, string(respBody), resp.Cookies(), nil
	}

	login := func(user string) []*http.Cookie {
		userRecord, err := auth.Authenticate(user, user)
		if err != nil {
			t.Fatalf("failed to authenticate %s: %v", user, err)
		}
		tokenStr, err := userRecord.Token.SignedString(app.GetJWTSigningKey())
		if err != nil {
			t.Fatalf("failed to sign token for %s: %v", user, err)
		}
		return []*http.Cookie{{Name: "Authorization", Value: tokenStr}}
	}

	t.Run("Create booking and owner relation", func(t *testing.T) {
		cookies := login("alice")

		if status, _, _, err := doRequest("GET", "/api/auth/me", "", "", cookies); err != nil {
			t.Fatalf("auth check failed: %v", err)
		} else if status != fiber.StatusOK {
			t.Fatalf("expected auth check 200 got %d", status)
		}

		body := `{"name":"Test Booking","description":"desc","duration_days":2}`
		if status, resp, _, err := doRequest("POST", "/api/bookings", body, "application/json", cookies); err != nil {
			t.Fatalf("create booking request failed: %v", err)
		} else if status != fiber.StatusOK {
			t.Fatalf("expected status %d got %d: %s", fiber.StatusOK, status, resp)
		} else {
			var booking db.Booking
			if err := json.Unmarshal([]byte(resp), &booking); err != nil {
				t.Fatalf("failed to unmarshal booking: %v", err)
			}

			if booking.Name != "Test Booking" {
				t.Fatalf("expected booking name 'Test Booking', got '%s'", booking.Name)
			}

			if people, err := db.BookingPeopleForBooking(booking.ID); err != nil {
				t.Fatalf("failed to fetch booking people: %v", err)
			} else if len(people) != 1 || people[0].Username != "alice" {
				t.Fatalf("expected alice to be booking owner, got %+v", people)
			}
		}
	})

	t.Run("Cart host reservation conflict", func(t *testing.T) {
		db.ResetBookingCart("alice")
		db.ResetBookingCart("bob")

		aliceCookies := login("alice")
		bobCookies := login("bob")

		if status, _, _, _ := doRequest("GET", "/api/auth/me", "", "", aliceCookies); status != fiber.StatusOK {
			t.Fatalf("expected alice auth check 200 got %d", status)
		}

		addHostPayload := fmt.Sprintf(`{"management_ip":"%s"}`, testHostIP)

		if status, resp, _, err := doRequest("POST", "/api/bookings/cart/hosts", addHostPayload, "application/json", aliceCookies); err != nil {
			t.Fatalf("alice failed to add host to cart: %v", err)
		} else if status != fiber.StatusOK {
			t.Fatalf("expected alice add host status %d got %d: %s", fiber.StatusOK, status, resp)
		}

		if status, _, _, _ := doRequest("POST", "/api/bookings/cart/hosts", addHostPayload, "application/json", bobCookies); status != fiber.StatusConflict {
			t.Fatalf("expected conflict for bob adding reserved host, got status %d", status)
		}
	})

	t.Run("Booking request uses cart hosts and frees reservation", func(t *testing.T) {
		db.ResetBookingCart("alice")
		db.ResetBookingCart("bob")

		aliceCookies := login("alice")
		bobCookies := login("bob")

		if status, _, _, _ := doRequest("GET", "/api/auth/me", "", "", aliceCookies); status != fiber.StatusOK {
			t.Fatalf("expected alice auth check 200 got %d", status)
		}

		// Create booking
		body := `{"name":"Request Booking","description":"desc","duration_days":1}`
		var booking db.Booking
		if status, resp, _, err := doRequest("POST", "/api/bookings", body, "application/json", aliceCookies); err != nil {
			t.Fatalf("create booking failed: %v", err)
		} else if status != fiber.StatusOK {
			t.Fatalf("expected create booking status %d got %d: %s", fiber.StatusOK, status, resp)
		} else if err := json.Unmarshal([]byte(resp), &booking); err != nil {
			t.Fatalf("failed to unmarshal booking: %v", err)
		}

		// Add host to cart
		addHostPayload := fmt.Sprintf(`{"management_ip":"%s"}`, testHostIP)
		if status, _, _, err := doRequest("POST", "/api/bookings/cart/hosts", addHostPayload, "application/json", aliceCookies); err != nil || status != fiber.StatusOK {
			t.Fatalf("alice failed to add host to cart status %d err %v", status, err)
		}

		requestPayload := `{"justification":"need resources","containers":[{"name":"ct1","template":"ubuntu","cores":2,"memory_mb":512,"disk_gb":10}],"vms":[{"name":"vm1","iso_selection":"", "cores":2,"memory_mb":1024,"disk_gb":20}]}`
		if status, resp, _, err := doRequest("POST", fmt.Sprintf("/api/bookings/%d/requests", booking.ID), requestPayload, "application/json", aliceCookies); err != nil {
			t.Fatalf("create booking request failed: %v", err)
		} else if status != fiber.StatusOK {
			t.Fatalf("expected create request status %d got %d: %s", fiber.StatusOK, status, resp)
		} else {
			var request db.BookingRequest
			if err := json.Unmarshal([]byte(resp), &request); err != nil {
				t.Fatalf("failed to unmarshal booking request: %v", err)
			}

			if len(request.Hosts) != 1 || request.Hosts[0].ManagementIP != testHostIP {
				t.Fatalf("expected request to include reserved host %s", testHostIP)
			}
		}

		// After request, cart should be cleared and host available for bob
		if status, _, _, _ := doRequest("POST", "/api/bookings/cart/hosts", addHostPayload, "application/json", bobCookies); status != fiber.StatusOK {
			t.Fatalf("expected host to be free for bob after request, got status %d", status)
		}
	})
}
