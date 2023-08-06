package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/henvic/pgtools"
	"github.com/henvic/pgxtutorial/internal/database"
	"github.com/henvic/pgxtutorial/internal/inventory"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB handles database communication with PostgreSQL.
type DB struct {
	// pool for accessing Postgres database.PGX
	pool *pgxpool.Pool

	// log is a log for the operations.
	log *slog.Logger
}

// NewDB creates a DB.
func NewDB(pool *pgxpool.Pool, logger *slog.Logger) DB {
	return DB{
		pool: pool,
		log:  logger,
	}
}

// TransactionContext returns a copy of the parent context which begins a transaction
// to PostgreSQL.
//
// Once the transaction is over, you must call db.Commit(ctx) to make the changes effective.
// This might live in the go-pkg/postgres package later for the sake of code reuse.
func (db DB) TransactionContext(ctx context.Context) (context.Context, error) {
	tx, err := db.conn(ctx).Begin(ctx)
	if err != nil {
		return nil, err
	}
	return context.WithValue(ctx, txCtx{}, tx), nil
}

// Commit transaction from context.
func (db DB) Commit(ctx context.Context) error {
	if tx, ok := ctx.Value(txCtx{}).(pgx.Tx); ok && tx != nil {
		return tx.Commit(ctx)
	}
	return errors.New("context has no transaction")
}

// Rollback transaction from context.
func (db DB) Rollback(ctx context.Context) error {
	if tx, ok := ctx.Value(txCtx{}).(pgx.Tx); ok && tx != nil {
		return tx.Rollback(ctx)
	}
	return errors.New("context has no transaction")
}

// WithAcquire returns a copy of the parent context which acquires a connection
// to PostgreSQL from pgxpool to make sure commands executed in series reuse the
// same database connection.
//
// To release the connection back to the pool, you must call postgres.Release(ctx).
//
// Example:
// dbCtx := db.WithAcquire(ctx)
// defer postgres.Release(dbCtx)
func (db DB) WithAcquire(ctx context.Context) (dbCtx context.Context, err error) {
	if _, ok := ctx.Value(connCtx{}).(*pgxpool.Conn); ok {
		panic("context already has a connection acquired")
	}
	res, err := db.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	return context.WithValue(ctx, connCtx{}, res), nil
}

// Release PostgreSQL connection acquired by context back to the pool.
func (db DB) Release(ctx context.Context) {
	if res, ok := ctx.Value(connCtx{}).(*pgxpool.Conn); ok && res != nil {
		res.Release()
	}
}

// txCtx key.
type txCtx struct{}

// connCtx key.
type connCtx struct{}

// conn returns a PostgreSQL transaction if one exists.
// If not, returns a connection if a connection has been acquired by calling WithAcquire.
// Otherwise, it returns *pgxpool.Pool which acquires the connection and closes it immediately after a SQL command is executed.
func (db DB) conn(ctx context.Context) database.PGXQuerier {
	if tx, ok := ctx.Value(txCtx{}).(pgx.Tx); ok && tx != nil {
		return tx
	}
	if res, ok := ctx.Value(connCtx{}).(*pgxpool.Conn); ok && res != nil {
		return res
	}
	return db.pool
}

var _ inventory.DB = (*DB)(nil) // Check if methods expected by inventory.DB are implemented correctly.

// CreateProduct creates a new product.
func (db DB) CreateProduct(ctx context.Context, params inventory.CreateProductParams) error {
	const sql = `INSERT INTO product ("id", "name", "description", "price") VALUES ($1, $2, $3, $4);`
	switch _, err := db.conn(ctx).Exec(ctx, sql, params.ID, params.Name, params.Description, params.Price); {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case err != nil:
		if sqlErr := db.productPgError(err); sqlErr != nil {
			return sqlErr
		}
		db.log.Error("cannot create product on database", slog.Any("error", err))
		return errors.New("cannot create product on database")
	}
	return nil
}

