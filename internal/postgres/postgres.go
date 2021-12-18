package postgres

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/hatch-studio/pgtools"
	"github.com/henvic/pgxtutorial/internal/database"
	"github.com/henvic/pgxtutorial/internal/inventory"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// DB handles database communication with PostgreSQL.
type DB struct {
	// Postgres database.PGX
	Postgres *pgxpool.Pool
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
func (db *DB) WithAcquire(ctx context.Context) (context.Context, error) {
	res, err := db.Postgres.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	return context.WithValue(ctx, connCtx{}, res), nil
}

// Release PostgreSQL connection acquired by context back to the pool.
func Release(ctx context.Context) {
	if res, ok := ctx.Value(connCtx{}).(*pgxpool.Conn); ok && res != nil {
		res.Release()
	}
}

// connCtx key.
type connCtx struct{}

// conn returns a PostgreSQL connection if a connection has been acquired by calling WithAcquire.
// Otherwise, it returns *pgxpool.Pool which acquires the connection and closes it immediately after a SQL command is executed.
func (db *DB) conn(ctx context.Context) database.PGX {
	if res, ok := ctx.Value(connCtx{}).(*pgxpool.Conn); ok && res != nil {
		return res
	}
	return db.Postgres
}

var _ inventory.DB = (*DB)(nil) // Check if methods expected by inventory.DB are implemented correctly.

// CreateProduct creates a new product.
func (db *DB) CreateProduct(ctx context.Context, params inventory.CreateProductParams) error {
	const sql = `INSERT INTO product ("id", "name", "description", "price") VALUES ($1, $2, $3, $4);`
	switch _, err := db.conn(ctx).Exec(ctx, sql, params.ID, params.Name, params.Description, params.Price); {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case err != nil:
		if sqlErr := db.productPgError(err); sqlErr != nil {
			return sqlErr
		}
		log.Printf("cannot create product on database: %v\n", err)
		return errors.New("cannot create product on database")
	}
	return nil
}

func (db *DB) productPgError(err error) error {
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
func (db *DB) UpdateProduct(ctx context.Context, params inventory.UpdateProductParams) error {
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
		log.Printf("cannot update product on database: %v\n", err)
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
func (db *DB) GetProduct(ctx context.Context, id string) (*inventory.Product, error) {
	var p product
	// The following pgtools.Wildcard() call returns:
	// "id","product_id","reviewer_id","title","description","score","created_at","modified_at"
	sql := fmt.Sprintf(`SELECT %s FROM "product" WHERE id = $1 LIMIT 1`, pgtools.Wildcard(p)) // #nosec G201
	rows, err := db.conn(ctx).Query(ctx, sql, id)
	if err == nil {
		defer rows.Close()
		err = pgxscan.ScanOne(&p, rows)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Printf("cannot get product from database: %v\n", err)
		return nil, errors.New("cannot get product from database")
	}
	return p.dto(), nil
}

// SearchProducts returns a list of products.
func (db *DB) SearchProducts(ctx context.Context, params inventory.SearchProductsParams) (*inventory.SearchProductsResponse, error) {
	var (
		args = []interface{}{"%" + params.QueryString + "%"}
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
		log.Printf("cannot get product count from the database: %v\n", err)
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

	switch rows, err := db.conn(ctx).Query(ctx, sql, args...); {
	case err == context.Canceled || err == context.DeadlineExceeded:
		return nil, err
	case err != nil:
		log.Printf("cannot get products from the database: %v\n", err)
		return nil, errors.New("cannot get products")
	default:
		defer rows.Close()
		rs := pgxscan.NewRowScanner(rows)
		for rows.Next() {
			var p product
			if err := rs.Scan(&p); err != nil {
				log.Printf("cannot scan returned rows from the database: %v\n", err)
				return nil, errors.New("cannot get products")
			}
			resp.Items = append(resp.Items, p.dto())
		}
		return &resp, nil
	}
}

// DeleteProduct from the database.
func (db *DB) DeleteProduct(ctx context.Context, id string) error {
	switch _, err := db.conn(ctx).Exec(ctx, `DELETE FROM "product" WHERE "id" = $1`, id); {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case err != nil:
		log.Printf("cannot delete product from database: %v\n", err)
		return errors.New("cannot delete product from database")
	}
	return nil
}

// CreateProductReview for a given product.
func (db *DB) CreateProductReview(ctx context.Context, params inventory.CreateProductReviewDBParams) error {
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
		log.Printf("cannot create review on database: %v\n", err)
		return errors.New("cannot create review on database")
	}
	return nil
}

func (db *DB) productReviewPgError(err error) error {
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
func (db *DB) UpdateProductReview(ctx context.Context, params inventory.UpdateProductReviewParams) error {
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
		log.Printf("cannot update review on database: %v\n", err)
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
func (db *DB) GetProductReview(ctx context.Context, id string) (*inventory.ProductReview, error) {
	// The following pgtools.Wildcard() call returns:
	// "id","product_id","reviewer_id","title","description","score","created_at","modified_at"
	var r review
	sql := fmt.Sprintf(`SELECT %s FROM "review" WHERE id = $1 LIMIT 1`, pgtools.Wildcard(r)) // #nosec G201
	rows, err := db.conn(ctx).Query(ctx, sql, id)
	if err == nil {
		defer rows.Close()
		err = pgxscan.ScanOne(&r, rows)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Printf("cannot get product review from database: %v\n", err)
		return nil, errors.New("cannot get product review from database")
	}
	return r.dto(), nil
}

// GetProductReviews gets reviews for a given product or from a given user.
func (db *DB) GetProductReviews(ctx context.Context, params inventory.ProductReviewsParams) (*inventory.ProductReviewsResponse, error) {
	var (
		args  []interface{}
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
		log.Printf("cannot get reviews count from the database: %v\n", err)
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
	if err != nil {
		log.Printf("cannot get reviews from the database: %v\n", err)
		return nil, errors.New("cannot get reviews")
	}
	defer rows.Close()
	rs := pgxscan.NewRowScanner(rows)
	for rows.Next() {
		var r review
		if err := rs.Scan(&r); err != nil {
			log.Printf("cannot scan returned rows from the database: %v\n", err)
			return nil, errors.New("cannot get reviews")
		}
		resp.Reviews = append(resp.Reviews, r.dto())
	}
	return resp, nil
}

// DeleteProductReview from the database.
func (db *DB) DeleteProductReview(ctx context.Context, id string) error {
	switch _, err := db.conn(ctx).Exec(ctx, `DELETE FROM "review" WHERE id = $1`, id); {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case err != nil:
		log.Printf("cannot delete review from database: %v\n", err)
		return errors.New("cannot delete review from database")
	}
	return nil
}
