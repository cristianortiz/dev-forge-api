package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/cristianortiz/dev-forge/internal/shared/config"
)

const durationMetric = "http.server.request.duration"

// harness wires a server to in-memory span and metric collectors.
type harness struct {
	app     *fiber.App
	spans   *tracetest.SpanRecorder
	metrics *sdkmetric.ManualReader
	logs    *observer.ObservedLogs
}

// newHarness builds the real server with the global providers replaced by
// in-memory ones, and registers routes that stand in for a handler calling a
// repository. The OTEL middleware reads the globals in server.New, so they are
// installed first.
func newHarness(t *testing.T, telemetryEnabled bool) *harness {
	t.Helper()

	spans := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	t.Cleanup(func() {
		otel.SetTracerProvider(tracenoop.NewTracerProvider())
		otel.SetMeterProvider(metricnoop.NewMeterProvider())
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
	})

	core, logs := observer.New(zapcore.DebugLevel)
	cfg := &config.Config{Environment: "test", OTEL: config.OTELConfig{Enabled: telemetryEnabled}}
	srv := New(cfg, zap.New(core))
	RegisterHealthRoutes(srv.App)

	srv.App.Get("/items/:id", func(c fiber.Ctx) error {
		// What a repository does: start a child span from the request context.
		_, span := otel.GetTracerProvider().Tracer("repo").Start(c.Context(), "SELECT")
		span.End()
		return c.JSON(fiber.Map{"id": c.Params("id")})
	})
	srv.App.Get("/missing", func(c fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotFound, "no such thing")
	})
	srv.App.Get("/fail", func(c fiber.Ctx) error {
		return errors.New("database is down")
	})
	srv.App.Get("/panic", func(c fiber.Ctx) error {
		panic("kaboom")
	})

	return &harness{app: srv.App, spans: spans, metrics: reader, logs: logs}
}

func (h *harness) get(t *testing.T, path string, headers ...string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := h.app.Test(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// serverSpan returns the single SERVER span recorded so far.
func (h *harness) serverSpan(t *testing.T) sdktrace.ReadOnlySpan {
	t.Helper()
	var found []sdktrace.ReadOnlySpan
	for _, s := range h.spans.Ended() {
		if s.SpanKind() == trace.SpanKindServer {
			found = append(found, s)
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected exactly one server span, got %d", len(found))
	}
	return found[0]
}

func attr(s sdktrace.ReadOnlySpan, key string) (attribute.Value, bool) {
	for _, kv := range s.Attributes() {
		if string(kv.Key) == key {
			return kv.Value, true
		}
	}
	return attribute.Value{}, false
}

// durationSeries returns the attribute sets of every http.server.request.duration series.
func (h *harness) durationSeries(t *testing.T) []attribute.Set {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := h.metrics.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	var series []attribute.Set
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != durationMetric {
				continue
			}
			hist, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("%s has unexpected data type %T", durationMetric, m.Data)
			}
			for _, dp := range hist.DataPoints {
				series = append(series, dp.Attributes)
			}
		}
	}
	return series
}

func (h *harness) requestLogs(t *testing.T) []observer.LoggedEntry {
	t.Helper()
	return h.logs.FilterMessage("request").All()
}

func TestOTEL_RoutedRequestProducesLinkedSpansAndMetrics(t *testing.T) {
	h := newHarness(t, true)

	resp := h.get(t, "/items/42")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	srvSpan := h.serverSpan(t)
	if srvSpan.Name() != "GET /items/:id" {
		t.Errorf("span name = %q, want the route template", srvSpan.Name())
	}
	if v, _ := attr(srvSpan, "http.route"); v.AsString() != "/items/:id" {
		t.Errorf("http.route = %q, want /items/:id", v.AsString())
	}
	if v, _ := attr(srvSpan, "http.response.status_code"); v.AsInt64() != 200 {
		t.Errorf("http.response.status_code = %d, want 200", v.AsInt64())
	}
	if srvSpan.Status().Code != 0 { // codes.Unset
		t.Errorf("span status = %v, want unset for a 200", srvSpan.Status().Code)
	}

	// The repository span must be a child of the request span, in the same trace.
	var child sdktrace.ReadOnlySpan
	for _, s := range h.spans.Ended() {
		if s.Name() == "SELECT" {
			child = s
		}
	}
	if child == nil {
		t.Fatal("repository span was not recorded")
	}
	if child.Parent().SpanID() != srvSpan.SpanContext().SpanID() {
		t.Error("repository span is not a child of the HTTP span (is the handler using c.Context()?)")
	}
	if child.SpanContext().TraceID() != srvSpan.SpanContext().TraceID() {
		t.Error("repository span belongs to a different trace")
	}

	// Clients get the trace ID, and nothing else about trace propagation.
	if got := resp.Header.Get(traceIDHeader); got != srvSpan.SpanContext().TraceID().String() {
		t.Errorf("%s = %q, want %q", traceIDHeader, got, srvSpan.SpanContext().TraceID())
	}
	for _, header := range []string{"Traceparent", "Tracestate", "Baggage"} {
		if got := resp.Header.Get(header); got != "" {
			t.Errorf("response leaks %s: %q", header, got)
		}
	}

	series := h.durationSeries(t)
	if len(series) != 1 {
		t.Fatalf("expected one duration series, got %d", len(series))
	}
	if v, _ := series[0].Value("http.route"); v.AsString() != "/items/:id" {
		t.Errorf("metric http.route = %q, want /items/:id", v.AsString())
	}
	if v, _ := series[0].Value("http.response.status_code"); v.AsInt64() != 200 {
		t.Errorf("metric status = %d, want 200", v.AsInt64())
	}
}