func (db DB) productPgError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return nil
	}
	if pgErr.Code == pgerrcode.UniqueViolation {
		return errors.New("product already exists")
	}
	if pgErr.Code == pgerrcode.CheckViolation {
		switch pgErr.ConstraintName {
		case "product_id_check":
			return errors.New("invalid product ID")
		case "product_name_check":
			return errors.New("invalid product name")
		case "product_price_check":
			return errors.New("invalid price")
		}
	}
	return nil
}

// ErrProductNotFound is returned when a product is not found.
var ErrProductNotFound = errors.New("product not found")

// UpdateProduct updates an existing product.
func (db DB) UpdateProduct(ctx context.Context, params inventory.UpdateProductParams) error {
	const sql = `UPDATE "product" SET
	"name" = COALESCE($1, "name"),
	"description" = COALESCE($2, "description"),
	"price" = COALESCE($3, "price"),
	"modified_at" = now()
	WHERE id = $4`
	ct, err := db.conn(ctx).Exec(ctx, sql,
		params.Name,
		params.Description,
		params.Price,
		params.ID)
	if err == context.Canceled || err == context.DeadlineExceeded {
		return err
	}
	if err != nil {
		if sqlErr := db.productPgError(err); sqlErr != nil {
			return sqlErr
		}
		db.log.Error("cannot update product on database", slog.Any("error", err))
		return errors.New("cannot update product on database")
	}
	if ct.RowsAffected() == 0 {
		return ErrProductNotFound
	}
	return nil
}

// product table.
type product struct {
	ID          string
	Name        string
	Description string
	Price       int
	CreatedAt   time.Time
	ModifiedAt  time.Time
}

func (p *product) dto() *inventory.Product {
	return &inventory.Product{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		Price:       p.Price,
		CreatedAt:   p.CreatedAt,
		ModifiedAt:  p.ModifiedAt,
	}
}

// GetProduct returns a product.
func (db DB) GetProduct(ctx context.Context, id string) (*inventory.Product, error) {
	var p product
	// The following pgtools.Wildcard() call returns:
	// "id","product_id","reviewer_id","title","description","score","created_at","modified_at"
	sql := fmt.Sprintf(`SELECT %s FROM "product" WHERE id = $1 LIMIT 1`, pgtools.Wildcard(p)) // #nosec G201
	rows, err := db.conn(ctx).Query(ctx, sql, id)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	if err == nil {
		p, err = pgx.CollectOneRow(rows, pgx.RowToStructByPos[product])
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		db.log.Error("cannot get product from database",
			slog.Any("id", id),
			slog.Any("error", err),
		)
		return nil, errors.New("cannot get product from database")
	}
	return p.dto(), nil
}

// SearchProducts returns a list of products.
func (db DB) SearchProducts(ctx context.Context, params inventory.SearchProductsParams) (*inventory.SearchProductsResponse, error) {
	var (
		args = []any{"%" + params.QueryString + "%"}
		w    = []string{"name LIKE $1"}
	)

	if params.MinPrice != 0 {
		args = append(args, params.MinPrice)
		w = append(w, fmt.Sprintf(`"price" >= $%d`, len(args)))
	}
	if params.MaxPrice != 0 {
		args = append(args, params.MaxPrice)
		w = append(w, fmt.Sprintf(`"price" <= $%d`, len(args)))
	}

	where := strings.Join(w, " AND ")
	sqlTotal := fmt.Sprintf(`SELECT COUNT(*) AS total FROM "product" WHERE %s`, where) // #nosec G201
	resp := inventory.SearchProductsResponse{
		Items: []*inventory.Product{},
	}
	switch err := db.conn(ctx).QueryRow(ctx, sqlTotal, args...).Scan(&resp.Total); {
	case err == context.Canceled || err == context.DeadlineExceeded:
		return nil, err
	case err != nil:
		db.log.Error("cannot get product count from the database", slog.Any("error", err))
		return nil, errors.New("cannot get product")
	}

	// Once the count query was made, add pagination args and query the results of the current page.
	sql := fmt.Sprintf(`SELECT * FROM "product" WHERE %s ORDER BY "id" DESC`, where) // #nosec G201
	if params.Pagination.Limit != 0 {
		args = append(args, params.Pagination.Limit)
		sql += fmt.Sprintf(` LIMIT $%d`, len(args))
	}
	if params.Pagination.Offset != 0 {
		args = append(args, params.Pagination.Offset)
		sql += fmt.Sprintf(` OFFSET $%d`, len(args))
	}

	rows, err := db.conn(ctx).Query(ctx, sql, args...)
	if err == context.Canceled || err == context.DeadlineExceeded {
		return nil, err
	}
	var products []product
	if err == nil {
		products, err = pgx.CollectRows(rows, pgx.RowToStructByPos[product])
	}
	if err != nil {
		db.log.Error("cannot get products from the database", slog.Any("error", err))
		return nil, errors.New("cannot get products")
	}
	for _, p := range products {
		resp.Items = append(resp.Items, p.dto())
	}
	return &resp, nil
}

