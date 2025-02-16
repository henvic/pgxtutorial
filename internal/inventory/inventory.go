package inventory

import (
	"context"
	"time"
)

// Product on the catalog.
type Product struct {
	ID          string
	Name        string
	Description string
	Price       int
	CreatedAt   time.Time
	ModifiedAt  time.Time
}

// CreateProductParams used by CreateProduct.
type CreateProductParams struct {
	ID          string
	Name        string
	Description string
	Price       int
}

func (p *CreateProductParams) validate() error {
	if p.ID == "" {
		return ValidationError{"missing product ID"}
	}
	if p.Name == "" {
		return ValidationError{"missing product name"}
	}
	if p.Description == "" {
		return ValidationError{"missing product description"}
	}
	if p.Price < 0 {
		return ValidationError{"price cannot be negative"}
	}
	return nil
}

// CreateProduct creates a new product.
func (s *Service) CreateProduct(ctx context.Context, params CreateProductParams) (err error) {
	if err := params.validate(); err != nil {
		return err
	}
	return s.db.CreateProduct(ctx, params)
}

// UpdateProductParams used by UpdateProduct.
type UpdateProductParams struct {
	ID          string
	Name        *string
	Description *string
	Price       *int
}

func (p *UpdateProductParams) validate() error {
	if p.ID == "" {
		return ValidationError{"missing product ID"}
	}
	if p.Name == nil && p.Description == nil && p.Price == nil {
		return ValidationError{"no product arguments to update"}
	}
	if p.Name != nil && *p.Name == "" {
		return ValidationError{"missing product name"}
	}
	if p.Description != nil && *p.Description == "" {
		return ValidationError{"missing product description"}
	}
	if p.Price != nil && *p.Price < 0 {
		return ValidationError{"price cannot be negative"}
	}
	return nil
}

// UpdateProduct creates a new product.
func (s *Service) UpdateProduct(ctx context.Context, params UpdateProductParams) (err error) {
	if err := params.validate(); err != nil {
		return err
	}
	return s.db.UpdateProduct(ctx, params)
}

// DeleteProduct deletes a product.
func (s *Service) DeleteProduct(ctx context.Context, id string) (err error) {
	if id == "" {
		return ValidationError{"missing product ID"}
	}
	return s.db.DeleteProduct(ctx, id)
}

// GetProduct returns a product.
func (s *Service) GetProduct(ctx context.Context, id string) (*Product, error) {
	if id == "" {
		return nil, ValidationError{"missing product ID"}
	}
	return s.db.GetProduct(ctx, id)
}

// SearchProductsParams used by SearchProducts.
type SearchProductsParams struct {
	QueryString string
	MinPrice    int
	MaxPrice    int
	Pagination  Pagination
}

func (p *SearchProductsParams) validate() error {
	if p.QueryString == "" {
		return ValidationError{"missing search string"}
	}
	if p.MinPrice < 0 {
		return ValidationError{"min price cannot be negative"}
	}
	if p.MaxPrice < 0 {
		return ValidationError{"max price cannot be negative"}
	}
	return p.Pagination.Validate()
}

// SearchProductsResponse from SearchProducts.
type SearchProductsResponse struct {
	Items []*Product
	Total int32
}

// SearchProducts returns a list of products.
func (s *Service) SearchProducts(ctx context.Context, params SearchProductsParams) (*SearchProductsResponse, error) {
	if err := params.validate(); err != nil {
		return nil, err
	}
	return s.db.SearchProducts(ctx, params)
}
