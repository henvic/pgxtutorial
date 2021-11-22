package database

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// NewPGXPool is a PostgreSQL connection pool for pgx.
//
// Usage:
// pgPool := database.NewPGXPool(context.Background(), "", &PGXStdLogger{}, pgx.LogLevelInfo)
// defer pgPool.Close() // Close any remaining connections before shutting down your application.
//
// Instead of passing a configuration explictly with a connString,
// you might use PG environment variables such as the following to configure the database:
// PGDATABASE, PGHOST, PGPORT, PGUSER, PGPASSWORD, PGCONNECT_TIMEOUT, etc.
// Reference: https://www.postgresql.org/docs/current/libpq-envars.html
func NewPGXPool(ctx context.Context, connString string, logger pgx.Logger, logLevel pgx.LogLevel) (*pgxpool.Pool, error) {
	conf, err := pgxpool.ParseConfig(connString) // Using environment variables instead of a connection string.
	if err != nil {
		return nil, err
	}

	conf.ConnConfig.Logger = logger

	// Set the log level for pgx, if set.
	if logLevel != 0 {
		conf.ConnConfig.LogLevel = logLevel
	}

	// pgx, by default, does some I/O operation on initialization of a pool to check if the database is reachable.
	// Comment the following line if you don't want pgx to try to connect pool once the Connect function is called,
	//
	// If comment it, and your application seems stuck, you probably forgot to set up PGCONNECT_TIMEOUT,
	// and your code is hanging waiting for a connection to be established.
	conf.LazyConnect = true

	// pgxpool default max number of connections is the number of CPUs on your machine returned by runtime.NumCPU().
	// This number is very conservative, and you might be able to improve performance for highly concurrent applications
	// by increasing it.
	// conf.MaxConns = runtime.NumCPU() * 5

	pool, err := pgxpool.ConnectConfig(ctx, conf)
	if err != nil {
		return nil, fmt.Errorf("pgx connection error: %w", err)
	}
	return pool, nil
}

// LogLevelFromEnv returns the pgx.LogLevel from the environment variable PGX_LOG_LEVEL.
// By default this is info (pgx.LogLevelInfo), which is good for development.
// For deployments, something like pgx.LogLevelWarn is better choice.
func LogLevelFromEnv() (pgx.LogLevel, error) {
	if level := os.Getenv("PGX_LOG_LEVEL"); level != "" {
		l, err := pgx.LogLevelFromString(level)
		if err != nil {
			return pgx.LogLevelDebug, fmt.Errorf("pgx configuration: %w", err)
		}
		return l, nil
	}
	return pgx.LogLevelInfo, nil
}

// PGXStdLogger prints pgx logs to the standard logger.
// os.Stderr by default.
type PGXStdLogger struct{}

func (l *PGXStdLogger) Log(ctx context.Context, level pgx.LogLevel, msg string, data map[string]interface{}) {
	args := make([]interface{}, 0, len(data)+2) // making space for arguments + level + msg
	args = append(args, level, msg)
	for k, v := range data {
		args = append(args, fmt.Sprintf("%s=%v", k, v))
	}
	log.Println(args...)
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
