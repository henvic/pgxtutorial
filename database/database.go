package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
	"go.opentelemetry.io/otel/trace"
)

// NewPGXPool is a PostgreSQL connection pool for pgx.
//
// Usage:
// pgPool := database.NewPGXPool(context.Background(), "", &PGXStdLogger{Logger: slog.Default()}, tracelog.LogLevelInfo, tracer)
// defer pgPool.Close() // Close any remaining connections before shutting down your application.
//
// Instead of passing a configuration explicitly with a connString,
// you might use PG environment variables such as the following to configure the database:
// PGDATABASE, PGHOST, PGPORT, PGUSER, PGPASSWORD, PGCONNECT_TIMEOUT, etc.
// Reference: https://www.postgresql.org/docs/current/libpq-envars.html
func NewPGXPool(ctx context.Context, connString string, logger tracelog.Logger, logLevel tracelog.LogLevel, tracer trace.TracerProvider) (*pgxpool.Pool, error) {
	conf, err := pgxpool.ParseConfig(connString) // Using environment variables instead of a connection string.
	if err != nil {
		return nil, err
	}

	// Use github.com/exaring/otelpgx if a tracer is defined.
	// Otherwise, use github.com/jackc/pgx/v5/tracelog with the logger.
	if tracer != nil {
		conf.ConnConfig.Tracer = otelpgx.NewTracer(otelpgx.WithTracerProvider(tracer))
	} else {
		conf.ConnConfig.Tracer = &tracelog.TraceLog{
			Logger:   logger,
			LogLevel: logLevel,
		}
	}

	// pgxpool default max number of connections is the number of CPUs on your machine returned by runtime.NumCPU().
	// This number is very conservative, and you might be able to improve performance for highly concurrent applications
	// by increasing it.
	// conf.MaxConns = runtime.NumCPU() * 5
	pool, err := pgxpool.NewWithConfig(ctx, conf)
	if err != nil {
		return nil, fmt.Errorf("pgx connection error: %w", err)
	}
	return pool, nil
}

// LogLevelFromEnv returns the tracelog.LogLevel from the environment variable PGX_LOG_LEVEL.
// By default this is info (tracelog.LogLevelInfo), which is good for development.
// For deployments, something like tracelog.LogLevelWarn is better choice.
func LogLevelFromEnv() (tracelog.LogLevel, error) {
	if level := os.Getenv("PGX_LOG_LEVEL"); level != "" {
		l, err := tracelog.LogLevelFromString(level)
		if err != nil {
			return tracelog.LogLevelDebug, fmt.Errorf("pgx configuration: %w", err)
		}
		return l, nil
	}
	return tracelog.LogLevelInfo, nil
}

// PGXStdLogger prints pgx logs to the standard logger.
// os.Stderr by default.
type PGXStdLogger struct {
	Logger *slog.Logger
}

func (l *PGXStdLogger) Log(ctx context.Context, level tracelog.LogLevel, msg string, data map[string]any) {
	attrs := make([]slog.Attr, 0, len(data)+1)
	attrs = append(attrs, slog.String("pgx_level", level.String()))
	for k, v := range data {
		attrs = append(attrs, slog.Any(k, v))
	}
	l.Logger.LogAttrs(ctx, slogLevel(level), msg, attrs...)
}

// slogLevel translates pgx log level to slog log level.
func slogLevel(level tracelog.LogLevel) slog.Level {
	switch level {
	case tracelog.LogLevelTrace, tracelog.LogLevelDebug:
		return slog.LevelDebug
	case tracelog.LogLevelInfo:
		return slog.LevelInfo
	case tracelog.LogLevelWarn:
		return slog.LevelWarn
	default:
		// If tracelog.LogLevelError, tracelog.LogLevelNone, or any other unknown level, use slog.LevelError.
		return slog.LevelError
	}
}

// PgErrors returns a multi-line error printing more information from *pgconn.PgError to make debugging faster.
func PgErrors(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	return fmt.Errorf(`%w
Code: %v
Detail: %v
Hint: %v
Position: %v
InternalPosition: %v
InternalQuery: %v
Where: %v
SchemaName: %v
TableName: %v
ColumnName: %v
DataTypeName: %v
ConstraintName: %v
File: %v:%v
Routine: %v`,
		err,
		pgErr.Code,
		pgErr.Detail,
		pgErr.Hint,
		pgErr.Position,
		pgErr.InternalPosition,
		pgErr.InternalQuery,
		pgErr.Where,
		pgErr.SchemaName,
		pgErr.TableName,
		pgErr.ColumnName,
		pgErr.DataTypeName,
		pgErr.ConstraintName,
		pgErr.File, pgErr.Line,
		pgErr.Routine)
}
