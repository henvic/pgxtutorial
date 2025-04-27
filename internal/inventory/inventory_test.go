package inventory_test

import (
	"context"
	"errors"
	"log"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/henvic/pgxtutorial/internal/inventory"
	"go.uber.org/mock/gomock"
)

func TestMain(m *testing.M) {
	if os.Getenv("INTEGRATION_TESTDB") != "true" {
		log.Printf("Skipping tests that require database connection")
		return
	}
	os.Exit(m.Run())
}

func TestServiceCreateProduct(t *testing.T) {
	t.Parallel()
	var service = serviceWithPostgres(t)
	type args struct {
		ctx    context.Context
		params inventory.CreateProductParams
	}
	tests := []struct {
		name    string
		mock    func(t testing.TB) *inventory.MockDB // Leave as nil for using a real database implementation.
		args    args
		want    *inventory.Product
		wantErr string
	}{
		{
			name: "empty",
			args: args{
				ctx:    t.Context(),
				params: inventory.CreateProductParams{},
			},
			wantErr: "missing product ID",
		},
		{
			name: "simple",
			args: args{
				ctx: t.Context(),
				params: inventory.CreateProductParams{
					ID:          "simple",
					Name:        "product name",
					Description: "product description",
					Price:       150,
				},
			},
			want: &inventory.Product{
				ID:          "simple",
				Name:        "product name",
				Description: "product description",
				Price:       150,
				CreatedAt:   time.Now(),
				ModifiedAt:  time.Now(),
			},
			wantErr: "",
		},
		{
			name: "no_product_name",
			args: args{
				ctx: t.Context(),
				params: inventory.CreateProductParams{
					ID:          "no_product_name",
					Name:        "",
					Description: "product description",
					Price:       150,
				},
			},
			wantErr: "missing product name",
		},
		{
			name: "no_product_description",
			args: args{
				ctx: t.Context(),
				params: inventory.CreateProductParams{
					ID:    "no_product_description",
					Name:  "product name",
					Price: 150,
				},
			},
			wantErr: "missing product description",
		},
		{
			name: "negative_price",
			args: args{
				ctx: t.Context(),
				params: inventory.CreateProductParams{
					ID:          "negative_price",
					Name:        "product name",
					Description: "product description",
					Price:       -5,
				},
			},
			wantErr: "price cannot be negative",
		},
		{
			name: "canceled_ctx",
			args: args{
				ctx: canceledContext(t.Context()),
				params: inventory.CreateProductParams{
					ID:          "simple",
					Name:        "product name",
					Description: "product description",
					Price:       150,
				},
			},
			wantErr: "context canceled",
		},
		{
			name: "database_error",
			mock: func(t testing.TB) *inventory.MockDB {
				ctrl := gomock.NewController(t)
				m := inventory.NewMockDB(ctrl)
				m.EXPECT().CreateProduct(gomock.Not(gomock.Nil()),
					inventory.CreateProductParams{
						ID:          "simple",
						Name:        "product name",
						Description: "product description",
						Price:       150,
					}).Return(errors.New("unexpected error"))
				return m
			},
			args: args{
				ctx: t.Context(),
				params: inventory.CreateProductParams{
					ID:          "simple",
					Name:        "product name",
					Description: "product description",
					Price:       150,
				},
			},
			wantErr: "unexpected error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// If tt.mock is nil, use real database implementation if available. Otherwise, skip the test.
			var s = service
			if tt.mock != nil {
				s = inventory.NewService(tt.mock(t))
			} else if s == nil {
				t.Skip("required database not found, skipping test")
			}
			err := s.CreateProduct(tt.args.ctx, tt.args.params)
			if err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("Service.CreateProduct() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			// Only check integration / real implementation database for data.
			if tt.mock != nil {
				return
			}
			// Reusing GetProduct to check if the product was created successfully.
			got, err := s.GetProduct(tt.args.ctx, tt.args.params.ID)
			if err != nil {
				t.Errorf("Service.GetProduct() error = %v", err)
			}
			if !cmp.Equal(tt.want, got, cmpopts.EquateApproxTime(time.Minute)) {
				t.Errorf("value returned by Service.GetProduct() doesn't match: %v", cmp.Diff(tt.want, got))
			}
		})
	}
}

