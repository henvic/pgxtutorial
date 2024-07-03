package inventory

import (
	"context"
	"crypto/rand"
)

// NewService creates an API service.
func NewService(db DB) *Service {
	return &Service{db: db}
}

// Service for the API.
type Service struct {
	db DB
}

// DB layer.
//
//go:generate mockgen --build_flags=--mod=mod -package inventory -destination mock_db_test.go . DB
type DB interface {
	// CreateProduct creates a new product.
	CreateProduct(ctx context.Context, params CreateProductParams) error

	// UpdateProduct updates an existing product.
	UpdateProduct(ctx context.Context, params UpdateProductParams) error

	// GetProduct returns a product.
	GetProduct(ctx context.Context, id string) (*Product, error)

	// SearchProducts returns a list of products.
	SearchProducts(ctx context.Context, params SearchProductsParams) (*SearchProductsResponse, error)

	// DeleteProduct deletes a product.
	DeleteProduct(ctx context.Context, id string) error

	// CreateProductReview for a given product.
	CreateProductReview(ctx context.Context, params CreateProductReviewDBParams) error

	// UpdateProductReview for a given product.
	UpdateProductReview(ctx context.Context, params UpdateProductReviewParams) error

	// GetProductReview gets a specific review.
	GetProductReview(ctx context.Context, id string) (*ProductReview, error)

	// GetProductReviews gets reviews for a given product or from a given user.
	GetProductReviews(ctx context.Context, params ProductReviewsParams) (*ProductReviewsResponse, error)

	// DeleteProductReview deletes a review.
	DeleteProductReview(ctx context.Context, id string) error
}

// ValidationError is returned when there is an invalid parameter received.
type ValidationError struct {
	s string
}

func (e ValidationError) Error() string {
	return e.s
}

// Pagination is used to paginate results.
//
// Usage:
//
//	Pagination{
//		Limit: limit,
//		Offset: (page - 1) * limit
//	}
type Pagination struct {
	// Limit is the maximum number of results to return on this page.
	Limit int

	// Offset is the number of results to skip from the beginning of the results.
	// Typically: (page number - 1) * limit.
	Offset int
}

// Validate pagination.
func (p *Pagination) Validate() error {
	if p.Limit < 1 {
		return ValidationError{"pagination limit must be at least 1"}
	}
	if p.Offset < 0 {
		return ValidationError{"pagination offset cannot be negative"}
	}
	return nil
}

// newID generates a random base-58 ID.
func newID() string {
	const (
		alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz" // base58
		size     = 11
	)
	var id = make([]byte, size)
	if _, err := rand.Read(id); err != nil {
		panic(err)
	}
	for i, p := range id {
		id[i] = alphabet[int(p)%len(alphabet)] // discard everything but the least significant bits
	}
	return string(id)
}
