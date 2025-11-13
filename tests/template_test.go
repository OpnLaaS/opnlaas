package tests

import (
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestTemplate(t *testing.T) {
	setup(t)
	defer cleanup(t)

	t.Log("This is a template test.")
}

func TestTemplateWithWebApp(t *testing.T) {
	setup(t)
	defer cleanup(t)

	var app *fiber.App = setupAppServer(t)
	defer cleanupAppServer(t, app)

	t.Log("This is a template test with web app.")
}
