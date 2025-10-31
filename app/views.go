package app

import (
	"github.com/gofiber/fiber/v2"
	"github.com/opnlaas/laas/auth"
)

func showLanding(c *fiber.Ctx) error {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)

	if user != nil {
		return c.Redirect("/dashboard")
	}

	return c.Render("landing", bindWithLocals(c, fiber.Map{"Title": "Welcome"}), "layout")
}

func showLogin(c *fiber.Ctx) error {
	return c.Render("login", fiber.Map{
		"Title": "Login",
	}, "layout")
}

func showLogout(c *fiber.Ctx) error {
	c.ClearCookie("Authorization")
	return c.Redirect("/login")
}

func showDashboard(c *fiber.Ctx) (err error) {
	var (
		user        *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)
		displayName string
	)

	if user != nil {
		if displayName, err = user.LDAPConn.DisplayName(); err != nil {
			return c.Render("dashboard", bindWithLocals(c, fiber.Map{
				"Title": "Dashboard",
				"User":  user.LDAPConn.Username,
				"Error": err.Error(),
			}), "layout")
		}
	} else {
		displayName = "Guest"
	}

	return c.Render("dashboard", bindWithLocals(c, fiber.Map{
		"Title":    "Dashboard",
		"User":     displayName,
		"LoggedIn": user != nil,
		"IsAdmin":  user != nil && user.Permissions() >= auth.AuthPermsAdministrator,
	}), "layout")
}
