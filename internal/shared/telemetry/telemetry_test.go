package telemetry

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log"
	lognoop "go.opentelemetry.io/otel/log/noop"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/cristianortiz/dev-forge/internal/shared/config"
)

var otelEnvVars = []string{
	"OTEL_SERVICE_NAME", "OTEL_RESOURCE_ATTRIBUTES",
	"OTEL_TRACES_EXPORTER", "OTEL_METRICS_EXPORTER", "OTEL_LOGS_EXPORTER",
	"OTEL_EXPORTER_OTLP_PROTOCOL", "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL",
	"OTEL_EXPORTER_OTLP_METRICS_PROTOCOL", "OTEL_EXPORTER_OTLP_LOGS_PROTOCOL",
	"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
	"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT",
	"OTEL_TRACES_SAMPLER", "OTEL_TRACES_SAMPLER_ARG",
	"OTEL_METRIC_EXPORT_INTERVAL", "OTEL_METRIC_EXPORT_TIMEOUT",
}

// isolate clears every OTEL_* variable the package reads and restores the global
// providers afterwards, since Setup installs process-wide state.
func isolate(t *testing.T) {
	t.Helper()
	for _, name := range otelEnvVars {
		t.Setenv(name, "")
	}
	t.Cleanup(func() {
		otel.SetTracerProvider(tracenoop.NewTracerProvider())
		otel.SetMeterProvider(metricnoop.NewMeterProvider())
		otel.SetLoggerProvider(lognoop.NewLoggerProvider())
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
		otel.SetErrorHandler(otel.ErrorHandlerFunc(func(error) {}))
	})
}

func testConfig(enabled bool) *config.Config {
	return &config.Config{
		Environment: "development",
		OTEL: config.OTELConfig{
			Enabled:         enabled,
			ServiceName:     "dev-forge-test",
			ServiceVersion:  "1.2.3",
			Environment:     "development",
			ShutdownTimeout: time.Second,
		},
	}
}

// fakeCollector records what an OTLP/HTTP exporter sends.
type fakeCollector struct {
	*httptest.Server
	mu     sync.Mutex
	bodies map[string][]byte
}

func newFakeCollector(t *testing.T) *fakeCollector {
	t.Helper()
	c := &fakeCollector{bodies: map[string][]byte{}}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.bodies[r.URL.Path] = append(c.bodies[r.URL.Path], body...)
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(c.Close)
	return c
}

func (c *fakeCollector) body(path string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.bodies[path]
	return b, ok
}

func emitTelemetry(ctx context.Context) {
	_, span := otel.GetTracerProvider().Tracer("test").Start(ctx, "unit-span")
	span.End()

	var rec log.Record
	rec.SetBody(attribute.StringValue("unit-log"))
	otel.GetLoggerProvider().Logger("test").Emit(ctx, rec)
}

func TestSetup_DisabledIsNoOp(t *testing.T) {
	isolate(t)
	t.Setenv("OTEL_TRACES_SAMPLER", "invalid-but-ignored-when-disabled")
	tpBefore := otel.GetTracerProvider()
	core, logs := observer.New(zap.InfoLevel)

	shutdown, err := Setup(context.Background(), testConfig(false), zap.New(core))
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	if otel.GetTracerProvider() != tpBefore {
		t.Error("tracer provider changed although telemetry is disabled")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown() error = %v", err)
	}
	if logs.FilterMessage("telemetry disabled").Len() != 1 {
		t.Error("expected one 'telemetry disabled' log entry")
	}
}

