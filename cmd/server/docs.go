package main

import (
	"encoding/json"
	"fmt"

	"github.com/cristianortiz/dev-forge/docs/openapi"
	"github.com/gofiber/fiber/v3"
	"github.com/yokeTH/gofiber-scalar/scalar/v3"
	"gopkg.in/yaml.v3"
)

func registerDocsRoutes(api fiber.Router) error {
	var document map[string]any
	if err := yaml.Unmarshal(openapi.Spec, &document); err != nil {
		return fmt.Errorf("decode embedded OpenAPI contract: %w", err)
	}
	specJSON, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode OpenAPI contract for Scalar: %w", err)
	}

	serveSpec := func(c fiber.Ctx) error {
		c.Set(fiber.HeaderContentType, "application/yaml; charset=utf-8")
		return c.Send(openapi.Spec)
	}

	api.Get("/openapi.yaml", serveSpec)

	scalarUI := scalar.New(scalar.Config{
		Path:              "/api/v1/docs",
		RawSpecUrl:        "openapi.json",
		Title:             "Dev Forge API",
		FileContentString: string(specJSON),
		ForceOffline:      scalar.ForceOfflineTrue,
	})
	api.Get("/docs", scalarUI)
	api.Get("/docs/*", scalarUI)
	return nil
}
