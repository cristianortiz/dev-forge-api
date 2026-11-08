package telemetry

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// The OpenTelemetry SDK ignores or degrades some invalid OTEL_* values (an
// unknown sampler, a malformed metric interval) after reporting them through an
// error handler, so the process would start with a configuration nobody asked
// for. validateEnv turns those cases into a startup error that names the
// offending variable.
//
// Exporters are limited to the ones that need no extra listener: "prometheus"
// is deliberately excluded because the platform ships every signal over OTLP.
var (
	supportedExporters = []string{"otlp", "console", "none"}
	supportedProtocols = []string{"grpc", "http/protobuf"}
)

func validateEnv() error {
	var errs []error

	for _, name := range []string{"OTEL_TRACES_EXPORTER", "OTEL_METRICS_EXPORTER", "OTEL_LOGS_EXPORTER"} {
		if v := os.Getenv(name); v != "" && !slices.Contains(supportedExporters, v) {
			errs = append(errs, fmt.Errorf("%s=%q is not supported: use one of %s",
				name, v, strings.Join(supportedExporters, ", ")))
		}
	}

	for _, name := range []string{
		"OTEL_EXPORTER_OTLP_PROTOCOL",
		"OTEL_EXPORTER_OTLP_TRACES_PROTOCOL",
		"OTEL_EXPORTER_OTLP_METRICS_PROTOCOL",
		"OTEL_EXPORTER_OTLP_LOGS_PROTOCOL",
	} {
		if v := os.Getenv(name); v != "" && !slices.Contains(supportedProtocols, v) {
			errs = append(errs, fmt.Errorf("%s=%q is not supported: use one of %s",
				name, v, strings.Join(supportedProtocols, ", ")))
		}
	}

	for _, name := range []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
		"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT",
	} {
		if v := os.Getenv(name); v != "" {
			if err := validateEndpoint(v); err != nil {
				errs = append(errs, fmt.Errorf("%s=%q: %w", name, v, err))
			}
		}
	}

	if _, err := buildSampler(); err != nil {
		errs = append(errs, err)
	}

	for _, name := range []string{"OTEL_METRIC_EXPORT_INTERVAL", "OTEL_METRIC_EXPORT_TIMEOUT"} {
		if v := os.Getenv(name); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err != nil || n <= 0 {
				errs = append(errs, fmt.Errorf("%s=%q must be a positive integer number of milliseconds", name, v))
			}
		}
	}

	return errors.Join(errs...)
}

func validateEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("must be a URL with an http:// or https:// scheme, for example http://localhost:4318")
	}
	return nil
}

// buildSampler returns the sampler selected by OTEL_TRACES_SAMPLER and
// OTEL_TRACES_SAMPLER_ARG, defaulting to parentbased_always_on. It is built here
// instead of delegating to the SDK so that an invalid value is an error rather
// than a silent fallback.
func buildSampler() (sdktrace.Sampler, error) {
	name := strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER")))
	arg := strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER_ARG"))

	ratio := func() (sdktrace.Sampler, error) {
		r := 1.0
		if arg != "" {
			var err error
			if r, err = strconv.ParseFloat(arg, 64); err != nil || r < 0 || r > 1 {
				return nil, fmt.Errorf("OTEL_TRACES_SAMPLER_ARG=%q must be a number between 0 and 1", arg)
			}
		}
		return sdktrace.TraceIDRatioBased(r), nil
	}

	switch name {
	case "", "parentbased_always_on":
		return sdktrace.ParentBased(sdktrace.AlwaysSample()), nil
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample()), nil
	case "always_on":
		return sdktrace.AlwaysSample(), nil
	case "always_off":
		return sdktrace.NeverSample(), nil
	case "traceidratio":
		return ratio()
	case "parentbased_traceidratio":
		s, err := ratio()
		if err != nil {
			return nil, err
		}
		return sdktrace.ParentBased(s), nil
	default:
		return nil, fmt.Errorf("OTEL_TRACES_SAMPLER=%q is not supported: use always_on, always_off, traceidratio, "+
			"parentbased_always_on, parentbased_always_off or parentbased_traceidratio", name)
	}
}
