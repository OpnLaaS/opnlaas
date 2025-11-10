package app

import (
	"crypto/rand"
	"maps"

	"github.com/gofiber/fiber/v2"
	"github.com/opnlaas/opnlaas/auth"
	"github.com/z46-dev/go-logger"
)

var (
	jwtSigningKey []byte         = make([]byte, 64)
	appLog        *logger.Logger = logger.NewLogger().SetPrefix("[APPL]", logger.BoldGreen)
)

func init() {
	if _, err := rand.Read(jwtSigningKey); err != nil {
		appLog.Errorf("failed to generate JWT signing key: %v\n", err)
		panic(err)
	}
}

// TO DO - Add variation for api endpoints,

func apiMustBeLoggedIn(c *fiber.Ctx) error {
	if auth.IsAuthenticated(c, jwtSigningKey) == nil {
		return c.SendStatus(401)
	}

	return c.Next()
}

func apiMustBeAdmin(c *fiber.Ctx) error {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)

	if user == nil || user.Permissions() < auth.AuthPermsAdministrator {
		return c.SendStatus(403)
	}

	return c.Next()
}

func routesMustBeLoggedIn(c *fiber.Ctx) error {
	if auth.IsAuthenticated(c, jwtSigningKey) == nil {
		return c.Redirect("/login")
	}

	return c.Next()
}

func routesMustBeAdmin(c *fiber.Ctx) error {
	var user *auth.AuthUser = auth.IsAuthenticated(c, jwtSigningKey)

	if user == nil || user.Permissions() < auth.AuthPermsAdministrator {
		c.Locals("Error", "You do not have permission to access that page.")
	}

	return c.Next()
}

func bindWithLocals(c *fiber.Ctx, binds fiber.Map) (out fiber.Map) {
	out = fiber.Map{}

	if errMsg := c.Locals("Error"); errMsg != nil {
		out["Error"] = errMsg
	}

	maps.Copy(out, binds)

	return
}
