package postgres

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/henvic/pgtools/sqltest"
	"github.com/henvic/pgxtutorial/internal/inventory"
)

var force = flag.Bool("force", false, "Force cleaning the database before starting")

func TestMain(m *testing.M) {
	if os.Getenv("INTEGRATION_TESTDB") != "true" {
		log.Printf("Skipping tests that require database connection")
		return
	}
	os.Exit(m.Run())
}

func TestTransactionContext(t *testing.T) {
	t.Parallel()

	migration := sqltest.New(t, sqltest.Options{
		Force: *force,
		Files: os.DirFS("../../migrations"),
	})
	pool := migration.Setup(context.Background(), "")
	db := NewDB(pool, slog.Default())

	ctx, err := db.TransactionContext(context.Background())
	if err != nil {
		t.Errorf("cannot create transaction context: %v", err)
	}
	defer db.Rollback(ctx)

	if err := db.Commit(ctx); err != nil {
		t.Errorf("cannot commit: %v", err)
	}
}

func TestTransactionContextCanceled(t *testing.T) {
	t.Parallel()

	migration := sqltest.New(t, sqltest.Options{
		Force: *force,
		Files: os.DirFS("../../migrations"),
	})
	pool := migration.Setup(context.Background(), "")
	db := NewDB(pool, slog.Default())

	canceledCtx, immediateCancel := context.WithCancel(context.Background())
	immediateCancel()

	if _, err := db.TransactionContext(canceledCtx); err != context.Canceled {
		t.Errorf("unexpected error value: %v", err)
	}
}

func TestCommitNoTransaction(t *testing.T) {
	t.Parallel()

	db := &DB{}
	if err := db.Commit(context.Background()); err.Error() != "context has no transaction" {
		t.Errorf("unexpected error value: %v", err)
	}
}

func TestRollbackNoTransaction(t *testing.T) {
	t.Parallel()

	db := &DB{}
	if err := db.Rollback(context.Background()); err.Error() != "context has no transaction" {
		t.Errorf("unexpected error value: %v", err)
	}
}

func TestWithAcquire(t *testing.T) {
	t.Parallel()
	migration := sqltest.New(t, sqltest.Options{
		Force: *force,
		Files: os.DirFS("../../migrations"),
	})
	pool := migration.Setup(context.Background(), "")
	db := NewDB(pool, slog.Default())

	// Reuse the same connection for executing SQL commands.
	dbCtx, err := db.WithAcquire(context.Background())
	if err != nil {
		t.Fatalf("unexpected DB.WithAcquire() error = %v", err)
	}
	defer db.Release(dbCtx)

	// Check if we can acquire a connection only for a given context.
	defer func() {
		want := "context already has a connection acquired"
		if r := recover(); r != want {
			t.Errorf("expected panic %v, got %v instead", want, r)
		}
	}()
	db.WithAcquire(dbCtx)
}

func TestWithAcquireClosedPool(t *testing.T) {
	t.Parallel()
	migration := sqltest.New(t, sqltest.Options{
		Force: *force,

		// Opt out of automatic tearing down migration as we want to close the connection pool before t.Cleanup() is called.
		SkipTeardown: true,

		Files: os.DirFS("../../migrations"),
	})
	pool := migration.Setup(context.Background(), "")
	db := NewDB(pool, slog.Default())
	migration.Teardown(context.Background())
	if _, err := db.WithAcquire(context.Background()); err == nil {
		t.Errorf("expected error acquiring pgx connection for context, got nil")
	}
}