func TestServiceUpdateProduct(t *testing.T) {
	t.Parallel()
	var service = serviceWithPostgres(t)
	// Add some products that will be modified next:
	createProducts(t, service, []inventory.CreateProductParams{
		{
			ID:          "product",
			Name:        "Original name",
			Description: "This is the original description",
			Price:       250,
		},
		{
			ID:          "another",
			Name:        "Is your SQL UPDATE call correct?",
			Description: "Only the price of this one should be modified",
			Price:       99,
		},
	})

	type args struct {
		ctx    context.Context
		params inventory.UpdateProductParams
	}
	tests := []struct {
		name    string
		mock    func(t testing.TB) *inventory.MockDB // Leave as nil for using a real database implementation.
		args    args
		want    *inventory.Product
		wantErr string
	}{
		{
			name: "empty",
			args: args{
				ctx:    t.Context(),
				params: inventory.UpdateProductParams{},
			},
			wantErr: "missing product ID",
		},
		{
			name: "no_product_name",
			args: args{
				ctx: t.Context(),
				params: inventory.UpdateProductParams{
					ID:          "no_product_name",
					Name:        ptr(""),
					Description: ptr("product description"),
					Price:       ptr(150),
				},
			},
			wantErr: "missing product name",
		},
		{
			name: "no_product_description",
			args: args{
				ctx: t.Context(),
				params: inventory.UpdateProductParams{
					ID:          "no_product_description",
					Name:        ptr("product name"),
					Description: ptr(""),
					Price:       ptr(150),
				},
			},
			wantErr: "missing product description",
		},
		{
			name: "negative_price",
			args: args{
				ctx: t.Context(),
				params: inventory.UpdateProductParams{
					ID:          "negative_price",
					Name:        ptr("product name"),
					Description: ptr("product description"),
					Price:       ptr(-5),
				},
			},
			wantErr: "price cannot be negative",
		},
		{
			name: "product_name_change",
			args: args{
				ctx: t.Context(),
				params: inventory.UpdateProductParams{
					ID:   "product",
					Name: ptr("A new name"),
				},
			},
			want: &inventory.Product{
				ID:          "product",
				Name:        "A new name",
				Description: "This is the original description",
				Price:       250,
			},
		},
		{
			name: "product_description_change",
			args: args{
				ctx: t.Context(),
				params: inventory.UpdateProductParams{
					ID:          "product",
					Description: ptr("A new description"),
				},
			},
			want: &inventory.Product{
				ID:          "product",
				Name:        "A new name",
				Description: "A new description",
				Price:       250,
			},
		},
		{
			name: "product_changes",
			args: args{
				ctx: t.Context(),
				params: inventory.UpdateProductParams{
					ID:          "product",
					Name:        ptr("Even another name"),
					Description: ptr("yet another description"),
					Price:       ptr(400),
				},
			},
			want: &inventory.Product{
				ID:          "product",
				Name:        "Even another name",
				Description: "yet another description",
				Price:       400,
			},
		},
		{
			name: "not_found",
			args: args{
				ctx: t.Context(),
				params: inventory.UpdateProductParams{
					ID:   "World",
					Name: ptr("Earth"),
				},
			},
			wantErr: "product not found",
		},
		{
			name: "update_product_check_violation",
			args: args{
				ctx: t.Context(),
				params: inventory.UpdateProductParams{
					ID:   "product",
					Name: ptr(""),
				},
			},
			wantErr: "missing product name",
		},
		{
			name: "canceled_ctx",
			args: args{
				ctx: canceledContext(t.Context()),
				params: inventory.UpdateProductParams{
					ID:   "product",
					Name: ptr("Earth"),
				},
			},
			wantErr: "context canceled",
		},
		{
			name: "deadline_exceeded_ctx",
			args: args{
				ctx: deadlineExceededContext(t.Context()),
				params: inventory.UpdateProductParams{
					ID:   "product",
					Name: ptr("Earth"),
				},
			},
			wantErr: "context deadline exceeded",
		},
		{
			name: "another_product_price_change",
			args: args{
				ctx: t.Context(),
				params: inventory.UpdateProductParams{
					ID:    "another",
					Price: ptr(97),
				},
			},
			want: &inventory.Product{
				ID:          "another",
				Name:        "Is your SQL UPDATE call correct?",
				Description: "Only the price of this one should be modified",
				Price:       97,
			},
		},
		{
			name: "no_changes",
			args: args{
				ctx: t.Context(),
				params: inventory.UpdateProductParams{
					ID: "no_changes",
				},
			},
			wantErr: "no product arguments to update",
		},
		{
			name: "database_error",
			mock: func(t testing.TB) *inventory.MockDB {
				ctrl := gomock.NewController(t)
				m := inventory.NewMockDB(ctrl)
				m.EXPECT().UpdateProduct(gomock.Not(gomock.Nil()),
					inventory.UpdateProductParams{
						ID:          "simple",
						Name:        ptr("product name"),
						Description: ptr("product description"),
						Price:       ptr(150),
					}).Return(errors.New("unexpected error"))
				return m
			},
			args: args{
				ctx: t.Context(),
				params: inventory.UpdateProductParams{
					ID:          "simple",
					Name:        ptr("product name"),
					Description: ptr("product description"),
					Price:       ptr(150),
				},
			},
			wantErr: "unexpected error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// If tt.mock is nil, use real database implementation if available. Otherwise, skip the test.
			var s = service
			if tt.mock != nil {
				s = inventory.NewService(tt.mock(t))
			} else if s == nil {
				t.Skip("required database not found, skipping test")
			}
			err := s.UpdateProduct(tt.args.ctx, tt.args.params)
			if err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("Service.UpdateProduct() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			got, err := s.GetProduct(tt.args.ctx, tt.args.params.ID)
			if err != nil {
				t.Errorf("Service.GetProduct() error = %v", err)
			}
			if got.CreatedAt.IsZero() {
				t.Error("Service.GetProduct() returned CreatedAt should not be zero")
			}
			if !got.CreatedAt.Before(got.ModifiedAt) {
				t.Error("Service.GetProduct() should return CreatedAt < ModifiedAt")
			}
			// Ignore or CreatedAt and ModifiedAt before comparing structs.
			if !cmp.Equal(tt.want, got, cmpopts.IgnoreFields(inventory.Product{}, "CreatedAt", "ModifiedAt")) {
				t.Errorf("value returned by DB.GetProduct() doesn't match: %v", cmp.Diff(tt.want, got))
			}
		})
	}
}

