package app

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
	"github.com/opnlaas/opnlaas/config"
)

func CreateApp() (app *fiber.App) {
	var templateEngine = html.New("./public/views", ".html")
	templateEngine.Reload(config.Config.WebServer.ReloadTemplatesOnEachRender)

	app = fiber.New(fiber.Config{
		Views: templateEngine,
	})

	// Pages
	app.Static("/static", "./public/static")

	app.Get("/", showLanding)
	app.Get("/login", showLogin)
	app.Get("/logout", showLogout)
	app.Get("/dashboard", mustBeLoggedIn, showDashboard)

	// Auth API
	app.Post("/api/auth/login", apiLogin)
	app.Post("/api/auth/logout", mustBeLoggedIn, apiLogout)

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

	// Hosts API
	app.Get("/api/hosts", apiHostsAll)
	app.Get("/api/hosts/:management_ip", apiHostByManagementIP)
	app.Post("/api/hosts", mustBeLoggedIn, mustBeAdmin, apiHostCreate)
	app.Delete("/api/hosts/:management_ip", mustBeLoggedIn, mustBeAdmin, apiHostDelete)
	app.Post("/api/hosts/:management_ip/power/:action", mustBeLoggedIn, mustBeAdmin, apiHostPowerControl)

	// ISO Images API
	app.Post("/api/iso-images", mustBeLoggedIn, mustBeAdmin, apiISOImagesCreate)
	return
}

func StartApp() (err error) {
	var app *fiber.App = CreateApp()
	if config.Config.WebServer.TlsDir != "" {
		err = app.ListenTLS(config.Config.WebServer.Address, config.Config.WebServer.TlsDir+"/fullchain.pem", config.Config.WebServer.TlsDir+"/privkey.pem")
		return
	}

	err = app.Listen(config.Config.WebServer.Address)
	return
}
