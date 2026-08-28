-- Historical invoicing ledger for recharge orders invoiced before enterprise
-- invoicing was introduced. Rows are immutable and cannot be undone.
CREATE TABLE IF NOT EXISTS invoice_historical_marks (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    source_type VARCHAR(32) NOT NULL,
    source_id BIGINT NOT NULL,
    source_reference VARCHAR(128) NOT NULL,
    amount DECIMAL(20, 2) NOT NULL CHECK (amount > 0),
    marked_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    marked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT invoice_historical_marks_source_type_check CHECK (source_type IN ('payment_order', 'redeem_code')),
    CONSTRAINT invoice_historical_marks_source_unique UNIQUE (source_type, source_id)
);

CREATE INDEX IF NOT EXISTS invoice_historical_marks_user_marked_at_idx
    ON invoice_historical_marks (user_id, marked_at DESC);
CREATE INDEX IF NOT EXISTS invoice_historical_marks_marked_by_idx
    ON invoice_historical_marks (marked_by);

COMMENT ON TABLE invoice_historical_marks IS 'Immutable audit ledger for recharge records invoiced before enterprise invoicing workflow';
