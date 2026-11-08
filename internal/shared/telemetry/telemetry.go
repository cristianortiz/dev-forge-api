// Package telemetry wires the OpenTelemetry SDK for traces, metrics and logs.
//
// Exporters, endpoint, protocol, sampling and the metric interval come from the
// standard OTEL_* environment variables (read by the SDK and autoexport), so the
// same binary runs against the local lab and the cluster by changing only the
// environment. OTEL_ENABLED is the application-level switch.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	otelruntime "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.uber.org/zap"

	"github.com/cristianortiz/dev-forge/internal/shared/config"
)

// ShutdownFunc flushes pending telemetry and releases the providers. It honours
// the context deadline and is safe to call more than once.
type ShutdownFunc func(ctx context.Context) error

// Setup installs the global tracer, meter and logger providers and propagators.
//
// An invalid OTEL_* configuration is returned as an error so the process does
// not start with settings nobody asked for. An unreachable backend is not an
// error: exporters connect lazily, queues are bounded and failures are logged
// at a limited rate.
//
// log must not export through OTLP itself, otherwise an export failure would
// log, which would be exported, which would fail again.
func Setup(ctx context.Context, cfg *config.Config, log *zap.Logger) (ShutdownFunc, error) {
	if !cfg.OTEL.Enabled {
		log.Info("telemetry disabled", zap.String("reason", "OTEL_ENABLED is false"))
		return func(context.Context) error { return nil }, nil
	}

	if err := validateEnv(); err != nil {
		return nil, fmt.Errorf("invalid OpenTelemetry configuration: %w", err)
	}

	res, err := newResource(ctx, cfg)
	if err != nil {
		return nil, err
	}
	sampler, err := buildSampler()
	if err != nil {
		return nil, err
	}

	var shutdowns []func(context.Context) error
	fail := func(err error) (ShutdownFunc, error) {
		// Nothing has been installed globally yet; release what was created.
		_ = shutdownAll(shutdowns)(ctx)
		return nil, err
	}

	spanExporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return fail(fmt.Errorf("create trace exporter (OTEL_TRACES_EXPORTER): %w", err))
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(spanExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	shutdowns = append(shutdowns, tp.Shutdown)

	metricReader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return fail(fmt.Errorf("create metric reader (OTEL_METRICS_EXPORTER): %w", err))
	}
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(metricReader))
	shutdowns = append(shutdowns, mp.Shutdown)

	logExporter, err := autoexport.NewLogExporter(ctx)
	if err != nil {
		return fail(fmt.Errorf("create log exporter (OTEL_LOGS_EXPORTER): %w", err))
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
	)
	shutdowns = append(shutdowns, lp.Shutdown)

	if err := otelruntime.Start(otelruntime.WithMeterProvider(mp)); err != nil {
		return fail(fmt.Errorf("start runtime metrics: %w", err))
	}

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetLoggerProvider(lp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	otel.SetErrorHandler(newErrorHandler(log, exportErrorLogInterval))

	log.Info("telemetry enabled",
		zap.String("traces_exporter", envOr("OTEL_TRACES_EXPORTER", "otlp")),
		zap.String("metrics_exporter", envOr("OTEL_METRICS_EXPORTER", "otlp")),
		zap.String("logs_exporter", envOr("OTEL_LOGS_EXPORTER", "otlp")),
		zap.String("protocol", envOr("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")),
		zap.String("sampler", envOr("OTEL_TRACES_SAMPLER", "parentbased_always_on")),
	)

	return shutdownAll(shutdowns), nil
}

// newResource builds the resource shared by the three signals. Precedence, from
// lowest to highest: SDK defaults, application config (service name, version and
// environment), then OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES.
func newResource(ctx context.Context, cfg *config.Config) (*resource.Resource, error) {
	own := resource.NewWithAttributes("",
		semconv.ServiceName(cfg.OTEL.ServiceName),
		semconv.ServiceVersion(cfg.OTEL.ServiceVersion),
		semconv.DeploymentEnvironmentNameKey.String(cfg.OTEL.Environment),
	)
	fromEnv, err := resource.New(ctx, resource.WithFromEnv())
	if err != nil {
		return nil, fmt.Errorf("invalid OTEL_RESOURCE_ATTRIBUTES: %w", err)
	}
	res, err := resource.Merge(resource.Default(), own)
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}
	res, err = resource.Merge(res, fromEnv)
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}
	return res, nil
}

// shutdownAll runs the shutdown functions concurrently so that one slow
// exporter cannot consume the whole deadline of the others.
func shutdownAll(fns []func(context.Context) error) ShutdownFunc {
	var (
		once   sync.Once
		result error
	)
	return func(ctx context.Context) error {
		once.Do(func() {
			errs := make([]error, len(fns))
			var wg sync.WaitGroup
			for i, fn := range fns {
				wg.Add(1)
				go func() {
					defer wg.Done()
					errs[i] = fn(ctx)
				}()
			}
			wg.Wait()
			result = errors.Join(errs...)
		})
		return result
	}
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
