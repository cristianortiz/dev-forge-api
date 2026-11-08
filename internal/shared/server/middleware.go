package server

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	sharedlogger "github.com/cristianortiz/dev-forge/internal/shared/logger"
)

const traceContextLocal = "otel_span_context"

// accessLogger writes one summary line per request with the status the client
// receives and, when tracing is on, the trace ID. It must be mounted outside the
// OTEL middleware (see New): that middleware answers handler errors itself and
// sets X-Trace-Id when it finishes. Without it (telemetry disabled), an error
// returned by the handler chain is mapped to its status here, the same way
// defaultErrorHandler does.
//
// The error detail is not repeated here; defaultErrorHandler logs it once.
func accessLogger(logger *zap.Logger) fiber.Handler {
	return func(c fiber.Ctx) (err error) {
		start := time.Now()

		// A panic skips the code after c.Next(); log it as the 500 the recover
		// middleware will send, then let it continue to that middleware.
		defer func() {
			if r := recover(); r != nil {
				logRequest(logger, c, start, fiber.StatusInternalServerError)
				panic(r)
			}
		}()

		err = c.Next()

		status := c.Response().StatusCode()
		if err != nil {
			status = statusFromError(err)
		}
		logRequest(logger, c, start, status)
		return err
	}
}

func logRequest(logger *zap.Logger, c fiber.Ctx, start time.Time, status int) {
	level := zapcore.InfoLevel
	switch {
	case status >= fiber.StatusInternalServerError:
		level = zapcore.ErrorLevel
	case status >= fiber.StatusBadRequest:
		level = zapcore.WarnLevel
	}

	ctx := c.Context()
	if spanContext, ok := c.Locals(traceContextLocal).(trace.SpanContext); ok {
		ctx = trace.ContextWithSpanContext(ctx, spanContext)
	}
	entry := sharedlogger.WithContext(logger, ctx).Check(level, "request")
	if entry == nil {
		return
	}
	fields := []zap.Field{
		zap.String("method", c.Method()),
		zap.String("route", requestRoute(c)),
		zap.Int("status", status),
		zap.Duration("latency", time.Since(start)),
		zap.String("request_id", c.GetRespHeader(requestIDHeader)),
	}
	entry.Write(fields...)
}

// requestRoute logs the route template, never user-controlled path segments.
func requestRoute(c fiber.Ctx) string {
	if route := c.Route(); route != nil {
		return route.Path
	}
	return ""
}

// statusFromError returns the HTTP status for an error returned by a handler.
func statusFromError(err error) int {
	var e *fiber.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return fiber.StatusInternalServerError
}

// defaultErrorHandler converts errors into a consistent JSON response and logs
// the error detail once. Server errors are logged at error level; client errors
// are already summarised by the access log, so their detail is only debug.
func defaultErrorHandler(logger *zap.Logger) fiber.ErrorHandler {
	return func(c fiber.Ctx, err error) error {
		code := statusFromError(err)
		message := "internal server error"

		var e *fiber.Error
		if errors.As(err, &e) {
			message = e.Message
		}

		if entry := sharedlogger.WithContext(logger, c.Context()).Check(errorLogLevel(code), "handler error"); entry != nil {
			entry.Write(
				zap.Int("status", code),
				zap.String("route", requestRoute(c)),
				zap.Error(err),
			)
		}

		return c.Status(code).JSON(fiber.Map{
			"error":      message,
			"request_id": c.GetRespHeader(requestIDHeader),
		})
	}
}

func errorLogLevel(status int) zapcore.Level {
	if status >= fiber.StatusInternalServerError {
		return zapcore.ErrorLevel
	}
	return zapcore.DebugLevel
}
