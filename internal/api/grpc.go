package api

import (
	"context"
	"errors"

	"github.com/henvic/pgxtutorial/internal/apiv1/apipb"
	"github.com/henvic/pgxtutorial/internal/inventory"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// InventoryGRPC services.
type InventoryGRPC struct {
	apipb.UnimplementedInventoryServer
	Inventory *inventory.Service
}

func (i *InventoryGRPC) SearchProducts(ctx context.Context, req *apipb.SearchProductsRequest) (*apipb.SearchProductsResponse, error) {
	params := inventory.SearchProductsParams{
		QueryString: req.QueryString,
	}
	if req.MinPrice != nil {
		params.MinPrice = int(*req.MinPrice)
	}
	if req.MaxPrice != nil {
		params.MaxPrice = int(*req.MaxPrice)
	}
	page, pp := 1, 50
	if req.Page != nil {
		page = int(*req.Page)
	}
	params.Pagination = inventory.Pagination{
		Limit:  pp * page,
		Offset: pp * (page - 1),
	}
	products, err := i.Inventory.SearchProducts(ctx, params)
	if err != nil {
		return nil, grpcAPIError(err)
	}

	items := []*apipb.Product{}
	for _, p := range products.Items {
		items = append(items, &apipb.Product{
			Id:          p.ID,
			Price:       int64(p.Price),
			Name:        p.Name,
			Description: p.Description,
		})
	}
	return &apipb.SearchProductsResponse{
		Total: int32(products.Total),
		Items: items,
	}, nil
}

// CreateProduct on the inventory.
func (i *InventoryGRPC) CreateProduct(ctx context.Context, req *apipb.CreateProductRequest) (*apipb.CreateProductResponse, error) {
	if err := i.Inventory.CreateProduct(ctx, inventory.CreateProductParams{
		ID:          req.Id,
		Name:        req.Name,
		Description: req.Description,
		Price:       int(req.Price),
	}); err != nil {
		return nil, grpcAPIError(err)
	}
	return &apipb.CreateProductResponse{}, nil
}

// UpdateProduct on the inventory.
func (i *InventoryGRPC) UpdateProduct(ctx context.Context, req *apipb.UpdateProductRequest) (*apipb.UpdateProductResponse, error) {
	params := inventory.UpdateProductParams{
		ID:          req.Id,
		Name:        req.Name,
		Description: req.Description,
	}
	if req.Price != nil {
		price := int(*req.Price)
		params.Price = &price
	}
	if err := i.Inventory.UpdateProduct(ctx, params); err != nil {
		return nil, grpcAPIError(err)
	}
	return &apipb.UpdateProductResponse{}, nil
}

// DeleteProduct on the inventory.
func (i *InventoryGRPC) DeleteProduct(ctx context.Context, req *apipb.DeleteProductRequest) (*apipb.DeleteProductResponse, error) {
	if err := i.Inventory.DeleteProduct(ctx, req.Id); err != nil {
		return nil, grpcAPIError(err)
	}
	return &apipb.DeleteProductResponse{}, nil
}

// GetProduct on the inventory.
func (i *InventoryGRPC) GetProduct(ctx context.Context, req *apipb.GetProductRequest) (*apipb.GetProductResponse, error) {
	product, err := i.Inventory.GetProduct(ctx, req.Id)
	if err != nil {
		return nil, grpcAPIError(err)
	}
	if product == nil {
		return nil, status.Error(codes.NotFound, "product not found")
	}
	return &apipb.GetProductResponse{
		Id:          product.ID,
		Price:       int64(product.Price),
		Name:        product.Name,
		Description: product.Description,
		CreatedAt:   product.CreatedAt.String(),
		ModifiedAt:  product.ModifiedAt.String(),
	}, nil
}

// CreateProductReview on the inventory.
func (i *InventoryGRPC) CreateProductReview(ctx context.Context, req *apipb.CreateProductReviewRequest) (*apipb.CreateProductReviewResponse, error) {
	id, err := i.Inventory.CreateProductReview(ctx, inventory.CreateProductReviewParams{
		ProductID:   req.ProductId,
		ReviewerID:  req.ReviewerId,
		Score:       int(req.Score),
		Title:       req.Title,
		Description: req.Description,
	})
	if err != nil {
		return nil, grpcAPIError(err)
	}
	return &apipb.CreateProductReviewResponse{
		Id: id,
	}, nil
}

func (i *InventoryGRPC) UpdateProductReview(ctx context.Context, req *apipb.UpdateProductReviewRequest) (*apipb.UpdateProductReviewResponse, error) {
	params := inventory.UpdateProductReviewParams{
		ID:          req.Id,
		Title:       req.Title,
		Description: req.Description,
	}
	if req.Score != nil {
		score := int(*req.Score)
		params.Score = &score
	}
	if err := i.Inventory.UpdateProductReview(ctx, params); err != nil {
		return nil, grpcAPIError(err)
	}
	return &apipb.UpdateProductReviewResponse{}, nil
}

func (i *InventoryGRPC) DeleteProductReview(ctx context.Context, req *apipb.DeleteProductReviewRequest) (*apipb.DeleteProductReviewResponse, error) {
	if err := i.Inventory.DeleteProductReview(ctx, req.Id); err != nil {
		return nil, grpcAPIError(err)
	}
	return &apipb.DeleteProductReviewResponse{}, nil
}

func (i *InventoryGRPC) GetProductReview(ctx context.Context, req *apipb.GetProductReviewRequest) (*apipb.GetProductReviewResponse, error) {
	review, err := i.Inventory.GetProductReview(ctx, req.Id)
	if err != nil {
		return nil, grpcAPIError(err)
	}
	if review == nil {
		return nil, status.Error(codes.NotFound, "review not found")
	}
	return &apipb.GetProductReviewResponse{
		Id:          review.ID,
		ProductId:   review.ProductID,
		ReviewerId:  review.ReviewerID,
		Score:       int32(review.Score),
		Title:       review.Title,
		Description: review.Description,
		CreatedAt:   review.CreatedAt.String(),
		ModifiedAt:  review.ModifiedAt.String(),
	}, nil
}

// grpcAPIError wraps an error with gRPC API codes, when possible.
func grpcAPIError(err error) error {
	switch {
	case err == context.DeadlineExceeded:
		return status.Error(codes.DeadlineExceeded, err.Error())
	case err == context.Canceled:
		return status.Error(codes.Canceled, err.Error())
	case errors.As(err, &inventory.ValidationError{}):
		return status.Errorf(codes.InvalidArgument, err.Error())
	default:
		return err
	}
}
