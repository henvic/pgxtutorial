package inventory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/henvic/pgxtutorial/internal/inventory"
	"go.uber.org/mock/gomock"
)

func TestServiceCreateProductReview(t *testing.T) {
	t.Parallel()
	var service = serviceWithPostgres(t)
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
		params inventory.CreateProductReviewParams
	}
	tests := []struct {
		name    string
		mock    func(t testing.TB) *inventory.MockDB // Leave as nil for using a real database implementation.
		args    args
		want    *inventory.ProductReview
		wantErr string
	}{
		{
			name: "missing_product_id",
			args: args{
				ctx: t.Context(),
				params: inventory.CreateProductReviewParams{
					ReviewerID:  "customer",
					Score:       5,
					Title:       "Anything",
					Description: "I don't really know what to say about this product, and am here just for the points.",
				},
			},
			wantErr: "missing product ID",
		},
		{
			name: "missing_reviewer_id",
			args: args{
				ctx: t.Context(),
				params: inventory.CreateProductReviewParams{
					ProductID:   "product",
					Score:       5,
					Title:       "Anything",
					Description: "I don't really know what to say about this product, and am here just for the points.",
				},
			},
			wantErr: "missing reviewer ID",
		},
		{
			name: "missing_title",
			args: args{
				ctx: t.Context(),
				params: inventory.CreateProductReviewParams{
					ProductID:   "product",
					ReviewerID:  "customer",
					Score:       5,
					Description: "I don't really know what to say about this product, and am here just for the points.",
				},
			},
			wantErr: "missing review title",
		},
		{
			name: "missing_description",
			args: args{
				ctx: t.Context(),
				params: inventory.CreateProductReviewParams{
					ProductID:  "product",
					ReviewerID: "customer",
					Score:      5,
					Title:      "Anything",
				},
			},
			wantErr: "missing review description",
		},
		{
			name: "invalid_score",
			args: args{
				ctx: t.Context(),
				params: inventory.CreateProductReviewParams{
					ProductID:   "product",
					ReviewerID:  "customer",
					Score:       51,
					Title:       "Anything",
					Description: "I don't really know what to say about this product, and am here just for the points.",
				},
			},
			wantErr: "invalid score",
		},
		{
			name: "success",
			args: args{
				ctx: t.Context(),
				params: inventory.CreateProductReviewParams{
					ProductID:   "product",
					ReviewerID:  "customer",
					Score:       5,
					Title:       "Anything",
					Description: "I don't really know what to say about this product, and am here just for the points.",
				},
			},
			want: &inventory.ProductReview{
				ProductID:   "product",
				ReviewerID:  "customer",
				Score:       5,
				Title:       "Anything",
				Description: "I don't really know what to say about this product, and am here just for the points.",
				CreatedAt:   time.Now(),
				ModifiedAt:  time.Now(),
			},
			wantErr: "",
		},
		{
			name: "canceled_ctx",
			args: args{
				ctx: canceledContext(t.Context()),
				params: inventory.CreateProductReviewParams{
					ProductID:   "product",
					ReviewerID:  "customer",
					Score:       5,
					Title:       "Anything",
					Description: "I don't really know what to say about this product, and am here just for the points.",
				},
			},
			wantErr: "context canceled",
		},
		{
			name: "database_error",
			mock: func(t testing.TB) *inventory.MockDB {
				ctrl := gomock.NewController(t)
				m := inventory.NewMockDB(ctrl)
				m.EXPECT().CreateProductReview(
					gomock.Not(gomock.Nil()),
					gomock.Any(),
				).Return(errors.New("unexpected error"))
				return m
			},
			args: args{
				ctx: t.Context(),
				params: inventory.CreateProductReviewParams{
					ProductID:   "product",
					ReviewerID:  "customer",
					Score:       5,
					Title:       "Anything",
					Description: "I don't really know what to say about this product, and am here just for the points.",
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
			id, err := s.CreateProductReview(tt.args.ctx, tt.args.params)
			if err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("Service.CreateProductReview() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if id == "" {
				t.Errorf("Service.CreateProductReview() returned empty ID")
			}
			// Only check integration / real implementation database for data.
			if tt.mock != nil {
				return
			}
			// Reusing GetProductReview to check if the product was created successfully.
			got, err := s.GetProductReview(tt.args.ctx, id)
			if err != nil {
				t.Errorf("Service.GetProductReview() error = %v", err)
			}
			if !cmp.Equal(tt.want, got, cmpopts.IgnoreFields(inventory.ProductReview{}, "ID"), cmpopts.EquateApproxTime(time.Minute)) {
				t.Errorf("value returned by Service.GetProductReview() doesn't match: %v", cmp.Diff(tt.want, got))
			}
		})
	}
}