func TestOTEL_MetricCardinalityStaysBounded(t *testing.T) {
	h := newHarness(t, true)

	for i := range 30 {
		h.get(t, "/items/"+strings.Repeat("x", i+1))       // 30 different IDs
		h.get(t, "/no-such-route-"+strings.Repeat("y", i)) // 30 different unmatched paths
	}

	series := h.durationSeries(t)
	if len(series) != 2 {
		for _, s := range series {
			t.Logf("series: %s", s.Encoded(attribute.DefaultEncoder()))
		}
		t.Fatalf("got %d series, want 2 (one route, one unmatched)", len(series))
	}
	for _, s := range series {
		if v, ok := s.Value("url.path"); ok {
			t.Errorf("metric carries url.path=%q", v.AsString())
		}
	}
}

func TestOTEL_StatusMapping(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantStatus  int
		wantError   bool // span status Error
		wantNoRoute bool // nothing matched, so http.route must be absent
		wantLevel   zapcore.Level
	}{
		{"handler returns 404", "/missing", http.StatusNotFound, false, false, zapcore.WarnLevel},
		{"no route matches", "/nope", http.StatusNotFound, false, true, zapcore.WarnLevel},
		{"handler returns a plain error", "/fail", http.StatusInternalServerError, true, false, zapcore.ErrorLevel},
		{"handler panics and is recovered", "/panic", http.StatusInternalServerError, true, false, zapcore.ErrorLevel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, true)

			resp := h.get(t, tt.path)
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("response status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			s := h.serverSpan(t)
			if got := s.Status().Code.String(); (got == "Error") != tt.wantError {
				t.Errorf("span status = %s, want error=%v", got, tt.wantError)
			}
			if _, hasRoute := attr(s, "http.route"); hasRoute == tt.wantNoRoute {
				t.Errorf("http.route present = %v, want %v", hasRoute, !tt.wantNoRoute)
			}
			if v, ok := attr(s, "http.response.status_code"); ok && v.AsInt64() != int64(tt.wantStatus) {
				t.Errorf("span status code = %d, want %d", v.AsInt64(), tt.wantStatus)
			}

			logs := h.requestLogs(t)
			if len(logs) != 1 {
				t.Fatalf("expected one access log entry, got %d", len(logs))
			}
			entry := logs[0]
			if entry.Level != tt.wantLevel {
				t.Errorf("access log level = %s, want %s", entry.Level, tt.wantLevel)
			}
			if got := entry.ContextMap()["status"]; got != int64(tt.wantStatus) {
				t.Errorf("access log status = %v, want %d (the status sent to the client)", got, tt.wantStatus)
			}
			if got := entry.ContextMap()["trace_id"]; got != s.SpanContext().TraceID().String() {
				t.Errorf("access log trace_id = %v, want %s", got, s.SpanContext().TraceID())
			}
		})
	}
}

func TestOTEL_PanicIsRecordedOnSpan(t *testing.T) {
	h := newHarness(t, true)
	h.get(t, "/panic")

	s := h.serverSpan(t)
	var hasException bool
	for _, e := range s.Events() {
		hasException = hasException || e.Name == "exception"
	}
	if !hasException {
		t.Error("the panic was not recorded as an exception event on the span")
	}
}

func TestOTEL_ErrorDetailIsLoggedOnce(t *testing.T) {
	tests := []struct {
		path      string
		wantLevel zapcore.Level
	}{
		{"/fail", zapcore.ErrorLevel},
		{"/panic", zapcore.ErrorLevel},
		{"/missing", zapcore.DebugLevel}, // client errors are summarised by the access log
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			h := newHarness(t, true)
			h.get(t, tt.path)

			details := h.logs.FilterMessage("handler error").All()
			if len(details) != 1 {
				t.Fatalf("expected the error detail once, got %d entries", len(details))
			}
			if details[0].Level != tt.wantLevel {
				t.Errorf("detail level = %s, want %s", details[0].Level, tt.wantLevel)
			}
		})
	}
}

