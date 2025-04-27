package inventory_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/henvic/pgtools/sqltest"
	"github.com/henvic/pgxtutorial/internal/inventory"
	"github.com/henvic/pgxtutorial/internal/postgres"
)

func createProducts(t testing.TB, s *inventory.Service, products []inventory.CreateProductParams) {
	for _, p := range products {
		if err := s.CreateProduct(t.Context(), p); err != nil {
			t.Errorf("Service.CreateProduct() error = %v", err)
		}
	}
}

func createProductReview(t testing.TB, s *inventory.Service, review inventory.CreateProductReviewParams) (id string) {
	id, err := s.CreateProductReview(t.Context(), review)
	if err != nil {
		t.Errorf("DB.CreateProductReview() error = %v", err)
	}
	return id
}

func canceledContext(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	return ctx
}

func deadlineExceededContext(ctx context.Context) context.Context {
	ctx, cancel := context.WithTimeout(ctx, -time.Second)
	cancel()
	return ctx
}

// ptr returns a pointer to the given value.
func ptr[T any](v T) *T {
	return &v
}

// serviceWithPostgres returns a new inventory.Service backed by a postgres.DB, if available.
// Otherwise, it returns nil.
func serviceWithPostgres(t *testing.T) *inventory.Service {
	t.Helper()
	var db *inventory.Service
	// Initialize migration and infrastructure for running tests that uses a real implementation of PostgreSQL
	// if the INTEGRATION_TESTDB environment variable is set to true.
	if os.Getenv("INTEGRATION_TESTDB") == "true" {
		migration := sqltest.New(t, sqltest.Options{
			Force:                   true,
			TemporaryDatabasePrefix: "test_inventory_pkg", // Avoid a clash between database names of packages on parallel execution.
			Files:                   os.DirFS("../../migrations"),
		})
		db = inventory.NewService(postgres.NewDB(migration.Setup(t.Context(), ""), slog.Default()))
	}
	return db
}
