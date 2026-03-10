package app

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	recovermw "github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/template/html/v2"
	"github.com/opnlaas/opnlaas/config"
)

func CreateApp() (app *fiber.App) {
	var templateEngine = html.New("./public/views", ".html")
	templateEngine.Reload(config.Config.WebServer.ReloadTemplatesOnEachRender)

	app = fiber.New(fiber.Config{
		Views:        templateEngine,
		BodyLimit:    10 * 1024 * 1024 * 1024, // i think thats 10 GB
		ErrorHandler: appErrorHandler,
	})
	app.Use(recovermw.New())
	app.Use(func(c *fiber.Ctx) error {
		if c.Method() == fiber.MethodPost && c.Path() == "/api/iso-images" {
			started := time.Now()
			appLog.Basicf("iso upload request start remote=%s content_length=%d\n", c.IP(), c.Request().Header.ContentLength())
			err := c.Next()
			if err != nil {
				appLog.Errorf("iso upload request failed remote=%s duration=%s error=%v", c.IP(), time.Since(started), err)
				return err
			}
			appLog.Basicf("iso upload request end remote=%s status=%d duration=%s\n", c.IP(), c.Response().StatusCode(), time.Since(started))
			return nil
		}

		return c.Next()
	})

	// Pages
	app.Static("/static", "./public/static")

	app.Get("/", showLanding)
	app.Get("/login", showLogin)
	app.Get("/logout", routesMustBeLoggedIn, showLogout)
	app.Get("/dashboard", routesMustBeLoggedIn, showDashboard)
	app.Get("/hosts", showHosts)

	// Auth API
	app.Post("/api/auth/login", apiLogin)
	app.Get("/api/auth/me", apiMustBeLoggedIn, apiAuthMe)
	app.Post("/api/auth/logout", apiMustBeLoggedIn, apiLogout)

	// Enums API
	app.Get("/api/enums/vendors", apiEnumsVendorNames)
	app.Get("/api/enums/form-factors", apiEnumsFormFactorNames)
	app.Get("/api/enums/management-types", apiEnumsManagementTypeNames)
	app.Get("/api/enums/power-states", apiEnumsPowerStateNames)
	app.Get("/api/enums/boot-modes", apiEnumsBootModeNames)
	app.Get("/api/enums/power-actions", apiEnumsPowerActionNames)
	app.Get("/api/enums/architectures", apiEnumsArchitectureNames)
	app.Get("/api/enums/distro-types", apiEnumsDistroTypeNames)
	app.Get("/api/enums/preconfigure-types", apiEnumsPreConfigureTypeNames)
	app.Get("/api/enums/booking-permission-levels", apiEnumsBookingPermissionLevelNames)
	app.Get("/api/enums/booking-statuses", apiEnumsBookingStatusNames)
	app.Get("/api/enums/booking-request-statuses", apiEnumsBookingRequestStatusNames)

	// Hosts API
	app.Get("/api/hosts", apiHostsAll)
	app.Get("/api/hosts/:management_ip", apiHostByManagementIP)
	app.Get("/api/operations/:operation_id", apiMustBeLoggedIn, apiAsyncOperationByID)
	app.Post("/api/hosts", apiMustBeLoggedIn, apiMustBeAdmin, apiHostCreate)
	app.Delete("/api/hosts/:management_ip", apiMustBeLoggedIn, apiMustBeAdmin, apiHostDelete)
	app.Post("/api/hosts/:management_ip/reprobe-system-info", apiMustBeLoggedIn, apiMustBeAdmin, apiHostReprobeSystemInfo)
	app.Post("/api/hosts/:management_ip/power/:action", apiMustBeLoggedIn, apiHostPowerControl)

	// ISO Images API
	app.Post("/api/iso-images", apiMustBeLoggedIn, apiMustBeAdmin, apiISOImagesCreate)
	app.Get("/api/iso-images", apiMustBeLoggedIn, apiISOImagesList)
	app.Delete("/api/iso-images/:iso_name", apiMustBeLoggedIn, apiMustBeAdmin, apiISOImagesDelete)

	// Booking API
	app.Post("/api/bookings", apiMustBeLoggedIn, apiBookingCreate)
	app.Get("/api/bookings", apiMustBeLoggedIn, apiBookingList)
	app.Get("/api/bookings/me", apiMustBeLoggedIn, apiBookingMyList)
	app.Get("/api/bookings/network/prefill", apiMustBeLoggedIn, apiBookingNetworkPrefill)
	app.Post("/api/bookings/deploy", apiMustBeLoggedIn, apiBookingDeploy)
	app.Get("/api/bookings/cart", apiMustBeLoggedIn, apiBookingCartSnapshot)
	app.Post("/api/bookings/cart/network", apiMustBeLoggedIn, apiBookingCartSetNetwork)
	app.Post("/api/bookings/cart/hosts", apiMustBeLoggedIn, apiBookingCartAddHost)
	app.Delete("/api/bookings/cart/hosts/:management_ip", apiMustBeLoggedIn, apiBookingCartRemoveHost)
	app.Get("/api/bookings/cart/counts", apiMustBeLoggedIn, apiBookingCartCounts)
	app.Get("/api/bookings/cart/hosts/available", apiMustBeLoggedIn, apiBookingCartAvailableHosts)
	app.Get("/api/bookings/:booking_id", apiMustBeLoggedIn, apiBookingByID)
	app.Get("/api/bookings/:booking_id/provisioning", apiMustBeLoggedIn, apiBookingProvisioningStatus)
	app.Post("/api/bookings/:booking_id/provisioning/cancel", apiMustBeLoggedIn, apiBookingProvisioningCancel)
	app.Delete("/api/bookings/:booking_id", apiMustBeLoggedIn, apiBookingDestroy)
	app.Post("/api/bookings/:booking_id/requests", apiMustBeLoggedIn, apiBookingCreateRequest)

	return
}

func appErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		code = fiberErr.Code
	}

	appLog.Errorf("request error method=%s path=%s status=%d remote=%s err=%v\n", c.Method(), c.OriginalURL(), code, c.IP(), err)

	if strings.HasPrefix(c.Path(), "/api/") {
		return c.Status(code).JSON(fiber.Map{"message": err.Error()})
	}

	return c.Status(code).SendString(err.Error())
}

func discoverTLSKeys(dir string) (certPath, keyPath string, found bool) {
	type Candidate struct {
		cert string
		key  string
	}

	candidates := []Candidate{
		{"fullchain.pem", "privkey.pem"},
		{"cert.pem", "key.pem"},
		{"tls.crt", "tls.key"},
		{"server.crt", "server.key"},
		{"webserver.crt", "webserver.key"},
	}

	for _, c := range candidates {
		certPath = filepath.Join(dir, c.cert)
		keyPath = filepath.Join(dir, c.key)

		if fileExists(certPath) && fileExists(keyPath) {
			return certPath, keyPath, true
		}
	}

	var crtFiles, keyFiles []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", "", false
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".crt") {
			crtFiles = append(crtFiles, filepath.Join(dir, name))
		}
		if strings.HasSuffix(name, ".key") {
			keyFiles = append(keyFiles, filepath.Join(dir, name))
		}
	}

	if len(crtFiles) > 0 && len(keyFiles) > 0 {
		return crtFiles[0], keyFiles[0], true
	}

	return "", "", false
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func StartApp() (err error) {
	var app *fiber.App = CreateApp()

	if len(config.Config.WebServer.RedirectServerAddresses) > 0 && len(config.Config.WebServer.RedirectServerAddresses[0]) > 0 {
		for _, redirectAddress := range config.Config.WebServer.RedirectServerAddresses {
			runHttpRedirectServer(redirectAddress, config.Config.WebServer.Address, config.Config.WebServer.TLSDir != "")
		}
	}

	if config.Config.WebServer.TLSDir != "" {
		var (
			certPath, keyPath string
			found             bool
		)

		if certPath, keyPath, found = discoverTLSKeys(config.Config.WebServer.TLSDir); !found {
			err = fiber.ErrInternalServerError
			return
		}

		err = app.ListenTLS(config.Config.WebServer.Address, certPath, keyPath)
		return
	}

	err = app.Listen(config.Config.WebServer.Address)
	return
}

func runHttpRedirectServer(address string, targetAddress string, useTLS bool) (err error) {
	var (
		redirectApp *fiber.App = fiber.New()
		targetPort  string
	)

	if _, targetPort, err = net.SplitHostPort(targetAddress); err != nil {
		return
	}

	redirectApp.Use(func(c *fiber.Ctx) error {
		var targetScheme, host string = "http", net.JoinHostPort(c.Hostname(), targetPort)

		if useTLS {
			targetScheme = "https"
		}

		if (useTLS && targetPort == "443") || (!useTLS && targetPort == "80") {
			host = c.Hostname()
		}

		return c.Redirect(targetScheme+"://"+host+c.OriginalURL(), fiber.StatusMovedPermanently)
	})

	go func() {
		if err := redirectApp.Listen(address); err != nil {
			panic(err)
		}
	}()

	return
}
