package api

import (
	"context"
	"errors"

	"github.com/henvic/pgxtutorial/internal/apiv1/apipb"
	"github.com/henvic/pgxtutorial/internal/inventory"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// InventoryGRPC services.
type InventoryGRPC struct {
	apipb.UnimplementedInventoryServer
	Inventory *inventory.Service
}

func (i *InventoryGRPC) SearchProducts(ctx context.Context, req *apipb.SearchProductsRequest) (*apipb.SearchProductsResponse, error) {
	params := inventory.SearchProductsParams{
		QueryString: req.GetQueryString(),
		MinPrice:    int(req.GetMinPrice()),
		MaxPrice:    int(req.GetMaxPrice()),
	}
	page, pp := int(req.GetPage()), 50

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
		items = append(items, apipb.Product_builder{
			Id:          proto.String(p.ID),
			Price:       proto.Int64(int64(p.Price)),
			Name:        proto.String(p.Name),
			Description: proto.String(p.Description),
		}.Build())
	}
	return apipb.SearchProductsResponse_builder{
		Total: proto.Int32(int32(products.Total)),
		Items: items,
	}.Build(), nil
}

// CreateProduct on the inventory.
func (i *InventoryGRPC) CreateProduct(ctx context.Context, req *apipb.CreateProductRequest) (*apipb.CreateProductResponse, error) {
	if err := i.Inventory.CreateProduct(ctx, inventory.CreateProductParams{
		ID:          req.GetId(),
		Name:        req.GetName(),
		Description: req.GetDescription(),
		Price:       int(req.GetPrice()),
	}); err != nil {
		return nil, grpcAPIError(err)
	}
	return &apipb.CreateProductResponse{}, nil
}

// UpdateProduct on the inventory.
func (i *InventoryGRPC) UpdateProduct(ctx context.Context, req *apipb.UpdateProductRequest) (*apipb.UpdateProductResponse, error) {
	params := inventory.UpdateProductParams{
		ID: req.GetId(),
	}
	if req.HasName() {
		params.Name = proto.String(req.GetName())
	}
	if req.HasDescription() {
		params.Description = proto.String(req.GetDescription())
	}
	if req.HasPrice() {
		price := int(req.GetPrice())
		params.Price = &price
	}
	if err := i.Inventory.UpdateProduct(ctx, params); err != nil {
		return nil, grpcAPIError(err)
	}
	return &apipb.UpdateProductResponse{}, nil
}

// DeleteProduct on the inventory.
func (i *InventoryGRPC) DeleteProduct(ctx context.Context, req *apipb.DeleteProductRequest) (*apipb.DeleteProductResponse, error) {
	if err := i.Inventory.DeleteProduct(ctx, req.GetId()); err != nil {
		return nil, grpcAPIError(err)
	}
	return &apipb.DeleteProductResponse{}, nil
}

// GetProduct on the inventory.
func (i *InventoryGRPC) GetProduct(ctx context.Context, req *apipb.GetProductRequest) (*apipb.GetProductResponse, error) {
	product, err := i.Inventory.GetProduct(ctx, req.GetId())
	if err != nil {
		return nil, grpcAPIError(err)
	}
	if product == nil {
		return nil, status.Error(codes.NotFound, "product not found")
	}
	return apipb.GetProductResponse_builder{
		Id:          proto.String(product.ID),
		Price:       proto.Int64(int64(product.Price)),
		Name:        proto.String(product.Name),
		Description: proto.String(product.Description),
		CreatedAt:   proto.String(product.CreatedAt.String()),
		ModifiedAt:  proto.String(product.ModifiedAt.String()),
	}.Build(), nil
}

// CreateProductReview on the inventory.
func (i *InventoryGRPC) CreateProductReview(ctx context.Context, req *apipb.CreateProductReviewRequest) (*apipb.CreateProductReviewResponse, error) {
	id, err := i.Inventory.CreateProductReview(ctx, inventory.CreateProductReviewParams{
		ProductID:   req.GetProductId(),
		ReviewerID:  req.GetReviewerId(),
		Score:       req.GetScore(),
		Title:       req.GetTitle(),
		Description: req.GetDescription(),
	})
	if err != nil {
		return nil, grpcAPIError(err)
	}
	return apipb.CreateProductReviewResponse_builder{
		Id: proto.String(id),
	}.Build(), nil
}

func (i *InventoryGRPC) UpdateProductReview(ctx context.Context, req *apipb.UpdateProductReviewRequest) (*apipb.UpdateProductReviewResponse, error) {
	params := inventory.UpdateProductReviewParams{
		ID:          req.GetId(),
		Title:       proto.String(req.GetTitle()),
		Description: proto.String(req.GetDescription()),
	}
	if req.HasScore() {
		params.Score = proto.Int32(req.GetScore())
	}
	if err := i.Inventory.UpdateProductReview(ctx, params); err != nil {
		return nil, grpcAPIError(err)
	}
	return &apipb.UpdateProductReviewResponse{}, nil
}

func (i *InventoryGRPC) DeleteProductReview(ctx context.Context, req *apipb.DeleteProductReviewRequest) (*apipb.DeleteProductReviewResponse, error) {
	if err := i.Inventory.DeleteProductReview(ctx, req.GetId()); err != nil {
		return nil, grpcAPIError(err)
	}
	return &apipb.DeleteProductReviewResponse{}, nil
}

func (i *InventoryGRPC) GetProductReview(ctx context.Context, req *apipb.GetProductReviewRequest) (*apipb.GetProductReviewResponse, error) {
	review, err := i.Inventory.GetProductReview(ctx, req.GetId())
	if err != nil {
		return nil, grpcAPIError(err)
	}
	if review == nil {
		return nil, status.Error(codes.NotFound, "review not found")
	}
	return apipb.GetProductReviewResponse_builder{
		Id:          proto.String(review.ID),
		ProductId:   proto.String(review.ProductID),
		ReviewerId:  proto.String(review.ReviewerID),
		Score:       proto.Int32(int32(review.Score)),
		Title:       proto.String(review.Title),
		Description: proto.String(review.Description),
		CreatedAt:   proto.String(review.CreatedAt.String()),
		ModifiedAt:  proto.String(review.ModifiedAt.String()),
	}.Build(), nil
}

// grpcAPIError wraps an error with gRPC API codes, when possible.
func grpcAPIError(err error) error {
	switch {
	case err == context.DeadlineExceeded:
		return status.Error(codes.DeadlineExceeded, err.Error())
	case err == context.Canceled:
		return status.Error(codes.Canceled, err.Error())
	case errors.As(err, &inventory.ValidationError{}):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return err
	}
}
