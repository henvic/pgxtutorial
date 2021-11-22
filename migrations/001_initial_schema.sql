-- Write your migrate up statements here

-- product table
CREATE TABLE product (
	id text PRIMARY KEY CHECK (ID != '') NOT NULL,
	name text NOT NULL CHECK (NAME != ''),
	description text NOT NULL,
	price int NOT NULL CHECK (price >= 0),
	created_at timestamp with time zone NOT NULL DEFAULT now(),
	modified_at timestamp with time zone NOT NULL DEFAULT now()
	-- If you want to use a soft delete strategy, you'll need something like:
	-- deleted_at timestamp with time zone DEFAULT now()
	-- or better: a product_history table to keep track of each change here.
);

COMMENT ON COLUMN product.id IS 'assume id is the barcode';
COMMENT ON COLUMN product.price IS 'price in the smaller subdivision possible (such as cents)';
CREATE INDEX product_name ON product(name text_pattern_ops);

-- review table
CREATE TABLE review (
	id text PRIMARY KEY CHECK (id != '') NOT NULL,
	product_id text NOT NULL REFERENCES product(id) ON DELETE CASCADE,
	reviewer_id text NOT NULL,
	title text NOT NULL CHECK (title != ''),
	description text NOT NULL,
	score int NOT NULL CHECK (score >= 0 AND score <= 5),
	created_at timestamp with time zone NOT NULL DEFAULT now(),
	modified_at timestamp with time zone NOT NULL DEFAULT now()
);

CREATE INDEX review_title ON review(title text_pattern_ops);

---- create above / drop below ----

-- Write your migrate down statements here. If this migration is irreversible
-- Then delete the separator line above.
DROP TABLE review;
DROP TABLE product;