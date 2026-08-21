-- Persist the request-level system charge multiplier so token unit prices can
-- be reconstructed without consulting the current system setting.
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS billing_charge_multiplier DECIMAL(10, 4) NOT NULL DEFAULT 1;

COMMENT ON COLUMN usage_logs.billing_charge_multiplier IS
    'System charge multiplier applied to input, output, and cache-read cost components for this request';
