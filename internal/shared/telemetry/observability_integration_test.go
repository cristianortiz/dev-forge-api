//go:build obsintegration

package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/cristianortiz/dev-forge/internal/shared/config"
	"github.com/cristianortiz/dev-forge/internal/shared/database"
	applog "github.com/cristianortiz/dev-forge/internal/shared/logger"
	"github.com/cristianortiz/dev-forge/internal/shared/server"
)

const (
	integrationServicePrefix = "dev-forge-obs-smoke-"
	integrationTraceHeader   = "X-Trace-Id"
)

func TestObservabilityIntegration(t *testing.T) {
	serviceName := integrationServicePrefix + uuid.NewString()
	cfg, shutdown, log := integrationTelemetry(t, serviceName, "", "")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := database.New(ctx, cfg, log)
	if err != nil {
		t.Fatalf("connect to integration database: %v", err)
	}
	t.Cleanup(func() {
		if db != nil {
			db.Close()
		}
	})

	srv := server.New(cfg, log)
	srv.App.Get("/_obs/smoke/ok", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	srv.App.Get("/_obs/smoke/client-error", func(c fiber.Ctx) error {
		return fiber.NewError(fiber.StatusBadRequest, "expected client error")
	})
	srv.App.Get("/_obs/smoke/server-error", func(c fiber.Ctx) error {
		return fiber.NewError(fiber.StatusInternalServerError, "expected server error")
	})
	srv.App.Get("/_obs/smoke/panic", func(fiber.Ctx) error {
		panic("observability integration panic")
	})
	srv.App.Get("/_obs/smoke/db", func(c fiber.Ctx) error {
		var result int
		if err := db.Pool.QueryRow(c.Context(), "SELECT 42").Scan(&result); err != nil {
			return err
		}
		return c.SendString(strconv.Itoa(result))
	})
	srv.App.Get("/_obs/smoke/items/:id", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	request := func(path string, wantStatus int) string {
		t.Helper()
		resp, err := srv.App.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			t.Fatalf("read GET %s response: %v", path, err)
		}
		if resp.StatusCode != wantStatus {
			t.Fatalf("GET %s status = %d, want %d", path, resp.StatusCode, wantStatus)
		}
		traceID := resp.Header.Get(integrationTraceHeader)
		if traceID == "" {
			t.Fatalf("GET %s did not return %s", path, integrationTraceHeader)
		}
		return traceID
	}

	request("/_obs/smoke/ok", http.StatusOK)
	request("/_obs/smoke/client-error", http.StatusBadRequest)
	request("/_obs/smoke/server-error", http.StatusInternalServerError)
	request("/_obs/smoke/panic", http.StatusInternalServerError)
	dbTraceID := request("/_obs/smoke/db", http.StatusOK)

	for range 30 {
		request("/_obs/smoke/items/"+uuid.NewString(), http.StatusOK)
		request("/_obs/smoke/missing/"+uuid.NewString(), http.StatusNotFound)
	}

	db.Close()
	db = nil
	if err := shutdown(ctx); err != nil {
		t.Fatalf("flush telemetry: %v", err)
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer queryCancel()
	grafana := newIntegrationGrafana()
	dataSources := integrationDataSources(t, grafana)
	promUID, ok := dataSources["prometheus"]
	if !ok {
		t.Fatal("Grafana Prometheus datasource is missing")
	}
	lokiUID, ok := dataSources["loki"]
	if !ok {
		t.Fatal("Grafana Loki datasource is missing")
	}
	tempoUID, ok := dataSources["tempo"]
	if !ok {
		t.Fatal("Grafana Tempo datasource is missing")
	}
	assertHTTPDashboard(t, grafana, queryCtx)

	okSelector := fmt.Sprintf(
		`http_server_request_duration_seconds_count{service_name=%q,http_route=%q,http_response_status_code="200"}`,
		serviceName, "/_obs/smoke/ok",
	)
	assertPrometheusSeries(t, grafana, promUID, okSelector, 1)

	bucketSelector := strings.TrimSuffix(strings.Replace(okSelector, "_count{", "_bucket{", 1), "}") + `,le="+Inf"}`
	assertPrometheusSeries(t, grafana, promUID, bucketSelector, 1)
	for _, expected := range []struct {
		route  string
		status string
	}{
		{route: "/_obs/smoke/client-error", status: "400"},
		{route: "/_obs/smoke/server-error", status: "500"},
		{route: "/_obs/smoke/panic", status: "500"},
		{route: "/_obs/smoke/db", status: "200"},
	} {
		selector := fmt.Sprintf(
			`http_server_request_duration_seconds_count{service_name=%q,http_route=%q,http_response_status_code=%q}`,
			serviceName, expected.route, expected.status,
		)
		assertPrometheusSeries(t, grafana, promUID, selector, 1)
	}

	itemSelector := fmt.Sprintf(
		`http_server_request_duration_seconds_count{service_name=%q,http_route=%q,http_response_status_code="200"}`,
		serviceName, "/_obs/smoke/items/:id",
	)
	assertPrometheusSeries(t, grafana, promUID, itemSelector, 1)

	notFoundSelector := fmt.Sprintf(
		`http_server_request_duration_seconds_count{service_name=%q,http_response_status_code="404"}`,
		serviceName,
	)
	notFoundSeries := assertPrometheusSeries(t, grafana, promUID, notFoundSelector, 1)
	for _, metric := range notFoundSeries {
		if _, ok := metric["http_route"]; ok {
			t.Errorf("unmatched route unexpectedly has http_route label: %v", metric)
		}
	}

	traceBody := eventually(t, "Tempo database trace", func() ([]byte, bool, error) {
		body, err := grafana.get(queryCtx, "/api/datasources/proxy/uid/"+url.PathEscape(tempoUID)+
			"/api/traces/"+url.PathEscape(dbTraceID), nil)
		return body, err == nil && bytes.Contains(body, []byte("query SELECT")), err
	})
	if !bytes.Contains(traceBody, []byte("SELECT 42")) {
		t.Errorf("database trace does not contain the expected parameterized query span: %s", traceBody)
	}

	lokiQuery := fmt.Sprintf(`{service_name=%q} | trace_id=%q`, serviceName, dbTraceID)
	logStream := eventually(t, "Loki access log", func() ([]byte, bool, error) {
		body, err := grafana.get(queryCtx, "/api/datasources/proxy/uid/"+url.PathEscape(lokiUID)+
			"/loki/api/v1/query_range", url.Values{
			"query": []string{lokiQuery},
			"start": []string{strconv.FormatInt(time.Now().Add(-10*time.Minute).UnixNano(), 10)},
			"end":   []string{strconv.FormatInt(time.Now().UnixNano(), 10)},
			"limit": []string{"20"},
		})
		if err != nil {
			return nil, false, err
		}
		streams, err := parseLokiStreams(body)
		if err != nil {
			return nil, false, err
		}
		return body, countLogMessage(streams, "request") == 1, nil
	})
	streams, err := parseLokiStreams(logStream)
	if err != nil {
		t.Fatalf("decode Loki response: %v", err)
	}
	if countLogMessage(streams, "request") != 1 {
		t.Fatalf("expected one access log for trace %s", dbTraceID)
	}
	for _, stream := range streams {
		for _, record := range stream.Records {
			if record.Message == "request" {
				if got := stream.Metadata["trace_id"]; got != dbTraceID {
					t.Errorf("Loki access log trace_id = %q, want %q", got, dbTraceID)
				}
				if stream.Metadata["span_id"] == "" {
					t.Error("Loki access log has no span_id metadata")
				}
			}
		}
	}

	lokiSeries := eventually(t, "Loki indexed series", func() ([]byte, bool, error) {
		body, err := grafana.get(queryCtx, "/api/datasources/proxy/uid/"+url.PathEscape(lokiUID)+
			"/loki/api/v1/series", url.Values{
			"match[]": []string{fmt.Sprintf(`{service_name=%q}`, serviceName)},
			"start":   []string{strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)},
			"end":     []string{strconv.FormatInt(time.Now().Unix(), 10)},
		})
		if err != nil {
			return nil, false, err
		}
		var result struct {
			Data []map[string]string `json:"data"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, false, err
		}
		return body, len(result.Data) > 0, nil
	})
	var indexedResponse struct {
		Data []map[string]string `json:"data"`
	}
	if err := json.Unmarshal(lokiSeries, &indexedResponse); err != nil {
		t.Fatalf("decode Loki series response: %v", err)
	}
	for _, labels := range indexedResponse.Data {
		if labels["service_name"] != serviceName {
			t.Errorf("Loki indexed service_name = %q, want %q", labels["service_name"], serviceName)
		}
		if labels["deployment_environment_name"] != "obsintegration" {
			t.Errorf("Loki indexed deployment_environment_name = %q, want obsintegration", labels["deployment_environment_name"])
		}
		for _, key := range []string{"trace_id", "span_id"} {
			if _, indexed := labels[key]; indexed {
				t.Errorf("Loki indexed high-cardinality %s label", key)
			}
		}
	}
}

func TestObservabilityUnsampledLogHasNoTraceIDs(t *testing.T) {
	serviceName := integrationServicePrefix + "unsampled-" + uuid.NewString()
	cfg, shutdown, log := integrationTelemetry(t, serviceName, "parentbased_traceidratio", "0")

	srv := server.New(cfg, log)
	srv.App.Get("/_obs/smoke/unsampled", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	resp, err := srv.App.Test(httptest.NewRequest(http.MethodGet, "/_obs/smoke/unsampled", nil))
	if err != nil {
		t.Fatalf("send unsampled request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	_ = resp.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		t.Fatalf("flush unsampled telemetry: %v", err)
	}

	queryCtx, queryCancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer queryCancel()
	grafana := newIntegrationGrafana()
	dataSources := integrationDataSources(t, grafana)
	lokiUID, ok := dataSources["loki"]
	if !ok {
		t.Fatal("Grafana Loki datasource is missing")
	}
	body := eventually(t, "unsampled Loki log", func() ([]byte, bool, error) {
		response, err := grafana.get(queryCtx, "/api/datasources/proxy/uid/"+url.PathEscape(lokiUID)+
			"/loki/api/v1/query_range", url.Values{
			"query": []string{fmt.Sprintf(`{service_name=%q} |= "request"`, serviceName)},
			"start": []string{strconv.FormatInt(time.Now().Add(-10*time.Minute).UnixNano(), 10)},
			"end":   []string{strconv.FormatInt(time.Now().UnixNano(), 10)},
			"limit": []string{"10"},
		})
		if err != nil {
			return nil, false, err
		}
		streams, err := parseLokiStreams(response)
		if err != nil {
			return nil, false, err
		}
		return response, countLogMessage(streams, "request") == 1, nil
	})
	streams, err := parseLokiStreams(body)
	if err != nil {
		t.Fatalf("decode Loki response: %v", err)
	}
	for _, stream := range streams {
		for _, record := range stream.Records {
			if record.Message != "request" {
				continue
			}
			for _, key := range []string{"trace_id", "span_id"} {
				if value := stream.Metadata[key]; value != "" && value != zeroTraceValue(key) {
					t.Errorf("unsampled log has valid %s metadata %q", key, value)
				}
			}
		}
	}
}

func integrationTelemetry(t *testing.T, serviceName, sampler, samplerArg string) (*config.Config, ShutdownFunc, *zap.Logger) {
	t.Helper()
	isolate(t)
	t.Setenv("OTEL_ENABLED", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", envOr("OBS_OTLP_ENDPOINT", "http://127.0.0.1:4318"))
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
	t.Setenv("OTEL_METRICS_EXPORTER", "otlp")
	t.Setenv("OTEL_LOGS_EXPORTER", "otlp")
	t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "1000")
	if sampler != "" {
		t.Setenv("OTEL_TRACES_SAMPLER", sampler)
		t.Setenv("OTEL_TRACES_SAMPLER_ARG", samplerArg)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load integration config: %v", err)
	}
	cfg.OTEL.Enabled = true
	cfg.OTEL.ServiceName = serviceName
	cfg.OTEL.Environment = "obsintegration"
	cfg.Environment = "obsintegration"
	cfg.Debug = false
	cfg.OTEL.ShutdownTimeout = 20 * time.Second

	baseLog := zap.NewNop()
	// Setup installs global providers used by both the HTTP middleware and pgx instrumentation.
	shutdown, err := Setup(context.Background(), cfg, baseLog)
	if err != nil {
		t.Fatalf("initialize integration telemetry: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.OTEL.ShutdownTimeout)
		defer cancel()
		if err := shutdown(ctx); err != nil {
			t.Errorf("shutdown integration telemetry: %v", err)
		}
	})
	return cfg, shutdown, applog.WithOTEL(baseLog)
}

type integrationGrafana struct {
	baseURL  string
	user     string
	password string
	client   *http.Client
}

func newIntegrationGrafana() integrationGrafana {
	return integrationGrafana{
		baseURL:  strings.TrimRight(envOr("OBS_GRAFANA_URL", "http://127.0.0.1:3000"), "/"),
		user:     envOr("OBS_GRAFANA_USER", "admin"),
		password: envOr("OBS_GRAFANA_PASSWORD", "admin"),
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (g integrationGrafana) get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	endpoint := g.baseURL + path
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(g.user, g.password)
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Grafana proxy returned %s: %s", resp.Status, body)
	}
	return body, nil
}

func integrationDataSources(t *testing.T, grafana integrationGrafana) map[string]string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	body, err := grafana.get(ctx, "/api/datasources", nil)
	if err != nil {
		t.Fatalf("list Grafana datasources: %v", err)
	}
	var dataSources []struct {
		Type string `json:"type"`
		UID  string `json:"uid"`
	}
	if err := json.Unmarshal(body, &dataSources); err != nil {
		t.Fatalf("decode Grafana datasources: %v", err)
	}
	result := make(map[string]string, len(dataSources))
	for _, dataSource := range dataSources {
		result[dataSource.Type] = dataSource.UID
	}
	return result
}

func assertHTTPDashboard(t *testing.T, grafana integrationGrafana, ctx context.Context) {
	t.Helper()
	body := eventually(t, "provisioned Dev Forge HTTP dashboard", func() ([]byte, bool, error) {
		body, err := grafana.get(ctx, "/api/dashboards/uid/dev-forge-red", nil)
		if err != nil {
			return nil, false, err
		}
		var response struct {
			Dashboard struct {
				Title string `json:"title"`
			} `json:"dashboard"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, false, err
		}
		return body, response.Dashboard.Title == "Dev Forge · Solicitudes HTTP", nil
	})
	var response struct {
		Dashboard struct {
			Title  string `json:"title"`
			Panels []struct {
				Title   string `json:"title"`
				Targets []struct {
					Expr string `json:"expr"`
				} `json:"targets"`
				Options struct {
					Legend struct {
						Calcs []string `json:"calcs"`
					} `json:"legend"`
				} `json:"options"`
			} `json:"panels"`
		} `json:"dashboard"`
		Meta struct {
			FolderTitle string `json:"folderTitle"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode Dev Forge HTTP dashboard: %v", err)
	}
	if response.Dashboard.Title != "Dev Forge · Solicitudes HTTP" || response.Meta.FolderTitle != "Dev Forge" {
		t.Fatalf("unexpected dashboard title/folder: %q / %q", response.Dashboard.Title, response.Meta.FolderTitle)
	}
	if len(response.Dashboard.Panels) != 7 {
		t.Fatalf("Dev Forge HTTP dashboard has %d panels, want 7", len(response.Dashboard.Panels))
	}
	var statusCountQuery, requestRateQuery string
	var requestRateCalcs []string
	for _, panel := range response.Dashboard.Panels {
		for _, target := range panel.Targets {
			if !strings.Contains(target.Expr, `service_name="dev-forge"`) {
				t.Errorf("dashboard panel %q does not filter for the real service", panel.Title)
			}
			if panel.Title == "Conteo por código HTTP" {
				statusCountQuery = target.Expr
			}
			if panel.Title == "Solicitudes/min por código HTTP" {
				requestRateQuery = target.Expr
				requestRateCalcs = panel.Options.Legend.Calcs
			}
		}
	}
	for _, expected := range []string{
		"round(sum by (http_response_status_code)",
		`service_name="dev-forge"`,
		"[$__range]",
		"> 0",
	} {
		if !strings.Contains(statusCountQuery, expected) {
			t.Errorf("HTTP status count query %q is missing %q", statusCountQuery, expected)
		}
	}
	for _, expected := range []string{
		"rate(http_server_request_duration_seconds_count",
		"[$__rate_interval])) * 60",
	} {
		if !strings.Contains(requestRateQuery, expected) {
			t.Errorf("HTTP request rate query %q is missing %q", requestRateQuery, expected)
		}
	}
	rateCalcs := strings.Join(requestRateCalcs, ",")
	for _, expected := range []string{"lastNotNull", "max"} {
		if !strings.Contains(rateCalcs, expected) {
			t.Errorf("HTTP request rate legend is missing %q: %v", expected, requestRateCalcs)
		}
	}
	if strings.Contains(rateCalcs, "sum") {
		t.Errorf("HTTP request rate legend includes the misleading sum calculation: %v", requestRateCalcs)
	}
}

type prometheusSeriesResponse struct {
	Status string              `json:"status"`
	Error  string              `json:"error"`
	Data   []map[string]string `json:"data"`
}

func assertPrometheusSeries(t *testing.T, grafana integrationGrafana, uid, selector string, want int) []map[string]string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	response := eventually(t, "Prometheus series "+selector, func() ([]byte, bool, error) {
		body, err := grafana.get(ctx, "/api/datasources/proxy/uid/"+url.PathEscape(uid)+
			"/api/v1/series", url.Values{
			"match[]": []string{selector},
			"start":   []string{strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)},
			"end":     []string{strconv.FormatInt(time.Now().Unix(), 10)},
		})
		if err != nil {
			return nil, false, err
		}
		var result prometheusSeriesResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, false, err
		}
		if result.Status != "success" {
			return nil, false, fmt.Errorf("Prometheus query failed: %s", result.Error)
		}
		return body, len(result.Data) >= want, nil
	})
	var result prometheusSeriesResponse
	if err := json.Unmarshal(response, &result); err != nil {
		t.Fatalf("decode Prometheus series: %v", err)
	}
	if len(result.Data) != want {
		t.Fatalf("Prometheus selector %s returned %d series, want %d: %v", selector, len(result.Data), want, result.Data)
	}
	return result.Data
}

type lokiStream struct {
	Metadata map[string]string
	Records  []lokiRecord
}

type lokiRecord struct {
	Message string
}

func parseLokiStreams(body []byte) ([]lokiStream, error) {
	var response struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   struct {
			Result []struct {
				Stream map[string]string `json:"stream"`
				Values [][]string        `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	if response.Status != "success" {
		return nil, fmt.Errorf("Loki query failed: %s", response.Error)
	}
	streams := make([]lokiStream, 0, len(response.Data.Result))
	for _, result := range response.Data.Result {
		stream := lokiStream{Metadata: result.Stream}
		for _, value := range result.Values {
			if len(value) != 2 {
				return nil, fmt.Errorf("unexpected Loki value shape: %v", value)
			}
			stream.Records = append(stream.Records, lokiRecord{Message: value[1]})
		}
		streams = append(streams, stream)
	}
	return streams, nil
}

func countLogMessage(streams []lokiStream, message string) int {
	count := 0
	for _, stream := range streams {
		for _, record := range stream.Records {
			if record.Message == message {
				count++
			}
		}
	}
	return count
}

func eventually(t *testing.T, what string, probe func() ([]byte, bool, error)) []byte {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		body, ready, err := probe()
		if err == nil && ready {
			return body
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("%s not ready yet", what)
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("timed out waiting for %s: %v", what, lastErr)
	return nil
}

func zeroTraceValue(key string) string {
	if key == "trace_id" {
		return trace.TraceID{}.String()
	}
	return trace.SpanID{}.String()
}
