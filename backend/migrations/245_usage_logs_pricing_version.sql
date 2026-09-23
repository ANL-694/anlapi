-- Keep the effective pricing snapshot on each usage log for audit and reconciliation.
-- Historical rows intentionally remain NULL; this migration never changes amounts.

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS pricing_version VARCHAR(128);
