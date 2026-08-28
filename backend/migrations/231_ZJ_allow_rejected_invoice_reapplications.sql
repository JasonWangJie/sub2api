-- Rejected invoice applications remain as audit history, but their recharge
-- sources must be eligible for a corrected application or historical backfill.
-- Active/completed claims are enforced by the invoice service state check and
-- per-source advisory transaction lock.
ALTER TABLE invoice_application_items
    DROP CONSTRAINT IF EXISTS invoice_application_items_source_unique;

-- Older environments that created the Ent schema directly may have the
-- generated index name instead of the migration constraint name.
DROP INDEX IF EXISTS invoiceapplicationitem_source_type_source_id;

CREATE INDEX IF NOT EXISTS invoiceapplicationitem_source_type_source_id
    ON invoice_application_items (source_type, source_id);