func TestCreateProduct(t *testing.T) {
	t.Parallel()
	migration := sqltest.New(t, sqltest.Options{
		Force: *force,
		Files: os.DirFS("../../migrations"),
	})
	pool := migration.Setup(context.Background(), "")
	db := NewDB(pool, slog.Default())

	type args struct {
		ctx    context.Context
		params inventory.CreateProductParams
	}
	tests := []struct {
		name    string
		args    args
		want    *inventory.Product
		wantErr string
	}{
		{
			name: "hello",
			args: args{
				ctx: context.Background(),
				params: inventory.CreateProductParams{
					ID:          "Hello",
					Name:        "A name",
					Description: "A description",
					Price:       14,
				},
			},
			want: &inventory.Product{
				ID:          "Hello",
				Name:        "A name",
				Description: "A description",
				Price:       14,
				CreatedAt:   time.Now(),
				ModifiedAt:  time.Now(),
			},
		},
		{
			name: "world",
			args: args{
				ctx: context.Background(),
				params: inventory.CreateProductParams{
					ID:          "World",
					Name:        "Earth",
					Description: "Universe",
					Price:       10,
				},
			},
			want: &inventory.Product{
				ID:          "World",
				Name:        "Earth",
				Description: "Universe",
				Price:       10,
				CreatedAt:   time.Now(),
				ModifiedAt:  time.Now(),
			},
		},
		{
			name: "world_already_exists",
			args: args{
				ctx: context.Background(),
				params: inventory.CreateProductParams{
					ID:          "World",
					Name:        "Earth",
					Description: "Universe",
					Price:       10,
				},
			},
			wantErr: "product already exists",
		},
		{
			name: "create_product_check_empty_id",
			args: args{
				ctx: context.Background(),
				params: inventory.CreateProductParams{
					ID:          "",
					Name:        "name",
					Description: "description",
					Price:       100,
				},
			},
			wantErr: "invalid product ID",
		},
		{
			name: "create_product_check_empty_name",
			args: args{
				ctx: context.Background(),
				params: inventory.CreateProductParams{
					ID:          "id",
					Name:        "",
					Description: "description",
					Price:       100,
				},
			},
			wantErr: "invalid product name",
		},
		{
			name: "create_product_check_negative_price",
			args: args{
				ctx: context.Background(),
				params: inventory.CreateProductParams{
					ID:          "id",
					Name:        "name",
					Description: "desc",
					Price:       -1,
				},
			},
			wantErr: "invalid price",
		},
		{
			name: "canceled_ctx",
			args: args{
				ctx: canceledContext(),
			},
			wantErr: "context canceled",
		},
		{
			name: "deadline_exceeded_ctx",
			args: args{
				ctx: deadlineExceededContext(),
			},
			wantErr: "context deadline exceeded",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.CreateProduct(tt.args.ctx, tt.args.params)
			if err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("DB.CreateProduct() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			// Reusing GetProduct to check if the product was created successfully.
			got, err := db.GetProduct(tt.args.ctx, tt.args.params.ID)
			if err != nil {
				t.Errorf("DB.GetProduct() error = %v", err)
			}
			// Ignore or CreatedAt and ModifiedAt before comparing structs.
			if !cmp.Equal(tt.want, got, cmpopts.IgnoreFields(inventory.Product{}, "CreatedAt", "ModifiedAt")) {
				t.Errorf("value returned by DB.GetProduct() doesn't match: %v", cmp.Diff(tt.want, got))
			}
		})
	}
}

