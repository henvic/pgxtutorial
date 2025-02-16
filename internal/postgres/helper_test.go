package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/henvic/pgxtutorial/internal/inventory"
)

func createProducts(t testing.TB, db DB, products []inventory.CreateProductParams) {
	for _, p := range products {
		if err := db.CreateProduct(t.Context(), p); err != nil {
			t.Errorf("DB.CreateProduct() error = %v", err)
		}
	}
}

func createProductReviews(t testing.TB, db DB, reviews []inventory.CreateProductReviewDBParams) {
	for _, r := range reviews {
		if err := db.CreateProductReview(t.Context(), r); err != nil {
			t.Errorf("DB.CreateProductReview() error = %v", err)
		}
	}
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