func TestServiceUpdateProductReview(t *testing.T) {
	t.Parallel()
	var service = serviceWithPostgres(t)
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
	// Add some product reviews that will be modified next:
	firstReviewID := createProductReview(t, service, inventory.CreateProductReviewParams{
		ProductID:   "product",
		ReviewerID:  "you",
		Score:       4,
		Title:       "My title",
		Description: "My description",
	})
	secondReviewID := createProductReview(t, service, inventory.CreateProductReviewParams{
		ProductID:   "product",
		ReviewerID:  "me",
		Score:       1,
		Title:       "three little birds",
		Description: "don't worry, about a thing",
	})
	type args struct {
		ctx    context.Context
		params inventory.UpdateProductReviewParams
	}
	tests := []struct {
		name    string
		mock    func(t testing.TB) *inventory.MockDB // Leave as nil for using a real database implementation.
		args    args
		want    *inventory.ProductReview
		wantErr string
	}{
		{
			name: "empty",
			args: args{
				ctx:    t.Context(),
				params: inventory.UpdateProductReviewParams{},
			},
			wantErr: "missing review ID",
		},
		{
			name: "invalid_review_score",
			args: args{
				ctx: t.Context(),
				params: inventory.UpdateProductReviewParams{
					ID:    "invalid_review_score",
					Score: ptr(int32(-5)),
				},
			},
			wantErr: "invalid score",
		},
		{
			name: "no_product_review_title",
			args: args{
				ctx: t.Context(),
				params: inventory.UpdateProductReviewParams{
					ID:    "no_product_review_title",
					Title: ptr(""),
				},
			},
			wantErr: "missing review title",
		},
		{
			name: "no_product_review_description",
			args: args{
				ctx: t.Context(),
				params: inventory.UpdateProductReviewParams{
					ID:          "no_product_review_desc",
					Description: ptr(""),
				},
			},
			wantErr: "missing review description",
		},
		{
			name: "not_found",
			args: args{
				ctx: t.Context(),
				params: inventory.UpdateProductReviewParams{
					ID:    "World",
					Title: ptr("Earth"),
				},
			},
			wantErr: "product review not found",
		},
		{
			name: "canceled_ctx",
			args: args{
				ctx: canceledContext(t.Context()),
				params: inventory.UpdateProductReviewParams{
					ID:    "product_review",
					Title: ptr("Earth"),
				},
			},
			wantErr: "context canceled",
		},
		{
			name: "deadline_exceeded_ctx",
			args: args{
				ctx: deadlineExceededContext(t.Context()),
				params: inventory.UpdateProductReviewParams{
					ID:    "product_review",
					Title: ptr("Earth"),
				},
			},
			wantErr: "context deadline exceeded",
		},
		{
			name: "no_changes",
			args: args{
				ctx: t.Context(),
				params: inventory.UpdateProductReviewParams{
					ID: "no_changes",
				},
			},
			wantErr: "no product review arguments to update",
		},
		{
			name: "success",
			args: args{
				ctx: t.Context(),
				params: inventory.UpdateProductReviewParams{
					ID:          firstReviewID,
					Score:       ptr(int32(3)),
					Title:       ptr("updated title"),
					Description: ptr("updated desc"),
				},
			},
			want: &inventory.ProductReview{
				ProductID:   "product",
				ReviewerID:  "you",
				Score:       3,
				Title:       "updated title",
				Description: "updated desc",
			},
		},
		{
			name: "success_score",
			args: args{
				ctx: t.Context(),
				params: inventory.UpdateProductReviewParams{
					ID:    secondReviewID,
					Score: ptr(int32(5)),
				},
			},
			want: &inventory.ProductReview{
				ProductID:   "product",
				ReviewerID:  "me",
				Score:       5,
				Title:       "three little birds",
				Description: "don't worry, about a thing",
			},
		},
		{
			name: "database_error",
			mock: func(t testing.TB) *inventory.MockDB {
				ctrl := gomock.NewController(t)
				m := inventory.NewMockDB(ctrl)
				m.EXPECT().UpdateProductReview(gomock.Not(gomock.Nil()),
					inventory.UpdateProductReviewParams{
						ID:          "simple",
						Title:       ptr("product review title"),
						Description: ptr("product review description"),
					}).Return(errors.New("unexpected error"))
				return m
			},
			args: args{
				ctx: t.Context(),
				params: inventory.UpdateProductReviewParams{
					ID:          "simple",
					Title:       ptr("product review title"),
					Description: ptr("product review description"),
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
			err := s.UpdateProductReview(tt.args.ctx, tt.args.params)
			if err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("Service.UpdateProductReview() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			got, err := s.GetProductReview(tt.args.ctx, tt.args.params.ID)
			if err != nil {
				t.Errorf("Service.GetProductReview() error = %v", err)
			}
			if got.CreatedAt.IsZero() {
				t.Error("Service.GetProductReview() returned CreatedAt should not be zero")
			}
			if !got.CreatedAt.Before(got.ModifiedAt) {
				t.Error("Service.GetProductReview() should return CreatedAt < ModifiedAt")
			}
			// Copy CreatedAt and ModifiedAt before comparing structs.
			// See TestCreateProduct for a strategy using cmpopts.EquateApproxTime instead.
			tt.want.CreatedAt = got.CreatedAt
			tt.want.ModifiedAt = got.ModifiedAt
			if !cmp.Equal(tt.want, got, cmpopts.IgnoreFields(inventory.ProductReview{}, "ID")) {
				t.Errorf("value returned by Service.GetProductReview() doesn't match: %v", cmp.Diff(tt.want, got))
			}
		})
	}
}

func TestServiceDeleteProductReview(t *testing.T) {
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
	reviewID := createProductReview(t, service, inventory.CreateProductReviewParams{
		ProductID:   "product",
		ReviewerID:  "you",
		Score:       4,
		Title:       "My title",
		Description: "My description",
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
			name: "missing_product_review_id",
			args: args{
				ctx: t.Context(),
				id:  "",
			},
			wantErr: "missing review ID",
		},
		{
			name: "success",
			args: args{
				ctx: t.Context(),
				id:  reviewID,
			},
			wantErr: "",
		},
		// calling delete multiple times should not fail
		{
			name: "already_deleted",
			args: args{
				ctx: t.Context(),
				id:  reviewID,
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
				id:  "abc",
			},
			wantErr: "context canceled",
		},
		{
			name: "deadline_exceeded_ctx",
			args: args{
				ctx: deadlineExceededContext(t.Context()),
				id:  "def",
			},
			wantErr: "context deadline exceeded",
		},
		{
			name: "database_error",
			args: args{
				ctx: t.Context(),
				id:  "ghi",
			},
			mock: func(t testing.TB) *inventory.MockDB {
				ctrl := gomock.NewController(t)
				m := inventory.NewMockDB(ctrl)
				m.EXPECT().DeleteProductReview(gomock.Not(gomock.Nil()), "ghi").Return(errors.New("unexpected error"))
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
			if err := s.DeleteProductReview(tt.args.ctx, tt.args.id); err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("Service.DeleteProductReview() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestServiceGetProductReview(t *testing.T) {
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
	// Add some product reviews that will be modified next:
	firstReviewID := createProductReview(t, service, inventory.CreateProductReviewParams{
		ProductID:   "product",
		ReviewerID:  "you",
		Score:       4,
		Title:       "My title",
		Description: "My description",
	})
	secondReviewID := createProductReview(t, service, inventory.CreateProductReviewParams{
		ProductID:   "product",
		ReviewerID:  "me",
		Score:       1,
		Title:       "three little birds",
		Description: "don't worry, about a thing",
	})
	type args struct {
		ctx context.Context
		id  string
	}
	tests := []struct {
		name    string
		args    args
		mock    func(t testing.TB) *inventory.MockDB // Leave as nil for using a real database implementation.
		want    *inventory.ProductReview
		wantErr string
	}{
		{
			name: "missing_product_review_id",
			args: args{
				ctx: t.Context(),
				id:  "",
			},
			wantErr: "missing review ID",
		},
		{
			name: "success",
			args: args{
				ctx: t.Context(),
				id:  firstReviewID,
			},
			want: &inventory.ProductReview{
				ProductID:   "product",
				ReviewerID:  "you",
				Score:       4,
				Title:       "My title",
				Description: "My description",
				CreatedAt:   time.Now(),
				ModifiedAt:  time.Now(),
			},
		},
		{
			name: "other",
			args: args{
				ctx: t.Context(),
				id:  secondReviewID,
			},
			want: &inventory.ProductReview{
				ProductID:   "product",
				ReviewerID:  "me",
				Score:       1,
				Title:       "three little birds",
				Description: "don't worry, about a thing",
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
				id:  "review_id",
			},
			wantErr: "context canceled",
		},
		{
			name: "deadline_exceeded_ctx",
			args: args{
				ctx: deadlineExceededContext(t.Context()),
				id:  "review_id",
			},
			wantErr: "context deadline exceeded",
		},
		{
			name: "database_error",
			args: args{
				ctx: t.Context(),
				id:  "review_id",
			},
			mock: func(t testing.TB) *inventory.MockDB {
				ctrl := gomock.NewController(t)
				m := inventory.NewMockDB(ctrl)
				m.EXPECT().GetProductReview(gomock.Not(gomock.Nil()), "review_id").Return(nil, errors.New("unexpected error"))
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
			got, err := s.GetProductReview(tt.args.ctx, tt.args.id)
			if err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("Service.GetProductReview() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if !cmp.Equal(tt.want, got,
				cmpopts.IgnoreFields(inventory.ProductReview{}, "ID"),
				cmpopts.EquateApproxTime(time.Minute)) {
				t.Errorf("value returned by Service.GetProductReview() doesn't match: %v", cmp.Diff(tt.want, got))
			}
		})
	}
}

func TestServiceGetProductReviews(t *testing.T) {
	t.Parallel()
	var service = serviceWithPostgres(t)
	createProducts(t, service, []inventory.CreateProductParams{
		{
			ID:          "chair",
			Name:        "Office chair with headrest",
			Description: "The best chair for your neck.",
			Price:       200,
		},
	})
	createProducts(t, service, []inventory.CreateProductParams{
		{
			ID:          "desk",
			Name:        "study desk",
			Description: "A beautiful desk.",
			Price:       1400,
		},
	})
	hackerDeskReviewID := createProductReview(t, service, inventory.CreateProductReviewParams{
		ProductID:   "desk",
		ReviewerID:  "hacker",
		Score:       5,
		Title:       "Solid work of art",
		Description: "I built it with the best wood I could find.",
	})
	ironworkerReviewID := createProductReview(t, service, inventory.CreateProductReviewParams{
		ProductID:   "desk",
		ReviewerID:  "ironworker",
		Score:       4,
		Title:       "Very Good",
		Description: "Nice steady study desk, but I bet it'd last much longer with a steel base.",
	})
	candlemakerReviewID := createProductReview(t, service, inventory.CreateProductReviewParams{
		ProductID:   "desk",
		ReviewerID:  "candlemaker",
		Score:       4,
		Title:       "Perfect",
		Description: "Good affordable desk. You should spread wax to polish it.",
	})
	glazierReviewID := createProductReview(t, service, inventory.CreateProductReviewParams{
		ProductID:   "desk",
		ReviewerID:  "glazier",
		Score:       1,
		Title:       "Excellent, not.",
		Description: "I prefer desks made out of glass, as I'm jealous of the tailor and the baker.",
	})
	hackerChairReviewID := createProductReview(t, service, inventory.CreateProductReviewParams{
		ProductID:   "chair",
		ReviewerID:  "hacker",
		Score:       3,
		Title:       "Nice chair",
		Description: "Comfy chair. I recommend.",
	})

	type args struct {
		ctx    context.Context
		params inventory.ProductReviewsParams
	}
	tests := []struct {
		name    string
		args    args
		mock    func(t testing.TB) *inventory.MockDB // Leave as nil for using a real database implementation.
		want    *inventory.ProductReviewsResponse
		wantErr string
	}{
		{
			name: "success",
			args: args{
				ctx: t.Context(),
				params: inventory.ProductReviewsParams{
					ProductID: "desk",
					Pagination: inventory.Pagination{
						Limit: 3,
					},
				},
			},
			want: &inventory.ProductReviewsResponse{
				Reviews: []*inventory.ProductReview{
					{
						ID:          glazierReviewID,
						ProductID:   "desk",
						ReviewerID:  "glazier",
						Score:       1,
						Title:       "Excellent, not.",
						Description: "I prefer desks made out of glass, as I'm jealous of the tailor and the baker.",
						CreatedAt:   time.Now(),
						ModifiedAt:  time.Now(),
					},
					{
						ID:          candlemakerReviewID,
						ProductID:   "desk",
						ReviewerID:  "candlemaker",
						Score:       4,
						Title:       "Perfect",
						Description: "Good affordable desk. You should spread wax to polish it.",
						CreatedAt:   time.Now(),
						ModifiedAt:  time.Now(),
					},
					{
						ID:          ironworkerReviewID,
						ProductID:   "desk",
						ReviewerID:  "ironworker",
						Score:       4,
						Title:       "Very Good",
						Description: "Nice steady study desk, but I bet it'd last much longer with a steel base.",
						CreatedAt:   time.Now(),
						ModifiedAt:  time.Now(),
					},
				},
				Total: 4,
			},
		},
		{
			name: "reviewer",
			args: args{
				ctx: t.Context(),
				params: inventory.ProductReviewsParams{
					ReviewerID: "hacker",
					Pagination: inventory.Pagination{
						Limit: 3,
					},
				},
			},
			want: &inventory.ProductReviewsResponse{
				Reviews: []*inventory.ProductReview{
					{
						ID:          hackerChairReviewID,
						ProductID:   "chair",
						ReviewerID:  "hacker",
						Score:       3,
						Title:       "Nice chair",
						Description: "Comfy chair. I recommend.",
						CreatedAt:   time.Now(),
						ModifiedAt:  time.Now(),
					},
					{
						ID:          hackerDeskReviewID,
						ProductID:   "desk",
						ReviewerID:  "hacker",
						Score:       5,
						Title:       "Solid work of art",
						Description: "I built it with the best wood I could find.",
						CreatedAt:   time.Now(),
						ModifiedAt:  time.Now(),
					},
				},
				Total: 2,
			},
		},
		{
			name: "missing_params",
			args: args{
				ctx: t.Context(),
				params: inventory.ProductReviewsParams{
					Pagination: inventory.Pagination{
						Limit: 10,
					},
				},
			},
			wantErr: "missing params: reviewer_id or product_id are required",
		},
		{
			name: "not_found",
			args: args{
				ctx: t.Context(),
				params: inventory.ProductReviewsParams{
					ProductID: "wardrobe",
					Pagination: inventory.Pagination{
						Limit: 10,
					},
				},
			},
			want: &inventory.ProductReviewsResponse{
				Reviews: []*inventory.ProductReview{},
				Total:   0,
			},
		},
		{
			name: "canceled_ctx",
			args: args{
				ctx: canceledContext(t.Context()),
				params: inventory.ProductReviewsParams{
					ProductID: "bench",
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
				params: inventory.ProductReviewsParams{
					ReviewerID: "glazier",
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
			got, err := s.GetProductReviews(tt.args.ctx, tt.args.params)
			if err == nil && tt.wantErr != "" || err != nil && tt.wantErr != err.Error() {
				t.Errorf("Service.GetProductReviews() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if !cmp.Equal(tt.want, got, cmpopts.EquateApproxTime(time.Minute)) {
				t.Errorf("value returned by Service.GetProductReviews() doesn't match: %v", cmp.Diff(tt.want, got))
			}
		})
	}
}