func TestOTEL_ProbesAreNotTraced(t *testing.T) {
	h := newHarness(t, true)

	for _, path := range []string{"/health", "/ready"} {
		if resp := h.get(t, path); resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, resp.StatusCode)
		}
	}

	if n := len(h.spans.Ended()); n != 0 {
		t.Errorf("probes produced %d spans", n)
	}
	if series := h.durationSeries(t); len(series) != 0 {
		t.Errorf("probes produced %d duration series", len(series))
	}
}

func TestOTEL_IncomingTraceContextIsLinkedNotContinued(t *testing.T) {
	h := newHarness(t, true)
	const callerTrace = "4bf92f3577b34da6a3ce929d0e0e4736"
	const callerSpan = "00f067aa0ba902b7"

	h.get(t, "/items/1", "traceparent", "00-"+callerTrace+"-"+callerSpan+"-01")

	s := h.serverSpan(t)
	if s.SpanContext().TraceID().String() == callerTrace {
		t.Error("the API continued a trace chosen by an untrusted caller")
	}
	if s.Parent().IsValid() {
		t.Error("the server span must be a new root")
	}
	links := s.Links()
	if len(links) != 1 || links[0].SpanContext.TraceID().String() != callerTrace {
		t.Errorf("expected one link to the caller's trace, got %v", links)
	}
}

func TestOTEL_ClientAddressIsNotRecorded(t *testing.T) {
	h := newHarness(t, true)
	h.get(t, "/items/1")

	s := h.serverSpan(t)
	for _, key := range []string{"client.address", "network.peer.address", "network.peer.port"} {
		if _, ok := attr(s, key); ok {
			t.Errorf("span records %s", key)
		}
	}
}

func TestAccessLogDoesNotExposeURLValuesOrClientIP(t *testing.T) {
	h := newHarness(t, true)
	resp := h.get(t, "/items/path-secret?access_token=query-secret")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	logs := h.requestLogs(t)
	if len(logs) != 1 {
		t.Fatalf("expected one access log entry, got %d", len(logs))
	}
	fields := logs[0].ContextMap()
	if got := fields["route"]; got != "/items/:id" {
		t.Errorf("logged route = %v, want route template /items/:id", got)
	}
	for _, key := range []string{"ip", "path"} {
		if _, ok := fields[key]; ok {
			t.Errorf("access log contains raw request field %q", key)
		}
	}
	for _, secret := range []string{"path-secret", "query-secret"} {
		if strings.Contains(logs[0].Message, secret) || strings.Contains(logs[0].ContextMap()["route"].(string), secret) {
			t.Errorf("access log contains URL value %q", secret)
		}
	}
}

func TestOTEL_DisabledServerStillLogsFinalStatus(t *testing.T) {
	tests := []struct {
		path       string
		wantStatus int64
	}{
		{"/items/1", 200},
		{"/missing", 404},
		{"/fail", 500},
		{"/panic", 500},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			h := newHarness(t, false)

			resp := h.get(t, tt.path)
			for _, s := range h.spans.Ended() {
				if s.SpanKind() == trace.SpanKindServer {
					t.Errorf("server span %q recorded although telemetry is disabled", s.Name())
				}
			}
			if resp.Header.Get(traceIDHeader) != "" {
				t.Errorf("%s set although telemetry is disabled", traceIDHeader)
			}

			logs := h.requestLogs(t)
			if len(logs) != 1 {
				t.Fatalf("expected one access log entry, got %d", len(logs))
			}
			if got := logs[0].ContextMap()["status"]; got != tt.wantStatus {
				t.Errorf("logged status = %v, want %d", got, tt.wantStatus)
			}
			if _, ok := logs[0].ContextMap()["trace_id"]; ok {
				t.Error("trace_id logged although telemetry is disabled")
			}
		})
	}
}

func TestCORS_ExposesCorrelationHeaders(t *testing.T) {
	h := newHarness(t, true)

	resp := h.get(t, "/items/1", "Origin", "https://ui.example.test")

	exposed := resp.Header.Get("Access-Control-Expose-Headers")
	for _, want := range []string{requestIDHeader, traceIDHeader} {
		if !strings.Contains(exposed, want) {
			t.Errorf("Access-Control-Expose-Headers = %q, missing %s", exposed, want)
		}
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatal(err)
	}
}

func TestErrorResponseKeepsItsJSONShape(t *testing.T) {
	h := newHarness(t, true)

	resp := h.get(t, "/missing")
	body, _ := io.ReadAll(resp.Body)

	if !strings.Contains(string(body), `"error":"no such thing"`) || !strings.Contains(string(body), `"request_id"`) {
		t.Errorf("unexpected error body: %s", body)
	}
	if got := resp.Header.Get(requestIDHeader); got == "" {
		t.Error("request id header missing")
	}
}
