-- Payment providers can charge currencies with three fractional digits.
-- Keep the original 18 integer digits while preserving the full gateway amount.
ALTER TABLE payment_orders
    ALTER COLUMN pay_amount TYPE DECIMAL(21,3);
