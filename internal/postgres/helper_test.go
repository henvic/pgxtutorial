package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/henvic/pgxtutorial/internal/inventory"
)

func createProducts(t testing.TB, db DB, products []inventory.CreateProductParams) {
	for _, p := range products {
		if err := db.CreateProduct(context.Background(), p); err != nil {
			t.Errorf("DB.CreateProduct() error = %v", err)
		}
	}
}

func createProductReviews(t testing.TB, db DB, reviews []inventory.CreateProductReviewDBParams) {
	for _, r := range reviews {
		if err := db.CreateProductReview(context.Background(), r); err != nil {
			t.Errorf("DB.CreateProductReview() error = %v", err)
		}
	}
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func deadlineExceededContext() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), -time.Second)
	cancel()
	return ctx
}

func newInt(n int) *int {
	return &n
}

func newString(s string) *string {
	return &s
}
