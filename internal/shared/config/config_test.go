package config

import (
	"testing"
	"time"
)

// ── Load defaults ─────────────────────────────────────────────────────────────

func TestLoad_Defaults(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"Environment", cfg.Environment, "development"},
		{"Debug", cfg.Debug, true},
		{"Server.Host", cfg.Server.Host, "0.0.0.0"},
		{"Server.Port", cfg.Server.Port, 9000},
		{"Database.Host", cfg.Database.Host, "localhost"},
		{"Database.Port", cfg.Database.Port, 5432},
		{"Database.User", cfg.Database.User, "postgres"},
		{"Database.Name", cfg.Database.Name, "dev_forge"},
		{"Database.SSLMode", cfg.Database.SSLMode, "disable"},
		{"Database.MaxConns", cfg.Database.MaxConns, int32(25)},
		{"Database.MinConns", cfg.Database.MinConns, int32(5)},
		{"Database.MaxConnLifetime", cfg.Database.MaxConnLifetime, 1 * time.Hour},
		{"Database.MaxConnIdleTime", cfg.Database.MaxConnIdleTime, 30 * time.Minute},
		{"Database.HealthCheckInterval", cfg.Database.HealthCheckInterval, 1 * time.Minute},
		{"Zitadel.Issuer", cfg.Zitadel.Issuer, "http://localhost:8080"},
		{"Zitadel.ClientID", cfg.Zitadel.ClientID, ""},
		{"OTEL.Enabled", cfg.OTEL.Enabled, false},
		{"OTEL.ServiceName", cfg.OTEL.ServiceName, "dev-forge"},
		{"OTEL.ServiceVersion", cfg.OTEL.ServiceVersion, "0.1.0"},
		{"OTEL.OTLPEndpoint", cfg.OTEL.OTLPEndpoint, "localhost:4317"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}

// ── Load from env vars ────────────────────────────────────────────────────────

func TestLoad_FromEnv(t *testing.T) {
	clearEnv(t)

	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("DEBUG", "false")
	t.Setenv("SERVER_HOST", "127.0.0.1")
	t.Setenv("SERVER_PORT", "8080")
	t.Setenv("DB_HOST", "db.internal")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_USER", "appuser")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_NAME", "mydb")
	t.Setenv("DB_SSL_MODE", "require")
	t.Setenv("DB_MAX_CONNS", "50")
	t.Setenv("DB_MIN_CONNS", "10")
	t.Setenv("DB_MAX_CONN_LIFETIME", "2h")
	t.Setenv("DB_MAX_CONN_IDLE_TIME", "15m")
	t.Setenv("DB_HEALTH_CHECK_INTERVAL", "30s")
	t.Setenv("ZITADEL_ISSUER", "https://auth.example.com")
	t.Setenv("ZITADEL_CLIENT_ID", "abc123")
	t.Setenv("OTEL_ENABLED", "true")
	t.Setenv("OTEL_SERVICE_NAME", "my-service")
	t.Setenv("OTEL_SERVICE_VERSION", "1.2.3")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "otel.internal:4317")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"Environment", cfg.Environment, "production"},
		{"Debug", cfg.Debug, false},
		{"Server.Host", cfg.Server.Host, "127.0.0.1"},
		{"Server.Port", cfg.Server.Port, 8080},
		{"Database.Host", cfg.Database.Host, "db.internal"},
		{"Database.Port", cfg.Database.Port, 5433},
		{"Database.User", cfg.Database.User, "appuser"},
		{"Database.Password", cfg.Database.Password, "secret"},
		{"Database.Name", cfg.Database.Name, "mydb"},
		{"Database.SSLMode", cfg.Database.SSLMode, "require"},
		{"Database.MaxConns", cfg.Database.MaxConns, int32(50)},
		{"Database.MinConns", cfg.Database.MinConns, int32(10)},
		{"Database.MaxConnLifetime", cfg.Database.MaxConnLifetime, 2 * time.Hour},
		{"Database.MaxConnIdleTime", cfg.Database.MaxConnIdleTime, 15 * time.Minute},
		{"Database.HealthCheckInterval", cfg.Database.HealthCheckInterval, 30 * time.Second},
		{"Zitadel.Issuer", cfg.Zitadel.Issuer, "https://auth.example.com"},
		{"Zitadel.ClientID", cfg.Zitadel.ClientID, "abc123"},
		{"OTEL.Enabled", cfg.OTEL.Enabled, true},
		{"OTEL.ServiceName", cfg.OTEL.ServiceName, "my-service"},
		{"OTEL.ServiceVersion", cfg.OTEL.ServiceVersion, "1.2.3"},
		{"OTEL.OTLPEndpoint", cfg.OTEL.OTLPEndpoint, "otel.internal:4317"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}

