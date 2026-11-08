package database

import (
	"context"
	"fmt"
	"time"

	"github.com/cristianortiz/dev-forge/internal/shared/config"
	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

// DB wraps a pgxpool connection pool and provides helper methods.
type DB struct {
	Pool   *pgxpool.Pool
	logger *zap.Logger
}

// newTracer builds the pgx query tracer from the global providers, so it must be
// created after telemetry.Setup. Spans are only recorded under a recording
// parent (the HTTP request span), which keeps startup and background pool
// activity out of the traces.
//
// Defaults worth knowing: db.query.text holds the SQL with $N placeholders and
// query arguments are never recorded (that needs otelpgx.WithIncludeQueryParameters,
// deliberately not used). Keep repositories parameterized: a value concatenated
// into the SQL would reach the span. Span names carry the operation kind
// ("query SELECT", "prepare SELECT"); without the prefix pgx's prepare step,
// which runs inside the query on first use, would look like a duplicate query.
func newTracer() *otelpgx.Tracer {
	return otelpgx.NewTracer(
		otelpgx.WithTracerProvider(otel.GetTracerProvider()),
		otelpgx.WithMeterProvider(otel.GetMeterProvider()),
		otelpgx.WithQuerySpanNamePrefix(),
	)
}

// New creates and verifies a pgxpool connection pool.
func New(ctx context.Context, cfg *config.Config, logger *zap.Logger) (*DB, error) {
	logger.Info("initializing PostgreSQL connection pool",
		zap.String("host", cfg.Database.Host),
		zap.Int("port", cfg.Database.Port),
		zap.String("database", cfg.Database.Name),
	)

	poolCfg, err := pgxpool.ParseConfig(cfg.Database.URL())
	if err != nil {
		return nil, fmt.Errorf("parse pool config: %w", err)
	}

	if cfg.OTEL.Enabled {
		poolCfg.ConnConfig.Tracer = newTracer()
		logger.Info("PostgreSQL OTEL tracing enabled")
	}

	poolCfg.MaxConns = cfg.Database.MaxConns
	poolCfg.MinConns = cfg.Database.MinConns
	poolCfg.MaxConnLifetime = cfg.Database.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.Database.MaxConnIdleTime
	poolCfg.HealthCheckPeriod = cfg.Database.HealthCheckInterval

	ctxTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctxTimeout, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(ctxTimeout); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	logger.Info("PostgreSQL connection pool ready")

	return &DB{Pool: pool, logger: logger}, nil
}

// Close shuts down the connection pool gracefully.
func (db *DB) Close() {
	db.logger.Info("closing PostgreSQL connection pool")
	db.Pool.Close()
}

// Ping verifies the database is reachable.
func (db *DB) Ping(ctx context.Context) error {
	return db.Pool.Ping(ctx)
}
