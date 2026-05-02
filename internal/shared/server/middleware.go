package server

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// zapLogger returns a Fiber middleware that logs each request with zap.
func zapLogger(logger *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		duration := time.Since(start)

		fields := []zap.Field{
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.Int("status", c.Response().StatusCode()),
			zap.Duration("latency", duration),
			zap.String("ip", c.IP()),
			zap.String("request_id", c.GetRespHeader("X-Request-Id")),
		}

		if err != nil {
			logger.Error("request error", append(fields, zap.Error(err))...)
		} else {
			logger.Info("request", fields...)
		}

		return err
	}
}

// defaultErrorHandler converts errors into a consistent JSON response.
func defaultErrorHandler(logger *zap.Logger) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		message := "internal server error"

		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
			message = e.Message
		}

		logger.Error("handler error",
			zap.Int("status", code),
			zap.String("path", c.Path()),
			zap.Error(err),
		)

		return c.Status(code).JSON(fiber.Map{
			"error":      message,
			"request_id": c.GetRespHeader("X-Request-Id"),
		})
	}
}
