package app

import (
	"github.com/gofiber/fiber/v2"
	"github.com/opnlaas/opnlaas/auth"
)

func showLanding(c *fiber.Ctx) error {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)

	return c.Render("landing", bindWithLocals(c, fiber.Map{
		"Title":    "Welcome",
		"IsAdmin":  user != nil && user.Permissions() >= auth.AuthPermsAdministrator,
		"LoggedIn": user != nil}),
		"layout")
}

func showLogin(c *fiber.Ctx) error {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
	return c.Render("login", fiber.Map{
		"Title":    "Login",
		"LoggedIn": user != nil,
	}, "layout")
}

func showLogout(c *fiber.Ctx) error {
	c.ClearCookie("Authorization")
	return c.Redirect("/login")
}

func showAdmin(c *fiber.Ctx) error {
	var (
		user        *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
		displayName string         = "Guest"
		username    string
	)
	
	if user != nil || !(user.Permissions() >= auth.AuthPermsAdministrator){
		c.Redirect("/")
	}
	return c.Render("admin", fiber.Map{
		"LoggedIn": user != nil,
		"IsAdmin":  user != nil && user.Permissions() >= auth.AuthPermsAdministrator,
		"User":     displayName,
		"Username": username,
	})
}

func showDashboard(c *fiber.Ctx) (err error) {
	var (
		user        *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
		displayName string         = "Guest"
		username    string
	)

	if user != nil {
		username = user.Username
		displayName = user.Username

		if user.LDAPConn != nil {
			if displayName, err = user.LDAPConn.DisplayName(); err != nil {
				return c.Render("dashboard", bindWithLocals(c, fiber.Map{
					"Title":    "Dashboard",
					"User":     user.Username,
					"LoggedIn": user != nil,
					"Error":    err.Error(),
				}), "layout")
			}
		}
	}

	return c.Render("dashboard", bindWithLocals(c, fiber.Map{
		"Title":    "Dashboard",
		"User":     displayName,
		"Username": username,
		"LoggedIn": user != nil,
		"IsAdmin":  user != nil && user.Permissions() >= auth.AuthPermsAdministrator,
	}), "layout")
}

func showHosts(c *fiber.Ctx) (err error) {
	var (
		user        *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
		displayName string         = "Guest"
		username    string
	)

	if user != nil {
		username = user.Username
		displayName = user.Username

		if user.LDAPConn != nil {
			if displayName, err = user.LDAPConn.DisplayName(); err != nil {
				return c.Render("hosts", bindWithLocals(c, fiber.Map{
					"Title":    "Hosts",
					"User":     user.Username,
					"LoggedIn": user != nil,
					"Error":    err.Error(),
				}), "layout")
			}
		}
	}

	return c.Render("hosts", bindWithLocals(c, fiber.Map{
		"Title":    "Hosts",
		"User":     displayName,
		"Username": username,
		"LoggedIn": user != nil,
		"IsAdmin":  user != nil && user.Permissions() >= auth.AuthPermsAdministrator,
	}), "layout")
}
