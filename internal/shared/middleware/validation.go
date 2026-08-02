package middleware

import (
	"github.com/gofiber/fiber/v2"

	"github.com/cristianortiz/dev-forge/internal/shared/validator"
)

// bodyKey is the c.Locals key used by ValidateBody / GetBody.
type bodyContextKey string

const bodyKey bodyContextKey = "validated_body"

// ValidateBody returns a Fiber middleware that parses the JSON request body into T
// and validates it using `validate` struct tags.
//
// On success the validated value is stored in c.Locals and c.Next() is called.
// On failure the chain is short-circuited with:
//   - 400 Bad Request  — malformed JSON
//   - 422 Unprocessable Entity — validation errors (e.g. "name: required; language: required")
//
// Usage in router:
//
//	templates.Post("/", middleware.ValidateBody[ports.CreateTemplateInput](), h.Create)
//	templates.Put("/:id", middleware.ValidateBody[ports.UpdateTemplateInput](), h.Update)
func ValidateBody[T any]() fiber.Handler {
	return func(c *fiber.Ctx) error {
		var input T
		if err := c.BodyParser(&input); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
		}
		if err := validator.Struct(input); err != nil {
			return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
		}
		c.Locals(bodyKey, input)
		return c.Next()
	}
}

// GetBody retrieves the validated body stored by ValidateBody[T].
// Must be called inside a handler that was preceded by ValidateBody[T] on the same route.
func GetBody[T any](c *fiber.Ctx) T {
	return c.Locals(bodyKey).(T)
}