// ── Validation ────────────────────────────────────────────────────────────────

func TestLoad_InvalidPort(t *testing.T) {
	clearEnv(t)
	t.Setenv("SERVER_PORT", "99999")

	_, err := Load()
	if err == nil {
		t.Error("Load() expected error for invalid port, got nil")
	}
}

func TestLoad_ZeroPort(t *testing.T) {
	clearEnv(t)
	t.Setenv("SERVER_PORT", "0")

	_, err := Load()
	if err == nil {
		t.Error("Load() expected error for port 0, got nil")
	}
}

// ── DatabaseConfig methods ────────────────────────────────────────────────────

func TestDatabaseConfig_DSN(t *testing.T) {
	d := DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "secret",
		Name:     "dev_forge",
		SSLMode:  "disable",
	}

	want := "host=localhost port=5432 user=postgres password=secret dbname=dev_forge sslmode=disable"
	got := d.DSN()
	if got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}

func TestDatabaseConfig_URL(t *testing.T) {
	d := DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "secret",
		Name:     "dev_forge",
		SSLMode:  "disable",
	}

	want := "postgres://postgres:secret@localhost:5432/dev_forge?sslmode=disable"
	got := d.URL()
	if got != want {
		t.Errorf("URL() = %q, want %q", got, want)
	}
}

// ── LogLevel ──────────────────────────────────────────────────────────────────

func TestLogLevel_DefaultProd(t *testing.T) {
	clearEnv(t)
	cfg := &Config{Debug: false}
	if got := cfg.LogLevel(); got != "info" {
		t.Errorf("LogLevel() = %q, want %q", got, "info")
	}
}

func TestLogLevel_DebugMode(t *testing.T) {
	clearEnv(t)
	cfg := &Config{Debug: true}
	if got := cfg.LogLevel(); got != "debug" {
		t.Errorf("LogLevel() = %q, want %q", got, "debug")
	}
}

func TestLogLevel_EnvOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("LOG_LEVEL", "warn")
	cfg := &Config{Debug: false}
	if got := cfg.LogLevel(); got != "warn" {
		t.Errorf("LogLevel() = %q, want %q", got, "warn")
	}
}

func TestLogLevel_EnvOverridesDebug(t *testing.T) {
	clearEnv(t)
	t.Setenv("LOG_LEVEL", "error")
	cfg := &Config{Debug: true}
	if got := cfg.LogLevel(); got != "error" {
		t.Errorf("LogLevel() = %q, want %q", got, "error")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// clearEnv unsets all env vars read by Load() so tests are isolated.
func clearEnv(t *testing.T) {
	t.Helper()
	vars := []string{
		"ENVIRONMENT", "DEBUG", "LOG_LEVEL",
		"SERVER_HOST", "SERVER_PORT",
		"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSL_MODE",
		"DB_MAX_CONNS", "DB_MIN_CONNS", "DB_MAX_CONN_LIFETIME",
		"DB_MAX_CONN_IDLE_TIME", "DB_HEALTH_CHECK_INTERVAL",
		"ZITADEL_ISSUER", "ZITADEL_CLIENT_ID",
		"OTEL_ENABLED", "OTEL_SERVICE_NAME", "OTEL_SERVICE_VERSION",
		"OTEL_EXPORTER_OTLP_ENDPOINT",
	}
	for _, v := range vars {
		t.Setenv(v, "")
	}
}
