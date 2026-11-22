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

const (
	traceContextLocal = "otel_span_context"
	errorCodeLocal    = "observability_error_code"
	errorMessageLocal = "observability_error_message"
	errorCauseLocal   = "observability_error_cause"
)

// SetErrorCode attaches a stable, low-cardinality code to the request log.
func SetErrorCode(c fiber.Ctx, code string) {
	if code != "" {
		c.Locals(errorCodeLocal, code)
	}
}

// SetErrorDetails adds the underlying cause to the single access log for a failed request.
func SetErrorDetails(c fiber.Ctx, code string, cause error) {
	SetErrorCode(c, code)
	if cause != nil {
		c.Locals(errorCauseLocal, cause)
	}
}

// accessLogger writes one summary line per request with the status the client
// receives and, when tracing is on, the trace ID. It must be mounted outside the
// OTEL middleware (see New): that middleware answers handler errors itself and
// sets X-Trace-Id when it finishes. Without it (telemetry disabled), an error
// returned by the handler chain is mapped to its status here, the same way
// defaultErrorHandler does.
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
			captureErrorDetails(c, err)
		}
		logRequest(logger, c, start, status)
		return err
	}
}

func captureErrorDetails(c fiber.Ctx, err error) {
	if _, ok := c.Locals(errorMessageLocal).(string); !ok {
		message := "internal server error"
		var fiberErr *fiber.Error
		if errors.As(err, &fiberErr) {
			message = fiberErr.Message
		}
		c.Locals(errorMessageLocal, message)
	}
	if _, ok := c.Locals(errorCauseLocal).(error); !ok {
		c.Locals(errorCauseLocal, err)
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
	if status >= fiber.StatusBadRequest {
		if message, ok := c.Locals(errorMessageLocal).(string); ok {
			fields = append(fields, zap.String("error_message", message))
		}
		if errorCode, ok := c.Locals(errorCodeLocal).(string); ok {
			fields = append(fields, zap.String("error_code", errorCode))
		}
		if cause, ok := c.Locals(errorCauseLocal).(error); ok {
			fields = append(fields, zap.Error(cause))
		}
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

// defaultErrorHandler converts errors into a consistent JSON response and stores
// safe details for accessLogger to emit once with the final request status.
func defaultErrorHandler() fiber.ErrorHandler {
	return func(c fiber.Ctx, err error) error {
		code := statusFromError(err)
		message := "internal server error"

		var e *fiber.Error
		if errors.As(err, &e) {
			message = e.Message
		}
		c.Locals(errorMessageLocal, message)
		if _, ok := c.Locals(errorCauseLocal).(error); !ok {
			c.Locals(errorCauseLocal, err)
		}

		return c.Status(code).JSON(fiber.Map{
			"error":      message,
			"request_id": c.GetRespHeader(requestIDHeader),
		})
	}
}
