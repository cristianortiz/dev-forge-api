package server

import (
	"github.com/gofiber/fiber/v2"
)

// RegisterHealthRoutes mounts the /health and /ready probes.
func RegisterHealthRoutes(app *fiber.App) {
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	app.Get("/ready", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ready"})
	})
}
