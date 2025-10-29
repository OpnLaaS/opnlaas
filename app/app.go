package app

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
	"github.com/opnlaas/laas/config"
)

func StartApp() error {
	var templateEngine = html.New("./public/views", ".html")
	templateEngine.Reload(config.Config.WebServer.ReloadTemplatesOnEachRender)

	var app = fiber.New(fiber.Config{
		Views: templateEngine,
	})

	// Pages
	app.Static("/static", "./public/static")

	app.Get("/", showLanding)
	app.Get("/login", showLogin)
	app.Get("/logout", showLogout)
	app.Get("/dashboard", mustBeLoggedIn, showDashboard)

	// API
	app.Post("/api/auth/login", apiLogin)
	app.Post("/api/auth/logout", mustBeLoggedIn, apiLogout)

	if config.Config.WebServer.TlsDir != "" {
		return app.ListenTLS(config.Config.WebServer.Address, config.Config.WebServer.TlsDir+"/fullchain.pem", config.Config.WebServer.TlsDir+"/privkey.pem")
	}

	return app.Listen(config.Config.WebServer.Address)
}