// DeleteProduct from the database.
func (db DB) DeleteProduct(ctx context.Context, id string) error {
	switch _, err := db.conn(ctx).Exec(ctx, `DELETE FROM "product" WHERE "id" = $1`, id); {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case err != nil:
		db.log.Error("cannot delete product from database", slog.Any("error", err))
		return errors.New("cannot delete product from database")
	}
	return nil
}

// CreateProductReview for a given product.
func (db DB) CreateProductReview(ctx context.Context, params inventory.CreateProductReviewDBParams) error {
	const sql = `
	INSERT INTO review (
		"id", "product_id", "reviewer_id",
		"title", "description", "score"
	)
	VALUES (
		$1, $2, $3,
		$4, $5, $6
	);`
	switch _, err := db.conn(ctx).Exec(ctx, sql,
		params.ID, params.ProductID, params.ReviewerID,
		params.Title, params.Description, params.Score); {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case err != nil:
		if sqlErr := db.productReviewPgError(err); sqlErr != nil {
			return sqlErr
		}
		db.log.Error("cannot create review on database", slog.Any("error", err))
		return errors.New("cannot create review on database")
	}
	return nil
}

func (db DB) productReviewPgError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return nil
	}
	if pgErr.Code == pgerrcode.UniqueViolation {
		return errors.New("product review already exists")
	}
	if pgErr.Code == pgerrcode.ForeignKeyViolation && pgErr.ConstraintName == "review_product_id_fkey" {
		return inventory.ErrCreateReviewNoProduct
	}
	if pgErr.Code == pgerrcode.CheckViolation {
		switch pgErr.ConstraintName {
		case "review_id_check":
			return errors.New("invalid product review ID")
		case "review_title_check":
			return errors.New("invalid title")
		case "review_score_check":
			return errors.New("invalid score")
		}
	}
	return nil
}

// ErrReviewNotFound is returned when a review is not found.
var ErrReviewNotFound = errors.New("product review not found")

// UpdateProductReview for a given product.
func (db DB) UpdateProductReview(ctx context.Context, params inventory.UpdateProductReviewParams) error {
	const sql = `UPDATE "review" SET
	"title" = COALESCE($1, "title"),
	"score" = COALESCE($2, "score"),
	"description" = COALESCE($3, "description"),
	"modified_at" = now()
	WHERE id = $4`

	switch ct, err := db.conn(ctx).Exec(ctx, sql, params.Title, params.Score, params.Description, params.ID); {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case err != nil:
		if sqlErr := db.productReviewPgError(err); sqlErr != nil {
			return sqlErr
		}
		db.log.Error("cannot update review on database", slog.Any("error", err))
		return errors.New("cannot update review on database")
	default:
		if ct.RowsAffected() == 0 {
			return ErrReviewNotFound
		}
		return nil
	}
}

// review table.
type review struct {
	ID          string
	ProductID   string
	ReviewerID  string
	Score       int
	Title       string
	Description string
	CreatedAt   time.Time
	ModifiedAt  time.Time
}

