-- Recharge credits accumulate in the wallet and lifetime total_recharged.
-- A single order can fit DECIMAL(20,8) while cumulative credits or holds overflow it.
ALTER TABLE users ALTER COLUMN balance TYPE DECIMAL(30,8);
ALTER TABLE users ALTER COLUMN frozen_balance TYPE DECIMAL(30,8);
ALTER TABLE users ALTER COLUMN total_recharged TYPE DECIMAL(30,8);
