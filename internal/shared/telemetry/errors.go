package telemetry

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

// exportErrorLogInterval is how often a failing exporter may reach the log.
const exportErrorLogInterval = time.Minute

// rateLimitedErrorHandler implements otel.ErrorHandler. A backend that is down
// makes every export attempt fail; logging each failure would flood the logs,
// so at most one is written per interval and the rest are only counted.
type rateLimitedErrorHandler struct {
	log      *zap.Logger
	interval time.Duration
	now      func() time.Time

	mu         sync.Mutex
	last       time.Time
	suppressed int
}

func newErrorHandler(log *zap.Logger, interval time.Duration) *rateLimitedErrorHandler {
	return &rateLimitedErrorHandler{log: log, interval: interval, now: time.Now}
}

func (h *rateLimitedErrorHandler) Handle(err error) {
	h.mu.Lock()
	now := h.now()
	if !h.last.IsZero() && now.Sub(h.last) < h.interval {
		h.suppressed++
		h.mu.Unlock()
		return
	}
	suppressed := h.suppressed
	h.suppressed = 0
	h.last = now
	h.mu.Unlock()

	h.log.Warn("telemetry error",
		zap.Error(err),
		zap.Int("suppressed_since_last_log", suppressed),
	)
}