func (r *review) dto() *inventory.ProductReview {
	return &inventory.ProductReview{
		ID:          r.ID,
		ProductID:   r.ProductID,
		ReviewerID:  r.ReviewerID,
		Score:       r.Score,
		Title:       r.Title,
		Description: r.Description,
		CreatedAt:   r.CreatedAt,
		ModifiedAt:  r.ModifiedAt,
	}
}

// GetProductReview gets a specific review.
func (db DB) GetProductReview(ctx context.Context, id string) (*inventory.ProductReview, error) {
	// The following pgtools.Wildcard() call returns:
	// "id","product_id","reviewer_id","title","description","score","created_at","modified_at"
	var r review
	sql := fmt.Sprintf(`SELECT %s FROM "review" WHERE id = $1 LIMIT 1`, pgtools.Wildcard(r)) // #nosec G201
	rows, err := db.conn(ctx).Query(ctx, sql, id)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	if err == nil {
		r, err = pgx.CollectOneRow(rows, pgx.RowToStructByPos[review])
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		db.log.Error("cannot get product review from database",
			slog.Any("id", id),
			slog.Any("error", err))
		return nil, errors.New("cannot get product review from database")
	}
	return r.dto(), nil
}

// GetProductReviews gets reviews for a given product or from a given user.
func (db DB) GetProductReviews(ctx context.Context, params inventory.ProductReviewsParams) (*inventory.ProductReviewsResponse, error) {
	var (
		args  []any
		where []string
	)
	if params.ProductID != "" {
		args = append(args, params.ProductID)
		where = append(where, fmt.Sprintf(`"product_id" = $%d`, len(args)))
	}
	if params.ReviewerID != "" {
		args = append(args, params.ReviewerID)
		where = append(where, fmt.Sprintf(`"reviewer_id" = $%d`, len(args)))
	}
	sql := fmt.Sprintf(`SELECT %s FROM "review"`, pgtools.Wildcard(review{})) // #nosec G201
	sqlTotal := `SELECT COUNT(*) AS total FROM "review"`
	if len(where) > 0 {
		w := " WHERE " + strings.Join(where, " AND ") // #nosec G202
		sql += w
		sqlTotal += w
	}

	resp := &inventory.ProductReviewsResponse{
		Reviews: []*inventory.ProductReview{},
	}
	err := db.conn(ctx).QueryRow(ctx, sqlTotal, args...).Scan(&resp.Total)
	if err == context.Canceled || err == context.DeadlineExceeded {
		return nil, err
	}
	if err != nil {
		db.log.Error("cannot get reviews count from the database", slog.Any("error", err))
		return nil, errors.New("cannot get reviews")
	}

	// Once the count query was made, add pagination args and query the results of the current page.
	sql += ` ORDER BY "created_at" DESC`
	if params.Pagination.Limit != 0 {
		args = append(args, params.Pagination.Limit)
		sql += fmt.Sprintf(` LIMIT $%d`, len(args))
	}
	if params.Pagination.Offset != 0 {
		args = append(args, params.Pagination.Offset)
		sql += fmt.Sprintf(` OFFSET $%d`, len(args))
	}
	rows, err := db.conn(ctx).Query(ctx, sql, args...)
	if err == context.Canceled || err == context.DeadlineExceeded {
		return nil, err
	}
	var reviews []review
	if err == nil {
		reviews, err = pgx.CollectRows(rows, pgx.RowToStructByPos[review])
	}
	if err != nil {
		db.log.Error("cannot get reviews from database", slog.Any("error", err))
		return nil, errors.New("cannot get reviews")
	}
	for _, r := range reviews {
		resp.Reviews = append(resp.Reviews, r.dto())
	}
	return resp, nil
}

// DeleteProductReview from the database.
func (db DB) DeleteProductReview(ctx context.Context, id string) error {
	switch _, err := db.conn(ctx).Exec(ctx, `DELETE FROM "review" WHERE id = $1`, id); {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case err != nil:
		db.log.Error("cannot delete review from database",
			slog.Any("id", id),
			slog.Any("error", err),
		)
		return errors.New("cannot delete review from database")
	}
	return nil
}
