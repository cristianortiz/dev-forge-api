package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for dev-forge loaded from environment variables.
type Config struct {
	Environment string
	Debug       bool

	Server   ServerConfig
	Database DatabaseConfig
	Zitadel  ZitadelConfig
	OTEL     OTELConfig
}

type ServerConfig struct {
	Host string
	Port int
}

type DatabaseConfig struct {
	Host                string
	Port                int
	User                string
	Password            string
	Name                string
	SSLMode             string
	MaxConns            int32
	MinConns            int32
	MaxConnLifetime     time.Duration
	MaxConnIdleTime     time.Duration
	HealthCheckInterval time.Duration
}

// DSN returns the PostgreSQL connection string.
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}

// URL returns the PostgreSQL connection URL (required by pgxpool.ParseConfig).
func (d DatabaseConfig) URL() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.Name, d.SSLMode,
	)
}

// ZitadelConfig holds OIDC configuration for validating JWTs issued by Zitadel.
type ZitadelConfig struct {
	Issuer   string // e.g. https://dev-forge-2hcwhk.us1.zitadel.cloud
	ClientID string // OIDC client ID of the dev-forge project app
	KeyPath  string // path to the JSON key file for JWT-profile introspection
}

type OTELConfig struct {
	Enabled        bool
	ServiceName    string
	ServiceVersion string
	OTLPEndpoint   string // gRPC endpoint, e.g. localhost:4317
	Environment    string
}

// LogLevel returns the desired zap log level from the LOG_LEVEL env var.
// Defaults to "info" in production and "debug" in development.
func (c *Config) LogLevel() string {
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		return v
	}
	if c.Debug {
		return "debug"
	}
	return "info"
}

// Load reads configuration from environment variables.
// All values have sensible defaults for local development.
func Load() (*Config, error) {
	cfg := &Config{
		Environment: getEnv("ENVIRONMENT", "development"),
		Debug:       getEnvBool("DEBUG", true),

		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 9000),
		},

		Database: DatabaseConfig{
			Host:                getEnv("DB_HOST", "localhost"),
			Port:                getEnvInt("DB_PORT", 5432),
			User:                getEnv("DB_USER", "postgres"),
			Password:            getEnv("DB_PASSWORD", "postgres"),
			Name:                getEnv("DB_NAME", "dev_forge"),
			SSLMode:             getEnv("DB_SSL_MODE", "disable"),
			MaxConns:            int32(getEnvInt("DB_MAX_CONNS", 25)),
			MinConns:            int32(getEnvInt("DB_MIN_CONNS", 5)),
			MaxConnLifetime:     getEnvDuration("DB_MAX_CONN_LIFETIME", 1*time.Hour),
			MaxConnIdleTime:     getEnvDuration("DB_MAX_CONN_IDLE_TIME", 30*time.Minute),
			HealthCheckInterval: getEnvDuration("DB_HEALTH_CHECK_INTERVAL", 1*time.Minute),
		},

		Zitadel: ZitadelConfig{
			Issuer:   getEnv("ZITADEL_ISSUER", "http://localhost:8080"),
			ClientID: getEnv("ZITADEL_CLIENT_ID", ""),
			KeyPath:  getEnv("ZITADEL_KEY_PATH", ""),
		},

		OTEL: OTELConfig{
			Enabled:        getEnvBool("OTEL_ENABLED", false),
			ServiceName:    getEnv("OTEL_SERVICE_NAME", "dev-forge"),
			ServiceVersion: getEnv("OTEL_SERVICE_VERSION", "0.1.0"),
			OTLPEndpoint:   getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
			Environment:    getEnv("ENVIRONMENT", "development"),
		},
	}

	return cfg, cfg.validate()
}

func (c *Config) validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid SERVER_PORT: %d", c.Server.Port)
	}
	if c.Database.Host == "" {
		return fmt.Errorf("DB_HOST is required")
	}
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultValue
}
