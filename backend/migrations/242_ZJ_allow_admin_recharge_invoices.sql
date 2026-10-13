-- Admin balance increases of at least 100 are now eligible for historical
-- invoice backfill. Keep the existing immutable ledger and source uniqueness.
ALTER TABLE invoice_historical_marks
    DROP CONSTRAINT IF EXISTS invoice_historical_marks_source_type_check;

ALTER TABLE invoice_historical_marks
    ADD CONSTRAINT invoice_historical_marks_source_type_check
    CHECK (source_type IN ('payment_order', 'redeem_code', 'admin_grant'));
