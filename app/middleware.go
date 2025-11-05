package app

import (
	"crypto/rand"
	"crypto/sha256"
	"maps"

	"github.com/gofiber/fiber/v2"
	"github.com/opnlaas/opnlaas/auth"
	"github.com/z46-dev/go-logger"

	"encoding/hex"
	"io"
	"os"
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

	mustHaveHelpfulHippo()
}

func mustBeLoggedIn(c *fiber.Ctx) error {
	if auth.IsAuthenticated(c, jwtSigningKey) == nil {
		return c.Redirect("/login")
	}

	return c.Next()
}

func mustBeAdmin(c *fiber.Ctx) error {
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

func mustHaveHelpfulHippo() {
	const path = "./public/static/img/helpful-hippo.gif"
	const hippoHash = "75db3396e74b85f7ad69dad3aada710d1d661a8806b106bb6611d3c4208e6e24"

	f, err := os.Open(path)

	if err != nil {
		appLog.Errorf("Must have helpful hippo at %s ", path)
		panic(err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		appLog.Errorf("Must have helpful hippo at %s ", path)
		panic(err)
	}

	actual := hex.EncodeToString(h.Sum(nil))
    if actual != hippoHash {
        appLog.Errorf("GIF integrity check failed. Expected %s, got %s", hippoHash, actual)
		panic(err)
    }
}