func TestServiceDeleteProduct(t *testing.T) {
	t.Parallel()
	var service = serviceWithPostgres(t)
	createProducts(t, service, []inventory.CreateProductParams{
		{
			ID:          "product",
			Name:        "Product name",
			Description: "Product description",
			Price:       123,
		},
	})

	type args struct {
		ctx context.Context
		id  string
	}
	tests := []struct {
		name    string
		args    args
		mock    func(t testing.TB) *inventory.MockDB // Leave as nil for using a real database implementation.
		wantErr string
	}{
		{
			name: "missing_product_id",
			args: args{
				ctx: t.Context(),
				id:  "",
			},
			wantErr: "missing product ID",
		},
		{
			name: "product",
			args: args{
				ctx: t.Context(),
				id:  "product",
			},
			wantErr: "",
		},
		// calling delete multiple times should not fail
		{
			name: "product_already_deleted",
			args: args{
				ctx: t.Context(),
				id:  "product",
			},
			wantErr: "",
		},
		// delete should be idempotent
		{
			name: "not_found",
			args: args{
				ctx: t.Context(),
				id:  "xyz",
			},
		},
		{
			name: "canceled_ctx",
			args: args{
				ctx: canceledContext(t.Context()),
				id:  "product",
			},
			wantErr: "context canceled",
		},
		{
			name: "deadline_exceeded_ctx",
			args: args{
				ctx: deadlineExceededContext(t.Context()),
				id:  "product",
			},
			wantErr: "context deadline exceeded",
		},
		{
			name: "database_error",
			args: args{
				ctx: t.Context(),
				id:  "product",
			},
			mock: func(t testing.TB) *inventory.MockDB {
				ctrl := gomock.NewController(t)
				m := inventory.NewMockDB(ctrl)
				m.EXPECT().DeleteProduct(gomock.Not(gomock.Nil()), "product").Return(errors.New("unexpected error"))
				return m
			},
			wantErr: "unexpected error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// If tt.mock is nil, use real database implementation if available. Otherwise, skip the test.
			var s = service
			if tt.mock != nil {
				s = inventory.NewService(tt.mock(t))
			} else if s == nil {
				t.Skip("required database not found, skipping test")
			}
			if err := s.DeleteProduct(tt.args.ctx, tt.args.id); err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("Service.DeleteProduct() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestServiceGetProduct(t *testing.T) {
	t.Parallel()
	var service = serviceWithPostgres(t)
	createProducts(t, service, []inventory.CreateProductParams{
		{
			ID:          "product",
			Name:        "A product name",
			Description: "A great description",
			Price:       10000,
		},
	})

	type args struct {
		ctx context.Context
		id  string
	}
	tests := []struct {
		name    string
		args    args
		mock    func(t testing.TB) *inventory.MockDB // Leave as nil for using a real database implementation.
		want    *inventory.Product
		wantErr string
	}{
		{
			name: "missing_product_id",
			args: args{
				ctx: t.Context(),
				id:  "",
			},
			wantErr: "missing product ID",
		},
		{
			name: "product",
			args: args{
				ctx: t.Context(),
				id:  "product",
			},
			want: &inventory.Product{
				ID:          "product",
				Name:        "A product name",
				Description: "A great description",
				Price:       10000,
				CreatedAt:   time.Now(),
				ModifiedAt:  time.Now(),
			},
		},
		{
			name: "not_found",
			args: args{
				ctx: t.Context(),
				id:  "not_found",
			},
			want: nil,
		},
		{
			name: "canceled_ctx",
			args: args{
				ctx: canceledContext(t.Context()),
				id:  "product",
			},
			wantErr: "context canceled",
		},
		{
			name: "deadline_exceeded_ctx",
			args: args{
				ctx: deadlineExceededContext(t.Context()),
				id:  "product",
			},
			wantErr: "context deadline exceeded",
		},
		{
			name: "database_error",
			args: args{
				ctx: t.Context(),
				id:  "product",
			},
			mock: func(t testing.TB) *inventory.MockDB {
				ctrl := gomock.NewController(t)
				m := inventory.NewMockDB(ctrl)
				m.EXPECT().GetProduct(gomock.Not(gomock.Nil()), "product").Return(nil, errors.New("unexpected error"))
				return m
			},
			wantErr: "unexpected error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// If tt.mock is nil, use real database implementation if available. Otherwise, skip the test.
			var s = service
			if tt.mock != nil {
				s = inventory.NewService(tt.mock(t))
			} else if s == nil {
				t.Skip("required database not found, skipping test")
			}
			got, err := s.GetProduct(tt.args.ctx, tt.args.id)
			if err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("Service.GetProduct() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if !cmp.Equal(tt.want, got, cmpopts.EquateApproxTime(time.Minute)) {
				t.Errorf("value returned by Service.GetProduct() doesn't match: %v", cmp.Diff(tt.want, got))
			}
		})
	}
}

func TestServiceSearchProducts(t *testing.T) {
	t.Parallel()
	var service = serviceWithPostgres(t)
	createProducts(t, service, []inventory.CreateProductParams{
		{
			ID:          "desk",
			Name:        "plain desk (home)",
			Description: "A plain desk",
			Price:       140,
		},
		{
			ID:          "chair",
			Name:        "office chair",
			Description: "Office chair",
			Price:       80,
		},
		{
			ID:          "table",
			Name:        "dining home table",
			Description: "dining table",
			Price:       120,
		},
		{
			ID:          "bed",
			Name:        "bed",
			Description: "small bed",
			Price:       100,
		},
	})

	type args struct {
		ctx    context.Context
		params inventory.SearchProductsParams
	}
	tests := []struct {
		name    string
		args    args
		mock    func(t testing.TB) *inventory.MockDB // Leave as nil for using a real database implementation.
		want    *inventory.SearchProductsResponse
		wantErr string
	}{
		{
			name: "product",
			args: args{
				ctx: t.Context(),
				params: inventory.SearchProductsParams{
					QueryString: "plain desk",
					Pagination: inventory.Pagination{
						Limit: 10,
					},
				},
			},
			want: &inventory.SearchProductsResponse{
				Items: []*inventory.Product{
					{
						ID:          "desk",
						Name:        "plain desk (home)",
						Description: "A plain desk",
						Price:       140,
						CreatedAt:   time.Now(),
						ModifiedAt:  time.Now(),
					},
				},
				Total: 1,
			},
			wantErr: "",
		},
		{
			name: "missing_search_term",
			args: args{
				ctx: t.Context(),
				params: inventory.SearchProductsParams{
					QueryString: "",
					Pagination: inventory.Pagination{
						Limit: 10,
					},
				},
			},
			wantErr: "missing search string",
		},
		{
			name: "negative_min_price",
			args: args{
				ctx: t.Context(),
				params: inventory.SearchProductsParams{
					QueryString: "value",
					MinPrice:    -1,
					Pagination: inventory.Pagination{
						Limit: 10,
					},
				},
			},
			wantErr: "min price cannot be negative",
		},
		{
			name: "negative_max_price",
			args: args{
				ctx: t.Context(),
				params: inventory.SearchProductsParams{
					QueryString: "value",
					MaxPrice:    -1,
					Pagination: inventory.Pagination{
						Limit: 10,
					},
				},
			},
			wantErr: "max price cannot be negative",
		},
		{
			name: "missing_pagination_limit",
			args: args{
				ctx: t.Context(),
				params: inventory.SearchProductsParams{
					QueryString: "plain desk",
				},
			},
			wantErr: "pagination limit must be at least 1",
		},
		{
			name: "bad_pagination_offset",
			args: args{
				ctx: t.Context(),
				params: inventory.SearchProductsParams{
					QueryString: "plain desk",
					Pagination: inventory.Pagination{
						Limit:  10,
						Offset: -1,
					},
				},
			},
			wantErr: "pagination offset cannot be negative",
		},
		{
			name: "product_very_expensive",
			args: args{
				ctx: t.Context(),
				params: inventory.SearchProductsParams{
					QueryString: "plain desk",
					MinPrice:    900,
					Pagination: inventory.Pagination{
						Limit: 10,
					},
				},
			},
			want: &inventory.SearchProductsResponse{
				Items: []*inventory.Product{},
				Total: 0,
			},
			wantErr: "",
		},
		{
			name: "home",
			args: args{
				ctx: t.Context(),
				params: inventory.SearchProductsParams{
					QueryString: "home",
					Pagination: inventory.Pagination{
						Limit: 10,
					},
				},
			},
			want: &inventory.SearchProductsResponse{
				Items: []*inventory.Product{
					{
						ID:          "table",
						Name:        "dining home table",
						Description: "dining table",
						Price:       120,
						CreatedAt:   time.Now(),
						ModifiedAt:  time.Now(),
					},
					{
						ID:          "desk",
						Name:        "plain desk (home)",
						Description: "A plain desk",
						Price:       140,
						CreatedAt:   time.Now(),
						ModifiedAt:  time.Now(),
					},
				},
				Total: 2,
			},
			wantErr: "",
		},
		{
			name: "home_paginated",
			args: args{
				ctx: t.Context(),
				params: inventory.SearchProductsParams{
					QueryString: "home",
					Pagination: inventory.Pagination{
						Limit:  1,
						Offset: 1,
					},
				},
			},
			want: &inventory.SearchProductsResponse{
				Items: []*inventory.Product{
					{
						ID:          "desk",
						Name:        "plain desk (home)",
						Description: "A plain desk",
						Price:       140,
						CreatedAt:   time.Now(),
						ModifiedAt:  time.Now(),
					},
				},
				Total: 2,
			},
			wantErr: "",
		},
		{
			name: "home_cheaper",
			args: args{
				ctx: t.Context(),
				params: inventory.SearchProductsParams{
					QueryString: "home",
					MaxPrice:    130,
					Pagination: inventory.Pagination{
						Limit: 10,
					},
				},
			},
			want: &inventory.SearchProductsResponse{
				Items: []*inventory.Product{
					{
						ID:          "table",
						Name:        "dining home table",
						Description: "dining table",
						Price:       120,
						CreatedAt:   time.Now(),
						ModifiedAt:  time.Now(),
					},
				},
				Total: 1,
			},
			wantErr: "",
		},
		{
			name: "not_found",
			args: args{
				ctx: t.Context(),
				params: inventory.SearchProductsParams{
					QueryString: "xyz",
					Pagination: inventory.Pagination{
						Limit: 10,
					},
				},
			},
			want: &inventory.SearchProductsResponse{
				Items: []*inventory.Product{},
				Total: 0,
			},
		},
		{
			name: "canceled_ctx",
			args: args{
				ctx: canceledContext(t.Context()),
				params: inventory.SearchProductsParams{
					QueryString: "xyz",
					Pagination: inventory.Pagination{
						Limit: 10,
					},
				},
			},
			wantErr: "context canceled",
		},
		{
			name: "deadline_exceeded_ctx",
			args: args{
				ctx: deadlineExceededContext(t.Context()),
				params: inventory.SearchProductsParams{
					QueryString: "xyz",
					Pagination: inventory.Pagination{
						Limit: 10,
					},
				},
			},
			wantErr: "context deadline exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// If tt.mock is nil, use real database implementation if available. Otherwise, skip the test.
			var s = service
			if tt.mock != nil {
				s = inventory.NewService(tt.mock(t))
			} else if s == nil {
				t.Skip("required database not found, skipping test")
			}
			got, err := s.SearchProducts(tt.args.ctx, tt.args.params)
			if err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("Service.SearchProducts() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if !cmp.Equal(tt.want, got, cmpopts.EquateApproxTime(time.Minute)) {
				t.Errorf("value returned by Service.SearchProducts() doesn't match: %v", cmp.Diff(tt.want, got))
			}
		})
	}
}
