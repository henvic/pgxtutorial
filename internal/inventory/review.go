package inventory

import (
	"context"
	"errors"
	"time"
)

// ProductReview of a product.
type ProductReview struct {
	ID          string
	ProductID   string
	ReviewerID  string
	Score       int
	Title       string
	Description string
	CreatedAt   time.Time
	ModifiedAt  time.Time
}

// CreateProductReviewParams is used when creating the review of a product.
type CreateProductReviewParams struct {
	ProductID   string
	ReviewerID  string
	Score       int
	Title       string
	Description string
}

func (p *CreateProductReviewParams) validate() error {
	if p.ProductID == "" {
		return ValidationError{"missing product ID"}
	}
	if p.ReviewerID == "" {
		return ValidationError{"missing reviewer ID"}
	}
	if err := validateScore(p.Score); err != nil {
		return err
	}
	if p.Title == "" {
		return ValidationError{"missing review title"}
	}
	if p.Description == "" {
		return ValidationError{"missing review description"}
	}
	return nil
}

// validateScore checks if the score is between 0 to 5.
func validateScore(score int) error {
	if score < 0 || score > 5 {
		return ValidationError{"invalid score"}
	}
	return nil
}

// CreateProductReviewParams is used when creating the review of a product in the database.
type CreateProductReviewDBParams struct {
	ID string
	CreateProductReviewParams
}

// ErrCreateReviewNoProduct is returned when a product review cannot be created because a product is not found.
var ErrCreateReviewNoProduct = errors.New("cannot find product to create review")

// CreateProductReview of a product.
func (s *Service) CreateProductReview(ctx context.Context, params CreateProductReviewParams) (id string, err error) {
	if err := params.validate(); err != nil {
		return "", err
	}

	id = newID()
	if err := s.db.CreateProductReview(ctx, CreateProductReviewDBParams{
		ID:                        id,
		CreateProductReviewParams: params,
	}); err != nil {
		return "", err
	}
	return id, nil
}

// UpdateProductReviewParams to use when updating an existing review.
type UpdateProductReviewParams struct {
	ID          string
	Score       *int
	Title       *string
	Description *string
}

func (p *UpdateProductReviewParams) validate() error {
	if p.ID == "" {
		return ValidationError{"missing review ID"}
	}
	if p.Score == nil && p.Title == nil && p.Description == nil {
		return ValidationError{"no product review arguments to update"}
	}
	if p.Score != nil {
		if err := validateScore(*p.Score); err != nil {
			return err
		}
	}
	if p.Title != nil && *p.Title == "" {
		return ValidationError{"missing review title"}
	}
	if p.Description != nil && *p.Description == "" {
		return ValidationError{"missing review description"}
	}
	return nil
}

// UpdateProductReview of a product.
func (s *Service) UpdateProductReview(ctx context.Context, params UpdateProductReviewParams) error {
	if err := params.validate(); err != nil {
		return err
	}
	return s.db.UpdateProductReview(ctx, params)
}

// DeleteProductReview of a product.
func (s *Service) DeleteProductReview(ctx context.Context, id string) error {
	if id == "" {
		return ValidationError{"missing review ID"}
	}
	return s.db.DeleteProductReview(ctx, id)
}

// GetProductReview gets a product review.
func (s *Service) GetProductReview(ctx context.Context, id string) (*ProductReview, error) {
	if id == "" {
		return nil, ValidationError{"missing review ID"}
	}
	return s.db.GetProductReview(ctx, id)
}

// ProductReviewsParams is used to get a list of reviews.
type ProductReviewsParams struct {
	ProductID  string
	ReviewerID string
	Pagination Pagination
}

// ProductReviewsResponse is the response from GetProductReviews.
type ProductReviewsResponse struct {
	Reviews []*ProductReview
	Total   int
}

// GetProductReviews gets a list of reviews.
func (s *Service) GetProductReviews(ctx context.Context, params ProductReviewsParams) (*ProductReviewsResponse, error) {
	if params.ReviewerID == "" && params.ProductID == "" {
		return nil, ValidationError{"missing params: reviewer_id or product_id are required"}
	}
	return s.db.GetProductReviews(ctx, params)
}