func TestSetup_InvalidConfigurationFailsAtStartup(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want []string
	}{
		{"unknown traces exporter", map[string]string{"OTEL_TRACES_EXPORTER": "jaeger"}, []string{"OTEL_TRACES_EXPORTER"}},
		{"prometheus exporter is excluded", map[string]string{"OTEL_METRICS_EXPORTER": "prometheus"}, []string{"OTEL_METRICS_EXPORTER"}},
		{"exporter lists are unsupported", map[string]string{"OTEL_LOGS_EXPORTER": "otlp,console"}, []string{"OTEL_LOGS_EXPORTER"}},
		{"unknown protocol", map[string]string{"OTEL_EXPORTER_OTLP_PROTOCOL": "http/json"}, []string{"OTEL_EXPORTER_OTLP_PROTOCOL"}},
		{"unknown per-signal protocol", map[string]string{"OTEL_EXPORTER_OTLP_METRICS_PROTOCOL": "udp"}, []string{"OTEL_EXPORTER_OTLP_METRICS_PROTOCOL"}},
		{"endpoint without scheme", map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "localhost:4317"}, []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318"}},
		{"endpoint with wrong scheme", map[string]string{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "ftp://collector"}, []string{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"}},
		{"unknown sampler", map[string]string{"OTEL_TRACES_SAMPLER": "sometimes"}, []string{"OTEL_TRACES_SAMPLER"}},
		{"sampler ratio not a number", map[string]string{"OTEL_TRACES_SAMPLER": "traceidratio", "OTEL_TRACES_SAMPLER_ARG": "abc"}, []string{"OTEL_TRACES_SAMPLER_ARG"}},
		{"sampler ratio above one", map[string]string{"OTEL_TRACES_SAMPLER": "parentbased_traceidratio", "OTEL_TRACES_SAMPLER_ARG": "1.5"}, []string{"OTEL_TRACES_SAMPLER_ARG"}},
		{"metric interval not a number", map[string]string{"OTEL_METRIC_EXPORT_INTERVAL": "10s"}, []string{"OTEL_METRIC_EXPORT_INTERVAL"}},
		{"metric interval zero", map[string]string{"OTEL_METRIC_EXPORT_INTERVAL": "0"}, []string{"OTEL_METRIC_EXPORT_INTERVAL"}},
		{"metric timeout negative", map[string]string{"OTEL_METRIC_EXPORT_TIMEOUT": "-5"}, []string{"OTEL_METRIC_EXPORT_TIMEOUT"}},
		{"malformed resource attributes", map[string]string{"OTEL_RESOURCE_ATTRIBUTES": "no-equals-sign"}, []string{"OTEL_RESOURCE_ATTRIBUTES"}},
		{"all problems are reported together", map[string]string{"OTEL_TRACES_EXPORTER": "jaeger", "OTEL_TRACES_SAMPLER": "sometimes"}, []string{"OTEL_TRACES_EXPORTER", "OTEL_TRACES_SAMPLER"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			tpBefore := otel.GetTracerProvider()

			shutdown, err := Setup(context.Background(), testConfig(true), zap.NewNop())
			if err == nil {
				t.Fatal("Setup() expected an error, got nil")
			}
			if shutdown != nil {
				t.Error("Setup() returned a shutdown func together with an error")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
			if otel.GetTracerProvider() != tpBefore {
				t.Error("a failed Setup() must not install global providers")
			}
		})
	}
}

func TestSetup_ExportsAllSignalsToConfiguredEndpoint(t *testing.T) {
	isolate(t)
	collector := newFakeCollector(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")

	shutdown, err := Setup(context.Background(), testConfig(true), zap.NewNop())
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	emitTelemetry(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}

	for _, path := range []string{"/v1/traces", "/v1/metrics", "/v1/logs"} {
		body, ok := collector.body(path)
		if !ok {
			t.Errorf("no request received at %s", path)
			continue
		}
		// The resource travels with every signal.
		for _, want := range []string{"dev-forge-test", "1.2.3", "deployment.environment.name"} {
			if !bytes.Contains(body, []byte(want)) {
				t.Errorf("%s payload does not contain %q", path, want)
			}
		}
	}
	if body, _ := collector.body("/v1/traces"); !bytes.Contains(body, []byte("unit-span")) {
		t.Error("the emitted span was not exported")
	}
	if body, _ := collector.body("/v1/logs"); !bytes.Contains(body, []byte("unit-log")) {
		t.Error("the emitted log record was not exported")
	}
	if body, _ := collector.body("/v1/metrics"); !bytes.Contains(body, []byte("go.memory.used")) {
		t.Error("runtime metrics were not exported")
	}
}

func TestSetup_ExporterNoneDisablesASingleSignal(t *testing.T) {
	isolate(t)
	collector := newFakeCollector(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)
	t.Setenv("OTEL_METRICS_EXPORTER", "none")

	shutdown, err := Setup(context.Background(), testConfig(true), zap.NewNop())
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	emitTelemetry(context.Background())
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}

	if _, ok := collector.body("/v1/metrics"); ok {
		t.Error("metrics were exported although OTEL_METRICS_EXPORTER=none")
	}
	if _, ok := collector.body("/v1/traces"); !ok {
		t.Error("traces should still be exported")
	}
}

func TestSetup_SamplerFromEnvironmentTakesEffect(t *testing.T) {
	isolate(t)
	collector := newFakeCollector(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)
	t.Setenv("OTEL_TRACES_SAMPLER", "always_off")

	shutdown, err := Setup(context.Background(), testConfig(true), zap.NewNop())
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	emitTelemetry(context.Background())
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}

	// The collector must be reachable (logs arrive) for "span absent" to prove sampling.
	if _, ok := collector.body("/v1/logs"); !ok {
		t.Fatal("collector received nothing; cannot conclude anything about sampling")
	}
	if body, ok := collector.body("/v1/traces"); ok && bytes.Contains(body, []byte("unit-span")) {
		t.Error("span was exported although OTEL_TRACES_SAMPLER=always_off")
	}
}

func TestSetup_UnreachableBackendDoesNotBlockOrFail(t *testing.T) {
	isolate(t)
	// Nothing listens on port 1.
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
	core, _ := observer.New(zap.WarnLevel)

	shutdown, err := Setup(context.Background(), testConfig(true), zap.New(core))
	if err != nil {
		t.Fatalf("an unreachable backend must not fail Setup(): %v", err)
	}

	start := time.Now()
	for range 100 {
		emitTelemetry(context.Background())
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("emitting telemetry blocked for %s", elapsed)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start = time.Now()
	_ = shutdown(ctx) // a failed final flush is expected; only the bound matters
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("shutdown took %s, expected it to honour the 500ms deadline", elapsed)
	}
}

func TestShutdown_IsIdempotent(t *testing.T) {
	isolate(t)
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	t.Setenv("OTEL_LOGS_EXPORTER", "none")

	shutdown, err := Setup(context.Background(), testConfig(true), zap.NewNop())
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	first := shutdown(context.Background())
	second := shutdown(context.Background())
	if first != nil || second != nil {
		t.Errorf("shutdown() errors = %v, %v; want nil, nil", first, second)
	}
}

func TestSetup_RegistersPropagatorsAndErrorHandler(t *testing.T) {
	isolate(t)
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	t.Setenv("OTEL_LOGS_EXPORTER", "none")
	core, logs := observer.New(zap.WarnLevel)

	shutdown, err := Setup(context.Background(), testConfig(true), zap.New(core))
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	defer shutdown(context.Background()) //nolint:errcheck

	fields := otel.GetTextMapPropagator().Fields()
	for _, want := range []string{"traceparent", "baggage"} {
		found := false
		for _, f := range fields {
			found = found || f == want
		}
		if !found {
			t.Errorf("propagator does not handle %q (fields: %v)", want, fields)
		}
	}

	otel.Handle(context.DeadlineExceeded)
	if logs.FilterMessage("telemetry error").Len() != 1 {
		t.Error("SDK errors are not routed to the application logger")
	}
}

func TestNewResource_Precedence(t *testing.T) {
	isolate(t)
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "service.version=9.9.9,team=platform")

	res, err := newResource(context.Background(), testConfig(true))
	if err != nil {
		t.Fatalf("newResource() error = %v", err)
	}
	get := func(key string) string {
		v, _ := res.Set().Value(attributeKey(key))
		return v.AsString()
	}

	if got := get("service.name"); got != "dev-forge-test" {
		t.Errorf("service.name = %q, want config value", got)
	}
	if got := get("service.version"); got != "9.9.9" {
		t.Errorf("service.version = %q, want the OTEL_RESOURCE_ATTRIBUTES override", got)
	}
	if got := get("deployment.environment.name"); got != "development" {
		t.Errorf("deployment.environment.name = %q, want config value", got)
	}
	if got := get("team"); got != "platform" {
		t.Errorf("team = %q, want value from OTEL_RESOURCE_ATTRIBUTES", got)
	}
}

func TestBuildSampler(t *testing.T) {
	tests := []struct {
		name, sampler, arg, want string
	}{
		{"default", "", "", "ParentBased{root:AlwaysOnSampler"},
		{"always_off", "always_off", "", "AlwaysOffSampler"},
		{"ratio", "traceidratio", "0.25", "TraceIDRatioBased{0.25}"},
		{"parentbased ratio", "parentbased_traceidratio", "0.5", "ParentBased{root:TraceIDRatioBased{0.5}"},
		{"ratio defaults to one", "traceidratio", "", "TraceIDRatioBased{1}"},
		{"case and spaces are ignored", " ALWAYS_ON ", "", "AlwaysOnSampler"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t)
			t.Setenv("OTEL_TRACES_SAMPLER", tt.sampler)
			t.Setenv("OTEL_TRACES_SAMPLER_ARG", tt.arg)

			s, err := buildSampler()
			if err != nil {
				t.Fatalf("buildSampler() error = %v", err)
			}
			if !strings.Contains(s.Description(), tt.want) {
				t.Errorf("Description() = %q, want it to contain %q", s.Description(), tt.want)
			}
		})
	}
}
