package app

import (
	"github.com/gofiber/fiber/v2"
	"github.com/opnlaas/laas/auth"
	"github.com/opnlaas/laas/hosts"
)

func apiLogin(c *fiber.Ctx) (err error) {
	var (
		username, password string = c.FormValue("username"), c.FormValue("password")
		user               *auth.AuthUser
		token              string
	)

	if user, err = auth.Authenticate(username, password); err == nil {
		if token, err = user.Token.SignedString(jwtSigningKey); err == nil {
			c.Cookie(&fiber.Cookie{
				Name:  "Authorization",
				Value: token,
			})

			return c.Redirect("/dashboard")
		}
	}

	return c.Render("login", fiber.Map{
		"Title":      "Login",
		"LoginError": err.Error(),
	}, "layout")
}

func apiLogout(c *fiber.Ctx) (err error) {
	c.ClearCookie("Authorization")
	return
}

// Enums API

func apiEnumsVendorNames(c *fiber.Ctx) (err error) {
	return c.JSON(hosts.VendorNameReverses)
}

func apiEnumsFormFactorNames(c *fiber.Ctx) (err error) {
	return c.JSON(hosts.FormFactorNameReverses)
}

func apiEnumsManagementTypeNames(c *fiber.Ctx) (err error) {
	return c.JSON(hosts.ManagementTypeNameReverses)
}

func apiEnumsPowerStateNames(c *fiber.Ctx) (err error) {
	return c.JSON(hosts.PowerStateNameReverses)
}

func apiEnumsBootModeNames(c *fiber.Ctx) (err error) {
	return c.JSON(hosts.BootModeNameReverses)
}