func TestUpdateProduct(t *testing.T) {
	t.Parallel()
	migration := sqltest.New(t, sqltest.Options{
		Force: *force,
		Files: os.DirFS("../../migrations"),
	})
	pool := migration.Setup(context.Background(), "")
	db := NewDB(pool, slog.Default())

	// Add some products that will be modified next:
	createProducts(t, db, []inventory.CreateProductParams{
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
		{
			ID:          "do_not_change",
			Name:        "Do not change",
			Description: "This should remain unchanged",
			Price:       123,
		},
	})

	type args struct {
		ctx    context.Context
		params inventory.UpdateProductParams
	}
	tests := []struct {
		name    string
		args    args
		want    *inventory.Product
		wantErr string
	}{
		{
			name: "product_name_change",
			args: args{
				ctx: context.Background(),
				params: inventory.UpdateProductParams{
					ID:   "product",
					Name: newString("A new name"),
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
				ctx: context.Background(),
				params: inventory.UpdateProductParams{
					ID:          "product",
					Description: newString("A new description"),
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
				ctx: context.Background(),
				params: inventory.UpdateProductParams{
					ID:          "product",
					Name:        newString("Even another name"),
					Description: newString("yet another description"),
					Price:       newInt(400),
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
				ctx: context.Background(),
				params: inventory.UpdateProductParams{
					ID:   "World",
					Name: newString("Earth"),
				},
			},
			wantErr: "product not found",
		},
		{
			name: "update_product_check_empty_name",
			args: args{
				ctx: context.Background(),
				params: inventory.UpdateProductParams{
					ID:   "product",
					Name: newString(""),
				},
			},
			wantErr: "invalid product name",
		},
		{
			name: "canceled_ctx",
			args: args{
				ctx: canceledContext(),
			},
			wantErr: "context canceled",
		},
		{
			name: "deadline_exceeded_ctx",
			args: args{
				ctx: deadlineExceededContext(),
			},
			wantErr: "context deadline exceeded",
		},
		{
			name: "another_product_price_change",
			args: args{
				ctx: context.Background(),
				params: inventory.UpdateProductParams{
					ID:    "another",
					Price: newInt(97),
				},
			},
			want: &inventory.Product{
				ID:          "another",
				Name:        "Is your SQL UPDATE call correct?",
				Description: "Only the price of this one should be modified",
				Price:       97,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.UpdateProduct(tt.args.ctx, tt.args.params)
			if err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("DB.UpdateProduct() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			got, err := db.GetProduct(tt.args.ctx, tt.args.params.ID)
			if err != nil {
				t.Errorf("DB.GetProduct() error = %v", err)
			}
			if got.CreatedAt.IsZero() {
				t.Error("DB.GetProduct() returned CreatedAt should not be zero")
			}
			if !got.CreatedAt.Before(got.ModifiedAt) {
				t.Error("DB.GetProduct() should return CreatedAt < ModifiedAt")
			}
			// Ignore or CreatedAt and ModifiedAt before comparing structs.
			if !cmp.Equal(tt.want, got, cmpopts.IgnoreFields(inventory.Product{}, "CreatedAt", "ModifiedAt")) {
				t.Errorf("value returned by DB.GetProduct() doesn't match: %v", cmp.Diff(tt.want, got))
			}
		})
	}

	// Verify UPDATE has a WHERE clause avoiding changing unrelated rows:
	got, err := db.GetProduct(context.Background(), "do_not_change")
	if err != nil {
		t.Errorf("DB.GetProduct() error = %v", err)
	}
	if got.CreatedAt != got.ModifiedAt {
		t.Errorf("DB.GetProduct() should return CreatedAt == ModifiedAt for unmodified product")
	}
	want := &inventory.Product{
		ID:          "do_not_change",
		Name:        "Do not change",
		Description: "This should remain unchanged",
		Price:       123,
	}
	// Ignore or CreatedAt and ModifiedAt before comparing structs.
	if !cmp.Equal(want, got, cmpopts.IgnoreFields(inventory.Product{}, "CreatedAt", "ModifiedAt")) {
		t.Errorf("value returned by DB.GetProduct() doesn't match: %v", cmp.Diff(want, got))
	}
}

func TestGetProduct(t *testing.T) {
	t.Parallel()
	migration := sqltest.New(t, sqltest.Options{
		Force: *force,
		Files: os.DirFS("../../migrations"),
	})
	pool := migration.Setup(context.Background(), "")
	db := NewDB(pool, slog.Default())

	createProducts(t, db, []inventory.CreateProductParams{
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
		want    *inventory.Product
		wantErr string
	}{
		{
			name: "product",
			args: args{
				ctx: context.Background(),
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
				ctx: context.Background(),
				id:  "not_found",
			},
			want: nil,
		},
		{
			name: "canceled_ctx",
			args: args{
				ctx: canceledContext(),
			},
			wantErr: "context canceled",
		},
		{
			name: "deadline_exceeded_ctx",
			args: args{
				ctx: deadlineExceededContext(),
			},
			wantErr: "context deadline exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.GetProduct(tt.args.ctx, tt.args.id)
			if err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("DB.GetProduct() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if !cmp.Equal(tt.want, got, cmpopts.EquateApproxTime(time.Minute)) {
				t.Errorf("value returned by DB.GetProduct() doesn't match: %v", cmp.Diff(tt.want, got))
			}
		})
	}
}

func TestSearchProducts(t *testing.T) {
	t.Parallel()
	migration := sqltest.New(t, sqltest.Options{
		Force: *force,
		Files: os.DirFS("../../migrations"),
	})
	pool := migration.Setup(context.Background(), "")
	db := NewDB(pool, slog.Default())

	// On this test, reuse the same connection for executing SQL commands
	// to check acquiring and releasing a connection passed via context is working as expected.
	dbCtx, err := db.WithAcquire(context.Background())
	if err != nil {
		t.Fatalf("unexpected DB.WithAcquire() error = %v", err)
	}
	defer db.Release(dbCtx)

	createProducts(t, db, []inventory.CreateProductParams{
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
		want    *inventory.SearchProductsResponse
		wantErr string
	}{
		{
			name: "product",
			args: args{
				ctx: dbCtx,
				params: inventory.SearchProductsParams{
					QueryString: "plain desk",
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
			name: "product_very_expensive",
			args: args{
				ctx: dbCtx,
				params: inventory.SearchProductsParams{
					QueryString: "plain desk",
					MinPrice:    900,
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
				ctx: dbCtx,
				params: inventory.SearchProductsParams{
					QueryString: "home",
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
				ctx: dbCtx,
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
				ctx: dbCtx,
				params: inventory.SearchProductsParams{
					QueryString: "home",
					MaxPrice:    130,
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
				ctx: context.Background(),
				params: inventory.SearchProductsParams{
					QueryString: "xyz",
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
				ctx: canceledContext(),
			},
			wantErr: "context canceled",
		},
		{
			name: "deadline_exceeded_ctx",
			args: args{
				ctx: deadlineExceededContext(),
			},
			wantErr: "context deadline exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.SearchProducts(tt.args.ctx, tt.args.params)
			if err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("DB.SearchProducts() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if !cmp.Equal(tt.want, got, cmpopts.EquateApproxTime(time.Minute)) {
				t.Errorf("value returned by DB.SearchProducts() doesn't match: %v", cmp.Diff(tt.want, got))
			}
		})
	}
}

func TestDeleteProduct(t *testing.T) {
	t.Parallel()
	migration := sqltest.New(t, sqltest.Options{
		Force: *force,
		Files: os.DirFS("../../migrations"),
	})
	pool := migration.Setup(context.Background(), "")
	db := NewDB(pool, slog.Default())

	createProducts(t, db, []inventory.CreateProductParams{
		{
			ID:          "product",
			Name:        "Product name",
			Description: "Product description",
			Price:       123,
		},
		{
			ID:    "do_not_erase",
			Name:  "Do not erase",
			Price: 123,
		},
	})

	type args struct {
		ctx context.Context
		id  string
	}
	tests := []struct {
		name    string
		args    args
		wantErr string
	}{
		{
			name: "product",
			args: args{
				ctx: context.Background(),
				id:  "product",
			},
			wantErr: "",
		},
		// calling delete multiple times should not fail
		{
			name: "product_already_deleted",
			args: args{
				ctx: context.Background(),
				id:  "product",
			},
			wantErr: "",
		},
		// delete should be idempotent
		{
			name: "not_found",
			args: args{
				ctx: context.Background(),
				id:  "xyz",
			},
		},
		{
			name: "canceled_ctx",
			args: args{
				ctx: canceledContext(),
			},
			wantErr: "context canceled",
		},
		{
			name: "deadline_exceeded_ctx",
			args: args{
				ctx: deadlineExceededContext(),
			},
			wantErr: "context deadline exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.DeleteProduct(tt.args.ctx, tt.args.id)
			if err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("DB.DeleteProduct() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			got, err := db.GetProduct(context.Background(), tt.args.id)
			if err != nil {
				t.Errorf("DB.GetProduct() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != nil {
				t.Errorf("DB.GetProduct() returned %v, but should return nil", got)
			}
		})
	}
	// Check if a limited number of rows were deleted by verifying one product ("do_not_erase") exists on the database.
	var total int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) as total FROM "product"`).Scan(&total); err != nil {
		t.Fatalf(`failed to query "product" table: %v`, err)
	}
	if total != 1 {
		t.Errorf("product table should have 1 row, but got %d", total)
	}
}

func TestCreateProductReview(t *testing.T) {
	t.Parallel()
	migration := sqltest.New(t, sqltest.Options{
		Force: *force,
		Files: os.DirFS("../../migrations"),
	})
	pool := migration.Setup(context.Background(), "")
	db := NewDB(pool, slog.Default())

	createProducts(t, db, []inventory.CreateProductParams{
		{
			ID:          "product",
			Name:        "Original name",
			Description: "This is the original description",
			Price:       250,
		},
	})

	type args struct {
		ctx    context.Context
		params inventory.CreateProductReviewDBParams
	}
	tests := []struct {
		name    string
		args    args
		want    *inventory.ProductReview
		wantErr string
	}{
		{
			name: "success",
			args: args{
				ctx: context.Background(),
				params: inventory.CreateProductReviewDBParams{
					ID: "review1",
					CreateProductReviewParams: inventory.CreateProductReviewParams{
						ProductID:   "product",
						ReviewerID:  "reviewer",
						Score:       5,
						Title:       "title",
						Description: "review",
					},
				},
			},
			want: &inventory.ProductReview{
				ID:          "review1",
				ProductID:   "product",
				ReviewerID:  "reviewer",
				Score:       5,
				Title:       "title",
				Description: "review",
				CreatedAt:   time.Now(),
				ModifiedAt:  time.Now(),
			},
		},
		{
			name: "invalid_id",
			args: args{
				ctx: context.Background(),
				params: inventory.CreateProductReviewDBParams{
					ID: "",
					CreateProductReviewParams: inventory.CreateProductReviewParams{
						ProductID:   "product",
						ReviewerID:  "reviewer",
						Score:       5,
						Title:       "title",
						Description: "review",
					},
				},
			},
			wantErr: "invalid product review ID",
		},
		{
			name: "invalid_title",
			args: args{
				ctx: context.Background(),
				params: inventory.CreateProductReviewDBParams{
					ID: "xyz",
					CreateProductReviewParams: inventory.CreateProductReviewParams{
						ProductID:   "product",
						ReviewerID:  "reviewer",
						Score:       5,
						Title:       "",
						Description: "review",
					},
				},
			},
			wantErr: "invalid title",
		},
		{
			name: "invalid_score",
			args: args{
				ctx: context.Background(),
				params: inventory.CreateProductReviewDBParams{
					ID: "xyz",
					CreateProductReviewParams: inventory.CreateProductReviewParams{
						ProductID:   "product",
						ReviewerID:  "reviewer",
						Score:       15,
						Title:       "abc",
						Description: "review",
					},
				},
			},
			wantErr: "invalid score",
		},
		{
			name: "review1_already_exists",
			args: args{
				ctx: context.Background(),
				params: inventory.CreateProductReviewDBParams{
					ID: "review1",
					CreateProductReviewParams: inventory.CreateProductReviewParams{
						ProductID:   "product",
						ReviewerID:  "reviewer",
						Score:       5,
						Title:       "title",
						Description: "review",
					},
				},
			},
			wantErr: "product review already exists",
		},
		{
			name: "product_id_not_found",
			args: args{
				ctx: context.Background(),
				params: inventory.CreateProductReviewDBParams{
					ID: "review_has_no_product_on_database",
					CreateProductReviewParams: inventory.CreateProductReviewParams{
						ProductID:   "product_not_found",
						ReviewerID:  "reviewer123",
						Score:       3,
						Title:       "review title",
						Description: "review description",
					},
				},
			},
			wantErr: "cannot find product to create review",
		},
		{
			name: "canceled_ctx",
			args: args{
				ctx: canceledContext(),
			},
			wantErr: "context canceled",
		},
		{
			name: "deadline_exceeded_ctx",
			args: args{
				ctx: deadlineExceededContext(),
			},
			wantErr: "context deadline exceeded",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.CreateProductReview(tt.args.ctx, tt.args.params)
			if err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("DB.CreateProductReview() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			// Reusing GetProduct to check if the product was created successfully.
			got, err := db.GetProductReview(tt.args.ctx, tt.args.params.ID)
			if err != nil {
				t.Errorf("DB.GetProduct() error = %v", err)
			}
			// Ignore or CreatedAt and ModifiedAt before comparing structs.
			if !cmp.Equal(tt.want, got, cmpopts.IgnoreFields(inventory.ProductReview{}, "CreatedAt", "ModifiedAt")) {
				t.Errorf("value returned by DB.GetProduct() doesn't match: %v", cmp.Diff(tt.want, got))
			}
		})
	}
}

func TestUpdateProductReview(t *testing.T) {
	t.Parallel()
	migration := sqltest.New(t, sqltest.Options{
		Force: *force,
		Files: os.DirFS("../../migrations"),
	})
	pool := migration.Setup(context.Background(), "")
	db := NewDB(pool, slog.Default())

	// Add some products that will be modified next:
	createProducts(t, db, []inventory.CreateProductParams{
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
	createProductReviews(t, db, []inventory.CreateProductReviewDBParams{
		{
			ID: "review_update_score",
			CreateProductReviewParams: inventory.CreateProductReviewParams{
				ProductID:   "product",
				ReviewerID:  "reviewerid1",
				Score:       4,
				Title:       "my review is good",
				Description: "my description is not",
			},
		},
		{
			ID: "review_update_title",
			CreateProductReviewParams: inventory.CreateProductReviewParams{
				ProductID:   "another",
				ReviewerID:  "reviewerid1",
				Score:       5,
				Title:       "my review is not so good",
				Description: "my description neither",
			},
		},
		{
			ID: "review_update_desc",
			CreateProductReviewParams: inventory.CreateProductReviewParams{
				ProductID:   "another",
				ReviewerID:  "reviewerid2",
				Score:       2,
				Title:       "I love this product",
				Description: "It doesn't only meet all my expectations! It goes way beyond them!!!",
			},
		},
		{
			ID: "review_update_multiple",
			CreateProductReviewParams: inventory.CreateProductReviewParams{
				ProductID:   "another",
				ReviewerID:  "reviewerid2",
				Score:       2,
				Title:       "I love this product",
				Description: "It doesn't only meet all my expectations! It goes way beyond them!!!",
			},
		},
		{
			ID: "do_not_change",
			CreateProductReviewParams: inventory.CreateProductReviewParams{
				ProductID:   "another",
				ReviewerID:  "one",
				Score:       1,
				Title:       "same",
				Description: "do not change",
			},
		},
	})

	type args struct {
		ctx    context.Context
		params inventory.UpdateProductReviewParams
	}
	tests := []struct {
		name    string
		args    args
		want    *inventory.ProductReview
		wantErr string
	}{
		{
			name: "update_score",
			args: args{
				ctx: context.Background(),
				params: inventory.UpdateProductReviewParams{
					ID:    "review_update_score",
					Score: newInt(5),
				},
			},
			want: &inventory.ProductReview{
				ID:          "review_update_score",
				ProductID:   "product",
				ReviewerID:  "reviewerid1",
				Score:       5,
				Title:       "my review is good",
				Description: "my description is not",
			},
		},
		{
			name: "update_invalid_score",
			args: args{
				ctx: context.Background(),
				params: inventory.UpdateProductReviewParams{
					ID:    "review_update_score",
					Score: newInt(542),
				},
			},
			wantErr: "invalid score",
		},
		{
			name: "update_title",
			args: args{
				ctx: context.Background(),
				params: inventory.UpdateProductReviewParams{
					ID:    "review_update_title",
					Title: newString("my review is really good"),
				},
			},
			want: &inventory.ProductReview{
				ID:          "review_update_title",
				ProductID:   "another",
				ReviewerID:  "reviewerid1",
				Score:       5,
				Title:       "my review is really good",
				Description: "my description neither",
			},
		},
		{
			name: "update_invalid_title",
			args: args{
				ctx: context.Background(),
				params: inventory.UpdateProductReviewParams{
					ID:    "review_update_score",
					Title: newString(""),
				},
			},
			wantErr: "invalid title",
		},
		{
			name: "update_desc",
			args: args{
				ctx: context.Background(),
				params: inventory.UpdateProductReviewParams{
					ID:          "review_update_desc",
					Description: newString("this is a string, right?"),
				},
			},
			want: &inventory.ProductReview{
				ID:          "review_update_desc",
				ProductID:   "another",
				ReviewerID:  "reviewerid2",
				Score:       2,
				Title:       "I love this product",
				Description: "this is a string, right?",
			},
		},
		{
			name: "update_multiple",
			args: args{
				ctx: context.Background(),
				params: inventory.UpdateProductReviewParams{
					ID:          "review_update_multiple",
					Score:       newInt(5),
					Title:       newString("hello, world"),
					Description: newString("abc"),
				},
			},
			want: &inventory.ProductReview{
				ID:          "review_update_multiple",
				ProductID:   "another",
				ReviewerID:  "reviewerid2",
				Score:       5,
				Title:       "hello, world",
				Description: "abc",
			},
		},
		{
			name: "not_found",
			args: args{
				ctx: context.Background(),
				params: inventory.UpdateProductReviewParams{
					ID:    "World",
					Title: newString("Earth"),
				},
			},
			wantErr: "product review not found",
		},
		{
			name: "canceled_ctx",
			args: args{
				ctx: canceledContext(),
			},
			wantErr: "context canceled",
		},
		{
			name: "deadline_exceeded_ctx",
			args: args{
				ctx: deadlineExceededContext(),
			},
			wantErr: "context deadline exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.UpdateProductReview(tt.args.ctx, tt.args.params)
			if err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("DB.UpdateProductReview() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			got, err := db.GetProductReview(tt.args.ctx, tt.args.params.ID)
			if err != nil {
				t.Errorf("DB.GetProductReview() error = %v", err)
			}
			if got.CreatedAt.IsZero() {
				t.Error("DB.GetProductReview() returned CreatedAt should not be zero")
			}
			if !got.CreatedAt.Before(got.ModifiedAt) {
				t.Error("DB.GetProductReview() should return CreatedAt < ModifiedAt")
			}
			// Ignore or CreatedAt and ModifiedAt before comparing structs.
			if !cmp.Equal(tt.want, got, cmpopts.IgnoreFields(inventory.ProductReview{}, "CreatedAt", "ModifiedAt")) {
				t.Errorf("value returned by DB.GetProductReview() doesn't match: %v", cmp.Diff(tt.want, got))
			}
		})
	}

	// Verify UPDATE has a WHERE clause avoiding changing unrelated rows:
	got, err := db.GetProductReview(context.Background(), "do_not_change")
	if err != nil {
		t.Errorf("DB.GetProductReview() error = %v", err)
	}
	if got.CreatedAt != got.ModifiedAt {
		t.Errorf("DB.GetProductReview() should return CreatedAt == ModifiedAt for unmodified product")
	}
	want := &inventory.ProductReview{
		ID:          "do_not_change",
		ProductID:   "another",
		ReviewerID:  "one",
		Score:       1,
		Title:       "same",
		Description: "do not change",
	}
	// Ignore or CreatedAt and ModifiedAt before comparing structs.
	if !cmp.Equal(want, got, cmpopts.IgnoreFields(inventory.ProductReview{}, "CreatedAt", "ModifiedAt")) {
		t.Errorf("value returned by DB.GetProductReview() doesn't match: %v", cmp.Diff(want, got))
	}
}

func TestGetProductReview(t *testing.T) {
	t.Parallel()
	migration := sqltest.New(t, sqltest.Options{
		Force: *force,
		Files: os.DirFS("../../migrations"),
	})
	pool := migration.Setup(context.Background(), "")
	db := NewDB(pool, slog.Default())

	createProducts(t, db, []inventory.CreateProductParams{
		{
			ID:          "product",
			Name:        "A product name",
			Description: "A great description",
			Price:       10000,
		},
	})
	createProductReviews(t, db, []inventory.CreateProductReviewDBParams{
		{
			ID: "review1",
			CreateProductReviewParams: inventory.CreateProductReviewParams{
				ProductID:   "product",
				ReviewerID:  "reviewerid1",
				Score:       4,
				Title:       "my review is good",
				Description: "my description is not",
			},
		},
		{
			ID: "review2",
			CreateProductReviewParams: inventory.CreateProductReviewParams{
				ProductID:   "product",
				ReviewerID:  "reviewerid1",
				Score:       5,
				Title:       "my review is not so good",
				Description: "my description neither",
			},
		},
	})

	type args struct {
		ctx context.Context
		id  string
	}
	tests := []struct {
		name    string
		args    args
		want    *inventory.ProductReview
		wantErr string
	}{
		{
			name: "success",
			args: args{
				ctx: context.Background(),
				id:  "review1",
			},
			want: &inventory.ProductReview{
				ID:          "review1",
				ProductID:   "product",
				ReviewerID:  "reviewerid1",
				Score:       4,
				Title:       "my review is good",
				Description: "my description is not",
				CreatedAt:   time.Now(),
				ModifiedAt:  time.Now(),
			},
		},
		{
			name: "success_2",
			args: args{
				ctx: context.Background(),
				id:  "review2",
			},
			want: &inventory.ProductReview{
				ID:          "review2",
				ProductID:   "product",
				ReviewerID:  "reviewerid1",
				Score:       5,
				Title:       "my review is not so good",
				Description: "my description neither",
				CreatedAt:   time.Now(),
				ModifiedAt:  time.Now(),
			},
		},
		{
			name: "not_found",
			args: args{
				ctx: context.Background(),
				id:  "not_found",
			},
			want: nil,
		},
		{
			name: "canceled_ctx",
			args: args{
				ctx: canceledContext(),
			},
			wantErr: "context canceled",
		},
		{
			name: "deadline_exceeded_ctx",
			args: args{
				ctx: deadlineExceededContext(),
			},
			wantErr: "context deadline exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.GetProductReview(tt.args.ctx, tt.args.id)
			if err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("DB.GetProductReview() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if !cmp.Equal(tt.want, got, cmpopts.EquateApproxTime(time.Minute)) {
				t.Errorf("value returned by DB.GetProductReview() doesn't match: %v", cmp.Diff(tt.want, got))
			}
		})
	}
}

func TestGetProductReviews(t *testing.T) {
	t.Parallel()
	migration := sqltest.New(t, sqltest.Options{
		Force: *force,
		Files: os.DirFS("../../migrations"),
	})
	pool := migration.Setup(context.Background(), "")
	db := NewDB(pool, slog.Default())

	createProducts(t, db, []inventory.CreateProductParams{
		{
			ID:          "goodproduct",
			Name:        "A product name",
			Description: "A great description",
			Price:       10000,
		},
	})
	createProducts(t, db, []inventory.CreateProductParams{
		{
			ID:          "greatproduct",
			Name:        "Another one",
			Description: "Another description",
			Price:       123,
		},
	})
	createProductReviews(t, db, []inventory.CreateProductReviewDBParams{
		{
			ID: "review1",
			CreateProductReviewParams: inventory.CreateProductReviewParams{
				ProductID:   "goodproduct",
				ReviewerID:  "reviewerid1",
				Score:       4,
				Title:       "my review is good",
				Description: "my description is not",
			},
		},
		{
			ID: "review2",
			CreateProductReviewParams: inventory.CreateProductReviewParams{
				ProductID:   "greatproduct",
				ReviewerID:  "reviewerid2",
				Score:       5,
				Title:       "little title",
				Description: "a desc",
			},
		},
		{
			ID: "review3",
			CreateProductReviewParams: inventory.CreateProductReviewParams{
				ProductID:   "greatproduct",
				ReviewerID:  "reviewerid2",
				Score:       3,
				Title:       "hello",
				Description: "ijk",
			},
		},
	})

	type args struct {
		ctx    context.Context
		params inventory.ProductReviewsParams
	}
	tests := []struct {
		name    string
		args    args
		want    *inventory.ProductReviewsResponse
		wantErr string
	}{
		{
			name: "success",
			args: args{
				ctx: context.Background(),
				params: inventory.ProductReviewsParams{
					ProductID: "greatproduct",
				},
			},
			want: &inventory.ProductReviewsResponse{
				Reviews: []*inventory.ProductReview{
					{
						ID:          "review3",
						ProductID:   "greatproduct",
						ReviewerID:  "reviewerid2",
						Score:       3,
						Title:       "hello",
						Description: "ijk",
					},
					{
						ID:          "review2",
						ProductID:   "greatproduct",
						ReviewerID:  "reviewerid2",
						Score:       5,
						Title:       "little title",
						Description: "a desc",
					},
				},
				Total: 2,
			},
		},
		{
			name: "limit",
			args: args{
				ctx: context.Background(),
				params: inventory.ProductReviewsParams{
					ProductID: "greatproduct",
					Pagination: inventory.Pagination{
						Limit:  1,
						Offset: 1,
					},
				},
			},
			want: &inventory.ProductReviewsResponse{
				Reviews: []*inventory.ProductReview{
					{
						ID:          "review2",
						ProductID:   "greatproduct",
						ReviewerID:  "reviewerid2",
						Score:       5,
						Title:       "little title",
						Description: "a desc",
					},
				},
				Total: 2,
			},
		},
		{
			name: "limit",
			args: args{
				ctx: context.Background(),
				params: inventory.ProductReviewsParams{
					ProductID: "greatproduct",
					Pagination: inventory.Pagination{
						Limit: 1,
					},
				},
			},
			want: &inventory.ProductReviewsResponse{
				Reviews: []*inventory.ProductReview{
					{
						ID:          "review3",
						ProductID:   "greatproduct",
						ReviewerID:  "reviewerid2",
						Score:       3,
						Title:       "hello",
						Description: "ijk",
					},
				},
				Total: 2,
			},
		},
		{
			name: "other",
			args: args{
				ctx: context.Background(),
				params: inventory.ProductReviewsParams{
					ProductID: "goodproduct",
				},
			},
			want: &inventory.ProductReviewsResponse{
				Reviews: []*inventory.ProductReview{
					{
						ID:          "review1",
						ProductID:   "goodproduct",
						ReviewerID:  "reviewerid1",
						Score:       4,
						Title:       "my review is good",
						Description: "my description is not",
					},
				},
				Total: 1,
			},
		},
		{
			name: "canceled_ctx",
			args: args{
				ctx: canceledContext(),
			},
			wantErr: "context canceled",
		},
		{
			name: "deadline_exceeded_ctx",
			args: args{
				ctx: deadlineExceededContext(),
			},
			wantErr: "context deadline exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.GetProductReviews(tt.args.ctx, tt.args.params)
			if err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("DB.GetProductReviews() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			// TODO(henvic): find out how to ignore specifically CreatedAt and ModifiedAt in this case with google/go-cmp.
			if !cmp.Equal(tt.want, got, cmpopts.IgnoreTypes(time.Time{})) {
				t.Errorf("value returned by DB.GetProductReviews() doesn't match: %v", cmp.Diff(tt.want, got))
			}
		})
	}
}

func TestDeleteProductReview(t *testing.T) {
	t.Parallel()
	migration := sqltest.New(t, sqltest.Options{
		Force: *force,
		Files: os.DirFS("../../migrations"),
	})
	pool := migration.Setup(context.Background(), "")
	db := NewDB(pool, slog.Default())

	createProducts(t, db, []inventory.CreateProductParams{
		{
			ID:          "product",
			Name:        "Product name",
			Description: "Product description",
			Price:       123,
		},
	})
	createProductReviews(t, db, []inventory.CreateProductReviewDBParams{
		{
			ID: "a",
			CreateProductReviewParams: inventory.CreateProductReviewParams{
				ProductID:   "product",
				ReviewerID:  "r1",
				Score:       4,
				Title:       "I love it",
				Description: "I don't know what else to use",
			},
		},
		{
			ID: "b",
			CreateProductReviewParams: inventory.CreateProductReviewParams{
				ProductID:   "product",
				ReviewerID:  "r2",
				Score:       3,
				Title:       "this is a title",
				Description: "this is a description",
			},
		},
		{
			ID: "do_not_erase",
			CreateProductReviewParams: inventory.CreateProductReviewParams{
				ProductID:   "product",
				ReviewerID:  "r2",
				Score:       5,
				Title:       "once again",
				Description: "pass or fail",
			},
		},
	})

	type args struct {
		ctx context.Context
		id  string
	}
	tests := []struct {
		name    string
		args    args
		wantErr string
	}{
		{
			name: "success",
			args: args{
				ctx: context.Background(),
				id:  "a",
			},
			wantErr: "",
		},
		// calling delete multiple times should not fail
		{
			name: "a_again",
			args: args{
				ctx: context.Background(),
				id:  "a",
			},
			wantErr: "",
		},
		// calling delete multiple times should not fail
		{
			name: "b",
			args: args{
				ctx: context.Background(),
				id:  "b",
			},
			wantErr: "",
		},
		// delete should be idempotent
		{
			name: "not_found",
			args: args{
				ctx: context.Background(),
				id:  "xyz",
			},
		},
		{
			name: "canceled_ctx",
			args: args{
				ctx: canceledContext(),
			},
			wantErr: "context canceled",
		},
		{
			name: "deadline_exceeded_ctx",
			args: args{
				ctx: deadlineExceededContext(),
			},
			wantErr: "context deadline exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.DeleteProductReview(tt.args.ctx, tt.args.id)
			if err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("DB.DeleteProductReview() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			got, err := db.GetProductReview(context.Background(), tt.args.id)
			if err != nil {
				t.Errorf("DB.GetProductReview() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != nil {
				t.Errorf("DB.GetProductReview() returned %v, but should return nil", got)
			}
		})
	}
	// Check if a limited number of rows were deleted by verifying one review ("do_not_erase") exists on the database.
	var total int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) as total FROM "review"`).Scan(&total); err != nil {
		t.Fatalf(`failed to query "review" table: %v`, err)
	}
	if total != 1 {
		t.Errorf(`"review" table should have 1 row, but got %d`, total)
	}
}
