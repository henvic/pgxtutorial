package database

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

func TestMain(m *testing.M) {
	if os.Getenv("INTEGRATION_TESTDB") != "true" {
		log.Printf("Skipping tests that require database connection")
		return
	}
	os.Exit(m.Run())
}

func TestNewPGXPool(t *testing.T) {
	t.Parallel()

	pool, err := NewPGXPool(context.Background(), "", &PGXStdLogger{}, pgx.LogLevelInfo)
	if err != nil {
		t.Fatalf("NewPGXPool() error: %v", err)
	}
	defer pool.Close()

	// Check reachability.
	if _, err = pool.Exec(context.Background(), `SELECT 1`); err != nil {
		t.Errorf("pool.Exec() error: %v", err)
	}
}

func TestNewPGXPoolErrors(t *testing.T) {
	t.Parallel()
	type args struct {
		ctx        context.Context
		connString string
		logger     pgx.Logger
		logLevel   pgx.LogLevel
	}
	tests := []struct {
		name    string
		args    args
		want    *pgxpool.Pool
		wantErr bool
	}{
		{
			name: "invalid_connection_string",
			args: args{
				ctx:        context.Background(),
				connString: "http://localhost",
				logger:     &PGXStdLogger{},
				logLevel:   pgx.LogLevelInfo,
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewPGXPool(tt.args.ctx, tt.args.connString, tt.args.logger, tt.args.logLevel)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewPGXPool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && got != nil {
				t.Errorf("NewPGXPool() = %v, want nil", got)
			}
		})
	}
}

func TestLogLevelFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		want    pgx.LogLevel
		wantErr string
	}{
		{
			name: "default",
			want: pgx.LogLevelInfo,
		},
		{
			name: "warn",
			env:  "warn",
			want: pgx.LogLevelWarn,
		},
		{
			name:    "error",
			env:     "bad",
			want:    pgx.LogLevelDebug,
			wantErr: "pgx configuration: invalid log level",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("PGX_LOG_LEVEL", tt.env)
			}
			got, err := LogLevelFromEnv()
			if err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("LogLevelFromEnv() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("LogLevelFromEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPgErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		err     error
		wantErr string
	}{
		{
			name:    "nil",
			wantErr: "",
		},
		{
			name:    "other",
			err:     context.Canceled,
			wantErr: "context canceled",
		},
		{
			name: "essential",
			err: &pgconn.PgError{
				Severity:         "ERROR",
				Message:          "msg",
				Code:             "007",
				Detail:           "detail",
				Hint:             "hint",
				Position:         2,
				InternalPosition: 4,
				InternalQuery:    "q",
				Where:            "w",
				SchemaName:       "public",
				TableName:        "names",
				ColumnName:       "field",
				DataTypeName:     "jsonb",
				ConstraintName:   "foo_id_fkey",
				File:             "main.c",
				Line:             14,
				Routine:          "a",
			},
			wantErr: `ERROR: msg (SQLSTATE 007)
Code: 007
Detail: detail
Hint: hint
Position: 2
InternalPosition: 4
InternalQuery: q
Where: w
SchemaName: public
TableName: names
ColumnName: field
DataTypeName: jsonb
ConstraintName: foo_id_fkey
File: main.c:14
Routine: a`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := PgErrors(tt.err); err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("PgErrors() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
